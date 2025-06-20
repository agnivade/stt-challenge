package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	stt "github.com/agnivade/stt_challenge"
)

func main() {
	s := stt.New()

	go func() {
		if err := s.Start(); err != nil {
			log.Fatalf("Server failed to start: %v\n", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	if err := s.Stop(); err != nil {
		log.Printf("Error during server shutdown: %v\n", err)
	}
}
