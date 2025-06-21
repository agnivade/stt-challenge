package stt_challenge

import (
	"io"
	"log"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/agnivade/stt_challenge/providers/mocks"
)

func TestServer_StartAndStop(t *testing.T) {
	// Create mock provider
	mockProvider := mocks.NewMockProvider(t)
	mockProvider.EXPECT().Name().Return("mock-provider").Maybe()

	// Create server with mock provider and silent logger
	server := New(mockProvider)
	server.log = log.New(io.Discard, "", 0)

	// Channel to capture any errors from Start()
	startErrChan := make(chan error, 1)

	// Start server in goroutine
	go func() {
		startErrChan <- server.Start()
	}()

	// Give server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Stop the server
	err := server.Stop()
	assert.NoError(t, err, "Server should stop without error")

	// Verify Start() method completed without error
	select {
	case startErr := <-startErrChan:
		assert.NoError(t, startErr, "Start() should complete without error after Stop()")
	case <-time.After(2 * time.Second):
		t.Fatal("Start() method should have completed after Stop() was called")
	}
}

func TestServer_MultipleProviders(t *testing.T) {
	// Create multiple mock providers
	mockProvider1 := mocks.NewMockProvider(t)
	mockProvider1.EXPECT().Name().Return("mock-provider-1").Maybe()
	
	mockProvider2 := mocks.NewMockProvider(t)
	mockProvider2.EXPECT().Name().Return("mock-provider-2").Maybe()

	// Create server with multiple providers
	server := New(mockProvider1, mockProvider2)
	server.log = log.New(io.Discard, "", 0)

	// Verify server was created with both providers
	assert.Len(t, server.providers, 2)
	assert.Equal(t, mockProvider1, server.providers[0])
	assert.Equal(t, mockProvider2, server.providers[1])

	// Test basic start/stop cycle
	startErrChan := make(chan error, 1)
	go func() {
		startErrChan <- server.Start()
	}()

	// Give server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Stop the server
	err := server.Stop()
	assert.NoError(t, err)

	// Verify Start() completed
	select {
	case startErr := <-startErrChan:
		assert.NoError(t, startErr)
	case <-time.After(2 * time.Second):
		t.Fatal("Start() should have completed")
	}
}