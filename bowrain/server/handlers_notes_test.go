package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseMentions(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"single mention", "Hey @alice check this", []string{"alice"}},
		{"multiple mentions", "@alice and @bob please review", []string{"alice", "bob"}},
		{"duplicate mentions", "@alice @alice @bob", []string{"alice", "bob"}},
		{"no mentions", "No mentions here", nil},
		{"empty string", "", nil},
		{"mention at start", "@admin hello", []string{"admin"}},
		{"mention at end", "hello @admin", []string{"admin"}},
		{"mention with numbers", "@user123 check", []string{"user123"}},
		{"mention with underscore", "@john_doe review", []string{"john_doe"}},
		{"email contains at sign", "email user@example.com", []string{"example"}},
		{"mention in middle of sentence", "Please ask @reviewer to check", []string{"reviewer"}},
		{"consecutive mentions", "@alice@bob", []string{"alice", "bob"}},
		{"mention with punctuation after", "@alice, please review", []string{"alice"}},
		{"only at sign", "@ nothing", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseMentions(tt.text)
			assert.Equal(t, tt.want, got)
		})
	}
}
