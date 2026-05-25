package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSedCmd(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		in      string
		want    string
		wantErr bool
		global  bool
	}{
		{name: "basic first", script: "s/a/b/", in: "aaa", want: "baa"},
		{name: "global", script: "s/a/b/g", in: "aaa", want: "bbb", global: true},
		{name: "ignore case", script: "s/HELLO/hi/gi", in: "hello HELLO", want: "hi hi", global: true},
		{name: "alt delimiter", script: "s|/usr|/opt|", in: "/usr/bin", want: "/opt/bin"},
		{name: "backref numbered", script: `s/(\w+),(\w+)/\2-\1/`, in: "foo,bar", want: "bar-foo"},
		{name: "ampersand whole match", script: `s/cat/[&]/g`, in: "cat cat", want: "[cat] [cat]", global: true},
		{name: "escaped ampersand literal", script: `s/x/a\&b/`, in: "x", want: "a&b"},
		{name: "escaped delimiter in pattern", script: `s/a\/b/Z/`, in: "a/b", want: "Z"},
		{name: "not substitution", script: "y/a/b/", wantErr: true},
		{name: "unterminated", script: "s/a", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := parseSedCmd(tc.script)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.global, c.global)
			got := sedProgram{c}.apply(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestSedProgramMultiple(t *testing.T) {
	prog, err := parseSedProgram([]string{"s/colour/color/g", "s/behaviour/behavior/g"})
	require.NoError(t, err)
	assert.Equal(t, "color and behavior", prog.apply("colour and behaviour"))
}

func TestSedReplToGo(t *testing.T) {
	assert.Equal(t, "${1}-${2}", sedReplToGo(`\1-\2`, '/'))
	assert.Equal(t, "${0}", sedReplToGo(`&`, '/'))
	assert.Equal(t, "literal $$ sign", sedReplToGo(`literal $ sign`, '/'))
	assert.Equal(t, "tab\there", sedReplToGo(`tab\there`, '/'))
}

func TestNormalizeSedInPlaceArgs(t *testing.T) {
	tests := []struct {
		in   []string
		want []string
	}{
		{[]string{"-i.bak", "s/a/b/", "f.txt"}, []string{"--in-place=.bak", "s/a/b/", "f.txt"}},
		{[]string{"-i", "s/a/b/"}, []string{"-i", "s/a/b/"}},
		{[]string{"-i=.bak", "x"}, []string{"-i=.bak", "x"}},
		{[]string{"--in-place=.orig", "x"}, []string{"--in-place=.orig", "x"}},
		{[]string{"-e", "s/a/b/", "f"}, []string{"-e", "s/a/b/", "f"}},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, NormalizeSedInPlaceArgs(tc.in))
	}
}

func TestMatcher(t *testing.T) {
	t.Run("any pattern matches", func(t *testing.T) {
		m, err := newMatcher([]string{"cat", "dog"}, matcherOpts{})
		require.NoError(t, err)
		assert.True(t, m.test("I have a dog"))
		assert.True(t, m.test("the cat sat"))
		assert.False(t, m.test("a fish"))
	})
	t.Run("ignore case", func(t *testing.T) {
		m, _ := newMatcher([]string{"hello"}, matcherOpts{ignoreCase: true})
		assert.True(t, m.test("HELLO THERE"))
	})
	t.Run("invert", func(t *testing.T) {
		m, _ := newMatcher([]string{"cat"}, matcherOpts{invert: true})
		assert.False(t, m.test("cat"))
		assert.True(t, m.test("dog"))
	})
	t.Run("fixed strings", func(t *testing.T) {
		m, _ := newMatcher([]string{"a.b"}, matcherOpts{fixedStrings: true})
		assert.True(t, m.test("x a.b y"))
		assert.False(t, m.test("axb")) // '.' is literal, not any-char
	})
	t.Run("word regexp", func(t *testing.T) {
		m, _ := newMatcher([]string{"cat"}, matcherOpts{wordRegexp: true})
		assert.True(t, m.test("the cat sat"))
		assert.False(t, m.test("category"))
	})
	t.Run("only matching", func(t *testing.T) {
		m, _ := newMatcher([]string{`\d+`}, matcherOpts{})
		assert.Equal(t, []string{"12", "34"}, m.findAll("a12b34"))
	})
	t.Run("invalid regexp", func(t *testing.T) {
		_, err := newMatcher([]string{"("}, matcherOpts{})
		require.Error(t, err)
	})
}

func TestHighlight(t *testing.T) {
	m, _ := newMatcher([]string{"cat"}, matcherOpts{})
	got := highlight("a cat and a cat", m.spans("a cat and a cat"))
	assert.Contains(t, got, colorMatch+"cat"+colorReset)
	// plain text outside matches preserved
	assert.Contains(t, got, "a ")
}

func TestMergeSpans(t *testing.T) {
	assert.Equal(t, [][]int{{0, 3}}, mergeSpans([][]int{{0, 2}, {1, 3}}))
	assert.Equal(t, [][]int{{0, 1}, {3, 4}}, mergeSpans([][]int{{3, 4}, {0, 1}}))
}

func TestBlockScopeText(t *testing.T) {
	b := model.NewBlock("b1", "hello")
	b.SourceLocale = "en"
	b.SetTargetText("fr", "bonjour")

	got, ok := blockScopeText(b, "")
	assert.True(t, ok)
	assert.Equal(t, "hello", got)

	got, ok = blockScopeText(b, "fr")
	assert.True(t, ok)
	assert.Equal(t, "bonjour", got)

	_, ok = blockScopeText(b, "de")
	assert.False(t, ok)

	empty := model.NewBlock("b2", "")
	_, ok = blockScopeText(empty, "")
	assert.False(t, ok)
}

// --- integration over the real reader/writer pipeline ---

func newToolboxApp(t *testing.T) *App {
	t.Helper()
	app := &App{SourceLang: "en", Encoding: "UTF-8"}
	app.InitRegistries()
	require.NotNil(t, app.FormatReg)
	return app
}

func TestStreamBlocksJSON(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "en.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"a":"Hello","b":"World"}`), 0o644))

	var texts []string
	fmtName, err := app.streamBlocks(context.Background(), path, func(_ int, b *model.Block) error {
		texts = append(texts, b.SourceText())
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, "json", fmtName)
	assert.ElementsMatch(t, []string{"Hello", "World"}, texts)
}

func TestEditDocumentSedJSON(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "en.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"a":"Hello world","b":"world peace"}`), 0o644))

	prog, err := parseSedProgram([]string{"s/world/EARTH/g"})
	require.NoError(t, err)
	tool := newSedTool(prog, "", true)

	var buf bytes.Buffer
	require.NoError(t, app.editDocument(context.Background(), path, tool, "", false, "", &buf))
	out := buf.String()
	assert.Contains(t, out, "Hello EARTH")
	assert.Contains(t, out, "EARTH peace")
	assert.NotContains(t, out, "world")

	// Source file must be untouched (stdout mode).
	orig, _ := os.ReadFile(path)
	assert.Contains(t, string(orig), "world")
}

func TestEditDocumentInPlaceWithBackup(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "en.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"a":"foo"}`), 0o644))

	prog, err := parseSedProgram([]string{"s/foo/bar/"})
	require.NoError(t, err)
	tool := newSedTool(prog, "", true)

	require.NoError(t, app.editDocument(context.Background(), path, tool, "", true, ".bak", nil))

	edited, _ := os.ReadFile(path)
	assert.Contains(t, string(edited), "bar")
	backup, err := os.ReadFile(path + ".bak")
	require.NoError(t, err)
	assert.Contains(t, string(backup), "foo")
}

// TestReadContentStdinCancel guards the Ctrl-C fix: cli.Run traps SIGINT and
// turns it into context cancellation rather than killing the process, so a
// blocking stdin read must observe the cancelled context and return promptly
// instead of hanging forever (the bug where `kcat` with no FILE swallowed
// Ctrl-C). We swap os.Stdin for a pipe whose write end stays open (no EOF), so
// io.ReadAll would block indefinitely without the context race.
func TestReadContentStdinCancel(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer w.Close() // keep the write end open: the read never sees EOF
	defer r.Close()

	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })

	ctx, cancel := context.WithCancel(context.Background())
	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		data, err := readContent(ctx, "-")
		done <- result{data, err}
	}()

	cancel()

	select {
	case got := <-done:
		require.ErrorIs(t, got.err, context.Canceled)
		assert.Nil(t, got.data)
	case <-time.After(2 * time.Second):
		t.Fatal("readContent did not return after context cancellation (would hang on Ctrl-C)")
	}
}

func TestReadContentFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0o644))

	// A file read ignores the context and returns the bytes; cancellation only
	// guards the blocking stdin path.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := readContent(ctx, path)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(got))
}

func TestBusyboxRoot(t *testing.T) {
	app := newToolboxApp(t)
	for _, name := range []string{"kgrep", "ksed", "kcat", "/usr/local/bin/kgrep", "kgrep.exe"} {
		root := BusyboxRoot(app, name)
		require.NotNil(t, root, "prog %q should map to a toolbox root", name)
	}
	assert.Nil(t, BusyboxRoot(app, "kapi"))
	assert.Nil(t, BusyboxRoot(app, "something-else"))
}
