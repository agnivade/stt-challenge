package main

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	stt "github.com/agnivade/stt_challenge"
	"github.com/gorilla/websocket"
)

const (
	testFramesPerBuffer = 1024
)

// mockWebSocketServer creates a test WebSocket server that can send and receive messages
func mockWebSocketServer(t *testing.T, handler func(*websocket.Conn)) *httptest.Server {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("WebSocket upgrade failed: %v", err)
		}
		defer conn.Close()
		handler(conn)
	}))

	return server
}

// createTestClient creates a Client instance for testing
func createTestClient(t *testing.T, conn *websocket.Conn, audioReader io.Reader, outputFile *os.File) *Client {
	logger := log.New(io.Discard, "", 0) // Suppress test output

	client := &Client{
		conn:                conn,
		audioReader:         audioReader,
		log:                 logger,
		msgBuffer:           NewMessageBuffer(10),
		similarityThreshold: 0.8,
	}

	if outputFile != nil {
		client.bufWriter = bufio.NewWriter(outputFile)
	}

	return client
}

// connectToTestServer connects to a test WebSocket server
func connectToTestServer(t *testing.T, server *httptest.Server) *websocket.Conn {
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to test server: %v", err)
	}
	return conn
}

func TestClient(t *testing.T) {
	t.Run("Start_and_Close", func(t *testing.T) {
		// Create a mock server that just closes the connection after a brief delay
		server := mockWebSocketServer(t, func(conn *websocket.Conn) {
			time.Sleep(100 * time.Millisecond)
		})
		defer server.Close()

		conn := connectToTestServer(t, server)
		defer conn.Close()

		// Create a simple audio reader that returns EOF immediately
		audioReader := strings.NewReader("")

		client := createTestClient(t, conn, audioReader, nil)

		// Start the client
		client.Start()

		// Wait a bit to ensure goroutines are running
		time.Sleep(50 * time.Millisecond)

		// Close the client
		client.Close()

		// Test passes if no deadlock occurs
	})

	t.Run("writer_SendsAudioData", func(t *testing.T) {
		var receivedRequests []stt.WebSocketRequest
		var mu sync.Mutex
		done := make(chan bool)

		// Create a mock server that collects received audio data
		server := mockWebSocketServer(t, func(conn *websocket.Conn) {
			for {
				var req stt.WebSocketRequest
				err := conn.ReadJSON(&req)
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						t.Logf("WebSocket read error: %v", err)
					}
					break
				}

				mu.Lock()
				receivedRequests = append(receivedRequests, req)
				if len(receivedRequests) >= 2 { // Expect at least 2 chunks
					close(done)
					mu.Unlock()
					return
				}
				mu.Unlock()
			}
		})
		defer server.Close()

		conn := connectToTestServer(t, server)
		defer conn.Close()

		// Open test.raw file
		testFile, err := os.Open("../../testdata/test.raw")
		if err != nil {
			t.Fatalf("Failed to open test.raw: %v", err)
		}
		defer testFile.Close()

		client := createTestClient(t, conn, testFile, nil)

		// Start only the writer goroutine
		client.wg.Add(1)
		go client.writer()

		// Wait for some data to be sent or timeout
		select {
		case <-done:
			// Success
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for audio data")
		}

		client.Close()

		// Verify we received audio data
		mu.Lock()
		defer mu.Unlock()

		if len(receivedRequests) == 0 {
			t.Fatal("No audio data received")
		}

		// Verify the first request contains audio data
		if len(receivedRequests[0].Buf) == 0 {
			t.Fatal("First request contains no audio data")
		}

		if len(receivedRequests) != 2 {
			t.Errorf("Expected to contain %d requests, got: %d", 2, len(receivedRequests))
		}
	})

	t.Run("reader_ProcessesResponses", func(t *testing.T) {
		responses := []stt.WebSocketResponse{
			{Sentence: "Hello world"},
			{Sentence: "This is a test"},
			{Sentence: "Speech recognition works"},
		}

		done := make(chan bool)

		// Create a mock server that sends predefined responses
		server := mockWebSocketServer(t, func(conn *websocket.Conn) {
			for _, resp := range responses {
				if err := conn.WriteJSON(resp); err != nil {
					t.Logf("Failed to send response: %v", err)
					return
				}
				time.Sleep(100 * time.Millisecond) // Small delay between responses
			}

			// Signal completion and keep connection open briefly
			time.Sleep(200 * time.Millisecond)
			close(done)
		})
		defer server.Close()

		conn := connectToTestServer(t, server)
		defer conn.Close()

		// Use empty reader since we're only testing the reader goroutine
		audioReader := strings.NewReader("")

		client := createTestClient(t, conn, audioReader, nil)

		// Capture stdout to verify output
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Start only the reader goroutine
		client.wg.Add(1)
		go client.reader()

		// Wait for responses to be processed or timeout
		select {
		case <-done:
			// Success
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for responses")
		}

		// Restore stdout and read captured output
		w.Close()
		os.Stdout = oldStdout

		var buf bytes.Buffer
		io.Copy(&buf, r)
		output := buf.String()

		client.Close()

		// Verify that responses were processed and printed
		for _, resp := range responses {
			if !strings.Contains(output, resp.Sentence) {
				t.Errorf("Expected output to contain '%s', got: %s", resp.Sentence, output)
			}
		}

		// Verify timestamp format is present
		if !strings.Contains(output, "[") || !strings.Contains(output, "]") {
			t.Error("Expected timestamp format [HH:MM:SS] in output")
		}
	})

	t.Run("reader_WritesToFile", func(t *testing.T) {
		responses := []stt.WebSocketResponse{
			{Sentence: "First transcription"},
			{Sentence: "Second transcription"},
		}

		done := make(chan bool)

		// Create a mock server that sends responses
		server := mockWebSocketServer(t, func(conn *websocket.Conn) {
			for _, resp := range responses {
				if err := conn.WriteJSON(resp); err != nil {
					t.Logf("Failed to send response: %v", err)
					return
				}
				time.Sleep(100 * time.Millisecond)
			}
			time.Sleep(200 * time.Millisecond)
			// After this, the conn is closed automatically.
		})
		defer server.Close()

		conn := connectToTestServer(t, server)
		defer conn.Close()

		// Create temporary output file
		tmpFile, err := os.CreateTemp("", "test_output_*.txt")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpFile.Name())
		defer tmpFile.Close()

		audioReader := strings.NewReader("")
		client := createTestClient(t, conn, audioReader, tmpFile)

		// Start only the reader goroutine
		client.wg.Add(1)
		go func() {
			defer close(done)
			client.reader()
		}()

		// Wait for processing or timeout
		select {
		case <-done:
			// Success
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for file writing")
		}

		// Flush the buffer and close client
		if client.bufWriter != nil {
			client.bufWriter.Flush()
		}
		client.Close()

		// Read the output file and verify content
		tmpFile.Seek(0, 0)
		content, err := io.ReadAll(tmpFile)
		if err != nil {
			t.Fatalf("Failed to read output file: %v", err)
		}

		fileContent := string(content)

		// Verify all responses were written to file
		for _, resp := range responses {
			if !strings.Contains(fileContent, resp.Sentence) {
				t.Errorf("Expected file to contain '%s', got: %s", resp.Sentence, fileContent)
			}
		}

		// Verify timestamp format
		if !strings.Contains(fileContent, "[") || !strings.Contains(fileContent, "]") {
			t.Error("Expected timestamp format in file output")
		}
	})

	t.Run("EndToEnd_Integration", func(t *testing.T) {
		responses := []stt.WebSocketResponse{
			{Sentence: "Integration test working"},
			{Sentence: "End to end success"},
		}

		audioReceived := make(chan bool, 1)
		responseSent := make(chan bool, 1)

		// Create a mock server that receives audio and sends responses
		server := mockWebSocketServer(t, func(conn *websocket.Conn) {
			// Read one audio message
			var req stt.WebSocketRequest
			if err := conn.ReadJSON(&req); err == nil && len(req.Buf) > 0 {
				audioReceived <- true
			}

			// Send responses
			for _, resp := range responses {
				if err := conn.WriteJSON(resp); err != nil {
					t.Errorf("Failed to send response: %v", err)
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
			responseSent <- true

			time.Sleep(200 * time.Millisecond)
		})
		defer server.Close()

		conn := connectToTestServer(t, server)
		defer conn.Close()

		// Open test.raw file
		testFile, err := os.Open("../../testdata/test.raw")
		if err != nil {
			t.Fatalf("Failed to open test.raw: %v", err)
		}
		defer testFile.Close()

		// Create temporary output file
		tmpFile, err := os.CreateTemp("", "integration_test_*.txt")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpFile.Name())
		defer tmpFile.Close()

		client := createTestClient(t, conn, testFile, tmpFile)

		// Start the client (both reader and writer)
		client.Start()

		// Wait for audio to be received
		select {
		case <-audioReceived:
			t.Log("Audio data received by server")
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for audio data")
		}

		// Wait for responses to be sent
		select {
		case <-responseSent:
			t.Log("Responses sent by server")
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for responses")
		}

		// Give some time for processing
		time.Sleep(300 * time.Millisecond)

		// Close client and flush output
		client.Close()
		if client.bufWriter != nil {
			client.bufWriter.Flush()
		}

		// Verify output file contains responses
		tmpFile.Seek(0, 0)
		content, err := io.ReadAll(tmpFile)
		if err != nil {
			t.Fatalf("Failed to read output file: %v", err)
		}

		fileContent := string(content)
		for _, resp := range responses {
			if !strings.Contains(fileContent, resp.Sentence) {
				t.Errorf("Expected file to contain '%s', got: %s", resp.Sentence, fileContent)
			}
		}
	})

	t.Run("ErrorHandling", func(t *testing.T) {
		t.Run("InvalidJSONResponse", func(t *testing.T) {
			done := make(chan bool)

			// Server sends invalid JSON
			server := mockWebSocketServer(t, func(conn *websocket.Conn) {
				conn.WriteMessage(websocket.TextMessage, []byte("invalid json"))
				time.Sleep(100 * time.Millisecond)
				close(done)
			})
			defer server.Close()

			conn := connectToTestServer(t, server)
			defer conn.Close()

			audioReader := strings.NewReader("")
			client := createTestClient(t, conn, audioReader, nil)

			client.wg.Add(1)
			go client.reader()

			select {
			case <-done:
				// Should handle invalid JSON gracefully
			case <-time.After(1 * time.Second):
				t.Fatal("Timeout")
			}

			client.Close()
		})

		t.Run("AudioReadError", func(t *testing.T) {
			// Server that just waits
			server := mockWebSocketServer(t, func(conn *websocket.Conn) {
				time.Sleep(500 * time.Millisecond)
			})
			defer server.Close()

			conn := connectToTestServer(t, server)
			defer conn.Close()

			// Reader that returns an error
			errorReader := &errorReader{err: io.ErrUnexpectedEOF}
			client := createTestClient(t, conn, errorReader, nil)

			client.wg.Add(1)
			go client.writer()

			// Should handle audio read error gracefully
			time.Sleep(200 * time.Millisecond)
			client.Close()
		})
	})
}

// errorReader is a helper for testing error conditions
type errorReader struct {
	err error
}

func (er *errorReader) Read(p []byte) (int, error) {
	return 0, er.err
}
