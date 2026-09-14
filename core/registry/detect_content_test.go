package registry

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
)

// TestDetectSniffsTheContentItIsGiven detects the format of a file read from
// somewhere other than the disk, such as a git object, by the content supplied
// for it.
func TestDetectSniffsTheContentItIsGiven(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("html", func() format.DataFormatReader {
		return newStubReaderWithSig("html", "HTML", []string{"text/html"}, []string{".html"})
	}, format.FormatSignature{Extensions: []string{".html"}, Sniff: func(b []byte) bool { return bytes.Contains(b, []byte("<html")) }}, "HTML")
	reg.RegisterReader("ahtml", func() format.DataFormatReader {
		return newStubReaderWithSig("ahtml", "Alt HTML", nil, []string{".html"})
	}, format.FormatSignature{Extensions: []string{".html"}, Sniff: func(b []byte) bool { return bytes.Contains(b, []byte("<alt-html")) }}, "Alt HTML")
	page := func() (io.ReadSeeker, error) { return strings.NewReader("<html><body>hi</body></html>"), nil }

	t.Run("a file no disk holds", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "page.html")
		name, err := reg.Detect(path, DetectOptions{Content: page})
		require.NoError(t, err)
		assert.Equal(t, FormatID("html"), name)

		name, err = reg.Detect(path, DetectOptions{})
		require.NoError(t, err)
		assert.Equal(t, FormatID("ahtml"), name, "must fail: without content the extension and priority decide")
	})

	t.Run("the content given, not the file on disk", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "page.html")
		require.NoError(t, os.WriteFile(path, []byte("<alt-html>on disk</alt-html>"), 0o644))
		name, err := reg.Detect(path, DetectOptions{Content: page})
		require.NoError(t, err)
		assert.Equal(t, FormatID("html"), name)
	})

	t.Run("content that cannot be read falls back to the extension", func(t *testing.T) {
		name, err := reg.Detect("page.html", DetectOptions{Content: func() (io.ReadSeeker, error) { return nil, os.ErrNotExist }})
		require.NoError(t, err)
		assert.Equal(t, FormatID("ahtml"), name)
	})
}
