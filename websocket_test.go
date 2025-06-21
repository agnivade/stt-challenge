package stt_challenge

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/agnivade/stt_challenge/providers"
	"github.com/agnivade/stt_challenge/providers/mocks"
)

// ThreadSafeBuffer is a thread-safe wrapper around bytes.Buffer
type ThreadSafeBuffer struct {
	mu  sync.RWMutex
	buf bytes.Buffer
}

func (tsb *ThreadSafeBuffer) Write(p []byte) (n int, err error) {
	tsb.mu.Lock()
	defer tsb.mu.Unlock()
	return tsb.buf.Write(p)
}

func (tsb *ThreadSafeBuffer) String() string {
	tsb.mu.RLock()
	defer tsb.mu.RUnlock()
	return tsb.buf.String()
}

func TestWebSocketHandleConnection(t *testing.T) {
	// Create mock provider and session
	mockProvider := mocks.NewMockProvider(t)
	mockSession := mocks.NewMockSession(t)

	// Setup expectations for provider selector
	mockProvider.EXPECT().Name().Return("mock-provider")
	mockProvider.EXPECT().NewSession(
		mock.AnythingOfType("*context.cancelCtx"),
		mock.AnythingOfType("providers.SessionConfig"),
	).Return(mockSession, nil)

	mockSession.EXPECT().ReceiveTranscription().Return(providers.TranscriptionResult{}, io.EOF)
	mockSession.EXPECT().Close().Return(nil)

	// Create server with mock provider
	server := New(mockProvider)
	server.log = log.New(io.Discard, "", 0)

	// Create test HTTP server
	testServer := httptest.NewServer(http.HandlerFunc(server.handleWebSocket))
	defer testServer.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	// Connect to WebSocket
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	assert.NoError(t, err)
	defer conn.Close()

	// Close connection immediately to trigger cleanup
	conn.Close()

	// Give time for server-side cleanup
	// TODO: All these sleeps are due to server not cleaning up connections
	// while exiting. Need to fix that.
	time.Sleep(100 * time.Millisecond)
}

func TestWebSocketAudioFlow(t *testing.T) {
	// Create mock provider and session
	mockProvider := mocks.NewMockProvider(t)
	mockSession := mocks.NewMockSession(t)

	// Test audio data
	audioData := []byte("test audio data")

	// Setup expectations
	mockProvider.EXPECT().Name().Return("mock-provider")
	mockProvider.EXPECT().NewSession(
		mock.AnythingOfType("*context.cancelCtx"),
		mock.AnythingOfType("providers.SessionConfig"),
	).Return(mockSession, nil)

	mockSession.EXPECT().SendAudio(audioData).Return(nil)
	mockSession.EXPECT().ReceiveTranscription().Return(providers.TranscriptionResult{}, io.EOF)
	mockSession.EXPECT().Close().Return(nil)

	// Create server with mock provider
	server := New(mockProvider)
	server.log = log.New(io.Discard, "", 0)

	// Create test HTTP server
	testServer := httptest.NewServer(http.HandlerFunc(server.handleWebSocket))
	defer testServer.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	// Connect to WebSocket
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	assert.NoError(t, err)
	defer conn.Close()

	// Send audio data
	audioMsg := WebSocketRequest{Buf: audioData}
	err = conn.WriteJSON(audioMsg)
	assert.NoError(t, err)

	// Close connection to trigger cleanup
	conn.Close()

	// Give time for server-side processing
	time.Sleep(100 * time.Millisecond)
}

func TestWebSocketTranscriptionFlow(t *testing.T) {
	// Create mock provider and session
	mockProvider := mocks.NewMockProvider(t)
	mockSession := mocks.NewMockSession(t)

	// Test data
	audioData := []byte("hello world audio")
	expectedTranscription := "Hello world"

	// Setup expectations
	mockProvider.EXPECT().Name().Return("mock-provider")
	mockProvider.EXPECT().NewSession(
		mock.AnythingOfType("*context.cancelCtx"),
		mock.AnythingOfType("providers.SessionConfig"),
	).Return(mockSession, nil)

	mockSession.EXPECT().SendAudio(audioData).Return(nil)
	mockSession.EXPECT().ReceiveTranscription().Return(
		providers.TranscriptionResult{
			Text:         expectedTranscription,
			IsFinal:      true,
			Confidence:   0.95,
			ProviderName: "mock-provider",
			ReceivedAt:   time.Now(),
		}, nil).Once()
	mockSession.EXPECT().ReceiveTranscription().Return(providers.TranscriptionResult{}, io.EOF).Once()
	mockSession.EXPECT().Close().Return(nil)

	// Create server with mock provider
	server := New(mockProvider)
	server.log = log.New(io.Discard, "", 0)

	// Create test HTTP server
	testServer := httptest.NewServer(http.HandlerFunc(server.handleWebSocket))
	defer testServer.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	// Connect to WebSocket
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	assert.NoError(t, err)
	defer conn.Close()

	// Send audio data
	audioMsg := WebSocketRequest{Buf: audioData}
	err = conn.WriteJSON(audioMsg)
	assert.NoError(t, err)

	// Read transcription response
	var response WebSocketResponse
	err = conn.ReadJSON(&response)
	assert.NoError(t, err)
	assert.Equal(t, expectedTranscription, response.Sentence)

	// Close connection
	conn.Close()

	// Give time for server-side cleanup
	time.Sleep(100 * time.Millisecond)
}

func TestWebSocketMultipleMessages(t *testing.T) {
	// Create mock provider and session
	mockProvider := mocks.NewMockProvider(t)
	mockSession := mocks.NewMockSession(t)

	// Test data
	audioData1 := []byte("first audio")
	audioData2 := []byte("second audio")
	transcription1 := "First transcription"
	transcription2 := "Second transcription"

	// Setup expectations
	mockProvider.EXPECT().Name().Return("mock-provider")
	mockProvider.EXPECT().NewSession(
		mock.AnythingOfType("*context.cancelCtx"),
		mock.AnythingOfType("providers.SessionConfig"),
	).Return(mockSession, nil)

	mockSession.EXPECT().SendAudio(audioData1).Return(nil)
	mockSession.EXPECT().SendAudio(audioData2).Return(nil)
	mockSession.EXPECT().ReceiveTranscription().Return(
		providers.TranscriptionResult{
			Text:         transcription1,
			IsFinal:      true,
			ProviderName: "mock-provider",
			ReceivedAt:   time.Now(),
		}, nil).Once()
	mockSession.EXPECT().ReceiveTranscription().Return(
		providers.TranscriptionResult{
			Text:         transcription2,
			IsFinal:      true,
			ProviderName: "mock-provider",
			ReceivedAt:   time.Now(),
		}, nil).Once()
	mockSession.EXPECT().ReceiveTranscription().Return(providers.TranscriptionResult{}, io.EOF).Once()
	mockSession.EXPECT().Close().Return(nil)

	// Create server with mock provider
	server := New(mockProvider)
	server.log = log.New(io.Discard, "", 0)

	// Create test HTTP server
	testServer := httptest.NewServer(http.HandlerFunc(server.handleWebSocket))
	defer testServer.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	// Connect to WebSocket
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	assert.NoError(t, err)
	defer conn.Close()

	// Send first audio message
	err = conn.WriteJSON(WebSocketRequest{Buf: audioData1})
	assert.NoError(t, err)

	// Send second audio message
	err = conn.WriteJSON(WebSocketRequest{Buf: audioData2})
	assert.NoError(t, err)

	// Read first transcription
	var response1 WebSocketResponse
	err = conn.ReadJSON(&response1)
	assert.NoError(t, err)
	assert.Equal(t, transcription1, response1.Sentence)

	// Read second transcription
	var response2 WebSocketResponse
	err = conn.ReadJSON(&response2)
	assert.NoError(t, err)
	assert.Equal(t, transcription2, response2.Sentence)

	// Close connection
	conn.Close()

	// Give time for server-side cleanup
	time.Sleep(100 * time.Millisecond)
}

func TestWebSocketInvalidJSON(t *testing.T) {
	// Create mock provider and session
	mockProvider := mocks.NewMockProvider(t)
	mockSession := mocks.NewMockSession(t)

	// Setup expectations
	mockProvider.EXPECT().Name().Return("mock-provider")
	mockProvider.EXPECT().NewSession(
		mock.AnythingOfType("*context.cancelCtx"),
		mock.AnythingOfType("providers.SessionConfig"),
	).Return(mockSession, nil)

	mockSession.EXPECT().ReceiveTranscription().Return(providers.TranscriptionResult{}, io.EOF)
	mockSession.EXPECT().Close().Return(nil)

	// Create server with mock provider
	server := New(mockProvider)
	server.log = log.New(io.Discard, "", 0)

	// Create test HTTP server
	testServer := httptest.NewServer(http.HandlerFunc(server.handleWebSocket))
	defer testServer.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	// Connect to WebSocket
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	assert.NoError(t, err)
	defer conn.Close()

	// Send invalid JSON
	err = conn.WriteMessage(websocket.TextMessage, []byte("invalid json"))
	assert.NoError(t, err)

	// Close connection
	conn.Close()

	// Give time for server-side cleanup
	time.Sleep(100 * time.Millisecond)
}

func TestWebSocketReceiveTranscriptionError(t *testing.T) {
	// Create mock provider and session
	mockProvider := mocks.NewMockProvider(t)
	mockSession := mocks.NewMockSession(t)

	// Test data
	audioData := []byte("test audio")

	// Setup expectations
	mockProvider.EXPECT().Name().Return("mock-provider")
	mockProvider.EXPECT().NewSession(
		mock.AnythingOfType("*context.cancelCtx"),
		mock.AnythingOfType("providers.SessionConfig"),
	).Return(mockSession, nil)

	mockSession.EXPECT().SendAudio(audioData).Return(nil)
	mockSession.EXPECT().ReceiveTranscription().Return(
		providers.TranscriptionResult{},
		errors.New("transcription service error")).Once()
	mockSession.EXPECT().Close().Return(nil)

	// Create server with mock provider and thread-safe log buffer
	logBuffer := &ThreadSafeBuffer{}
	server := New(mockProvider)
	server.log = log.New(logBuffer, "", 0)

	// Create test HTTP server
	testServer := httptest.NewServer(http.HandlerFunc(server.handleWebSocket))
	defer testServer.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	// Connect to WebSocket
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	assert.NoError(t, err)

	// Send audio data
	audioMsg := WebSocketRequest{Buf: audioData}
	err = conn.WriteJSON(audioMsg)
	assert.NoError(t, err)

	require.NoError(t, conn.Close())
	// // The connection should close due to the transcription error
	// // Give time for server-side processing and error handling
	time.Sleep(200 * time.Millisecond)

	// Verify error was logged
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "Provider mock-provider transcription error")
}

func TestWebSocketSendAudioError(t *testing.T) {
	// Create mock provider and session
	mockProvider := mocks.NewMockProvider(t)
	mockSession := mocks.NewMockSession(t)

	// Test data
	audioData := []byte("test audio")

	// Setup expectations
	mockProvider.EXPECT().Name().Return("mock-provider")
	mockProvider.EXPECT().NewSession(
		mock.AnythingOfType("*context.cancelCtx"),
		mock.AnythingOfType("providers.SessionConfig"),
	).Return(mockSession, nil)

	mockSession.EXPECT().SendAudio(audioData).Return(errors.New("audio send error"))
	mockSession.EXPECT().ReceiveTranscription().Return(providers.TranscriptionResult{}, io.EOF).Once()
	mockSession.EXPECT().Close().Return(nil)

	// Create server with mock provider and thread-safe log buffer
	logBuffer := &ThreadSafeBuffer{}
	server := New(mockProvider)
	server.log = log.New(logBuffer, "", 0)

	// Create test HTTP server
	testServer := httptest.NewServer(http.HandlerFunc(server.handleWebSocket))
	defer testServer.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	// Connect to WebSocket
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	assert.NoError(t, err)

	// Send audio data (this should trigger the SendAudio error)
	audioMsg := WebSocketRequest{Buf: audioData}
	err = conn.WriteJSON(audioMsg)
	assert.NoError(t, err)

	require.NoError(t, conn.Close())
	// The error should be logged but not cause connection termination
	// Give time for server-side processing
	time.Sleep(100 * time.Millisecond)

	// Verify error was logged
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "Provider mock-provider audio send failed")
}

func TestWebSocketProviderSessionCreationError(t *testing.T) {
	// Create mock provider
	mockProvider := mocks.NewMockProvider(t)

	// Setup expectations - provider fails to create session
	mockProvider.EXPECT().Name().Return("mock-provider")
	mockProvider.EXPECT().NewSession(
		mock.AnythingOfType("*context.cancelCtx"),
		mock.AnythingOfType("providers.SessionConfig"),
	).Return(nil, errors.New("failed to create session"))

	// Create server with mock provider and thread-safe log buffer
	logBuffer := &ThreadSafeBuffer{}
	server := New(mockProvider)
	server.log = log.New(logBuffer, "", 0)

	// Create test HTTP server
	testServer := httptest.NewServer(http.HandlerFunc(server.handleWebSocket))
	defer testServer.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	// Attempt to connect to WebSocket
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	// The connection should be established initially
	// But it should be closed immediately due to session creation failure
	// Give time for server-side cleanup
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, conn.Close())

	// Verify error was logged
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "Failed to create provider selector")
}
