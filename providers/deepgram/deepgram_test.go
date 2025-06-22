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
func createTestSession() (*Session, *ChannelHandler) {
	channelHandler := NewChannelHandler()
	session := &Session{
		ctx:            context.Background(),
		channelHandler: channelHandler,
	}
	return session, channelHandler
}

func TestSession_ProcessMessage(t *testing.T) {
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
			name: "non-final result - should not return",
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
			name: "empty alternatives - should not return",
			messageResp: &api.MessageResponse{
				IsFinal: true,
				Channel: api.Channel{
					Alternatives: []api.Alternative{},
				},
			},
			expectResult: false,
		},
		{
			name: "empty transcript after trimming - should not return",
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
			name: "empty transcript - should not return",
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
			session, _ := createTestSession()

			// Call the processMessage method directly
			result := session.processMessage(tt.messageResp)

			if tt.expectResult {
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedResult.Text, result.Text)
				assert.Equal(t, tt.expectedResult.IsFinal, result.IsFinal)
				assert.Equal(t, tt.expectedResult.Confidence, result.Confidence)
				assert.Equal(t, tt.expectedResult.ProviderName, result.ProviderName)
				// Check that ReceivedAt is set and recent
				assert.True(t, result.ReceivedAt.After(time.Now().Add(-time.Second)))
				assert.True(t, result.ReceivedAt.Before(time.Now().Add(time.Second)))
			} else {
				assert.Nil(t, result)
			}
		})
	}
}

func TestSession_ReceiveTranscription_MessageChannel(t *testing.T) {
	session, channelHandler := createTestSession()

	// Test receiving a final message
	go func() {
		time.Sleep(10 * time.Millisecond) // Small delay to ensure ReceiveTranscription is waiting
		channelHandler.messageChan <- &api.MessageResponse{
			IsFinal: true,
			Channel: api.Channel{
				Alternatives: []api.Alternative{
					{
						Transcript: "hello world",
						Confidence: 0.95,
					},
				},
			},
		}
	}()

	result, err := session.ReceiveTranscription()
	assert.NoError(t, err)
	assert.Equal(t, "hello world", result.Text)
	assert.True(t, result.IsFinal)
	assert.Equal(t, float32(0.95), result.Confidence)
	assert.Equal(t, "deepgram", result.ProviderName)
}

func TestSession_ReceiveTranscription_SkipNonFinal(t *testing.T) {
	session, channelHandler := createTestSession()

	// Test that non-final messages are skipped
	go func() {
		time.Sleep(10 * time.Millisecond)
		// Send a non-final message first
		channelHandler.messageChan <- &api.MessageResponse{
			IsFinal: false,
			Channel: api.Channel{
				Alternatives: []api.Alternative{
					{
						Transcript: "hello",
						Confidence: 0.8,
					},
				},
			},
		}
		// Then send a final message
		channelHandler.messageChan <- &api.MessageResponse{
			IsFinal: true,
			Channel: api.Channel{
				Alternatives: []api.Alternative{
					{
						Transcript: "hello world",
						Confidence: 0.95,
					},
				},
			},
		}
	}()

	result, err := session.ReceiveTranscription()
	assert.NoError(t, err)
	assert.Equal(t, "hello world", result.Text) // Should get the final message, not the non-final
	assert.True(t, result.IsFinal)
}

func TestSession_ReceiveTranscription_ErrorChannel(t *testing.T) {
	session, channelHandler := createTestSession()

	// Test receiving an error
	go func() {
		time.Sleep(10 * time.Millisecond)
		channelHandler.errorChan <- &api.ErrorResponse{
			Type:        "error",
			Description: "test error",
		}
	}()

	_, err := session.ReceiveTranscription()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "test error")
}

func TestSession_ReceiveTranscription_CloseChannel(t *testing.T) {
	session, channelHandler := createTestSession()

	// Test receiving a close event
	go func() {
		time.Sleep(10 * time.Millisecond)
		channelHandler.closeChan <- &api.CloseResponse{}
	}()

	_, err := session.ReceiveTranscription()
	assert.Error(t, err)
	assert.ErrorIs(t, err, io.EOF)
}

func TestSession_ReceiveTranscription_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	channelHandler := NewChannelHandler()
	session := &Session{
		ctx:            ctx,
		channelHandler: channelHandler,
	}

	// Cancel the context
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err := session.ReceiveTranscription()
	assert.Error(t, err)
	assert.ErrorIs(t, err, io.EOF)
}

func TestSession_ReceiveTranscription_ContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	channelHandler := NewChannelHandler()
	session := &Session{
		ctx:            ctx,
		channelHandler: channelHandler,
	}

	_, err := session.ReceiveTranscription()
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestSession_ReceiveTranscription_ConsumeOtherChannels(t *testing.T) {
	session, channelHandler := createTestSession()

	// Test that other channel events are consumed but don't affect the result
	go func() {
		time.Sleep(10 * time.Millisecond)
		// Send events to other channels
		channelHandler.openChan <- &api.OpenResponse{}
		channelHandler.metadataChan <- &api.MetadataResponse{}
		channelHandler.speechStartedChan <- &api.SpeechStartedResponse{}
		channelHandler.utteranceEndChan <- &api.UtteranceEndResponse{}
		unhandledData := []byte("test")
		channelHandler.unhandledChan <- &unhandledData

		// Then send the actual message we want
		time.Sleep(10 * time.Millisecond)
		channelHandler.messageChan <- &api.MessageResponse{
			IsFinal: true,
			Channel: api.Channel{
				Alternatives: []api.Alternative{
					{
						Transcript: "hello world",
						Confidence: 0.95,
					},
				},
			},
		}
	}()

	result, err := session.ReceiveTranscription()
	assert.NoError(t, err)
	assert.Equal(t, "hello world", result.Text)
}

func TestSession_SendAudio(t *testing.T) {
	tests := []struct {
		name        string
		audioData   []byte
		setupMock   func(*mockdgWriter)
		expectedErr error
	}{
		{
			name:      "successful send",
			audioData: []byte("test audio data"),
			setupMock: func(m *mockdgWriter) {
				m.EXPECT().Write([]byte("test audio data")).Return(len("test audio data"), nil)
			},
			expectedErr: nil,
		},
		{
			name:      "write error",
			audioData: []byte("test audio data"),
			setupMock: func(m *mockdgWriter) {
				m.EXPECT().Write([]byte("test audio data")).Return(0, errors.New("write failed"))
			},
			expectedErr: errors.New("write failed"),
		},
		{
			name:      "empty audio data",
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
				ctx:    context.Background(),
				client: mockClient,
			}

			err := session.SendAudio(tt.audioData)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSession_Close(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(*mockdgWriter)
	}{
		{
			name: "successful close",
			setupMock: func(m *mockdgWriter) {
				m.EXPECT().Stop().Return()
			},
		},
		{
			name:      "close with nil client",
			setupMock: func(m *mockdgWriter) {}, // No mock expectations for nil client test
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

			channelHandler := NewChannelHandler()
			session := &Session{
				ctx:            context.Background(),
				client:         mockClient,
				channelHandler: channelHandler,
			}

			err := session.Close()
			assert.NoError(t, err)
		})
	}
}

func TestChannelHandler_InterfaceMethods(t *testing.T) {
	handler := NewChannelHandler()

	// Test that all interface methods return the correct channel pointers
	t.Run("GetOpen", func(t *testing.T) {
		channels := handler.GetOpen()
		assert.Len(t, channels, 1)
		assert.Equal(t, &handler.openChan, channels[0])
	})

	t.Run("GetMessage", func(t *testing.T) {
		channels := handler.GetMessage()
		assert.Len(t, channels, 1)
		assert.Equal(t, &handler.messageChan, channels[0])
	})

	t.Run("GetMetadata", func(t *testing.T) {
		channels := handler.GetMetadata()
		assert.Len(t, channels, 1)
		assert.Equal(t, &handler.metadataChan, channels[0])
	})

	t.Run("GetSpeechStarted", func(t *testing.T) {
		channels := handler.GetSpeechStarted()
		assert.Len(t, channels, 1)
		assert.Equal(t, &handler.speechStartedChan, channels[0])
	})

	t.Run("GetUtteranceEnd", func(t *testing.T) {
		channels := handler.GetUtteranceEnd()
		assert.Len(t, channels, 1)
		assert.Equal(t, &handler.utteranceEndChan, channels[0])
	})

	t.Run("GetClose", func(t *testing.T) {
		channels := handler.GetClose()
		assert.Len(t, channels, 1)
		assert.Equal(t, &handler.closeChan, channels[0])
	})

	t.Run("GetError", func(t *testing.T) {
		channels := handler.GetError()
		assert.Len(t, channels, 1)
		assert.Equal(t, &handler.errorChan, channels[0])
	})

	t.Run("GetUnhandled", func(t *testing.T) {
		channels := handler.GetUnhandled()
		assert.Len(t, channels, 1)
		assert.Equal(t, &handler.unhandledChan, channels[0])
	})
}
