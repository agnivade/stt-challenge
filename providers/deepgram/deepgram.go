package deepgram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	api "github.com/deepgram/deepgram-go-sdk/v3/pkg/api/listen/v1/websocket/interfaces"
	interfaces "github.com/deepgram/deepgram-go-sdk/v3/pkg/client/interfaces"
	client "github.com/deepgram/deepgram-go-sdk/v3/pkg/client/listen"

	"github.com/agnivade/stt_challenge/providers"
)

const providerName = "deepgram"

// dgWriter is a local interface that wraps the methods we need
// from listenv1ws.WSCallback to enable easier testing
type dgWriter interface {
	io.Writer
	Stop()
}

// ChannelHandler implements the LiveMessageChan interface for receiving Deepgram messages
type ChannelHandler struct {
	openChan          chan *api.OpenResponse
	messageChan       chan *api.MessageResponse
	metadataChan      chan *api.MetadataResponse
	speechStartedChan chan *api.SpeechStartedResponse
	utteranceEndChan  chan *api.UtteranceEndResponse
	closeChan         chan *api.CloseResponse
	errorChan         chan *api.ErrorResponse
	unhandledChan     chan *[]byte
}

// NewChannelHandler creates a new handler with initialized channels
func NewChannelHandler() *ChannelHandler {
	return &ChannelHandler{
		openChan:          make(chan *api.OpenResponse, 1),
		messageChan:       make(chan *api.MessageResponse, 10),
		metadataChan:      make(chan *api.MetadataResponse, 1),
		speechStartedChan: make(chan *api.SpeechStartedResponse, 1),
		utteranceEndChan:  make(chan *api.UtteranceEndResponse, 1),
		closeChan:         make(chan *api.CloseResponse, 1),
		errorChan:         make(chan *api.ErrorResponse, 1),
		unhandledChan:     make(chan *[]byte, 1),
	}
}

// GetOpen returns slice of channels for open events
func (ch *ChannelHandler) GetOpen() []*chan *api.OpenResponse {
	return []*chan *api.OpenResponse{&ch.openChan}
}

// GetMessage returns slice of channels for message events
func (ch *ChannelHandler) GetMessage() []*chan *api.MessageResponse {
	return []*chan *api.MessageResponse{&ch.messageChan}
}

// GetMetadata returns slice of channels for metadata events
func (ch *ChannelHandler) GetMetadata() []*chan *api.MetadataResponse {
	return []*chan *api.MetadataResponse{&ch.metadataChan}
}

// GetSpeechStarted returns slice of channels for speech started events
func (ch *ChannelHandler) GetSpeechStarted() []*chan *api.SpeechStartedResponse {
	return []*chan *api.SpeechStartedResponse{&ch.speechStartedChan}
}

// GetUtteranceEnd returns slice of channels for utterance end events
func (ch *ChannelHandler) GetUtteranceEnd() []*chan *api.UtteranceEndResponse {
	return []*chan *api.UtteranceEndResponse{&ch.utteranceEndChan}
}

// GetClose returns slice of channels for close events
func (ch *ChannelHandler) GetClose() []*chan *api.CloseResponse {
	return []*chan *api.CloseResponse{&ch.closeChan}
}

// GetError returns slice of channels for error events
func (ch *ChannelHandler) GetError() []*chan *api.ErrorResponse {
	return []*chan *api.ErrorResponse{&ch.errorChan}
}

// GetUnhandled returns slice of channels for unhandled events
func (ch *ChannelHandler) GetUnhandled() []*chan *[]byte {
	return []*chan *[]byte{&ch.unhandledChan}
}

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

	// Create channel handler
	channelHandler := NewChannelHandler()

	// Create Deepgram WebSocket client using channels
	dgClient, err := client.NewWSUsingChan(ctx, "", cOptions, tOptions, channelHandler)
	if err != nil {
		return nil, err
	}

	// Create session
	session := &Session{
		ctx:            ctx,
		client:         dgClient,
		channelHandler: channelHandler,
	}

	// Connect to Deepgram
	if success := dgClient.Connect(); !success {
		return nil, errors.New("failed to connect to deepgram")
	}

	return session, nil
}

// Session implements the providers.Session interface for Deepgram's speech-to-text API.
type Session struct {
	ctx            context.Context
	client         dgWriter
	channelHandler *ChannelHandler
}

// SendAudio sends audio data to the Deepgram stream.
func (s *Session) SendAudio(audioData []byte) error {
	_, err := s.client.Write(audioData)
	return err
}

// ReceiveTranscription receives transcription results from the Deepgram stream.
// It blocks until a final result is available or an error occurs.
func (s *Session) ReceiveTranscription() (providers.TranscriptionResult, error) {
	for {
		select {
		case msg := <-s.channelHandler.messageChan:
			if msg == nil {
				continue
			}
			result := s.processMessage(msg)
			if result != nil {
				return *result, nil
			}
		case err := <-s.channelHandler.errorChan:
			if err != nil {
				return providers.TranscriptionResult{}, fmt.Errorf("%s", err)
			}
		case <-s.channelHandler.closeChan:
			// Connection closed by Deepgram
			return providers.TranscriptionResult{}, io.EOF
		case <-s.channelHandler.openChan:
			// Consume open events (no action needed)
		case <-s.channelHandler.metadataChan:
			// Consume metadata events (no action needed)
		case <-s.channelHandler.speechStartedChan:
			// Consume speech started events (no action needed)
		case <-s.channelHandler.utteranceEndChan:
			// Consume utterance end events (no action needed)
		case <-s.channelHandler.unhandledChan:
			// Consume unhandled events (no action needed)
		case <-s.ctx.Done():
			if s.ctx.Err() == context.Canceled {
				return providers.TranscriptionResult{}, io.EOF
			}
			return providers.TranscriptionResult{}, s.ctx.Err()
		}
	}
}

// processMessage processes a single transcription message and returns a result if it should be sent
func (s *Session) processMessage(msg *api.MessageResponse) *providers.TranscriptionResult {
	// Process transcription results
	if len(msg.Channel.Alternatives) == 0 {
		return nil
	}

	alternative := msg.Channel.Alternatives[0]
	sentence := strings.TrimSpace(alternative.Transcript)
	if sentence == "" {
		return nil
	}

	result := &providers.TranscriptionResult{
		Text:         sentence,
		IsFinal:      msg.IsFinal,
		Confidence:   float32(alternative.Confidence),
		ProviderName: providerName,
		ReceivedAt:   time.Now(),
	}

	// Only return final results to match our interface expectation
	if result.IsFinal {
		return result
	}
	return nil
}

// Close closes the Deepgram session.
func (s *Session) Close() error {
	if s.client != nil {
		s.client.Stop()
	}

	// Closing the channels manually leads to race conditions because
	// the deepgram client still tries to send any in-flight messages to those channels.
	// Even the deepgram demo doesn't close the channels. So we leave it like this.
	// close(s.channelHandler.openChan)
	// close(s.channelHandler.messageChan)
	// close(s.channelHandler.metadataChan)
	// close(s.channelHandler.speechStartedChan)
	// close(s.channelHandler.utteranceEndChan)
	// close(s.channelHandler.closeChan)
	// close(s.channelHandler.errorChan)
	// close(s.channelHandler.unhandledChan)

	return nil
}
