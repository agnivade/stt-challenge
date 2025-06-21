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
	enableGoogle := flag.Bool("google", true, "Enable Google Speech provider")
	enableDeepgram := flag.Bool("deepgram", true, "Enable Deepgram provider")
	flag.Parse()

	// Create providers based on flags
	var providerList []providers.Provider
	var cleanupFuncs []func() error

	if *enableGoogle {
		provider, cleanup, err := createGoogleProvider()
		if err != nil {
			log.Printf("Failed to create Google provider: %v", err)
		} else {
			providerList = append(providerList, provider)
			if cleanup != nil {
				cleanupFuncs = append(cleanupFuncs, cleanup)
			}
		}
	}

	if *enableDeepgram {
		provider, cleanup, err := createDeepgramProvider()
		if err != nil {
			log.Printf("Failed to create Deepgram provider: %v", err)
		} else {
			providerList = append(providerList, provider)
			if cleanup != nil {
				cleanupFuncs = append(cleanupFuncs, cleanup)
			}
		}
	}

	if len(providerList) == 0 {
		log.Fatalf("No providers available. Enable at least one provider.")
	}

	// Cleanup all providers on exit
	defer func() {
		for _, cleanup := range cleanupFuncs {
			if err := cleanup(); err != nil {
				log.Printf("Error during provider cleanup: %v", err)
			}
		}
	}()

	log.Printf("Starting server with %d provider(s)", len(providerList))

	// Create server with all providers
	s := stt.New(providerList...)

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
