package backend

import (
	"testing"

	"github.com/neokapi/neokapi/host/pluginhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ensureFormatPlugin must return without attempting any (network) install when
// the host already has a reader for the format, or when the format is not one
// installed on demand. These are the only branches safe to exercise offline —
// the install branch (e.g. "pdf" → kapi-pdfium) hits the registry and is covered
// by the pluginhost install tests, not here.
func TestEnsureFormatPlugin_Noop(t *testing.T) {
	app := NewApp()

	// json has an in-core reader → early return, no install attempt.
	require.True(t, app.formatReg.HasReader("json"))
	require.NotPanics(t, func() { app.ensureFormatPlugin("json") })
	assert.True(t, app.formatReg.HasReader("json"))

	// An unknown format is not installed on demand → early return.
	const unknown = "definitely-not-a-real-format"
	require.NotPanics(t, func() { app.ensureFormatPlugin(unknown) })
	assert.False(t, app.formatReg.HasReader(unknown))
}

// PDF is installed on demand, and the plugin named for it is kapi-pdfium (the
// contract the on-demand install relies on).
func TestOnDemandFormatPlugins_PDF(t *testing.T) {
	assert.Contains(t, onDemandFormats, "pdf")
	plugin, ok := pluginhost.FormatProvider(nil, "pdf")
	assert.True(t, ok)
	assert.Equal(t, "pdfium", plugin)
}
