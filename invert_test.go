package jsonot

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInvert(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Invert positive integer",
			input:    `{"p":["p1", "p2"], "t": "na", "o": 100}`,
			expected: `{"p":["p1", "p2"], "t": "na", "o": -100}`,
		},
		{
			name:     "Invert negative integer",
			input:    `{"p":["p1", "p2"], "t": "na", "o": -100}`,
			expected: `{"p":["p1", "p2"], "t": "na", "o": 100}`,
		},
		{
			name:     "Invert positive float",
			input:    `{"p":["p1", "p2"], "t": "na", "o": 10.101}`,
			expected: `{"p":["p1", "p2"], "t": "na", "o": -10.101}`,
		},
		{
			name:     "Invert negative float",
			input:    `{"p":["p1", "p2"], "t": "na", "o": -10.101}`,
			expected: `{"p":["p1", "p2"], "t": "na", "o": 10.101}`,
		},
		{
			name:     "Invert positive integer with key",
			input:    `{"p":["p1", "p2"], "na": 100}`,
			expected: `{"p":["p1", "p2"], "na": -100}`,
		},
		{
			name:     "Invert negative integer with key",
			input:    `{"p":["p1", "p2"], "na": -100}`,
			expected: `{"p":["p1", "p2"], "na": 100}`,
		},
		{
			name:     "Invert positive float with key",
			input:    `{"p":["p1", "p2"], "na": 10.01}`,
			expected: `{"p":["p1", "p2"], "na": -10.01}`,
		},
		{
			name:     "Invert negative float with key",
			input:    `{"p":["p1", "p2"], "na": -10.01}`,
			expected: `{"p":["p1", "p2"], "na": 10.01}`,
		},
		{
			name:     "Invert object with positive integer",
			input:    `{"p":["p1", "p2"], "t": "text", "o":{"p":10, "i":"hello"}}`,
			expected: `{"p":["p1", "p2"], "t": "text", "o":{"p":10, "d":"hello"}}`,
		},
		{
			name:     "Invert object with negative integer",
			input:    `{"p":["p1", "p2"], "t": "text", "o":{"p":10, "d":"hello"}}`,
			expected: `{"p":["p1", "p2"], "t": "text", "o":{"p":10, "i":"hello"}}`,
		},
	}

	ot := NewJSONOperationTransformer()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Unmarshal the input string to an Operation
			input, err := UnmarshalValue([]byte(tt.input))
			if err != nil {
				t.Fatalf("UnmarshalValue() error = %v", err)
			}
			operation := ot.OperationComponentFromValue(input)
			result := operation.MustGet().Invert()
			if result.IsError() {
				t.Fatalf("Invert() error = %v", result.Error())
			}

			assert.JSONEq(t, tt.expected, string(result.MustGet().ToNode().RawMessage()), "Invert() result mismatch")
		})
	}
}
