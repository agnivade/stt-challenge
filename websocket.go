package stt_challenge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	speechpb "google.golang.org/genproto/googleapis/cloud/speech/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type WebSocketRequest struct {
	Buf []byte `json:"buf"`
}

type WebSocketResponse struct {
	Sentence string `json:"sentence"`
}

type WebConn struct {
	conn     *websocket.Conn
	log      *log.Logger
	wg       sync.WaitGroup
	stream   speechpb.Speech_StreamingRecognizeClient
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
	s.log.Println("Opening stream...")
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
		conn:     conn,
		log:      s.log,
		stream:   stream,
		cancelFn: cancel,
	}

	// TODO: keep track of webconns and close them properly on server shutdown.
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
	wc.log.Println("Closing stream...")
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
		if errors.Is(err, io.EOF) || status.Code(err) == codes.Canceled {
			return
		}
		if err != nil {
			// TODO: Handle Audio timeout properly and support resumable streams.
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
