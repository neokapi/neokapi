package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeAppName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"bravo-conv-123", "bravo-conv-123"},
		{"bravo-CONV-ABC", "bravo-conv-abc"},
		{"bravo_conv_123", "bravo-conv-123"},
		{"-leading-hyphen", "leading-hyphen"},
		{"trailing-hyphen-", "trailing-hyphen"},
		{"a-very-long-name-that-exceeds-thirty-two-characters", "a-very-long-name-that-exceeds-th"},
		{"special!chars@here#now", "specialcharsherenow"},
		{"---", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeAppName(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestACAConfigInterfaceCompliance(t *testing.T) {
	// Verify at compile-time that ACARuntime implements ContainerRuntime.
	var _ ContainerRuntime = (*ACARuntime)(nil)
}
