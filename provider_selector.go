package stt_challenge

import (
	"context"
	"errors"
	"io"
	"log"
	"sync"
	"time"

	"github.com/agnivade/stt_challenge/providers"
)

// ProviderResultWithSeq wraps transcription result with sequence number
type ProviderResultWithSeq struct {
	Result providers.TranscriptionResult
	// Caveat: this WILL wrap around after 1<<64 - 1 times
	// We leave that as a future exercise.
	SeqNum uint64
}

// ProviderSelector manages multiple transcription providers and dynamically
// selects the best provider based on latency and performance metrics.
type ProviderSelector struct {
	sessions            []providers.Session
	providerNames       []string
	audioInput          chan []byte
	transcriptionOutput chan providers.TranscriptionResult
	transcriptionBuffer chan providers.TranscriptionResult

	// Active provider tracking
	activeProvider      string
	providerResults     map[string][]ProviderResultWithSeq
	providerSeqCounters map[string]uint64

	ctx    context.Context
	cancel context.CancelFunc
	log    *log.Logger
	wg     sync.WaitGroup
}

// NewProviderSelector creates a new provider selector with the given providers.
func NewProviderSelector(providersList []providers.Provider, config providers.SessionConfig, logger *log.Logger) (*ProviderSelector, error) {
	selectorCtx, cancel := context.WithCancel(context.Background())

	ps := &ProviderSelector{
		sessions:            make([]providers.Session, 0, len(providersList)),
		providerNames:       make([]string, 0, len(providersList)),
		audioInput:          make(chan []byte, 100), // Buffered channel
		transcriptionOutput: make(chan providers.TranscriptionResult, 10),
		transcriptionBuffer: make(chan providers.TranscriptionResult, 100),
		providerResults:     make(map[string][]ProviderResultWithSeq),
		providerSeqCounters: make(map[string]uint64),
		ctx:                 selectorCtx,
		cancel:              cancel,
		log:                 logger,
	}

	// Create sessions for all providers
	for _, provider := range providersList {
		session, err := provider.NewSession(selectorCtx, config)
		if err != nil {
			ps.log.Printf("Failed to create session for provider %s: %v", provider.Name(), err)
			// Continue with other providers
			continue
		}

		ps.sessions = append(ps.sessions, session)
		ps.providerNames = append(ps.providerNames, provider.Name())
	}

	if len(ps.sessions) == 0 {
		cancel()
		return nil, errors.New("no providers available")
	}

	// Initialize active provider to first available
	ps.activeProvider = ps.providerNames[0]

	// Start goroutines
	ps.wg.Add(1)
	go ps.audioDistributor()

	ps.wg.Add(1)
	go ps.heuristicSelector()

	for i, session := range ps.sessions {
		ps.wg.Add(1)
		go ps.transcriptionCollector(session, ps.providerNames[i])
	}

	return ps, nil
}

// SendAudio implements the providers.Session interface
func (ps *ProviderSelector) SendAudio(audioData []byte) error {
	select {
	case ps.audioInput <- audioData:
		return nil
	case <-ps.ctx.Done():
		if ps.ctx.Err() == context.Canceled {
			return io.EOF
		}
		return ps.ctx.Err()
	}
}

// ReceiveTranscription implements the providers.Session interface
func (ps *ProviderSelector) ReceiveTranscription() (providers.TranscriptionResult, error) {
	select {
	case result := <-ps.transcriptionOutput:
		return result, nil
	case <-ps.ctx.Done():
		if ps.ctx.Err() == context.Canceled {
			return providers.TranscriptionResult{}, io.EOF
		}
		return providers.TranscriptionResult{}, ps.ctx.Err()
	}
}

// Close implements the providers.Session interface
func (ps *ProviderSelector) Close() error {
	// Cancel reader stream, to allow for transcriptionCollector to exit.
	ps.cancel()
	// Close will be called after ws reader exits. So there's no chance
	// of writing to ps.audioInput again.
	close(ps.audioInput)

	// Close all sessions
	for _, session := range ps.sessions {
		if err := session.Close(); err != nil {
			ps.log.Printf("Error closing session: %v", err)
		}
	}

	ps.wg.Wait()
	close(ps.transcriptionOutput)
	close(ps.transcriptionBuffer)

	ps.log.Println("Closing provider selector....")
	return nil
}

// audioDistributor distributes audio data to all provider sessions synchronously
func (ps *ProviderSelector) audioDistributor() {
	defer ps.wg.Done()

	for audioData := range ps.audioInput {
		// Copy buffer for each provider to avoid race conditions
		var wg sync.WaitGroup

		for i, session := range ps.sessions {
			wg.Add(1)
			go func(s providers.Session, providerID int) {
				defer wg.Done()

				// Create a copy of the audio data
				// TODO: this is expensive, need to reuse byte buffers.
				audioCopy := make([]byte, len(audioData))
				copy(audioCopy, audioData)

				if err := s.SendAudio(audioCopy); err != nil {
					if errors.Is(err, io.EOF) {
						return
					}
					ps.log.Printf("Provider %s audio send failed: %v",
						ps.providerNames[providerID], err)
				}
			}(session, i)
		}

		// Wait for all providers to receive audio before processing next chunk
		wg.Wait()
	}
}

// transcriptionCollector collects transcription results from a single provider
func (ps *ProviderSelector) transcriptionCollector(session providers.Session, providerName string) {
	defer ps.wg.Done()

	for {
		result, err := session.ReceiveTranscription()
		if err == io.EOF {
			return
		}
		// TODO: Handle Audio timeout properly and support resumable streams.
		if err != nil {
			ps.log.Printf("Provider %s transcription error: %v", providerName, err)
			return
		}

		select {
		case ps.transcriptionBuffer <- result:
		case <-ps.ctx.Done():
			return
		}
	}
}

// heuristicSelector implements the active provider streaming strategy
func (ps *ProviderSelector) heuristicSelector() {
	defer ps.wg.Done()

	windowTicker := time.NewTicker(2 * time.Second)
	// No need for defer windowTicker.Stop() post Go 1.23

	for {
		select {
		case result := <-ps.transcriptionBuffer:
			// We are not storing intermediate results.
			if !result.IsFinal {
				continue
			}

			// Increment sequence number for this provider
			ps.providerSeqCounters[result.ProviderName]++
			seqNum := ps.providerSeqCounters[result.ProviderName]

			// Store result with sequence number
			resultWithSeq := ProviderResultWithSeq{
				Result: result,
				SeqNum: seqNum,
			}
			ps.providerResults[result.ProviderName] = append(ps.providerResults[result.ProviderName], resultWithSeq)

			// If result is from active provider, forward immediately
			if result.ProviderName == ps.activeProvider {
				select {
				case ps.transcriptionOutput <- result:
				case <-ps.ctx.Done():
					return
				}
			}

		case <-windowTicker.C:
			// Update active provider based on latency analysis
			ps.updateActiveProvider()

			// // Clean up old results to prevent memory buildup
			ps.clearOldResults()
		case <-ps.ctx.Done():
			return
		}
	}
}

// updateActiveProvider selects the provider with the most recent activity
//
// This is just a poor-man's implementation of "most recently active provider".
// A better way would be to track the actual duration from the time a request was
// sent to the time we received. Unfortunately, there does not seem to be any
// tracking ID mechanism in the API calls, so we'd have to wrap the streams and
// manually track time ourselves.
//
// One could also keep a track of average confidence in a sliding window, but
// Google documentation says even that number is unreliable.
//
// Keeping it simple for now.
func (ps *ProviderSelector) updateActiveProvider() {
	if len(ps.providerResults) == 0 {
		return
	}

	bestProvider := ""
	mostRecentTime := time.Time{}

	// Find provider with most recent activity (indicating fastest response)
	windowStart := time.Now().Add(-2 * time.Second)

	for providerName, results := range ps.providerResults {
		if len(results) == 0 {
			continue
		}

		// Find most recent final result from this provider within the window
		for _, resultWithSeq := range results {
			if resultWithSeq.Result.ReceivedAt.After(windowStart) && resultWithSeq.Result.ReceivedAt.After(mostRecentTime) {
				mostRecentTime = resultWithSeq.Result.ReceivedAt
				bestProvider = providerName
			}
		}
	}

	// Switch active provider if we found a faster one
	if bestProvider != "" && bestProvider != ps.activeProvider {
		oldProvider := ps.activeProvider
		ps.log.Printf("Switching active provider from %s to %s (most recent: %v)",
			oldProvider, bestProvider, mostRecentTime)

		// Send any missed messages from the new active provider
		ps.sendMissedMessages(oldProvider, bestProvider)

		ps.activeProvider = bestProvider
	}
}

// sendMissedMessages sends any messages from the new active provider that have
// higher sequence numbers than the old provider, assuming they represent missed transcriptions.
// This does assume that the Utterance gap of each provider would be the same, which may not be the case, but it's good enough for now.
// A side issue is that it does not play well with replayed audio since
// the samples get sent too quickly.
func (ps *ProviderSelector) sendMissedMessages(oldProvider, newProvider string) {
	oldResults := ps.providerResults[oldProvider]
	newResults := ps.providerResults[newProvider]

	if len(oldResults) == 0 || len(newResults) == 0 {
		return
	}

	// Get the last sequence number from the old provider
	lastOldSeq := oldResults[len(oldResults)-1].SeqNum

	// Send any results from new provider with higher sequence numbers
	for _, resultWithSeq := range newResults {
		if resultWithSeq.SeqNum > lastOldSeq {
			ps.log.Printf("Sending missed message from %s (seq:%d): %s",
				newProvider, resultWithSeq.SeqNum, resultWithSeq.Result.Text)

			select {
			case ps.transcriptionOutput <- resultWithSeq.Result:
			case <-ps.ctx.Done():
				return
			}
		}
	}
}

// clearOldResults removes old results to prevent memory buildup
func (ps *ProviderSelector) clearOldResults() {
	cutoff := time.Now().Add(-5 * time.Second) // Keep 5 seconds of history

	for providerName, results := range ps.providerResults {
		filtered := results[:0]
		for _, resultWithSeq := range results {
			if resultWithSeq.Result.ReceivedAt.After(cutoff) {
				filtered = append(filtered, resultWithSeq)
			}
		}
		ps.providerResults[providerName] = filtered
	}
}
