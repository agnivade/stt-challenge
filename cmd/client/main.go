package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	stt "github.com/agnivade/stt_challenge"
	"github.com/gordonklaus/portaudio"
	"github.com/gorilla/websocket"
)

const (
	sampleRate      = 16000
	framesPerBuffer = 1024
)

type Client struct {
	conn        *websocket.Conn
	audioStream *portaudio.Stream
	audioBuffer []int16
	wg          sync.WaitGroup
	log         *log.Logger
	outputFile  *os.File
	bufWriter   *bufio.Writer
}

func main() {
	var serverURL = flag.String("url", "ws://localhost:8081/ws", "WebSocket server URL")
	var outputPath = flag.String("output", "", "Output file path for transcriptions (optional)")
	flag.Parse()

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
	conn, _, err := websocket.DefaultDialer.Dial(*serverURL, nil)
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

	// Setup output file if specified
	if *outputPath != "" {
		outputFile, err := os.Create(*outputPath)
		if err != nil {
			logger.Printf("Failed to create output file: %v\n", err)
			return
		}
		defer outputFile.Close()

		client.outputFile = outputFile
		client.bufWriter = bufio.NewWriter(outputFile)
		defer client.bufWriter.Flush()
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
	var buf bytes.Buffer

	for {
		buf.Reset()

		_, r, err := c.conn.NextReader()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.log.Printf("WebSocket read error: %v\n", err)
			}
			return
		}

		if _, err := buf.ReadFrom(r); err != nil {
			c.log.Printf("Failed to read from WebSocket reader: %v\n", err)
			continue
		}

		var response stt.WebSocketResponse
		if err := json.Unmarshal(buf.Bytes(), &response); err != nil {
			c.log.Printf("Failed to unmarshal response: %v\n", err)
			continue
		}

		timestamp := time.Now().Format("15:04:05")
		line := fmt.Sprintf("[%s] %s\n", timestamp, response.Sentence)

		fmt.Print(line)

		// Write to file if output file is specified
		if c.bufWriter != nil {
			if _, err := c.bufWriter.WriteString(line); err != nil {
				c.log.Printf("Failed to write to output file: %v\n", err)
			} else {
				c.bufWriter.Flush()
			}
		}
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
