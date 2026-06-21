package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// upperChatProvider returns a mock LLM provider whose Chat uppercases the user
// text — a deterministic stand-in for a faithful rewrite.
func upperChatProvider() *aiprovider.MockProvider {
	p := aiprovider.NewMockProvider()
	p.ChatFunc = func(_ context.Context, msgs []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		last := ""
		for i := len(msgs) - 1; i >= 0; i-- {
			if msgs[i].Role == "user" {
				last = msgs[i].Text()
				break
			}
		}
		return &aiprovider.ChatResponse{Content: strings.ToUpper(last), Model: "mock"}, nil
	}
	return p
}

// replaceChatProvider returns a mock provider whose Chat replaces old→new in the
// user text, leaving placeholder tags untouched.
func replaceChatProvider(old, repl string) *aiprovider.MockProvider {
	p := aiprovider.NewMockProvider()
	p.ChatFunc = func(_ context.Context, msgs []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		last := ""
		for i := len(msgs) - 1; i >= 0; i-- {
			if msgs[i].Role == "user" {
				last = msgs[i].Text()
				break
			}
		}
		return &aiprovider.ChatResponse{Content: strings.ReplaceAll(last, old, repl), Model: "mock"}, nil
	}
	return p
}

// TestRewriteFaithfulJSONRoundTrip proves the moat over a real JSON reader/
// writer: only the editable values change; the keys, structure, and byte
// formatting of the document are preserved exactly.
func TestRewriteFaithfulJSONRoundTrip(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "en.json")
	const src = `{"greeting":"Hello world","note":"Keep me"}`
	require.NoError(t, os.WriteFile(path, []byte(src), 0o644))

	tl := aitools.NewRewriteTool(upperChatProvider(), aitools.RewriteConfig{Instruction: "shout"})

	var buf bytes.Buffer
	require.NoError(t, app.editDocument(context.Background(), path, tl, "", false, "", &buf))

	// The skeleton (keys, braces, separators) is byte-identical; only the values
	// changed — exactly what "edit the content inside the file" must guarantee.
	assert.Equal(t, `{"greeting":"HELLO WORLD","note":"KEEP ME"}`, buf.String())
}

// TestRewriteFaithfulHTMLInlineCode proves an inline code (a bold span) survives
// the rewrite end-to-end: the span still wraps the rewritten word.
func TestRewriteFaithfulHTMLInlineCode(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "page.html")
	const html = `<!DOCTYPE html><html><body><p>Hello <b>world</b></p></body></html>`
	require.NoError(t, os.WriteFile(path, []byte(html), 0o644))

	tl := aitools.NewRewriteTool(replaceChatProvider("world", "planet"), aitools.RewriteConfig{Instruction: "swap"})

	var buf bytes.Buffer
	require.NoError(t, app.editDocument(context.Background(), path, tl, "", false, "", &buf))
	out := buf.String()
	assert.Contains(t, out, "<b>planet</b>", "the bold span must still wrap the rewritten word")
	assert.NotContains(t, out, "world")
}

// TestRewriteInPlaceWritesBackup proves in-place editing rewrites the file and
// keeps a backup when a suffix is given.
func TestRewriteInPlaceWritesBackup(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "en.json")
	const src = `{"greeting":"Hello world"}`
	require.NoError(t, os.WriteFile(path, []byte(src), 0o644))

	tl := aitools.NewRewriteTool(upperChatProvider(), aitools.RewriteConfig{Instruction: "shout"})
	require.NoError(t, app.runRewrite(context.Background(), []string{path}, tl, true, ".bak"))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, `{"greeting":"HELLO WORLD"}`, string(got))

	backup, err := os.ReadFile(path + ".bak")
	require.NoError(t, err)
	assert.Equal(t, src, string(backup), "the backup must hold the original bytes")
}

// TestRewriteDiffPrintsAndWritesNothing proves --diff renders a unified before/
// after of the changed blocks and leaves the file on disk untouched.
func TestRewriteDiffPrintsAndWritesNothing(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "en.json")
	const src = `{"greeting":"Hello world","note":"Keep me"}`
	require.NoError(t, os.WriteFile(path, []byte(src), 0o644))

	tl := aitools.NewRewriteTool(upperChatProvider(), aitools.RewriteConfig{Instruction: "shout"})

	var buf bytes.Buffer
	require.NoError(t, app.runRewriteDiff(context.Background(), []string{path}, tl, &buf))

	out := buf.String()
	// A unified diff of every changed block: the original on '-', the rewrite on '+'.
	assert.Contains(t, out, "-Hello world")
	assert.Contains(t, out, "+HELLO WORLD")
	assert.Contains(t, out, "-Keep me")
	assert.Contains(t, out, "+KEEP ME")

	// The file on disk is unchanged — --diff is a pure dry run.
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, src, string(got), "--diff must not modify the file")
}
