package verify

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeNumber(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"21,000,000", "21000000"},
		{"21 000 000", "21000000"},
		{"21000000", "21000000"},
		{"  21,000,000  ", "21000000"},
		{"-42", "-42"},
		{"3.14159", "3.14159"},
		{"$1,000", "1000"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := Normalize(tt.input, FormatNumber)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeHex(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"0xA9059CBB", "0xa9059cbb"},
		{"a9059cbb", "0xa9059cbb"},
		{"0xa9059cbb", "0xa9059cbb"},
		{"  0xABC  ", "0xabc"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := Normalize(tt.input, FormatHex)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeAddress(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"0xDEADBEEF123456789012345678901234567890AB", "0xdeadbeef123456789012345678901234567890ab"},
		{"DEADBEEF123456789012345678901234567890AB", "0xdeadbeef123456789012345678901234567890ab"},
		{"  0xDEADBEEF123456789012345678901234567890AB  ", "0xdeadbeef123456789012345678901234567890ab"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := Normalize(tt.input, FormatAddress)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Dr. John Smith", "john smith"},
		{"Mr. John Smith", "john smith"},
		{"JOHN SMITH", "john smith"},
		{"  Prof. Jane Doe  ", "jane doe"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := Normalize(tt.input, FormatName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeText(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"The Answer", "answer"},
		{"A quick brown fox", "quick brown fox"},
		{"An apple", "apple"},
		{"  THE ANSWER  ", "answer"},
		{"Bitcoin", "bitcoin"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := Normalize(tt.input, FormatText)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeGeneral(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  HELLO  WORLD  ", "hello world"},
		{"!!!HELLO!!!", "hello"},
		{"...test...", "test"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := Normalize(tt.input, "")
			assert.Equal(t, tt.expected, result)
		})
	}
}
