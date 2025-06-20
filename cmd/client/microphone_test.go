package main

import "testing"

func TestInt16SliceToByteSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    []int16
		expected []byte
	}{
		{
			name:     "Empty slice",
			input:    []int16{},
			expected: []byte{},
		},
		{
			name:     "Single positive value",
			input:    []int16{258}, // 0x0102
			expected: []byte{0x02, 0x01}, // little-endian: low byte first
		},
		{
			name:     "Single negative value",
			input:    []int16{-1}, // 0xFFFF
			expected: []byte{0xFF, 0xFF},
		},
		{
			name:     "Zero value",
			input:    []int16{0},
			expected: []byte{0x00, 0x00},
		},
		{
			name:     "Multiple values",
			input:    []int16{256, 1, -32768}, // 0x0100, 0x0001, 0x8000
			expected: []byte{0x00, 0x01, 0x01, 0x00, 0x00, 0x80},
		},
		{
			name:     "Max positive value",
			input:    []int16{32767}, // 0x7FFF
			expected: []byte{0xFF, 0x7F},
		},
		{
			name:     "Min negative value",
			input:    []int16{-32768}, // 0x8000
			expected: []byte{0x00, 0x80},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := int16SliceToByteSlice(tt.input)
			
			if len(result) != len(tt.expected) {
				t.Errorf("Expected length %d, got %d", len(tt.expected), len(result))
				return
			}
			
			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("At index %d: expected 0x%02X, got 0x%02X", i, expected, result[i])
				}
			}
			
			// Verify the slice length is exactly double the input length
			expectedLen := len(tt.input) * 2
			if len(result) != expectedLen {
				t.Errorf("Expected result length %d (2 * input length), got %d", expectedLen, len(result))
			}
		})
	}
}
