package stt_challenge

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"sync"

	"github.com/agnivade/stt_challenge/providers"
	"github.com/gorilla/websocket"
)

// WebSocketRequest represents an audio data message sent from the client to the server.
// It contains raw audio bytes that will be forwarded to the Google Speech API.
type WebSocketRequest struct {
	Buf []byte `json:"buf"`
}

// WebSocketResponse represents a transcription result sent from the server to the client.
// It contains the final transcribed text and confidence score from the transcription provider.
type WebSocketResponse struct {
	Sentence   string  `json:"sentence"`
	Confidence float32 `json:"confidence"`
}

// WebConn represents a WebSocket connection that bridges client audio data
// with speech transcription providers. It manages bidirectional
// communication between the WebSocket client and the transcription service.
type WebConn struct {
	conn    *websocket.Conn
	log     *log.Logger
	wg      sync.WaitGroup
	session providers.Session
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  8192,
		WriteBufferSize: 8192,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.log.Printf("WebSocket upgrade failed: %v\n", err)
		return
	}

	s.log.Println("Creating provider selector...")
	config := providers.SessionConfig{
		SampleRate:     16000,
		LanguageCode:   "en-US",
		InterimResults: true,
	}

	selector, err := NewProviderSelector(s.providers, config, s.log)
	if err != nil {
		s.log.Printf("Failed to create provider selector: %v\n", err)
		conn.Close()
		return
	}

	webConn := &WebConn{
		conn:    conn,
		log:     s.log,
		session: selector,
	}

	// Register connection for tracking
	s.addConn(webConn)
	defer s.removeConn(webConn)
	webConn.Start()
}

func (wc *WebConn) Start() {
	defer wc.conn.Close()

	wc.wg.Add(1)
	go func() {
		defer wc.wg.Done()
		wc.writer()
	}()

	wc.reader()
	wc.log.Println("Closing transcription session...")
	// Close session, which will cancel context and allow writer to exit
	// Important to call this _after_ wc.reader() exits.
	wc.session.Close()
	wc.wg.Wait()
}

// Stop gracefully closes the WebSocket connection and waits for all
// goroutines to finish. This method is safe to call multiple times.
func (wc *WebConn) Stop() {
	// Close the connection, which will cause reader() to exit
	wc.conn.Close()
	// Wait for all goroutines to finish
	wc.wg.Wait()
}

func (wc *WebConn) reader() {
	var buf bytes.Buffer

	for {
		// Reuse the buffer
		buf.Reset()

		_, r, err := wc.conn.NextReader()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				wc.log.Printf("WebSocket read error: %v\n", err)
			}
			break
		}

		if _, err := buf.ReadFrom(r); err != nil {
			wc.log.Printf("Failed to read from WebSocket reader: %v\n", err)
			continue
		}

		var req WebSocketRequest
		if err := json.Unmarshal(buf.Bytes(), &req); err != nil {
			wc.log.Printf("Failed to unmarshal WebSocket message: %v\n", err)
			continue
		}

		// Send audio bytes to transcription session
		if err := wc.session.SendAudio(req.Buf); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			wc.log.Printf("session.SendAudio error: %v\n", err)
		}
	}
}

func (wc *WebConn) writer() {
	for {
		result, err := wc.session.ReceiveTranscription()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			wc.log.Printf("session.ReceiveTranscription error: %v\n", err)
			return
		}

		response := WebSocketResponse{
			Sentence:   result.Text,
			Confidence: result.Confidence,
		}

		if err := wc.conn.WriteJSON(response); err != nil {
			wc.log.Printf("WebSocket write error: %v\n", err)
			return
		}
	}
}
