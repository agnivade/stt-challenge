package stt_challenge

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	speech "cloud.google.com/go/speech/apiv1"
)

type Server struct {
	srv          *http.Server
	log          *log.Logger
	speechClient *speech.Client
}

func New() *Server {
	logger := log.New(os.Stderr, "", log.LstdFlags|log.Lshortfile)

	ctx := context.Background()
	speechClient, err := speech.NewClient(ctx)
	if err != nil {
		logger.Fatalf("Failed to create speech client: %v", err)
	}

	mux := http.NewServeMux()

	server := &Server{
		srv: &http.Server{
			Addr:         ":8081",
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
			Handler:      mux,
		},
		log:          logger,
		speechClient: speechClient,
	}

	mux.HandleFunc("/ws", server.handleWebSocket)

	return server
}

func (s *Server) Start() error {
	var wg sync.WaitGroup
	errChan := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.log.Printf("Starting server on %s", s.srv.Addr)
		errChan <- s.srv.ListenAndServe()
	}()

	wg.Wait()
	close(errChan)
	for err := range errChan {
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}

	return nil
}

func (s *Server) Stop() error {
	s.log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Close the speech client
	if s.speechClient != nil {
		s.speechClient.Close()
	}

	return s.srv.Shutdown(ctx)
}
