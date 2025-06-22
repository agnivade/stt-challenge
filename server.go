package stt_challenge

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/agnivade/stt_challenge/providers"
)

type Server struct {
	srv       *http.Server
	log       *log.Logger
	providers []providers.Provider

	// Connection tracking
	mu    sync.Mutex
	conns map[*WebConn]struct{}
}

func New(port string, providers ...providers.Provider) *Server {
	logger := log.New(os.Stderr, "", log.LstdFlags|log.Lshortfile)
	mux := http.NewServeMux()

	server := &Server{
		srv: &http.Server{
			Addr:         ":" + port,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
			Handler:      mux,
		},
		log:       logger,
		providers: providers,
		conns:     make(map[*WebConn]struct{}),
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

// addConn registers a WebSocket connection for tracking
func (s *Server) addConn(wc *WebConn) {
	s.mu.Lock()
	s.conns[wc] = struct{}{}
	s.mu.Unlock()
}

// removeConn unregisters a WebSocket connection
func (s *Server) removeConn(wc *WebConn) {
	s.mu.Lock()
	delete(s.conns, wc)
	s.mu.Unlock()
}

// stopAllConns gracefully stops all active WebSocket connections
func (s *Server) stopAllConns() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// TODO: optimize to stop each connection in parallel
	for wc := range s.conns {
		wc.Stop()
	}
}

func (s *Server) Stop() error {
	s.log.Println("Shutting down server...")

	// First, stop accepting new connections and close existing ones
	s.stopAllConns()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return s.srv.Shutdown(ctx)
}
