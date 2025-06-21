package providers

import (
	"context"
)

// Provider creates transcription sessions for streaming speech-to-text conversion.
// Different providers can implement this interface to support various speech services
// like Google Speech, Azure Speech, AWS Transcribe, etc.
type Provider interface {
	// NewSession creates a new transcription session with the given configuration.
	// The context can be used to cancel the session creation process.
	NewSession(ctx context.Context, config SessionConfig) (Session, error)
}

// Session handles streaming transcription for a single connection.
// It manages the lifecycle of audio streaming and transcription result retrieval.
type Session interface {
	// SendAudio sends raw audio data to the transcription service.
	// Audio data should match the format specified in SessionConfig.
	SendAudio(audioData []byte) error

	// ReceiveTranscription blocks until a transcription result is available.
	// It returns the transcription result or an error if transcription fails.
	// Returns io.EOF when the session is closed and no more results are available.
	ReceiveTranscription() (TranscriptionResult, error)

	// Close gracefully closes the transcription session and releases resources.
	// After calling Close, SendAudio and ReceiveTranscription should not be called.
	// Also, care must be taken that the readers and writers must be stopped
	// before calling Close.
	Close() error
}

// SessionConfig holds provider-agnostic configuration for transcription sessions.
// Providers can extend this with provider-specific options using the Extensions field.
type SessionConfig struct {
	// SampleRate is the audio sample rate in Hz (e.g., 16000)
	SampleRate int

	// LanguageCode specifies the language for transcription (e.g., "en-US")
	LanguageCode string

	// InterimResults indicates whether to return interim (non-final) results
	InterimResults bool

	// Extensions allows providers to specify additional configuration options
	// using a map of key-value pairs specific to their implementation
	Extensions map[string]interface{}
}

// TranscriptionResult represents a transcription result with metadata.
type TranscriptionResult struct {
	// Text is the transcribed text
	Text string

	// IsFinal indicates whether this is a final result or interim
	IsFinal bool

	// Confidence is the confidence score (0.0 to 1.0) if available
	Confidence float32
}
