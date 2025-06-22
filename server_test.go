package stt_challenge

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"

	"github.com/agnivade/stt_challenge/providers/mocks"
)

func TestServer_StartAndStop(t *testing.T) {
	// Create mock provider
	mockProvider := mocks.NewMockProvider(t)
	mockProvider.EXPECT().Name().Return("mock-provider").Maybe()

	// Create server with mock provider and silent logger
	server := New("8081", mockProvider)
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
	server := New("8081", mockProvider1, mockProvider2)
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

func TestServer_AddConn(t *testing.T) {
	// Create server
	mockProvider := mocks.NewMockProvider(t)
	server := New("8081", mockProvider)
	server.log = log.New(io.Discard, "", 0)

	// Create mock WebConn objects
	webConn1 := &WebConn{}
	webConn2 := &WebConn{}

	// Test adding connections
	server.addConn(webConn1)
	assert.Len(t, server.conns, 1)
	assert.Contains(t, server.conns, webConn1)

	server.addConn(webConn2)
	assert.Len(t, server.conns, 2)
	assert.Contains(t, server.conns, webConn1)
	assert.Contains(t, server.conns, webConn2)

	// Test adding same connection twice (should not duplicate)
	server.addConn(webConn1)
	assert.Len(t, server.conns, 2) // Still 2, not 3
}

func TestServer_RemoveConn(t *testing.T) {
	// Create server
	mockProvider := mocks.NewMockProvider(t)
	server := New("8081", mockProvider)
	server.log = log.New(io.Discard, "", 0)

	// Create mock WebConn objects
	webConn1 := &WebConn{}
	webConn2 := &WebConn{}

	// Add connections first
	server.addConn(webConn1)
	server.addConn(webConn2)
	assert.Len(t, server.conns, 2)

	// Test removing connections
	server.removeConn(webConn1)
	assert.Len(t, server.conns, 1)
	// Check that webConn1 is not in the map anymore
	_, exists := server.conns[webConn1]
	assert.False(t, exists)
	// Check that webConn2 is still there
	_, exists = server.conns[webConn2]
	assert.True(t, exists)

	server.removeConn(webConn2)
	assert.Len(t, server.conns, 0)

	// Test removing non-existent connection (should be safe)
	server.removeConn(webConn1)
	assert.Len(t, server.conns, 0)
}

func TestServer_StopAllConns_EmptyConnections(t *testing.T) {
	// Create server with no connections
	mockProvider := mocks.NewMockProvider(t)
	server := New("8081", mockProvider)
	server.log = log.New(io.Discard, "", 0)

	assert.Len(t, server.conns, 0)

	// Test stopAllConns with no connections (should not panic)
	server.stopAllConns()

	// Should still have no connections
	assert.Len(t, server.conns, 0)
}

func createMockWebSocketConnection(t *testing.T) *websocket.Conn {
	// Create a test server that upgrades to WebSocket
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade: %v", err)
		}
		defer conn.Close()

		// Keep connection alive for a bit
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	// Connect to the test server
	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}

	return conn
}

func TestServer_StopAllConns_WithRealConnections(t *testing.T) {
	// Create server
	mockProvider := mocks.NewMockProvider(t)
	server := New("8081", mockProvider)
	server.log = log.New(io.Discard, "", 0)

	// Create WebConns with real WebSocket connections
	conn1 := createMockWebSocketConnection(t)
	conn2 := createMockWebSocketConnection(t)

	webConn1 := &WebConn{conn: conn1}
	webConn2 := &WebConn{conn: conn2}

	server.addConn(webConn1)
	server.addConn(webConn2)
	assert.Len(t, server.conns, 2)

	// Test stopAllConns - this should call Stop() on each connection
	server.stopAllConns()

	// Verify connections are still in the map (they don't auto-remove in this test)
	assert.Len(t, server.conns, 2)

	// Connections should be closed now, so further operations should fail
	err := conn1.WriteMessage(websocket.TextMessage, []byte("test"))
	assert.Error(t, err) // Should fail because connection is closed

	err = conn2.WriteMessage(websocket.TextMessage, []byte("test"))
	assert.Error(t, err) // Should fail because connection is closed
}

func TestServer_StopWithConnections_FullLifecycle(t *testing.T) {
	// Create server
	mockProvider := mocks.NewMockProvider(t)
	server := New("8081", mockProvider)
	server.log = log.New(io.Discard, "", 0)

	// Start server in goroutine
	startErrChan := make(chan error, 1)
	go func() {
		startErrChan <- server.Start()
	}()

	// Give server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Add some mock connections with real WebSocket connections
	conn1 := createMockWebSocketConnection(t)
	conn2 := createMockWebSocketConnection(t)

	webConn1 := &WebConn{conn: conn1}
	webConn2 := &WebConn{conn: conn2}

	server.addConn(webConn1)
	server.addConn(webConn2)
	assert.Len(t, server.conns, 2)

	// Stop the server - this should call stopAllConns which closes connections
	err := server.Stop()
	assert.NoError(t, err)

	// Verify Start() completed
	select {
	case startErr := <-startErrChan:
		assert.NoError(t, startErr)
	case <-time.After(2 * time.Second):
		t.Fatal("Start() should have completed")
	}

	// Verify connections were closed by stopAllConns
	err = conn1.WriteMessage(websocket.TextMessage, []byte("test"))
	assert.Error(t, err) // Should fail because connection was closed by stopAllConns

	err = conn2.WriteMessage(websocket.TextMessage, []byte("test"))
	assert.Error(t, err) // Should fail because connection was closed by stopAllConns
}
