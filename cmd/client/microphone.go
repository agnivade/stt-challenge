package main

import (
	"github.com/gordonklaus/portaudio"
)

const (
	sampleRate      = 16000
	framesPerBuffer = 1024
)

// MicrophoneReader implements io.ReadCloser for capturing audio from the microphone.
// It uses PortAudio to capture 16-bit PCM audio at 16kHz sample rate.
type MicrophoneReader struct {
	stream *portaudio.Stream
	buffer []int16
}

// NewMicrophoneReader creates a new MicrophoneReader that captures audio from the default input device.
// It initializes PortAudio, opens an audio stream, and starts recording.
// The caller must call Close() to properly clean up resources.
func NewMicrophoneReader() (*MicrophoneReader, error) {
	// Initialize PortAudio
	if err := portaudio.Initialize(); err != nil {
		return nil, err
	}

	// Create audio buffer
	buffer := make([]int16, framesPerBuffer)

	// Open default audio stream
	stream, err := portaudio.OpenDefaultStream(1, 0, float64(sampleRate), len(buffer), buffer)
	if err != nil {
		portaudio.Terminate()
		return nil, err
	}

	if err := stream.Start(); err != nil {
		stream.Close()
		portaudio.Terminate()
		return nil, err
	}

	return &MicrophoneReader{
		stream: stream,
		buffer: buffer,
	}, nil
}

// Read implements io.Reader. It captures one frame of audio data from the microphone
// and copies it to the provided buffer. The audio data is converted from int16 samples
// to little-endian byte format.
func (m *MicrophoneReader) Read(p []byte) (int, error) {
	if err := m.stream.Read(); err != nil {
		return 0, err
	}

	audioBytes := int16SliceToByteSlice(m.buffer)
	n := copy(p, audioBytes)
	return n, nil
}

// Close implements io.Closer. It stops the audio stream, closes it, and terminates PortAudio.
func (m *MicrophoneReader) Close() error {
	var err error
	if m.stream != nil {
		if stopErr := m.stream.Stop(); stopErr != nil {
			err = stopErr
		}
		if closeErr := m.stream.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}
	portaudio.Terminate()
	return err
}

// int16SliceToByteSlice converts a slice of int16 audio samples to a byte slice
// using little-endian encoding. Each int16 sample is converted to 2 bytes.
func int16SliceToByteSlice(in []int16) []byte {
	out := make([]byte, len(in)*2)
	for i, v := range in {
		// little-endian
		out[2*i] = byte(v)
		out[2*i+1] = byte(v >> 8)
	}
	return out
}
