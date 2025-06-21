package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	speech "cloud.google.com/go/speech/apiv1"
	stt "github.com/agnivade/stt_challenge"
	"github.com/agnivade/stt_challenge/providers"
	"github.com/agnivade/stt_challenge/providers/deepgram"
	"github.com/agnivade/stt_challenge/providers/google"
)

func main() {
	// Parse command line flags
	providerName := flag.String("provider", "google", "Transcription provider to use (google or deepgram)")
	flag.Parse()

	// Create the appropriate provider
	provider, cleanup, err := createProvider(*providerName)
	if err != nil {
		log.Fatalf("Failed to create provider: %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

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

func createProvider(providerName string) (providers.Provider, func() error, error) {
	switch providerName {
	case "google":
		return createGoogleProvider()
	case "deepgram":
		return createDeepgramProvider()
	default:
		return nil, nil, fmt.Errorf("unknown provider: %s. Supported providers: google, deepgram", providerName)
	}
}

func createGoogleProvider() (providers.Provider, func() error, error) {
	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") == "" {
		return nil, nil, fmt.Errorf("GOOGLE_APPLICATION_CREDENTIALS environment variable is required for Google provider")
	}

	ctx := context.Background()
	speechClient, err := speech.NewClient(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Google Speech client: %v", err)
	}

	provider := google.NewProvider(speechClient)

	return provider, speechClient.Close, nil
}

func createDeepgramProvider() (providers.Provider, func() error, error) {
	apiKey := os.Getenv("DEEPGRAM_API_KEY")
	if apiKey == "" {
		return nil, nil, fmt.Errorf("DEEPGRAM_API_KEY environment variable is required for Deepgram provider")
	}

	provider := deepgram.NewProvider(apiKey)
	return provider, nil, nil // No cleanup needed for Deepgram
}
