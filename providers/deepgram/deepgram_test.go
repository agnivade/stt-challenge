package deepgram

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/agnivade/stt_challenge/providers"
	api "github.com/deepgram/deepgram-go-sdk/v3/pkg/api/listen/v1/websocket/interfaces"
)

// createTestSession creates a minimal session for testing
func createTestSession() *Session {
	return &Session{
		ctx:           context.Background(),
		resultChannel: make(chan providers.TranscriptionResult, 10),
		errorChannel:  make(chan error, 1),
		closed:        false,
	}
}

func TestCallbackHandler_Message(t *testing.T) {
	tests := []struct {
		name           string
		messageResp    *api.MessageResponse
		expectResult   bool
		expectedResult providers.TranscriptionResult
	}{
		{
			name: "final result with valid transcript",
			messageResp: &api.MessageResponse{
				IsFinal: true,
				Channel: api.Channel{
					Alternatives: []api.Alternative{
						{
							Transcript: "hello world",
							Confidence: 0.95,
						},
					},
				},
			},
			expectResult: true,
			expectedResult: providers.TranscriptionResult{
				Text:         "hello world",
				IsFinal:      true,
				Confidence:   0.95,
				ProviderName: "deepgram",
			},
		},
		{
			name: "final result with whitespace trimming",
			messageResp: &api.MessageResponse{
				IsFinal: true,
				Channel: api.Channel{
					Alternatives: []api.Alternative{
						{
							Transcript: "  hello world  ",
							Confidence: 0.9,
						},
					},
				},
			},
			expectResult: true,
			expectedResult: providers.TranscriptionResult{
				Text:         "hello world",
				IsFinal:      true,
				Confidence:   0.9,
				ProviderName: "deepgram",
			},
		},
		{
			name: "non-final result - should not send",
			messageResp: &api.MessageResponse{
				IsFinal: false,
				Channel: api.Channel{
					Alternatives: []api.Alternative{
						{
							Transcript: "hello",
							Confidence: 0.8,
						},
					},
				},
			},
			expectResult: false,
		},
		{
			name: "empty alternatives - should not send",
			messageResp: &api.MessageResponse{
				IsFinal: true,
				Channel: api.Channel{
					Alternatives: []api.Alternative{},
				},
			},
			expectResult: false,
		},
		{
			name: "empty transcript after trimming - should not send",
			messageResp: &api.MessageResponse{
				IsFinal: true,
				Channel: api.Channel{
					Alternatives: []api.Alternative{
						{
							Transcript: "   ",
							Confidence: 0.5,
						},
					},
				},
			},
			expectResult: false,
		},
		{
			name: "empty transcript - should not send",
			messageResp: &api.MessageResponse{
				IsFinal: true,
				Channel: api.Channel{
					Alternatives: []api.Alternative{
						{
							Transcript: "",
							Confidence: 0.5,
						},
					},
				},
			},
			expectResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := createTestSession()
			handler := &CallbackHandler{
				session: session,
			}

			// Call the Message method
			err := handler.Message(tt.messageResp)
			assert.NoError(t, err)

			if tt.expectResult {
				// Should receive a result
				select {
				case result := <-session.resultChannel:
					assert.Equal(t, tt.expectedResult.Text, result.Text)
					assert.Equal(t, tt.expectedResult.IsFinal, result.IsFinal)
					assert.Equal(t, tt.expectedResult.Confidence, result.Confidence)
					assert.Equal(t, tt.expectedResult.ProviderName, result.ProviderName)
					// Check that ReceivedAt is set and recent
					assert.True(t, result.ReceivedAt.After(time.Now().Add(-time.Second)))
					assert.True(t, result.ReceivedAt.Before(time.Now().Add(time.Second)))
				case <-time.After(100 * time.Millisecond):
					t.Fatal("Expected to receive a transcription result")
				}
			} else {
				// Should not receive a result
				select {
				case result := <-session.resultChannel:
					t.Fatalf("Unexpected result received: %+v", result)
				case <-time.After(100 * time.Millisecond):
					// Expected - no result should be sent
				}
			}
		})
	}
}

func TestCallbackHandler_Error(t *testing.T) {
	tests := []struct {
		name        string
		errorResp   *api.ErrorResponse
		expectError bool
	}{
		{
			name: "valid error response",
			errorResp: &api.ErrorResponse{
				Type:        "error",
				Description: "test error",
			},
			expectError: true,
		},
		{
			name:        "nil error response",
			errorResp:   nil,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := createTestSession()
			handler := &CallbackHandler{
				session: session,
			}

			// Call the Error method
			err := handler.Error(tt.errorResp)
			assert.NoError(t, err)

			if tt.expectError {
				// Should receive an error
				select {
				case receivedErr := <-session.errorChannel:
					assert.Error(t, receivedErr)
					assert.Contains(t, receivedErr.Error(), "test error")
				case <-time.After(100 * time.Millisecond):
					t.Fatal("Expected to receive an error")
				}
			} else {
				// Should not receive an error
				select {
				case receivedErr := <-session.errorChannel:
					t.Fatalf("Unexpected error received: %v", receivedErr)
				case <-time.After(100 * time.Millisecond):
					// Expected - no error should be sent
				}
			}
		})
	}
}

func TestCallbackHandler_NoOpMethods(t *testing.T) {
	session := createTestSession()
	handler := &CallbackHandler{
		session: session,
	}

	// Test all the no-op methods - they should not return errors
	t.Run("Metadata", func(t *testing.T) {
		err := handler.Metadata(&api.MetadataResponse{})
		assert.NoError(t, err)
	})

	t.Run("SpeechStarted", func(t *testing.T) {
		err := handler.SpeechStarted(&api.SpeechStartedResponse{})
		assert.NoError(t, err)
	})

	t.Run("UtteranceEnd", func(t *testing.T) {
		err := handler.UtteranceEnd(&api.UtteranceEndResponse{})
		assert.NoError(t, err)
	})

	t.Run("Close", func(t *testing.T) {
		err := handler.Close(&api.CloseResponse{})
		assert.NoError(t, err)
	})

	t.Run("Open", func(t *testing.T) {
		err := handler.Open(&api.OpenResponse{})
		assert.NoError(t, err)
	})

	t.Run("UnhandledEvent", func(t *testing.T) {
		err := handler.UnhandledEvent([]byte("test message"))
		assert.NoError(t, err)
	})
}

func TestCallbackHandler_ChannelFull(t *testing.T) {
	// Create a session with a smaller channel buffer to test full channel behavior
	session := &Session{
		ctx:           context.Background(),
		resultChannel: make(chan providers.TranscriptionResult, 1), // Small buffer
		errorChannel:  make(chan error, 1),
		closed:        false,
	}

	handler := &CallbackHandler{
		session: session,
	}

	// Fill the channel first
	session.resultChannel <- providers.TranscriptionResult{Text: "first message"}

	// Now try to send another message - it should be dropped
	messageResp := &api.MessageResponse{
		IsFinal: true,
		Channel: api.Channel{
			Alternatives: []api.Alternative{
				{
					Transcript: "second message",
					Confidence: 0.9,
				},
			},
		},
	}

	err := handler.Message(messageResp)
	assert.NoError(t, err)

	// Verify only the first message is in the channel
	select {
	case result := <-session.resultChannel:
		assert.Equal(t, "first message", result.Text)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected to receive the first message")
	}

	// Verify no second message was queued (channel should be empty now)
	select {
	case result := <-session.resultChannel:
		t.Fatalf("Unexpected second message received: %+v", result)
	case <-time.After(100 * time.Millisecond):
		// Expected - second message should have been dropped
	}
}

func TestCallbackHandler_ErrorChannelFull(t *testing.T) {
	// Create a session with a full error channel
	session := &Session{
		ctx:           context.Background(),
		resultChannel: make(chan providers.TranscriptionResult, 10),
		errorChannel:  make(chan error, 1),
		closed:        false,
	}

	// Fill the error channel
	session.errorChannel <- assert.AnError

	handler := &CallbackHandler{
		session: session,
	}

	// Try to send another error - it should be dropped
	errorResp := &api.ErrorResponse{
		Type:        "error",
		Description: "second error",
	}

	err := handler.Error(errorResp)
	assert.NoError(t, err)

	// Verify only the first error is in the channel
	select {
	case receivedErr := <-session.errorChannel:
		assert.Equal(t, assert.AnError, receivedErr)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected to receive the first error")
	}

	// Verify no second error was queued (channel should be empty now)
	select {
	case receivedErr := <-session.errorChannel:
		t.Fatalf("Unexpected second error received: %v", receivedErr)
	case <-time.After(100 * time.Millisecond):
		// Expected - second error should have been dropped
	}
}

func TestSession_SendAudio(t *testing.T) {
	tests := []struct {
		name        string
		closed      bool
		audioData   []byte
		setupMock   func(*mockdgWriter)
		expectedErr error
	}{
		{
			name:      "successful send",
			closed:    false,
			audioData: []byte("test audio data"),
			setupMock: func(m *mockdgWriter) {
				m.EXPECT().Write([]byte("test audio data")).Return(len("test audio data"), nil)
			},
			expectedErr: nil,
		},
		{
			name:      "write error",
			closed:    false,
			audioData: []byte("test audio data"),
			setupMock: func(m *mockdgWriter) {
				m.EXPECT().Write([]byte("test audio data")).Return(0, errors.New("write failed"))
			},
			expectedErr: errors.New("write failed"),
		},
		{
			name:        "session closed",
			closed:      true,
			audioData:   []byte("test audio data"),
			setupMock:   func(m *mockdgWriter) {}, // No mock setup needed when closed
			expectedErr: io.EOF,
		},
		{
			name:      "empty audio data",
			closed:    false,
			audioData: []byte{},
			setupMock: func(m *mockdgWriter) {
				m.EXPECT().Write([]byte{}).Return(0, nil)
			},
			expectedErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockdgWriter(t)
			tt.setupMock(mockClient)

			session := &Session{
				ctx:           context.Background(),
				client:        mockClient,
				resultChannel: make(chan providers.TranscriptionResult, 10),
				errorChannel:  make(chan error, 1),
				closed:        tt.closed,
			}

			err := session.SendAudio(tt.audioData)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				if errors.Is(tt.expectedErr, io.EOF) {
					assert.ErrorIs(t, err, io.EOF)
				} else {
					assert.Equal(t, tt.expectedErr.Error(), err.Error())
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSession_ReceiveTranscription(t *testing.T) {
	tests := []struct {
		name           string
		setupSession   func() *Session
		expectedResult providers.TranscriptionResult
		expectedErr    error
	}{
		{
			name: "receive result from channel",
			setupSession: func() *Session {
				session := &Session{
					ctx:           context.Background(),
					resultChannel: make(chan providers.TranscriptionResult, 1),
					errorChannel:  make(chan error, 1),
				}
				// Pre-populate the result channel
				session.resultChannel <- providers.TranscriptionResult{
					Text:         "hello world",
					IsFinal:      true,
					Confidence:   0.95,
					ProviderName: "deepgram",
				}
				return session
			},
			expectedResult: providers.TranscriptionResult{
				Text:         "hello world",
				IsFinal:      true,
				Confidence:   0.95,
				ProviderName: "deepgram",
			},
			expectedErr: nil,
		},
		{
			name: "receive error from channel",
			setupSession: func() *Session {
				session := &Session{
					ctx:           context.Background(),
					resultChannel: make(chan providers.TranscriptionResult, 1),
					errorChannel:  make(chan error, 1),
				}
				// Pre-populate the error channel
				session.errorChannel <- errors.New("transcription error")
				return session
			},
			expectedResult: providers.TranscriptionResult{},
			expectedErr:    errors.New("transcription error"),
		},
		{
			name: "context canceled",
			setupSession: func() *Session {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately
				return &Session{
					ctx:           ctx,
					resultChannel: make(chan providers.TranscriptionResult, 1),
					errorChannel:  make(chan error, 1),
				}
			},
			expectedResult: providers.TranscriptionResult{},
			expectedErr:    io.EOF,
		},
		{
			name: "context with deadline exceeded",
			setupSession: func() *Session {
				ctx, cancel := context.WithTimeout(context.Background(), -time.Second) // Already expired
				defer cancel()
				return &Session{
					ctx:           ctx,
					resultChannel: make(chan providers.TranscriptionResult, 1),
					errorChannel:  make(chan error, 1),
				}
			},
			expectedResult: providers.TranscriptionResult{},
			expectedErr:    context.DeadlineExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := tt.setupSession()

			result, err := session.ReceiveTranscription()

			if tt.expectedErr != nil {
				assert.Error(t, err)
				if errors.Is(tt.expectedErr, io.EOF) {
					assert.ErrorIs(t, err, io.EOF)
				} else if errors.Is(tt.expectedErr, context.DeadlineExceeded) {
					assert.ErrorIs(t, err, context.DeadlineExceeded)
				} else {
					assert.Equal(t, tt.expectedErr.Error(), err.Error())
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult.Text, result.Text)
				assert.Equal(t, tt.expectedResult.IsFinal, result.IsFinal)
				assert.Equal(t, tt.expectedResult.Confidence, result.Confidence)
				assert.Equal(t, tt.expectedResult.ProviderName, result.ProviderName)
			}
		})
	}
}

func TestSession_Close(t *testing.T) {
	tests := []struct {
		name         string
		alreadyClosed bool
		setupMock    func(*mockdgWriter)
		expectedErr  error
	}{
		{
			name:         "successful close",
			alreadyClosed: false,
			setupMock: func(m *mockdgWriter) {
				m.EXPECT().Stop().Return()
			},
			expectedErr: nil,
		},
		{
			name:         "already closed",
			alreadyClosed: true,
			setupMock:    func(m *mockdgWriter) {}, // No mock expectations when already closed
			expectedErr:  nil,
		},
		{
			name:         "close with nil client",
			alreadyClosed: false,
			setupMock:    func(m *mockdgWriter) {}, // No mock expectations for nil client test
			expectedErr:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mockClient dgWriter
			if tt.name == "close with nil client" {
				mockClient = nil
			} else {
				mock := newMockdgWriter(t)
				tt.setupMock(mock)
				mockClient = mock
			}

			session := &Session{
				ctx:           context.Background(),
				client:        mockClient,
				resultChannel: make(chan providers.TranscriptionResult, 10),
				errorChannel:  make(chan error, 1),
				closed:        tt.alreadyClosed,
			}

			err := session.Close()

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
				assert.True(t, session.closed)
			}
		})
	}
}
