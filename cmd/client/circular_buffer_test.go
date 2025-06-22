package main

import (
	"fmt"
	"strings"
	"testing"
)

// Basic Functionality Tests

func TestNewMessageBuffer(t *testing.T) {
	tests := []struct {
		name         string
		capacity     int
		wantCapacity int
		wantSize     int
		wantHead     int
	}{
		{"small buffer", 1, 1, 0, 0},
		{"medium buffer", 10, 10, 0, 0},
		{"large buffer", 100, 100, 0, 0},
		{"zero capacity", 0, 1, 0, 0},      // defaults to 1
		{"negative capacity", -1, 1, 0, 0}, // defaults to 1
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := NewMessageBuffer(tt.capacity)
			if buf.capacity != tt.wantCapacity {
				t.Errorf("NewMessageBuffer() capacity = %v, want %v", buf.capacity, tt.wantCapacity)
			}
			if buf.size != tt.wantSize {
				t.Errorf("NewMessageBuffer() size = %v, want %v", buf.size, tt.wantSize)
			}
			if buf.head != tt.wantHead {
				t.Errorf("NewMessageBuffer() head = %v, want %v", buf.head, tt.wantHead)
			}
			if len(buf.messages) != tt.wantCapacity {
				t.Errorf("NewMessageBuffer() messages length = %v, want %v", len(buf.messages), tt.wantCapacity)
			}
		})
	}
}

func TestAdd(t *testing.T) {
	t.Run("single message", func(t *testing.T) {
		buf := NewMessageBuffer(5)
		buf.Add("hello")

		if buf.size != 1 {
			t.Errorf("Add() size = %v, want 1", buf.size)
		}
		if buf.head != 1 {
			t.Errorf("Add() head = %v, want 1", buf.head)
		}
		if buf.messages[0] != "hello" {
			t.Errorf("Add() messages[0] = %v, want 'hello'", buf.messages[0])
		}
	})

	t.Run("fill to capacity", func(t *testing.T) {
		buf := NewMessageBuffer(3)
		messages := []string{"msg1", "msg2", "msg3"}

		for i, msg := range messages {
			buf.Add(msg)
			if buf.size != i+1 {
				t.Errorf("Add() size = %v, want %v", buf.size, i+1)
			}
		}

		if buf.size != 3 {
			t.Errorf("Add() final size = %v, want 3", buf.size)
		}
		if buf.head != 0 { // wrapped around
			t.Errorf("Add() final head = %v, want 0", buf.head)
		}
	})

	t.Run("add beyond capacity", func(t *testing.T) {
		buf := NewMessageBuffer(2)
		buf.Add("msg1")
		buf.Add("msg2")
		buf.Add("msg3") // should overwrite msg1

		if buf.size != 2 {
			t.Errorf("Add() size = %v, want 2", buf.size)
		}
		if buf.messages[0] != "msg3" {
			t.Errorf("Add() messages[0] = %v, want 'msg3'", buf.messages[0])
		}
		if buf.messages[1] != "msg2" {
			t.Errorf("Add() messages[1] = %v, want 'msg2'", buf.messages[1])
		}
	})

	t.Run("empty and whitespace messages", func(t *testing.T) {
		buf := NewMessageBuffer(3)
		buf.Add("")
		buf.Add("   ")
		buf.Add("\t\n")

		if buf.size != 3 {
			t.Errorf("Add() size = %v, want 3", buf.size)
		}
	})
}

// Circular Buffer Mechanics Tests

func TestCircularOverwrite(t *testing.T) {
	t.Run("wraparound behavior", func(t *testing.T) {
		buf := NewMessageBuffer(3)
		messages := []string{"a", "b", "c", "d", "e"}

		for _, msg := range messages {
			buf.Add(msg)
		}

		// Should contain: [d, e, c] with head=2
		expected := []string{"d", "e", "c"}
		for i, exp := range expected {
			if buf.messages[i] != exp {
				t.Errorf("Add() messages[%d] = %v, want %v", i, buf.messages[i], exp)
			}
		}

		if buf.head != 2 {
			t.Errorf("Add() head = %v, want 2", buf.head)
		}
		if buf.size != 3 {
			t.Errorf("Add() size = %v, want 3", buf.size)
		}
	})

	t.Run("capacity one immediate overwrite", func(t *testing.T) {
		buf := NewMessageBuffer(1)
		buf.Add("first")
		buf.Add("second")
		buf.Add("third")

		if buf.size != 1 {
			t.Errorf("Add() size = %v, want 1", buf.size)
		}
		if buf.messages[0] != "third" {
			t.Errorf("Add() messages[0] = %v, want 'third'", buf.messages[0])
		}
	})
}

func TestBufferState(t *testing.T) {
	buf := NewMessageBuffer(4)

	// Test size never exceeds capacity
	for i := 0; i < 10; i++ {
		buf.Add("message")
		if buf.size > buf.capacity {
			t.Errorf("Add() size %v exceeds capacity %v", buf.size, buf.capacity)
		}
	}

	// Test head position calculation
	buf = NewMessageBuffer(3)
	expectedHeads := []int{1, 2, 0, 1, 2}
	for i, expectedHead := range expectedHeads {
		buf.Add("msg")
		if buf.head != expectedHead {
			t.Errorf("Add() iteration %d: head = %v, want %v", i, buf.head, expectedHead)
		}
	}
}

// Similarity Detection Tests

func TestIsSimilar_ExactMatches(t *testing.T) {
	buf := NewMessageBuffer(5)
	buf.Add("hello world")

	tests := []struct {
		name      string
		message   string
		threshold float64
		want      bool
	}{
		{"identical strings", "hello world", 0.8, true},
		{"case difference", "Hello World", 0.8, true},
		{"whitespace difference", "  hello world  ", 0.8, true},
		{"mixed case and whitespace", "  HeLLo WoRLd  ", 0.8, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buf.IsSimilar(tt.message, tt.threshold)
			if got != tt.want {
				t.Errorf("IsSimilar() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsSimilar_ThresholdBoundaries(t *testing.T) {
	buf := NewMessageBuffer(5)
	buf.Add("hello") // length 5

	tests := []struct {
		name      string
		message   string
		threshold float64
		want      bool
	}{
		// "hallo" has distance 1 from "hello", similarity = 1 - 1/5 = 0.8
		{"exactly at threshold", "hallo", 0.8, true},
		{"just below threshold", "hallo", 0.81, false},
		{"just above threshold", "hallo", 0.79, true},
		{"threshold 0.0", "xyz", 0.0, true},
		{"threshold 1.0", "hello", 1.0, true},
		{"threshold 1.0 different", "hallo", 1.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buf.IsSimilar(tt.message, tt.threshold)
			if got != tt.want {
				t.Errorf("IsSimilar() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsSimilar_LevenshteinCases(t *testing.T) {
	buf := NewMessageBuffer(10)
	buf.Add("hello world")

	tests := []struct {
		name      string
		message   string
		threshold float64
		want      bool
	}{
		{"single substitution", "hallo world", 0.8, true},
		{"multiple substitutions", "hallo warld", 0.7, true},
		{"insertion", "hello world!", 0.8, true},
		{"deletion", "hello worl", 0.8, true},
		{"completely different", "goodbye universe", 0.8, false},
		{"shorter similar", "hello", 0.4, true},
		{"longer similar", "hello world how are you", 0.4, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buf.IsSimilar(tt.message, tt.threshold)
			if got != tt.want {
				t.Errorf("IsSimilar('%s') = %v, want %v", tt.message, got, tt.want)
			}
		})
	}
}

// Edge Cases Tests

func TestEmptyStrings(t *testing.T) {
	t.Run("empty vs empty", func(t *testing.T) {
		buf := NewMessageBuffer(3)
		buf.Add("")

		if !buf.IsSimilar("", 0.8) {
			t.Error("IsSimilar() empty vs empty should be true")
		}
	})

	t.Run("empty vs non-empty", func(t *testing.T) {
		buf := NewMessageBuffer(3)
		buf.Add("")

		if buf.IsSimilar("hello", 0.8) {
			t.Error("IsSimilar() empty vs non-empty should be false")
		}
	})

	t.Run("non-empty vs empty", func(t *testing.T) {
		buf := NewMessageBuffer(3)
		buf.Add("hello")

		if buf.IsSimilar("", 0.8) {
			t.Error("IsSimilar() non-empty vs empty should be false")
		}
	})
}

func TestSpecialCharacters(t *testing.T) {
	buf := NewMessageBuffer(5)

	tests := []struct {
		stored    string
		test      string
		threshold float64
		want      bool
	}{
		{"café", "cafe", 0.7, true},
		{"Hello, world!", "Hello world", 0.7, true},
		{"test123", "test456", 0.5, true},
		{"test@#$", "test!@#", 0.7, true},
		{"🎉 party", "party", 0.5, true},
	}

	for _, tt := range tests {
		t.Run(tt.stored+" vs "+tt.test, func(t *testing.T) {
			buf = NewMessageBuffer(5) // Reset buffer
			buf.Add(tt.stored)
			got := buf.IsSimilar(tt.test, tt.threshold)
			if got != tt.want {
				t.Errorf("IsSimilar('%s' vs '%s') = %v, want %v", tt.stored, tt.test, got, tt.want)
			}
		})
	}
}

func TestExtremeValues(t *testing.T) {
	t.Run("very long messages", func(t *testing.T) {
		buf := NewMessageBuffer(3)
		longMsg := strings.Repeat("a", 1000)
		buf.Add(longMsg)

		similarLongMsg := strings.Repeat("a", 999) + "b"
		if !buf.IsSimilar(similarLongMsg, 0.9) {
			t.Error("IsSimilar() should handle very long messages")
		}
	})

	t.Run("single character messages", func(t *testing.T) {
		buf := NewMessageBuffer(3)
		buf.Add("a")

		if !buf.IsSimilar("a", 0.8) {
			t.Error("IsSimilar() should handle single character exact match")
		}
		if buf.IsSimilar("b", 0.8) {
			t.Error("IsSimilar() should handle single character different")
		}
	})

	t.Run("whitespace only messages", func(t *testing.T) {
		buf := NewMessageBuffer(3)
		buf.Add("   ")

		if !buf.IsSimilar("\t\t\t", 0.8) {
			t.Error("IsSimilar() should normalize whitespace-only messages")
		}
	})
}

// Normalization Function Tests

func TestNormalizeMessage(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"  hello  ", "hello"},
		{"HeLLo WoRLd", "hello world"},
		{"hello    world", "hello    world"}, // internal spaces preserved
		{"\thello\n", "hello"},
		{"", ""},
		{"   ", ""},
		{"UPPER", "upper"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeMessage(tt.input)
			if got != tt.want {
				t.Errorf("normalizeMessage('%s') = '%s', want '%s'", tt.input, got, tt.want)
			}
		})
	}
}

// Similarity Algorithm Tests

func TestSimilarityCalculation(t *testing.T) {
	tests := []struct {
		msg1      string
		msg2      string
		threshold float64
		want      bool
	}{
		// Known calculations
		{"abc", "abc", 0.8, true},     // distance=0, similarity=1.0
		{"abc", "ab", 0.8, false},     // distance=1, similarity=1-1/3=0.667
		{"abc", "ab", 0.6, true},      // distance=1, similarity=1-1/3=0.667
		{"hello", "hallo", 0.8, true}, // distance=1, similarity=1-1/5=0.8
		{"test", "best", 0.7, true},   // distance=1, similarity=1-1/4=0.75
	}

	for _, tt := range tests {
		t.Run(tt.msg1+" vs "+tt.msg2, func(t *testing.T) {
			got := isSimilarMessage(tt.msg1, tt.msg2, tt.threshold)
			if got != tt.want {
				t.Errorf("isSimilarMessage('%s', '%s', %.2f) = %v, want %v",
					tt.msg1, tt.msg2, tt.threshold, got, tt.want)
			}
		})
	}
}

// Performance Tests

func BenchmarkAdd(b *testing.B) {
	sizes := []int{10, 100, 1000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			buf := NewMessageBuffer(size)
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				buf.Add("test message")
			}
		})
	}
}

func BenchmarkIsSimilar(b *testing.B) {
	buf := NewMessageBuffer(100)

	// Fill buffer
	for i := 0; i < 100; i++ {
		buf.Add(fmt.Sprintf("message number %d", i))
	}

	testMsg := "message number 50"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.IsSimilar(testMsg, 0.8)
	}
}

func BenchmarkSimilarityDifferentLengths(b *testing.B) {
	lengths := []int{10, 50, 100, 500}

	for _, length := range lengths {
		b.Run(fmt.Sprintf("length-%d", length), func(b *testing.B) {
			buf := NewMessageBuffer(10)
			longMsg := strings.Repeat("a", length)
			buf.Add(longMsg)

			testMsg := strings.Repeat("a", length-1) + "b"

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				buf.IsSimilar(testMsg, 0.8)
			}
		})
	}
}

// Error Handling & Robustness Tests

func TestInvalidInputs(t *testing.T) {
	t.Run("extreme threshold values", func(t *testing.T) {
		buf := NewMessageBuffer(5)
		buf.Add("test")

		// Should not panic with extreme values
		buf.IsSimilar("test", -1.0)
		buf.IsSimilar("test", 2.0)
		buf.IsSimilar("test", 0.0)
		buf.IsSimilar("test", 1.0)
	})
}

func TestMemoryLimits(t *testing.T) {
	t.Run("large buffer size", func(t *testing.T) {
		// This should not cause issues
		buf := NewMessageBuffer(10000)
		if buf.capacity != 10000 {
			t.Errorf("Large buffer creation failed")
		}
	})

	t.Run("very long message", func(t *testing.T) {
		buf := NewMessageBuffer(5)
		veryLongMsg := strings.Repeat("abcdefghij", 1000) // 10k characters

		buf.Add(veryLongMsg)
		if buf.size != 1 {
			t.Errorf("Failed to add very long message")
		}
	})
}
