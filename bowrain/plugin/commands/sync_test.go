package commands

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShortPushID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{name: "empty", id: "", want: ""},
		{name: "short non-conforming", id: "abc", want: "abc"},
		{name: "exactly eight", id: "12345678", want: "12345678"},
		{name: "longer than eight truncated", id: "1234567890abcdef", want: "12345678"},
		{name: "single char", id: "x", want: "x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotPanics(t, func() {
				got := shortPushID(tt.id)
				assert.Equal(t, tt.want, got)
			})
		})
	}
}

// TestShortPushID_RendersSensibly ensures a short, non-conforming push_id is
// rendered in the waiting message without panicking on an out-of-range slice.
func TestShortPushID_RendersSensibly(t *testing.T) {
	var msg string
	require.NotPanics(t, func() {
		msg = fmt.Sprintf("Waiting for translations (push_id: %s)...", shortPushID("abc"))
	})
	assert.Equal(t, "Waiting for translations (push_id: abc)...", msg)
}
