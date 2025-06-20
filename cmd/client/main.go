package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	stt "github.com/agnivade/stt_challenge"
	"github.com/gordonklaus/portaudio"
	"github.com/gorilla/websocket"
)

const (
	sampleRate      = 16000
	framesPerBuffer = 1024
	serverURL       = "ws://localhost:8081/ws"
)

type Client struct {
	conn        *websocket.Conn
	audioStream *portaudio.Stream
	audioBuffer []int16
	wg          sync.WaitGroup
	log         *log.Logger
}

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags|log.Lshortfile)

	// Initialize PortAudio
	if err := portaudio.Initialize(); err != nil {
		logger.Printf("portaudio.Initialize: %v\n", err)
		return
	}
	defer portaudio.Terminate()

	// Create audio buffer
	audioBuffer := make([]int16, framesPerBuffer)

	// Open default audio stream
	audioStream, err := portaudio.OpenDefaultStream(1, 0, float64(sampleRate), len(audioBuffer), audioBuffer)
	if err != nil {
		logger.Printf("OpenDefaultStream: %v\n", err)
		return
	}
	defer audioStream.Close()

	if err := audioStream.Start(); err != nil {
		logger.Printf("audioStream.Start: %v\n", err)
		return
	}
	defer audioStream.Stop()

	// Connect to WebSocket server
	conn, _, err := websocket.DefaultDialer.Dial(serverURL, nil)
	if err != nil {
		logger.Printf("WebSocket dial failed: %v\n", err)
		return
	}
	defer conn.Close()

	client := &Client{
		conn:        conn,
		audioStream: audioStream,
		audioBuffer: audioBuffer,
		log:         logger,
	}

	fmt.Println("Recording... Press Ctrl+C to stop.")
	// Start client
	client.Start()

	// Wait for interrupt signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	client.Close()
	fmt.Println("\nDone.")
}

func (c *Client) Start() {
	c.wg.Add(2)
	go c.reader()
	go c.writer()
}

func (c *Client) reader() {
	defer c.wg.Done()
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.log.Printf("WebSocket read error: %v\n", err)
			}
			return
		}

		var response stt.WebSocketResponse
		if err := json.Unmarshal(message, &response); err != nil {
			c.log.Printf("Failed to unmarshal response: %v\n", err)
			continue
		}

		fmt.Printf("Transcription: %s\n", response.Sentence)
	}
}

func (c *Client) writer() {
	defer c.wg.Done()
	for {
		if err := c.audioStream.Read(); err != nil {
			c.log.Printf("Audio read error: %v\n", err)
			break
		}

		audioBytes := int16SliceToByteSlice(c.audioBuffer)

		request := stt.WebSocketRequest{
			Buf: audioBytes,
		}

		if err := c.conn.WriteJSON(request); err != nil {
			if !errors.Is(err, net.ErrClosed) {
				c.log.Printf("WebSocket write error: %v\n", err)
			}
			return
		}
	}
}

func (c *Client) Close() {
	c.log.Println("Closing client...")
	if c.conn != nil {
		c.conn.Close()
	}
	c.wg.Wait()
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
