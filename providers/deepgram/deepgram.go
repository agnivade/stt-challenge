package deepgram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	api "github.com/deepgram/deepgram-go-sdk/v3/pkg/api/listen/v1/websocket/interfaces"
	interfaces "github.com/deepgram/deepgram-go-sdk/v3/pkg/client/interfaces"
	client "github.com/deepgram/deepgram-go-sdk/v3/pkg/client/listen"
	listenv1ws "github.com/deepgram/deepgram-go-sdk/v3/pkg/client/listen/v1/websocket"

	"github.com/agnivade/stt_challenge/providers"
)

const providerName = "deepgram"

// Provider implements the providers.Provider interface for Deepgram's speech-to-text API.
type Provider struct {
	apiKey string
}

// NewProvider creates a new Deepgram provider with the given API key.
func NewProvider(apiKey string) *Provider {
	client.InitWithDefault()

	return &Provider{
		apiKey: apiKey,
	}
}

// Name returns the name of the provider.
func (p *Provider) Name() string {
	return providerName
}

// NewSession creates a new Deepgram transcription session.
func (p *Provider) NewSession(ctx context.Context, config providers.SessionConfig) (providers.Session, error) {
	// Configure Deepgram client options
	cOptions := &interfaces.ClientOptions{
		APIKey:          p.apiKey,
		EnableKeepAlive: true,
	}

	// Configure transcription options
	tOptions := &interfaces.LiveTranscriptionOptions{
		Model:          "nova-3",
		Keyterm:        []string{"deepgram"},
		Language:       config.LanguageCode,
		Punctuate:      true,
		Encoding:       "linear16",
		Channels:       1,
		SampleRate:     config.SampleRate,
		VadEvents:      true,
		InterimResults: config.InterimResults,
		UtteranceEndMs: "1000",
	}

	// Create session with callback handler
	session := &Session{
		ctx:           ctx,
		resultChannel: make(chan providers.TranscriptionResult, 10),
		errorChannel:  make(chan error, 1),
		closed:        false,
	}

	// Create callback handler
	callback := &CallbackHandler{
		session: session,
	}

	// Create Deepgram WebSocket client
	dgClient, err := client.NewWSUsingCallback(ctx, "", cOptions, tOptions, callback)
	if err != nil {
		return nil, err
	}

	session.client = dgClient

	// Connect to Deepgram
	if success := dgClient.Connect(); !success {
		return nil, errors.New("failed to connect to deepgram")
	}

	return session, nil
}

// Session implements the providers.Session interface for Deepgram's speech-to-text API.
type Session struct {
	ctx           context.Context
	client        *listenv1ws.WSCallback
	resultChannel chan providers.TranscriptionResult
	errorChannel  chan error
	mu            sync.RWMutex
	// This flag is needed to coordinate between the session
	// and the callback handler. Otherwise, there could be a case
	// where webconn has exited the read loop and ran session.Close
	// and a stray message arrives, trying to send something to a closed
	// channel.
	// This is still not 100% race free. Potentially look into using
	// client.NewWSUsingChan for better coordination.
	closed bool
}

// SendAudio sends audio data to the Deepgram stream.
func (s *Session) SendAudio(audioData []byte) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return io.EOF
	}

	_, err := s.client.Write(audioData)
	return err
}

// ReceiveTranscription receives transcription results from the Deepgram stream.
// It blocks until a final result is available or an error occurs.
func (s *Session) ReceiveTranscription() (providers.TranscriptionResult, error) {
	select {
	case result := <-s.resultChannel:
		return result, nil
	case err := <-s.errorChannel:
		return providers.TranscriptionResult{}, err
	case <-s.ctx.Done():
		if s.ctx.Err() == context.Canceled {
			return providers.TranscriptionResult{}, io.EOF
		}
		return providers.TranscriptionResult{}, s.ctx.Err()
	}
}

// Close closes the Deepgram session.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	close(s.resultChannel)
	close(s.errorChannel)

	if s.client != nil {
		s.client.Stop()
	}

	return nil
}

// CallbackHandler implements Deepgram callback interfaces to handle transcription events.
type CallbackHandler struct {
	session *Session
}

// Message handles transcription result messages from Deepgram.
func (c *CallbackHandler) Message(mr *api.MessageResponse) error {
	// Process transcription results
	if len(mr.Channel.Alternatives) == 0 {
		return nil
	}

	alternative := mr.Channel.Alternatives[0]
	sentence := strings.TrimSpace(alternative.Transcript)
	if sentence == "" {
		return nil
	}

	result := providers.TranscriptionResult{
		Text:         sentence,
		IsFinal:      mr.IsFinal,
		Confidence:   float32(alternative.Confidence),
		ProviderName: providerName,
		ReceivedAt:   time.Now(),
	}

	// Only send final results to match our interface expectation
	if result.IsFinal {
		select {
		case c.session.resultChannel <- result:
		default:
			// Channel is full, drop the message
		}
	}
	// }

	return nil
}

// Metadata handles metadata messages from Deepgram.
func (c *CallbackHandler) Metadata(md *api.MetadataResponse) error {
	// Handle metadata if needed (currently no-op)
	return nil
}

// SpeechStarted handles speech start events from Deepgram.
func (c *CallbackHandler) SpeechStarted(ssr *api.SpeechStartedResponse) error {
	// Handle speech started events if needed (currently no-op)
	return nil
}

// UtteranceEnd handles utterance end events from Deepgram.
func (c *CallbackHandler) UtteranceEnd(ue *api.UtteranceEndResponse) error {
	// Handle utterance end events if needed (currently no-op)
	return nil
}

// Close handles connection close events from Deepgram.
func (c *CallbackHandler) Close(cr *api.CloseResponse) error {
	return nil
}

// Error handles error events from Deepgram.
func (c *CallbackHandler) Error(err *api.ErrorResponse) error {
	// Forward errors to the session
	if err != nil {
		select {
		case c.session.errorChannel <- fmt.Errorf("%s", err):
		default:
		}
	}
	return nil
}

// Open handles connection open events from Deepgram.
func (c *CallbackHandler) Open(or *api.OpenResponse) error {
	// Handle connection open events if needed (currently no-op)
	return nil
}

// UnhandledEvent handles any unhandled message types from Deepgram.
func (c *CallbackHandler) UnhandledEvent(byMsg []byte) error {
	// Handle unhandled messages if needed (currently no-op)
	return nil
}
