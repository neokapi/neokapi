package host

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewConfiguredReader is finding 7: the five preset-reader sites now share
// one path. A plain format resolves to a reader; an unknown format errors
// cleanly with the ref in the message.
func TestNewConfiguredReader(t *testing.T) {
	a := &App{}
	a.InitRegistries()

	reader, name, err := a.NewConfiguredReader("json")
	require.NoError(t, err)
	assert.Equal(t, "json", name)
	require.NotNil(t, reader)

	_, _, err = a.NewConfiguredReader("definitely-not-a-format")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "definitely-not-a-format")
}

// TestApplyFormatConfigStripsOutputKeys guards the behavioral half of finding 7:
// the shared reader path runs config through applyFormatConfig, which strips the
// reserved output.* writer keys — the step the MCP copy skipped, so an
// output.newline in a preset would otherwise reach the reader's ApplyMap and be
// rejected as unknown.
func TestApplyFormatConfigStripsOutputKeys(t *testing.T) {
	a := &App{}
	a.InitRegistries()

	reader, err := a.FormatReg.NewReader("json")
	require.NoError(t, err)
	configurable, ok := reader.(format.Configurable)
	require.True(t, ok, "json reader is configurable")

	// A bare output.* key must be stripped, not applied to the reader.
	require.NoError(t, applyFormatConfig(configurable, map[string]any{"output.newline": "lf"}))
}
