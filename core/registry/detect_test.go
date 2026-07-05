package registry

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDetect pins the consolidated Detect(path, opts) entry point against the
// behaviors the deprecated five-method ladder provided.
func TestDetect(t *testing.T) {
	newReg := func() *FormatRegistry {
		reg := NewFormatRegistry()
		reg.RegisterReader("json", func() format.DataFormatReader {
			return newStubReaderWithSig("json", "JSON", []string{"application/json"}, []string{".json"})
		}, format.FormatSignature{Extensions: []string{".json"}}, "JSON")
		reg.RegisterFormatInfo("okf_json", FormatInfo{
			DisplayName: "Okapi JSON",
			Extensions:  []string{".json"},
			Source:      "okapi-bridge",
			HasReader:   true,
		})
		reg.SetFormatPriority("okf_json", format.DefaultPluginPriority)
		return reg
	}

	t.Run("zero options detect a file, falling back to extension when unreadable", func(t *testing.T) {
		reg := newReg()
		// The path does not exist: content sniffing is skipped and the
		// deterministic extension/priority pick applies (plugin priority wins).
		name, err := reg.Detect(filepath.Join(t.TempDir(), "missing.json"), DetectOptions{})
		require.NoError(t, err)
		assert.Equal(t, FormatID("okf_json"), name)
	})

	t.Run("ExtensionOnly accepts a path or a bare extension", func(t *testing.T) {
		reg := newReg()
		name, err := reg.Detect("some/dir/file.json", DetectOptions{ExtensionOnly: true})
		require.NoError(t, err)
		assert.Equal(t, FormatID("okf_json"), name)

		name, err = reg.Detect(".json", DetectOptions{ExtensionOnly: true})
		require.NoError(t, err)
		assert.Equal(t, FormatID("okf_json"), name)

		_, err = reg.Detect("no-extension", DetectOptions{ExtensionOnly: true})
		assert.Error(t, err)
	})

	t.Run("AllowedSources restricts detection", func(t *testing.T) {
		reg := newReg()
		name, err := reg.Detect("file.json", DetectOptions{ExtensionOnly: true, AllowedSources: []string{SourceBuiltIn}})
		require.NoError(t, err)
		assert.Equal(t, FormatID("json"), name)

		_, err = reg.Detect("file.xyz", DetectOptions{ExtensionOnly: true, AllowedSources: []string{SourceBuiltIn}})
		assert.Error(t, err)
	})

	t.Run("PriorityOverrides break ties per call", func(t *testing.T) {
		reg := NewFormatRegistry()
		for _, name := range []string{"okf_regex", "okf_vtt"} {
			reg.RegisterFormatInfo(FormatID(name), FormatInfo{
				DisplayName: name,
				Extensions:  []string{".srt"},
				Source:      "okapi-bridge",
				HasReader:   true,
			})
			reg.SetFormatPriority(FormatID(name), format.DefaultPluginPriority)
		}
		opts := DetectOptions{ExtensionOnly: true, AllowedSources: []string{SourceBuiltIn, "okapi-bridge"}}

		name, err := reg.Detect("ep1.srt", opts)
		require.NoError(t, err)
		assert.Equal(t, FormatID("okf_regex"), name, "alphabetical tiebreak without an override")

		opts.PriorityOverrides = map[string]int{"okf_vtt": 110}
		name, err = reg.Detect("ep1.srt", opts)
		require.NoError(t, err)
		assert.Equal(t, FormatID("okf_vtt"), name)
	})

	t.Run("content sniffing picks among formats sharing an extension", func(t *testing.T) {
		reg := NewFormatRegistry()
		reg.RegisterReader("html", func() format.DataFormatReader {
			return newStubReaderWithSig("html", "HTML", []string{"text/html"}, []string{".html"})
		}, format.FormatSignature{Extensions: []string{".html"}, Sniff: func(b []byte) bool { return bytes.Contains(b, []byte("<html")) }}, "HTML")
		reg.RegisterReader("ahtml", func() format.DataFormatReader {
			return newStubReaderWithSig("ahtml", "Alt HTML", nil, []string{".html"})
		}, format.FormatSignature{Extensions: []string{".html"}, Sniff: func(b []byte) bool { return bytes.Contains(b, []byte("<alt-html")) }}, "Alt HTML")

		path := filepath.Join(t.TempDir(), "page.html")
		require.NoError(t, os.WriteFile(path, []byte("<html><body>hi</body></html>"), 0o644))

		name, err := reg.Detect(path, DetectOptions{})
		require.NoError(t, err)
		assert.Equal(t, FormatID("html"), name)
	})
}
