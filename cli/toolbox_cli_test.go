package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureStdout (func() error variant) is defined in verify_test.go and reused
// here — the toolbox run* functions print directly to os.Stdout, so capturing
// it exercises the real output-formatting code paths.

func writeToolboxFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

func TestRunCatOutput(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := writeToolboxFile(t, dir, "en.json", `{"a":"Hello","b":"World"}`)

	t.Run("plain", func(t *testing.T) {
		out, err := captureStdout(t, func() error {
			return app.runCat(context.Background(), &cobra.Command{}, []string{path}, catOptions{})
		})
		require.NoError(t, err)
		assert.Equal(t, "Hello\nWorld\n", out)
	})

	t.Run("numbered", func(t *testing.T) {
		out, err := captureStdout(t, func() error {
			return app.runCat(context.Background(), &cobra.Command{}, []string{path}, catOptions{number: true})
		})
		require.NoError(t, err)
		assert.Contains(t, out, "     1\tHello")
		assert.Contains(t, out, "     2\tWorld")
	})

	t.Run("target locale with no translation prints nothing", func(t *testing.T) {
		out, err := captureStdout(t, func() error {
			return app.runCat(context.Background(), &cobra.Command{}, []string{path}, catOptions{targetLoc: "fr"})
		})
		require.NoError(t, err)
		assert.Empty(t, out)
	})
}

func TestRunGrepOutput(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := writeToolboxFile(t, dir, "en.json", `{"a":"Hello world","b":"keep world","c":"nope"}`)

	mustMatcher := func(t *testing.T, pat string, o matcherOpts) *matcher {
		m, err := newMatcher([]string{pat}, o)
		require.NoError(t, err)
		return m
	}

	t.Run("match with block numbers, single file has no name prefix", func(t *testing.T) {
		out, err := captureStdout(t, func() error {
			return app.runGrep(context.Background(), []string{path}, mustMatcher(t, "world", matcherOpts{}), grepOptions{number: true})
		})
		require.NoError(t, err)
		assert.Equal(t, "1:Hello world\n2:keep world\n", out)
	})

	t.Run("count", func(t *testing.T) {
		out, err := captureStdout(t, func() error {
			return app.runGrep(context.Background(), []string{path}, mustMatcher(t, "world", matcherOpts{}), grepOptions{count: true})
		})
		require.NoError(t, err)
		assert.Equal(t, "2\n", out)
	})

	t.Run("files-with-matches", func(t *testing.T) {
		out, err := captureStdout(t, func() error {
			return app.runGrep(context.Background(), []string{path}, mustMatcher(t, "world", matcherOpts{}), grepOptions{filesWith: true})
		})
		require.NoError(t, err)
		assert.Equal(t, path+"\n", out)
	})

	t.Run("invert", func(t *testing.T) {
		out, err := captureStdout(t, func() error {
			return app.runGrep(context.Background(), []string{path}, mustMatcher(t, "world", matcherOpts{invert: true}), grepOptions{})
		})
		require.NoError(t, err)
		assert.Equal(t, "nope\n", out)
	})

	t.Run("no match returns ErrSilentExit and prints nothing", func(t *testing.T) {
		out, err := captureStdout(t, func() error {
			return app.runGrep(context.Background(), []string{path}, mustMatcher(t, "zzz", matcherOpts{}), grepOptions{})
		})
		assert.Empty(t, out)
		assert.ErrorIs(t, err, ErrSilentExit, "no-match must signal exit 1 via ErrSilentExit")
	})

	t.Run("missing file returns exit 2 (trouble) and stays silent", func(t *testing.T) {
		out, err := captureStdout(t, func() error {
			return app.runGrep(context.Background(), []string{dir + "/missing.json"}, mustMatcher(t, "world", matcherOpts{}), grepOptions{})
		})
		assert.Empty(t, out)
		require.Error(t, err)
		assert.Equal(t, ExitUsage, ExitCode(nil, err), "a read error must map to exit 2 (grep trouble)")
		assert.ErrorIs(t, err, ErrSilentExit, "trouble suppresses the summary message")
	})

	t.Run("two files get name prefixes", func(t *testing.T) {
		path2 := writeToolboxFile(t, dir, "more.json", `{"x":"world tour"}`)
		out, err := captureStdout(t, func() error {
			return app.runGrep(context.Background(), []string{path, path2}, mustMatcher(t, "world", matcherOpts{}), grepOptions{})
		})
		require.NoError(t, err)
		assert.Contains(t, out, path+":Hello world")
		assert.Contains(t, out, path2+":world tour")
	})
}

func TestRunSedStdout(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := writeToolboxFile(t, dir, "en.json", `{"a":"Hello world"}`)

	prog, err := parseSedProgram([]string{"s/world/EARTH/g"})
	require.NoError(t, err)
	sedTool := newSedTool(prog, "", true)

	out, cerr := captureStdout(t, func() error {
		return app.runSed(context.Background(), []string{path}, sedTool, "", false, "")
	})
	require.NoError(t, cerr)
	assert.Contains(t, out, "Hello EARTH")
	// stdout mode must not touch the original file
	orig, _ := os.ReadFile(path)
	assert.Contains(t, string(orig), "world")
}

// TestEditDocumentDocxRoundtrip proves ksed edits the prose inside a real .docx
// and the writer reconstructs a still-readable document (faithful round-trip).
// Skipped if the openxml fixtures aren't present.
func TestEditDocumentDocxRoundtrip(t *testing.T) {
	fixtures, _ := filepath.Glob("../core/formats/openxml/testdata/*.docx")
	if len(fixtures) == 0 {
		t.Skip("no openxml .docx fixtures available")
	}
	app := newToolboxApp(t)

	// Find a fixture whose source text contains a plain alphabetic word we can
	// rewrite deterministically.
	var src, word string
	for _, fx := range fixtures {
		var firstWord string
		_, err := app.streamBlocks(context.Background(), fx, func(_ int, b *model.Block) error {
			if firstWord != "" {
				return nil
			}
			for w := range strings.FieldsSeq(b.SourceText()) {
				if isAlphaWord(w) {
					firstWord = w
					return nil
				}
			}
			return nil
		})
		if err == nil && firstWord != "" {
			src, word = fx, firstWord
			break
		}
	}
	if src == "" {
		t.Skip("no suitable .docx fixture with a plain word")
	}

	dir := t.TempDir()
	dst := filepath.Join(dir, "doc.docx")
	in, err := os.ReadFile(src)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dst, in, 0o644))

	prog, err := parseSedProgram([]string{"s/" + word + "/ZZWORDZZ/g"})
	require.NoError(t, err)
	sedTool := newSedTool(prog, "", true)

	// Edit in place.
	require.NoError(t, app.editDocument(context.Background(), dst, sedTool, "", true, "", nil))

	// Re-read the rewritten .docx: it must still parse, and the replacement must
	// be visible in the extracted text.
	var found bool
	_, err = app.streamBlocks(context.Background(), dst, func(_ int, b *model.Block) error {
		if strings.Contains(b.SourceText(), "ZZWORDZZ") {
			found = true
		}
		return nil
	})
	require.NoError(t, err, "rewritten .docx must still be readable")
	assert.True(t, found, "replacement text must survive the round-trip")
}

// TestResolveFormatNameStdin verifies the toolbox's format resolution — the one
// path both files and stdin share — routes content (no filename) through the
// container-aware detector, so piped documents are recognised rather than
// blindly treated as plain text.
func TestResolveFormatNameStdin(t *testing.T) {
	app := newToolboxApp(t)

	t.Run("piped JSON is detected by content", func(t *testing.T) {
		assert.Equal(t, "json", app.resolveFormatName(stdinName, []byte(`{"a":"hello","b":"world"}`)))
	})

	t.Run("piped plain text falls back to plaintext", func(t *testing.T) {
		assert.Equal(t, fallbackFormat, app.resolveFormatName(stdinName, []byte("just some words\n")))
	})

	t.Run("explicit --format wins over content", func(t *testing.T) {
		app.FormatFlag = "plaintext"
		defer func() { app.FormatFlag = "" }()
		assert.Equal(t, "plaintext", app.resolveFormatName(stdinName, []byte(`{"a":"b"}`)))
	})

	t.Run("file extension still wins for paths", func(t *testing.T) {
		// .md extension chosen even though the bytes look like JSON.
		assert.Equal(t, "markdown", app.resolveFormatName("notes.md", []byte(`{"a":"b"}`)))
	})

	t.Run("piped .docx is detected as openxml, not epub", func(t *testing.T) {
		fixtures, _ := filepath.Glob("../core/formats/openxml/testdata/*.docx")
		if len(fixtures) == 0 {
			t.Skip("no .docx fixtures available")
		}
		content, err := os.ReadFile(fixtures[0])
		require.NoError(t, err)
		assert.Equal(t, "openxml", app.resolveFormatName(stdinName, content))
	})
}

func findCmd(cmds []*cobra.Command, name string) *cobra.Command {
	for _, c := range cmds {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

// TestToolboxProxiesDetached verifies the hidden `kapi grep|sed|cat` proxies are
// flag-detached: -v/-c carry their grep meaning (not kapi's --verbose/--config),
// because the proxy delegates to the standalone command rather than inheriting
// kapi's persistent flags.
func TestToolboxProxiesDetached(t *testing.T) {
	app := newToolboxApp(t)

	t.Run("all three are hidden and flag-detached", func(t *testing.T) {
		for _, name := range []string{"grep", "sed", "cat"} {
			c := findCmd(app.NewToolboxProxies(), name)
			require.NotNil(t, c, "proxy %q must exist", name)
			assert.True(t, c.Hidden, "%q must be hidden from kapi --help", name)
			assert.True(t, c.DisableFlagParsing, "%q must not inherit kapi's persistent flags", name)
		}
	})

	dir := t.TempDir()
	path := writeToolboxFile(t, dir, "en.json", `{"a":"hello world","b":"nope"}`)

	t.Run("kapi grep -v inverts (not verbose)", func(t *testing.T) {
		grep := findCmd(app.NewToolboxProxies(), "grep")
		out, err := captureStdout(t, func() error {
			grep.SetArgs([]string{"-v", "world", path})
			return grep.Execute()
		})
		require.NoError(t, err)
		assert.Equal(t, "nope\n", out)
	})

	t.Run("kapi grep -c counts (not config)", func(t *testing.T) {
		grep := findCmd(app.NewToolboxProxies(), "grep")
		out, err := captureStdout(t, func() error {
			grep.SetArgs([]string{"-c", "world", path})
			return grep.Execute()
		})
		require.NoError(t, err)
		assert.Equal(t, "1\n", out)
	})

	t.Run("kapi sed -i.bak edits in place via proxy", func(t *testing.T) {
		p := writeToolboxFile(t, dir, "s.json", `{"x":"foo"}`)
		sed := findCmd(app.NewToolboxProxies(), "sed")
		sed.SetArgs([]string{"-i.bak", "s/foo/bar/", p})
		require.NoError(t, sed.Execute())
		edited, _ := os.ReadFile(p)
		assert.Contains(t, string(edited), "bar")
		backup, err := os.ReadFile(p + ".bak")
		require.NoError(t, err)
		assert.Contains(t, string(backup), "foo")
	})
}

func isAlphaWord(s string) bool {
	if len(s) < 3 {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}
