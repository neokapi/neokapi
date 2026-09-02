package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConvOutputDirWritesOnePerInput is the headline batch conversion:
// `kconv <many files> --to md -o out/` writes one file per input, named after
// the input and re-extensioned for the target format. The directory is created
// if it does not exist, so the trailing slash is all the user has to type.
func TestConvOutputDirWritesOnePerInput(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	a := writeToolboxFile(t, dir, "a.md", "# A\n")
	b := writeToolboxFile(t, dir, "b.md", "# B\n")
	out := filepath.Join(dir, "converted") + string(os.PathSeparator)

	require.NoError(t, app.RunConv(context.Background(), []string{a, b}, ConvOptions{To: "html", Output: out}))

	for name, want := range map[string]string{"a.html": "<h1>A</h1>", "b.html": "<h1>B</h1>"} {
		got, err := os.ReadFile(filepath.Join(dir, "converted", name))
		require.NoError(t, err, "%s should have been written", name)
		assert.Contains(t, string(got), want)
	}
}

// TestConvOutputDirAcceptsExistingDirectory: a directory that already exists is
// recognised without the trailing slash, the way `cp` and `mv` treat one.
func TestConvOutputDirAcceptsExistingDirectory(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	src := writeToolboxFile(t, dir, "notes.md", "# Notes\n")
	out := filepath.Join(dir, "site")
	require.NoError(t, os.Mkdir(out, 0o755))

	require.NoError(t, app.RunConv(context.Background(), []string{src}, ConvOptions{To: "html", Output: out}))
	assert.FileExists(t, filepath.Join(out, "notes.html"))
}

// TestConvRecursiveMirrorsTree: with -r a directory argument is walked, and the
// output directory mirrors the input's sub-tree. Flattening instead would make
// docs/a/intro.md and docs/b/intro.md fight over one output name.
func TestConvRecursiveMirrorsTree(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	require.NoError(t, os.MkdirAll(filepath.Join(docs, "guide"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(docs, "ref"), 0o755))
	writeToolboxFile(t, docs, "index.md", "# Index\n")
	writeToolboxFile(t, filepath.Join(docs, "guide"), "intro.md", "# Guide intro\n")
	writeToolboxFile(t, filepath.Join(docs, "ref"), "intro.md", "# Ref intro\n")
	out := filepath.Join(dir, "site")

	require.NoError(t, app.RunConv(context.Background(), []string{docs},
		ConvOptions{To: "html", Output: out + string(os.PathSeparator), Recursive: true}))

	assert.FileExists(t, filepath.Join(out, "index.html"))
	guide, err := os.ReadFile(filepath.Join(out, "guide", "intro.html"))
	require.NoError(t, err)
	assert.Contains(t, string(guide), "<h1>Guide intro</h1>")
	ref, err := os.ReadFile(filepath.Join(out, "ref", "intro.html"))
	require.NoError(t, err, "same-named files in sibling directories must not collide")
	assert.Contains(t, string(ref), "<h1>Ref intro</h1>")
}

// TestConvOutputDirRejectsCollision: two inputs whose stems collide would leave
// one silently overwritten by the other. The second is reported and skipped —
// the first is still converted, and the run exits 2.
func TestConvOutputDirRejectsCollision(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	md := writeToolboxFile(t, dir, "report.md", "# From Markdown\n")
	html := writeToolboxFile(t, dir, "report.html", "<h1>From HTML</h1>")
	out := filepath.Join(dir, "out") + string(os.PathSeparator)

	stderr, err := captureStderr(t, func() error {
		return app.RunConv(context.Background(), []string{md, html}, ConvOptions{To: "doclang", Output: out})
	})
	require.Error(t, err)
	assert.Equal(t, ExitUsage, ExitCode(nil, err))
	assert.Contains(t, stderr, "would overwrite")

	got, rerr := os.ReadFile(filepath.Join(dir, "out", "report.dclg.xml"))
	require.NoError(t, rerr)
	assert.Contains(t, string(got), "From Markdown", "the first input still converts; only the colliding one is skipped")
}

// TestConvOutputDirRefusesToOverwriteInput: `kconv notes.md --to md -o .` would
// resolve to the input itself. Converting a file onto itself truncates it
// before the reader has finished, so it is refused, not attempted.
func TestConvOutputDirRefusesToOverwriteInput(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	src := writeToolboxFile(t, dir, "notes.md", "# Notes\n")

	stderr, err := captureStderr(t, func() error {
		return app.RunConv(context.Background(), []string{src},
			ConvOptions{To: "markdown", Output: dir + string(os.PathSeparator)})
	})
	require.Error(t, err)
	assert.Contains(t, stderr, "would overwrite the input")

	got, rerr := os.ReadFile(src)
	require.NoError(t, rerr)
	assert.Equal(t, "# Notes\n", string(got), "the input is untouched")
}

// TestConvOutputDirRejectsStdin: standard input has no name to derive an output
// filename from.
func TestConvOutputDirRejectsStdin(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	stderr, err := captureStderr(t, func() error {
		return app.RunConv(context.Background(), []string{StdinName},
			ConvOptions{To: "html", Output: filepath.Join(dir, "out") + string(os.PathSeparator)})
	})
	require.Error(t, err)
	assert.Contains(t, stderr, "standard input has no filename")
}

// TestConvSingleOutputFileStillRejectsMultipleInputs: -o that names a file is
// unchanged — but the message now points at the directory form.
func TestConvSingleOutputFileStillRejectsMultipleInputs(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	a := writeToolboxFile(t, dir, "a.md", "# A\n")
	b := writeToolboxFile(t, dir, "b.md", "# B\n")

	err := app.RunConv(context.Background(), []string{a, b},
		ConvOptions{To: "html", Output: filepath.Join(dir, "out.html")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Pass a directory")
}

// TestResolveTargetFormatFromOutputDir: an output directory has no extension to
// infer a target format from, and must say so in those terms.
func TestResolveTargetFormatFromOutputDir(t *testing.T) {
	app := newToolboxApp(t)
	_, err := app.ResolveTargetFormat("", "converted/")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "output directory")
}

// TestCommonDir covers the rule that decides how much of the input tree the
// output mirrors, over the argument shapes the shell and kapi both produce.
func TestCommonDir(t *testing.T) {
	sep := string(filepath.Separator)
	tests := []struct {
		name  string
		files []string
		want  string
	}{
		{name: "flat glob in one directory", files: []string{"docs/a.md", "docs/b.md"}, want: "docs"},
		{name: "nested tree", files: []string{"docs/a.md", "docs/guide/b.md"}, want: "docs"},
		{name: "sibling trees", files: []string{"docs/a.md", "src/b.md"}, want: "."},
		{name: "cwd files", files: []string{"a.md", "b.md"}, want: "."},
		{name: "stdin is ignored", files: []string{StdinName, "docs/a.md"}, want: "docs"},
		{name: "absolute", files: []string{sep + "tmp" + sep + "x" + sep + "a.md", sep + "tmp" + sep + "x" + sep + "b.md"},
			want: sep + "tmp" + sep + "x"},
		{name: "no shared root", files: []string{sep + "tmp" + sep + "a.md", "docs/b.md"}, want: ""},
		{name: "nothing", files: nil, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, commonDir(tc.files))
		})
	}
}
