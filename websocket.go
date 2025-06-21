package stt_challenge

import (
	"bytes"
	"context"
	"encoding/json"
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
// It contains the final transcribed text from the Google Speech API.
type WebSocketResponse struct {
	Sentence string `json:"sentence"`
}

// WebConn represents a WebSocket connection that bridges client audio data
// with speech transcription providers. It manages bidirectional
// communication between the WebSocket client and the transcription service.
type WebConn struct {
	conn     *websocket.Conn
	log      *log.Logger
	wg       sync.WaitGroup
	session  providers.Session
	cancelFn context.CancelFunc
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.log.Println("Creating transcription session...")
	config := providers.SessionConfig{
		SampleRate:     16000,
		LanguageCode:   "en-US",
		InterimResults: false,
	}

	session, err := s.provider.NewSession(ctx, config)
	if err != nil {
		s.log.Printf("Failed to create transcription session: %v\n", err)
		conn.Close()
		return
	}

	webConn := &WebConn{
		conn:     conn,
		log:      s.log,
		session:  session,
		cancelFn: cancel,
	}

	// TODO: keep track of webconns and close them properly on server shutdown.
	webConn.Start()
}

func (wc *WebConn) Start() {
	defer wc.conn.Close()
	defer wc.session.Close()

	wc.wg.Add(1)
	go func() {
		defer wc.wg.Done()
		wc.writer()
	}()

	wc.reader()
	wc.log.Println("Closing transcription session...")
	// Cancel reader stream, to allow for the writer to exit.
	wc.cancelFn()
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
			wc.log.Printf("session.SendAudio error: %v\n", err)
		}
	}
}

func (wc *WebConn) writer() {
	for {
		result, err := wc.session.ReceiveTranscription()
		if err == io.EOF {
			return
		}
		if err != nil {
			// TODO: Handle Audio timeout properly and support resumable streams.
			wc.log.Printf("session.ReceiveTranscription error: %v\n", err)
			return
		}

		if result.IsFinal {
			response := WebSocketResponse{
				Sentence: result.Text,
			}

			if err := wc.conn.WriteJSON(response); err != nil {
				wc.log.Printf("WebSocket write error: %v\n", err)
				return
			}
		}
	}
}
