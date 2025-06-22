package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	stt "github.com/agnivade/stt_challenge"
	"github.com/gorilla/websocket"
)

// Client represents a speech-to-text client that streams audio data to a WebSocket server
// and receives transcription results. It manages the WebSocket connection, audio input,
// and optional output file writing.
type Client struct {
	conn        *websocket.Conn
	audioReader io.Reader
	wg          sync.WaitGroup
	log         *log.Logger
	bufWriter   *bufio.Writer
}

func main() {
	var serverURL = flag.String("url", "ws://localhost:8081/ws", "WebSocket server URL")
	var outputPath = flag.String("output", "", "Output file path for transcriptions (optional)")
	var inputFile = flag.String("input", "", "Input audio file path (useful for testing)")
	flag.Parse()

	logger := log.New(os.Stderr, "", log.LstdFlags|log.Lshortfile)

	// Initialize audio reader (either file or microphone)
	var audioReader io.ReadCloser
	if *inputFile != "" {
		file, err := os.Open(*inputFile)
		if err != nil {
			logger.Printf("Failed to open input file: %v\n", err)
			return
		}
		audioReader = file
		logger.Printf("Using input file: %s\n", *inputFile)
	} else {
		micReader, err := NewMicrophoneReader()
		if err != nil {
			logger.Printf("Failed to initialize microphone: %v\n", err)
			return
		}
		audioReader = micReader
		logger.Println("Using microphone input")
	}
	defer audioReader.Close()

	// Connect to WebSocket server
	conn, _, err := websocket.DefaultDialer.Dial(*serverURL, nil)
	if err != nil {
		logger.Printf("WebSocket dial failed: %v\n", err)
		return
	}
	defer conn.Close()

	client := &Client{
		conn:        conn,
		audioReader: audioReader,
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
			c.log.Printf("WebSocket read error: %v\n", err)
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
		line := fmt.Sprintf("[%s] %s (confidence: %.2f)\n", timestamp, response.Sentence, response.Confidence)

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
	buf := make([]byte, framesPerBuffer*2) // int16 * 2 bytes each

	for {
		// This should never block as long as there is data
		// coming from the reader. So we don't need to make it cancellable.
		n, err := c.audioReader.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			c.log.Printf("Audio read error: %v\n", err)
			break
		}

		request := stt.WebSocketRequest{
			Buf: buf[:n],
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
