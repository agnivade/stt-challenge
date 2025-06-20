package stt_challenge

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	speechpb "google.golang.org/genproto/googleapis/cloud/speech/v1"
)

type WebSocketRequest struct {
	Buf []byte `json:"buf"`
}

type WebSocketResponse struct {
	Sentence string `json:"sentence"`
}

type WebConn struct {
	conn   *websocket.Conn
	log    *log.Logger
	wg     sync.WaitGroup
	stream speechpb.Speech_StreamingRecognizeClient
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

	ctx := context.Background()
	stream, err := s.speechClient.StreamingRecognize(ctx)
	if err != nil {
		s.log.Printf("StreamingRecognize failed: %v\n", err)
		conn.Close()
		return
	}

	// Send initial configuration
	req := &speechpb.StreamingRecognizeRequest{
		StreamingRequest: &speechpb.StreamingRecognizeRequest_StreamingConfig{
			StreamingConfig: &speechpb.StreamingRecognitionConfig{
				Config: &speechpb.RecognitionConfig{
					Encoding:        speechpb.RecognitionConfig_LINEAR16,
					SampleRateHertz: 16000,
					LanguageCode:    "en-US",
				},
				InterimResults: false,
			},
		},
	}
	if err := stream.Send(req); err != nil {
		s.log.Printf("Error sending config: %v\n", err)
		conn.Close()
		return
	}

	webConn := &WebConn{
		conn:   conn,
		log:    s.log,
		stream: stream,
	}

	webConn.Start()
}

func (wc *WebConn) Start() {
	defer wc.conn.Close()
	defer wc.stream.CloseSend()

	wc.wg.Add(1)
	go func() {
		defer wc.wg.Done()
		wc.writer()
	}()

	wc.reader()
	wc.wg.Wait()
}

func (wc *WebConn) reader() {
	for {
		_, message, err := wc.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				wc.log.Printf("WebSocket read error: %v\n", err)
			}
			break
		}

		var req WebSocketRequest
		if err := json.Unmarshal(message, &req); err != nil {
			wc.log.Printf("Failed to unmarshal WebSocket message: %v\n", err)
			continue
		}

		// Send audio bytes to speech stream
		speechReq := &speechpb.StreamingRecognizeRequest{
			StreamingRequest: &speechpb.StreamingRecognizeRequest_AudioContent{
				AudioContent: req.Buf,
			},
		}
		if err := wc.stream.Send(speechReq); err != nil {
			wc.log.Printf("stream.Send error: %v\n", err)
		}
	}
}

func (wc *WebConn) writer() {
	for {
		resp, err := wc.stream.Recv()
		if err == io.EOF {
			return
		}
		if err != nil {
			wc.log.Printf("stream.Recv error: %v\n", err)
			return
		}

		for _, result := range resp.Results {
			if result.IsFinal {
				sentence := result.Alternatives[0].Transcript
				response := WebSocketResponse{
					Sentence: sentence,
				}

				responseData, err := json.Marshal(response)
				if err != nil {
					wc.log.Printf("Failed to marshal response: %v\n", err)
					continue
				}

				if err := wc.conn.WriteMessage(websocket.TextMessage, responseData); err != nil {
					wc.log.Printf("WebSocket write error: %v\n", err)
					return
				}
			}
		}
	}
}
