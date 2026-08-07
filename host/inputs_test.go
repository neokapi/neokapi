package host

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withTerminalStdin makes the resolver believe stdin is an interactive
// terminal for the duration of the test. Under `go test` stdin is always
// redirected, so without this seam the exact condition behind the reported
// `kapi stats` hang can never be reproduced.
func withTerminalStdin(t *testing.T) {
	t.Helper()
	prev := stdinIsPipe
	stdinIsPipe = func() bool { return false }
	t.Cleanup(func() { stdinIsPipe = prev })
}

func withPipedStdin(t *testing.T) {
	t.Helper()
	prev := stdinIsPipe
	stdinIsPipe = func() bool { return true }
	t.Cleanup(func() { stdinIsPipe = prev })
}

func writeFile(t *testing.T, path, body string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

// envCmd builds a Command with captured IO for resolver tests.
func envCmd(t *testing.T) (*EnvCommand, *bytes.Buffer) {
	t.Helper()
	var stderr bytes.Buffer
	cmd := NewEnvCommand(context.Background(), "test")
	cmd.SetErr(&stderr)
	return cmd, &stderr
}

// TestResolveInputs_TerminalStdinNeverBlocks is the regression guard for the
// reported hang: `kapi stats` from a directory with no project, on a terminal,
// used to block on stdin with no indication. It must now report what it wants.
func TestResolveInputs_TerminalStdinNeverBlocks(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	t.Chdir(t.TempDir())
	withTerminalStdin(t)

	cmd, _ := envCmd(t)
	a := &App{}

	files, err := a.ResolveInputs(cmd, nil, InputOptions{Command: "kapi stats"})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNoInput)
	assert.Nil(t, files)
	assert.Equal(t, ExitUsage, ExitCode(cmd, err), "no input is a usage error")
	assert.Contains(t, err.Error(), "kapi stats")
	assert.Contains(t, err.Error(), "standard input", "the message must say how to feed it stdin")
}

// TestResolveInputs_PipedStdinIsRead keeps the Unix contract: a real pipe is
// read implicitly, because there the user did redirect something.
func TestResolveInputs_PipedStdinIsRead(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	t.Chdir(t.TempDir())
	withPipedStdin(t)

	cmd, _ := envCmd(t)
	files, err := (&App{}).ResolveInputs(cmd, nil, InputOptions{Command: "kapi stats"})
	require.NoError(t, err)
	assert.Equal(t, []string{StdinName}, files)
}

// TestResolveInputs_ProjectContentBeatsStdin asserts the project-aware default
// takes precedence even when stdin is a pipe: inside a project, "no arguments"
// means the project's content, not whatever happens to be on the stream.
func TestResolveInputs_ProjectContentBeatsStdin(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "src", "en.json"), `{"greeting":"Hello"}`)
	writeFile(t, filepath.Join(dir, "kapi.yaml"),
		"version: v1\nname: inputs\ndefaults:\n  source_language: en\ncollections:\n  - name: app\n    content:\n      - path: src/**/*.json\n")
	t.Chdir(dir)
	withPipedStdin(t)

	cmd, stderr := envCmd(t)
	files, err := (&App{}).ResolveInputs(cmd, nil, InputOptions{Command: "kapi stats"})
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Contains(t, files[0], "en.json")
	assert.Contains(t, stderr.String(), "tracked by", "the implied input must be announced, not silent")
}

// TestResolveInputs_QuietSuppressesTheNotice keeps --quiet honest: the implied
// input still applies, it is simply not narrated.
func TestResolveInputs_QuietSuppressesTheNotice(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "src", "en.json"), `{"greeting":"Hello"}`)
	writeFile(t, filepath.Join(dir, "kapi.yaml"),
		"version: v1\nname: inputs\ndefaults:\n  source_language: en\ncollections:\n  - name: app\n    content:\n      - path: src/**/*.json\n")
	t.Chdir(dir)

	cmd, stderr := envCmd(t)
	files, err := (&App{Quiet: true}).ResolveInputs(cmd, nil, InputOptions{Command: "kapi stats"})
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Empty(t, stderr.String())
}

// TestResolveInputs_EmptyProjectSaysSo: a recipe that tracks nothing must say
// so rather than falling through to stdin, which reads as a hang.
func TestResolveInputs_EmptyProjectSaysSo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "kapi.yaml"),
		"version: v1\nname: empty\ndefaults:\n  source_language: en\n")
	t.Chdir(dir)
	withPipedStdin(t)

	cmd, _ := envCmd(t)
	_, err := (&App{}).ResolveInputs(cmd, nil, InputOptions{Command: "kapi stats"})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNoInput)
	assert.Contains(t, err.Error(), "tracks no content")
}

func TestExpandArgs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.json"), `{}`)
	writeFile(t, filepath.Join(dir, "b.md"), `# b`)
	writeFile(t, filepath.Join(dir, "sub", "c.json"), `{}`)
	writeFile(t, filepath.Join(dir, "sub", "deep", "d.json"), `{}`)
	writeFile(t, filepath.Join(dir, "sub", "~$lock.docx"), `junk`)
	writeFile(t, filepath.Join(dir, ".hidden", "e.json"), `{}`)

	rel := func(paths []string) []string {
		out := make([]string, 0, len(paths))
		for _, p := range paths {
			r, err := filepath.Rel(dir, p)
			require.NoError(t, err)
			out = append(out, filepath.ToSlash(r))
		}
		return out
	}

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "recursive doublestar reaches every depth",
			args: []string{filepath.Join(dir, "**", "*.json")},
			want: []string{"a.json", "sub/c.json", "sub/deep/d.json"},
		},
		{
			name: "single-level star stays at one level",
			args: []string{filepath.Join(dir, "*.json")},
			want: []string{"a.json"},
		},
		{
			name: "alternation",
			args: []string{filepath.Join(dir, "*.{json,md}")},
			want: []string{"a.json", "b.md"},
		},
		{
			name: "directory expands to its files, skipping hidden dirs and junk",
			args: []string{filepath.Join(dir, "sub")},
			want: []string{"sub/c.json", "sub/deep/d.json"},
		},
		{
			name: "plain path passes through",
			args: []string{filepath.Join(dir, "a.json")},
			want: []string{"a.json"},
		},
		{
			name: "repeated matches are de-duplicated",
			args: []string{filepath.Join(dir, "a.json"), filepath.Join(dir, "*.json")},
			want: []string{"a.json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandArgs(tt.args, InputOptions{})
			require.NoError(t, err)
			assert.Equal(t, tt.want, rel(got))
		})
	}
}

// TestExpandArgs_UnmatchedPatternIsReported: a pattern that matches nothing is
// surfaced through OnSkip rather than silently contributing zero files.
func TestExpandArgs_UnmatchedPatternIsReported(t *testing.T) {
	dir := t.TempDir()
	var skipped []string
	got, err := expandArgs([]string{filepath.Join(dir, "*.nope")}, InputOptions{
		OnSkip: func(path string, _ error) { skipped = append(skipped, path) },
	})
	require.NoError(t, err)
	assert.Empty(t, got)
	require.Len(t, skipped, 1)
	assert.Contains(t, skipped[0], "*.nope")
}

// TestExpandArgs_LiteralBracketsResolveToTheFile: a real file whose name
// contains glob metacharacters must resolve to itself, not be treated as a
// pattern.
func TestExpandArgs_LiteralBracketsResolveToTheFile(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, filepath.Join(dir, "a[1].json"), `{}`)
	got, err := expandArgs([]string{p}, InputOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{p}, got)
}

// TestSplitGlobRoot pins the prefix split the doublestar expansion depends on.
func TestSplitGlobRoot(t *testing.T) {
	tests := []struct {
		pattern  string
		wantRoot string
		wantRel  string
	}{
		{"src/**/*.json", "src", "**/*.json"},
		{"**/*.md", ".", "**/*.md"},
		{"*.json", ".", "*.json"},
		{"a/b/c/*.txt", filepath.Join("a", "b", "c"), "*.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			root, rel := splitGlobRoot(tt.pattern)
			assert.Equal(t, tt.wantRoot, root)
			assert.Equal(t, tt.wantRel, rel)
		})
	}
}

// TestProgressBar pins the shared bar renderer used by both the progress
// reporter and the status pipeline column.
func TestProgressBar(t *testing.T) {
	assert.Equal(t, "░░░░░░░░░░", ProgressBar(0, 10, 10))
	assert.Equal(t, "█████░░░░░", ProgressBar(5, 10, 10))
	assert.Equal(t, "██████████", ProgressBar(10, 10, 10))
	assert.Equal(t, "██████████", ProgressBar(99, 10, 10), "over-complete clamps")
	assert.Equal(t, "░░░░░░░░░░", ProgressBar(1, 0, 10), "unknown total stays empty")
}

// TestProgressStaysSilentWhenFast: a run that finishes inside ProgressDelay
// must print nothing at all, so short commands keep their exact output.
func TestProgressStaysSilentWhenFast(t *testing.T) {
	cmd, stderr := envCmd(t)
	p := (&App{}).NewProgress(cmd, "reading", 3)
	p.Step("a")
	p.Advance()
	p.Done()
	assert.Empty(t, stderr.String())
}

// TestProgressQuietIsNoOp: --quiet suppresses progress entirely, and the nil
// reporter it yields must stay safe to call.
func TestProgressQuietIsNoOp(t *testing.T) {
	cmd, stderr := envCmd(t)
	p := (&App{Quiet: true}).NewProgress(cmd, "reading", 3)
	assert.Nil(t, p)
	p.Step("a")
	p.Advance()
	p.SetTotal(9)
	p.Done()
	assert.Empty(t, stderr.String())
}
