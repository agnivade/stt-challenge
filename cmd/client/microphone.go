package main

import (
	"github.com/gordonklaus/portaudio"
)

const (
	sampleRate      = 16000
	framesPerBuffer = 1024
)

type MicrophoneReader struct {
	stream *portaudio.Stream
	buffer []int16
}

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

func (m *MicrophoneReader) Read(p []byte) (int, error) {
	if err := m.stream.Read(); err != nil {
		return 0, err
	}

	audioBytes := int16SliceToByteSlice(m.buffer)
	n := copy(p, audioBytes)
	return n, nil
}

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

func int16SliceToByteSlice(in []int16) []byte {
	out := make([]byte, len(in)*2)
	for i, v := range in {
		// little-endian
		out[2*i] = byte(v)
		out[2*i+1] = byte(v >> 8)
	}
	return out
}