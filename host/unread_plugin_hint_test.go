package host

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/registry"
)

// The install hint for content no installed reader opens names the plugin that
// supplies the format, which is the name `kapi plugins install` resolves. When
// kapi knows no plugin that supplies the format, the hint says so and prints no
// command.

var errNoReader = fmt.Errorf("open: %w", registry.ErrUnknownFormat)

func TestNoReaderErrorNamesThePluginThatSuppliesTheFormat(t *testing.T) {
	for _, tc := range []struct{ format, plugin string }{
		{"okf_idml", "okapi-bridge"},
		{"okf_openxml", "okapi-bridge"},
		{"sourcecode", "sourcecode"},
		{"pdf", "pdfium"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			err := NoReaderError(errNoReader, "doc", tc.format)
			require.ErrorIs(t, err, registry.ErrUnknownFormat)
			assert.Contains(t, err.Error(), fmt.Sprintf("format %q", tc.format))
			assert.Contains(t, err.Error(), "(kapi plugins install "+tc.plugin+")")
			if tc.plugin != tc.format {
				assert.NotContains(t, err.Error(), "(kapi plugins install "+tc.format+")")
			}
		})
	}
}

func TestNoReaderErrorSaysWhenNoKnownPluginSuppliesTheFormat(t *testing.T) {
	err := NoReaderError(errNoReader, "doc.frob", "frob")
	require.ErrorIs(t, err, registry.ErrUnknownFormat)
	assert.Contains(t, err.Error(), `format "frob"`)
	assert.Contains(t, err.Error(), "no known plugin supplies it")
	assert.NotContains(t, err.Error(), "kapi plugins install")
}

func TestUnreadSetNamesEachPluginOnceAndTheFormatsNoPluginSupplies(t *testing.T) {
	u := NewUnreadSet()
	require.True(t, u.Skip(errNoReader, "pkg/doc.idml", "okf_idml"))
	require.True(t, u.Skip(errNoReader, "pkg/deck.pptx", "okf_openxml"))
	require.True(t, u.Skip(errNoReader, "data/x.frob", "frob"))

	summary := u.summary()
	assert.Equal(t, 1, countOf(summary, "kapi plugins install okapi-bridge"), "two bridge formats need one install: %s", summary)
	assert.Contains(t, summary, `no known plugin supplies format "frob"`)
	assert.NotContains(t, summary, "kapi plugins install okf_")
	assert.NotContains(t, summary, "kapi plugins install frob")

	byFile := map[string]string{}
	for _, w := range u.warnings() {
		byFile[w.Source] = w.Message
	}
	assert.Contains(t, byFile["pkg/doc.idml"], "(kapi plugins install okapi-bridge)")
	assert.Contains(t, byFile["data/x.frob"], "no known plugin supplies it")
	assert.NotContains(t, byFile["data/x.frob"], "kapi plugins install")

	assert.Contains(t, noReaderReason(errNoReader, "okf_idml"), "(kapi plugins install okapi-bridge)")
}

func countOf(s, sub string) int {
	n := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			n++
		}
	}
	return n
}
