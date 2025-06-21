package google

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/genproto/googleapis/cloud/speech/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/agnivade/stt_challenge/providers"
)

func TestSession_SendAudio(t *testing.T) {
	tests := []struct {
		name        string
		audioData   []byte
		setupMock   func(*mockstreamingRecognizeClient)
		expectedErr error
	}{
		{
			name:      "successful send",
			audioData: []byte("test audio data"),
			setupMock: func(m *mockstreamingRecognizeClient) {
				m.EXPECT().Send(mock.AnythingOfType("*speechpb.StreamingRecognizeRequest")).Return(nil)
			},
			expectedErr: nil,
		},
		{
			name:      "send error",
			audioData: []byte("test audio data"),
			setupMock: func(m *mockstreamingRecognizeClient) {
				m.EXPECT().Send(mock.AnythingOfType("*speechpb.StreamingRecognizeRequest")).Return(errors.New("send failed"))
			},
			expectedErr: errors.New("send failed"),
		},
		{
			name:      "empty audio data",
			audioData: []byte{},
			setupMock: func(m *mockstreamingRecognizeClient) {
				m.EXPECT().Send(mock.AnythingOfType("*speechpb.StreamingRecognizeRequest")).Return(nil)
			},
			expectedErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStream := newMockstreamingRecognizeClient(t)
			tt.setupMock(mockStream)

			session := &Session{
				stream: mockStream,
				ctx:    context.Background(),
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

func TestSession_ReceiveTranscription(t *testing.T) {
	tests := []struct {
		name           string
		setupMock      func(*mockstreamingRecognizeClient)
		expectedResult providers.TranscriptionResult
		expectedErr    error
	}{
		{
			name: "successful transcription with final result",
			setupMock: func(m *mockstreamingRecognizeClient) {
				response := &speech.StreamingRecognizeResponse{
					Results: []*speech.StreamingRecognitionResult{
						{
							IsFinal: true,
							Alternatives: []*speech.SpeechRecognitionAlternative{
								{
									Transcript: "hello world",
									Confidence: 0.95,
								},
							},
						},
					},
				}
				m.EXPECT().Recv().Return(response, nil)
			},
			expectedResult: providers.TranscriptionResult{
				Text:         "hello world",
				IsFinal:      true,
				Confidence:   0.95,
				ProviderName: "google",
			},
			expectedErr: nil,
		},
		{
			name: "non-final result followed by final result",
			setupMock: func(m *mockstreamingRecognizeClient) {
				// First call returns non-final result
				nonFinalResponse := &speech.StreamingRecognizeResponse{
					Results: []*speech.StreamingRecognitionResult{
						{
							IsFinal: false,
							Alternatives: []*speech.SpeechRecognitionAlternative{
								{
									Transcript: "hello",
									Confidence: 0.8,
								},
							},
						},
					},
				}
				// Second call returns final result
				finalResponse := &speech.StreamingRecognizeResponse{
					Results: []*speech.StreamingRecognitionResult{
						{
							IsFinal: true,
							Alternatives: []*speech.SpeechRecognitionAlternative{
								{
									Transcript: "hello world",
									Confidence: 0.95,
								},
							},
						},
					},
				}
				m.EXPECT().Recv().Return(nonFinalResponse, nil).Once()
				m.EXPECT().Recv().Return(finalResponse, nil).Once()
			},
			expectedResult: providers.TranscriptionResult{
				Text:         "hello world",
				IsFinal:      true,
				Confidence:   0.95,
				ProviderName: "google",
			},
			expectedErr: nil,
		},
		{
			name: "empty alternatives",
			setupMock: func(m *mockstreamingRecognizeClient) {
				// First response with empty alternatives
				emptyResponse := &speech.StreamingRecognizeResponse{
					Results: []*speech.StreamingRecognitionResult{
						{
							IsFinal:      true,
							Alternatives: []*speech.SpeechRecognitionAlternative{},
						},
					},
				}
				// Second response with valid alternatives
				validResponse := &speech.StreamingRecognizeResponse{
					Results: []*speech.StreamingRecognitionResult{
						{
							IsFinal: true,
							Alternatives: []*speech.SpeechRecognitionAlternative{
								{
									Transcript: "test",
									Confidence: 0.9,
								},
							},
						},
					},
				}
				m.EXPECT().Recv().Return(emptyResponse, nil).Once()
				m.EXPECT().Recv().Return(validResponse, nil).Once()
			},
			expectedResult: providers.TranscriptionResult{
				Text:         "test",
				IsFinal:      true,
				Confidence:   0.9,
				ProviderName: "google",
			},
			expectedErr: nil,
		},
		{
			name: "io.EOF error",
			setupMock: func(m *mockstreamingRecognizeClient) {
				m.EXPECT().Recv().Return(nil, io.EOF)
			},
			expectedResult: providers.TranscriptionResult{},
			expectedErr:    io.EOF,
		},
		{
			name: "context canceled error",
			setupMock: func(m *mockstreamingRecognizeClient) {
				m.EXPECT().Recv().Return(nil, status.Error(codes.Canceled, "context canceled"))
			},
			expectedResult: providers.TranscriptionResult{},
			expectedErr:    io.EOF,
		},
		{
			name: "other grpc error",
			setupMock: func(m *mockstreamingRecognizeClient) {
				m.EXPECT().Recv().Return(nil, status.Error(codes.Internal, "internal error"))
			},
			expectedResult: providers.TranscriptionResult{},
			expectedErr:    status.Error(codes.Internal, "internal error"),
		},
		{
			name: "generic error",
			setupMock: func(m *mockstreamingRecognizeClient) {
				m.EXPECT().Recv().Return(nil, errors.New("network error"))
			},
			expectedResult: providers.TranscriptionResult{},
			expectedErr:    errors.New("network error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStream := newMockstreamingRecognizeClient(t)
			tt.setupMock(mockStream)

			session := &Session{
				stream: mockStream,
				ctx:    context.Background(),
			}

			result, err := session.ReceiveTranscription()

			if tt.expectedErr != nil {
				assert.Error(t, err)
				if errors.Is(tt.expectedErr, io.EOF) {
					assert.ErrorIs(t, err, io.EOF)
				} else {
					assert.Equal(t, tt.expectedErr.Error(), err.Error())
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult.Text, result.Text)
				assert.Equal(t, tt.expectedResult.IsFinal, result.IsFinal)
				assert.Equal(t, tt.expectedResult.Confidence, result.Confidence)
				assert.Equal(t, tt.expectedResult.ProviderName, result.ProviderName)
				// Check that ReceivedAt is set and recent
				assert.True(t, result.ReceivedAt.After(time.Now().Add(-time.Second)))
				assert.True(t, result.ReceivedAt.Before(time.Now().Add(time.Second)))
			}
		})
	}
}

func TestSession_Close(t *testing.T) {
	tests := []struct {
		name        string
		setupMock   func(*mockstreamingRecognizeClient)
		expectedErr error
	}{
		{
			name: "successful close",
			setupMock: func(m *mockstreamingRecognizeClient) {
				m.EXPECT().CloseSend().Return(nil)
			},
			expectedErr: nil,
		},
		{
			name: "close error",
			setupMock: func(m *mockstreamingRecognizeClient) {
				m.EXPECT().CloseSend().Return(errors.New("close failed"))
			},
			expectedErr: errors.New("close failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStream := newMockstreamingRecognizeClient(t)
			tt.setupMock(mockStream)

			session := &Session{
				stream: mockStream,
				ctx:    context.Background(),
			}

			err := session.Close()

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
