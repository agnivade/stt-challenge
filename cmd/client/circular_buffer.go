package main

import (
	"strings"
	"sync"

	"github.com/agnivade/levenshtein"
)

// MessageBuffer implements a circular buffer for storing recent messages for deduplication
type MessageBuffer struct {
	messages []string
	head     int
	size     int
	capacity int
	mu       sync.RWMutex
}

// NewMessageBuffer creates a new message buffer with the specified capacity
func NewMessageBuffer(capacity int) *MessageBuffer {
	// Handle invalid capacity values
	if capacity <= 0 {
		capacity = 1 // Default to minimum useful capacity
	}
	
	return &MessageBuffer{
		messages: make([]string, capacity),
		capacity: capacity,
	}
}

// Add adds a new message to the buffer
func (mb *MessageBuffer) Add(message string) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	
	mb.messages[mb.head] = message
	mb.head = (mb.head + 1) % mb.capacity
	if mb.size < mb.capacity {
		mb.size++
	}
}

// IsSimilar checks if a message is similar to any message in the buffer
func (mb *MessageBuffer) IsSimilar(message string, threshold float64) bool {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	
	normalizedMsg := normalizeMessage(message)
	
	for i := 0; i < mb.size; i++ {
		bufferMsg := normalizeMessage(mb.messages[i])
		if isSimilarMessage(normalizedMsg, bufferMsg, threshold) {
			return true
		}
	}
	return false
}

// normalizeMessage normalizes a message for comparison
func normalizeMessage(msg string) string {
	return strings.ToLower(strings.TrimSpace(msg))
}

// isSimilarMessage checks if two messages are similar based on Levenshtein distance
func isSimilarMessage(msg1, msg2 string, threshold float64) bool {
	if msg1 == msg2 {
		return true
	}
	
	if msg1 == "" || msg2 == "" {
		return false
	}
	
	distance := levenshtein.ComputeDistance(msg1, msg2)
	maxLen := len(msg1)
	if len(msg2) > maxLen {
		maxLen = len(msg2)
	}
	
	similarity := 1.0 - (float64(distance) / float64(maxLen))
	return similarity >= threshold
}