package google

import (
	"context"
	"errors"
	"io"
	"time"

	speech "cloud.google.com/go/speech/apiv1"
	"cloud.google.com/go/speech/apiv1/speechpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/agnivade/stt_challenge/providers"
)

const providerName = "google"

// streamingRecognizeClient is a local interface that wraps the methods we need
// from speechpb.Speech_StreamingRecognizeClient to enable easier testing
type streamingRecognizeClient interface {
	Send(*speechpb.StreamingRecognizeRequest) error
	Recv() (*speechpb.StreamingRecognizeResponse, error)
	CloseSend() error
}

// Provider implements the providers.Provider interface for Google Speech-to-Text API.
type Provider struct {
	client *speech.Client
}

// NewProvider creates a new Google Speech provider with the given client.
func NewProvider(client *speech.Client) *Provider {
	return &Provider{
		client: client,
	}
}

// Name returns the name of the provider.
func (p *Provider) Name() string {
	return providerName
}

// NewSession creates a new Google Speech transcription session.
func (p *Provider) NewSession(ctx context.Context, config providers.SessionConfig) (providers.Session, error) {
	stream, err := p.client.StreamingRecognize(ctx)
	if err != nil {
		return nil, err
	}

	// Send initial configuration
	req := &speechpb.StreamingRecognizeRequest{
		StreamingRequest: &speechpb.StreamingRecognizeRequest_StreamingConfig{
			StreamingConfig: &speechpb.StreamingRecognitionConfig{
				Config: &speechpb.RecognitionConfig{
					Encoding:        speechpb.RecognitionConfig_LINEAR16,
					SampleRateHertz: int32(config.SampleRate),
					LanguageCode:    config.LanguageCode,
				},
				InterimResults: config.InterimResults,
			},
		},
	}

	if err := stream.Send(req); err != nil {
		stream.CloseSend()
		return nil, err
	}

	return &Session{
		stream: stream,
		ctx:    ctx,
	}, nil
}

// Session implements the providers.Session interface for Google Speech-to-Text API.
type Session struct {
	stream streamingRecognizeClient
	ctx    context.Context
}

// SendAudio sends audio data to the Google Speech stream.
func (s *Session) SendAudio(audioData []byte) error {
	req := &speechpb.StreamingRecognizeRequest{
		StreamingRequest: &speechpb.StreamingRecognizeRequest_AudioContent{
			AudioContent: audioData,
		},
	}
	return s.stream.Send(req)
}

// ReceiveTranscription receives transcription results from the Google Speech stream.
// It blocks until a final result is available or an error occurs.
func (s *Session) ReceiveTranscription() (providers.TranscriptionResult, error) {
	for {
		resp, err := s.stream.Recv()
		if errors.Is(err, io.EOF) || status.Code(err) == codes.Canceled {
			return providers.TranscriptionResult{}, io.EOF
		}
		if err != nil {
			return providers.TranscriptionResult{}, err
		}

		for _, result := range resp.Results {
			if result.IsFinal && len(result.Alternatives) > 0 {
				alt := result.Alternatives[0]
				return providers.TranscriptionResult{
					Text:         alt.Transcript,
					IsFinal:      true,
					Confidence:   alt.Confidence,
					ProviderName: providerName,
					ReceivedAt:   time.Now(),
				}, nil
			}
		}
		// Continue loop if no final results found
	}
}

// Close closes the Google Speech stream.
func (s *Session) Close() error {
	return s.stream.CloseSend()
}
