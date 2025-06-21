package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	speech "cloud.google.com/go/speech/apiv1"
	stt "github.com/agnivade/stt_challenge"
	"github.com/agnivade/stt_challenge/providers/google"
)

func main() {
	// Create Google Speech client
	ctx := context.Background()
	speechClient, err := speech.NewClient(ctx)
	if err != nil {
		log.Fatalf("Failed to create speech client: %v", err)
	}
	defer speechClient.Close()

	// Create Google Speech provider
	provider := google.NewProvider(speechClient)

	// Create server with the provider
	s := stt.New(provider)

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
