package stt_challenge

import (
	"context"
	"io"
	"log"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agnivade/stt_challenge/providers"
)

func TestProviderSelector_updateActiveProvider(t *testing.T) {
	tests := []struct {
		name           string
		activeProvider string
		results        map[string][]ProviderResultWithSeq
		expectedActive string
		expectedSwitch bool
	}{
		{
			name:           "no results - no change",
			activeProvider: "provider1",
			results:        map[string][]ProviderResultWithSeq{},
			expectedActive: "provider1",
			expectedSwitch: false,
		},
		{
			name:           "empty results - no change",
			activeProvider: "provider1",
			results: map[string][]ProviderResultWithSeq{
				"provider1": {},
				"provider2": {},
			},
			expectedActive: "provider1",
			expectedSwitch: false,
		},
		{
			name:           "single provider with recent activity - no change",
			activeProvider: "provider1",
			results: map[string][]ProviderResultWithSeq{
				"provider1": {
					{
						Result: providers.TranscriptionResult{
							Text:         "hello",
							IsFinal:      true,
							ProviderName: "provider1",
							ReceivedAt:   time.Now().Add(-500 * time.Millisecond),
						},
						SeqNum: 1,
					},
				},
			},
			expectedActive: "provider1",
			expectedSwitch: false,
		},
		{
			name:           "switch to provider with more recent activity",
			activeProvider: "provider1",
			results: map[string][]ProviderResultWithSeq{
				"provider1": {
					{
						Result: providers.TranscriptionResult{
							Text:         "hello",
							IsFinal:      true,
							ProviderName: "provider1",
							ReceivedAt:   time.Now().Add(-1500 * time.Millisecond),
						},
						SeqNum: 1,
					},
				},
				"provider2": {
					{
						Result: providers.TranscriptionResult{
							Text:         "world",
							IsFinal:      true,
							ProviderName: "provider2",
							ReceivedAt:   time.Now().Add(-200 * time.Millisecond),
						},
						SeqNum: 1,
					},
				},
			},
			expectedActive: "provider2",
			expectedSwitch: true,
		},
		{
			name:           "old results outside window - no change",
			activeProvider: "provider1",
			results: map[string][]ProviderResultWithSeq{
				"provider1": {
					{
						Result: providers.TranscriptionResult{
							Text:         "hello",
							IsFinal:      true,
							ProviderName: "provider1",
							ReceivedAt:   time.Now().Add(-3 * time.Second),
						},
						SeqNum: 1,
					},
				},
				"provider2": {
					{
						Result: providers.TranscriptionResult{
							Text:         "world",
							IsFinal:      true,
							ProviderName: "provider2",
							ReceivedAt:   time.Now().Add(-4 * time.Second),
						},
						SeqNum: 1,
					},
				},
			},
			expectedActive: "provider1",
			expectedSwitch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal provider selector for testing
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			ps := &ProviderSelector{
				activeProvider:  tt.activeProvider,
				providerResults: tt.results,
				ctx:             ctx,
				cancel:          cancel,
				log:             log.New(&ThreadSafeBuffer{}, "", 0),
			}

			// Track if sendMissedMessages would be called
			originalActive := ps.activeProvider

			// Call the method under test
			ps.updateActiveProvider()

			// Check if active provider changed as expected
			assert.Equal(t, tt.expectedActive, ps.activeProvider)

			// Verify if switch occurred
			switchOccurred := originalActive != ps.activeProvider
			assert.Equal(t, tt.expectedSwitch, switchOccurred)
		})
	}
}

func TestProviderSelector_clearOldResults(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name            string
		results         map[string][]ProviderResultWithSeq
		expectedResults map[string][]ProviderResultWithSeq
	}{
		{
			name:            "empty results",
			results:         map[string][]ProviderResultWithSeq{},
			expectedResults: map[string][]ProviderResultWithSeq{},
		},
		{
			name: "all recent results - no removal",
			results: map[string][]ProviderResultWithSeq{
				"provider1": {
					{
						Result: providers.TranscriptionResult{
							Text:         "recent1",
							ReceivedAt:   now.Add(-1 * time.Second),
							ProviderName: "provider1",
						},
						SeqNum: 1,
					},
					{
						Result: providers.TranscriptionResult{
							Text:         "recent2",
							ReceivedAt:   now.Add(-2 * time.Second),
							ProviderName: "provider1",
						},
						SeqNum: 2,
					},
				},
			},
			expectedResults: map[string][]ProviderResultWithSeq{
				"provider1": {
					{
						Result: providers.TranscriptionResult{
							Text:         "recent1",
							ReceivedAt:   now.Add(-1 * time.Second),
							ProviderName: "provider1",
						},
						SeqNum: 1,
					},
					{
						Result: providers.TranscriptionResult{
							Text:         "recent2",
							ReceivedAt:   now.Add(-2 * time.Second),
							ProviderName: "provider1",
						},
						SeqNum: 2,
					},
				},
			},
		},
		{
			name: "mixed recent and old results - remove old",
			results: map[string][]ProviderResultWithSeq{
				"provider1": {
					{
						Result: providers.TranscriptionResult{
							Text:         "old1",
							ReceivedAt:   now.Add(-10 * time.Second),
							ProviderName: "provider1",
						},
						SeqNum: 1,
					},
					{
						Result: providers.TranscriptionResult{
							Text:         "recent1",
							ReceivedAt:   now.Add(-2 * time.Second),
							ProviderName: "provider1",
						},
						SeqNum: 2,
					},
					{
						Result: providers.TranscriptionResult{
							Text:         "old2",
							ReceivedAt:   now.Add(-8 * time.Second),
							ProviderName: "provider1",
						},
						SeqNum: 3,
					},
				},
				"provider2": {
					{
						Result: providers.TranscriptionResult{
							Text:         "recent2",
							ReceivedAt:   now.Add(-1 * time.Second),
							ProviderName: "provider2",
						},
						SeqNum: 1,
					},
				},
			},
			expectedResults: map[string][]ProviderResultWithSeq{
				"provider1": {
					{
						Result: providers.TranscriptionResult{
							Text:         "recent1",
							ReceivedAt:   now.Add(-2 * time.Second),
							ProviderName: "provider1",
						},
						SeqNum: 2,
					},
				},
				"provider2": {
					{
						Result: providers.TranscriptionResult{
							Text:         "recent2",
							ReceivedAt:   now.Add(-1 * time.Second),
							ProviderName: "provider2",
						},
						SeqNum: 1,
					},
				},
			},
		},
		{
			name: "all old results - remove all",
			results: map[string][]ProviderResultWithSeq{
				"provider1": {
					{
						Result: providers.TranscriptionResult{
							Text:         "old1",
							ReceivedAt:   now.Add(-10 * time.Second),
							ProviderName: "provider1",
						},
						SeqNum: 1,
					},
					{
						Result: providers.TranscriptionResult{
							Text:         "old2",
							ReceivedAt:   now.Add(-8 * time.Second),
							ProviderName: "provider1",
						},
						SeqNum: 2,
					},
				},
			},
			expectedResults: map[string][]ProviderResultWithSeq{
				"provider1": {},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal provider selector for testing
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			ps := &ProviderSelector{
				providerResults: tt.results,
				ctx:             ctx,
				cancel:          cancel,
				log:             log.New(&ThreadSafeBuffer{}, "", 0),
			}

			// Call the method under test
			ps.clearOldResults()

			// Check that old results were cleared
			assert.Equal(t, tt.expectedResults, ps.providerResults)
		})
	}
}

func TestProviderSelector_sendMissedMessages(t *testing.T) {
	tests := []struct {
		name        string
		oldProvider string
		newProvider string
		results     map[string][]ProviderResultWithSeq
		expectSent  []providers.TranscriptionResult
	}{
		{
			name:        "no results for either provider",
			oldProvider: "provider1",
			newProvider: "provider2",
			results:     map[string][]ProviderResultWithSeq{},
			expectSent:  nil,
		},
		{
			name:        "no results for old provider",
			oldProvider: "provider1",
			newProvider: "provider2",
			results: map[string][]ProviderResultWithSeq{
				"provider2": {
					{
						Result: providers.TranscriptionResult{
							Text:         "hello",
							ProviderName: "provider2",
						},
						SeqNum: 1,
					},
				},
			},
			expectSent: nil,
		},
		{
			name:        "no results for new provider",
			oldProvider: "provider1",
			newProvider: "provider2",
			results: map[string][]ProviderResultWithSeq{
				"provider1": {
					{
						Result: providers.TranscriptionResult{
							Text:         "hello",
							ProviderName: "provider1",
						},
						SeqNum: 1,
					},
				},
			},
			expectSent: nil,
		},
		{
			name:        "new provider has higher sequence numbers",
			oldProvider: "provider1",
			newProvider: "provider2",
			results: map[string][]ProviderResultWithSeq{
				"provider1": {
					{
						Result: providers.TranscriptionResult{
							Text:         "old1",
							ProviderName: "provider1",
						},
						SeqNum: 1,
					},
					{
						Result: providers.TranscriptionResult{
							Text:         "old2",
							ProviderName: "provider1",
						},
						SeqNum: 2,
					},
				},
				"provider2": {
					{
						Result: providers.TranscriptionResult{
							Text:         "new1",
							ProviderName: "provider2",
						},
						SeqNum: 1,
					},
					{
						Result: providers.TranscriptionResult{
							Text:         "new3",
							ProviderName: "provider2",
						},
						SeqNum: 3,
					},
					{
						Result: providers.TranscriptionResult{
							Text:         "new4",
							ProviderName: "provider2",
						},
						SeqNum: 4,
					},
				},
			},
			expectSent: []providers.TranscriptionResult{
				{
					Text:         "new3",
					ProviderName: "provider2",
				},
				{
					Text:         "new4",
					ProviderName: "provider2",
				},
			},
		},
		{
			name:        "new provider has same or lower sequence numbers",
			oldProvider: "provider1",
			newProvider: "provider2",
			results: map[string][]ProviderResultWithSeq{
				"provider1": {
					{
						Result: providers.TranscriptionResult{
							Text:         "old1",
							ProviderName: "provider1",
						},
						SeqNum: 3,
					},
				},
				"provider2": {
					{
						Result: providers.TranscriptionResult{
							Text:         "new1",
							ProviderName: "provider2",
						},
						SeqNum: 1,
					},
					{
						Result: providers.TranscriptionResult{
							Text:         "new2",
							ProviderName: "provider2",
						},
						SeqNum: 2,
					},
				},
			},
			expectSent: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a provider selector with buffered channel to capture sent messages
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			ps := &ProviderSelector{
				providerResults:     tt.results,
				transcriptionOutput: make(chan providers.TranscriptionResult, 10),
				ctx:                 ctx,
				cancel:              cancel,
				log:                 log.New(&ThreadSafeBuffer{}, "", 0),
			}

			// Call the method under test
			ps.sendMissedMessages(tt.oldProvider, tt.newProvider)

			// Collect sent messages
			var sentMessages []providers.TranscriptionResult
			done := false
			for !done {
				select {
				case msg := <-ps.transcriptionOutput:
					sentMessages = append(sentMessages, msg)
				case <-time.After(10 * time.Millisecond):
					done = true
				}
			}

			// Verify sent messages match expectations
			require.Equal(t, len(tt.expectSent), len(sentMessages), "Number of sent messages should match")
			for i, expected := range tt.expectSent {
				assert.Equal(t, expected.Text, sentMessages[i].Text)
				assert.Equal(t, expected.ProviderName, sentMessages[i].ProviderName)
			}
		})
	}
}

func TestProviderSelector_SendAudio(t *testing.T) {
	t.Run("returns io.EOF when context is canceled", func(t *testing.T) {
		// Create a provider selector with a cancelable context and unbuffered channel
		ctx, cancel := context.WithCancel(context.Background())

		ps := &ProviderSelector{
			audioInput: make(chan []byte), // Unbuffered channel so write will block
			ctx:        ctx,
			cancel:     cancel,
			log:        log.New(&ThreadSafeBuffer{}, "", 0),
		}

		// Cancel the context immediately
		cancel()

		// Try to send audio data - this will block on the channel write,
		// forcing the select to choose the context.Done() case
		audioData := []byte("test audio data")
		err := ps.SendAudio(audioData)

		// Verify that io.EOF is returned when context is canceled
		assert.ErrorIs(t, err, io.EOF)
	})

	t.Run("successfully sends audio when context is active", func(t *testing.T) {
		// Create a provider selector with an active context
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ps := &ProviderSelector{
			audioInput: make(chan []byte, 1),
			ctx:        ctx,
			cancel:     cancel,
			log:        log.New(&ThreadSafeBuffer{}, "", 0),
		}

		// Send audio data
		audioData := []byte("test audio data")
		err := ps.SendAudio(audioData)

		// Verify no error occurred
		assert.NoError(t, err)

		// Verify audio data was sent to the channel
		select {
		case receivedData := <-ps.audioInput:
			assert.Equal(t, audioData, receivedData)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Expected audio data to be sent to channel")
		}
	})
}
