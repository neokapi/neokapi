package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveTargetFormat covers the --to / -o target-format resolution.
func TestResolveTargetFormat(t *testing.T) {
	app := newToolboxApp(t)
	tests := []struct {
		name    string
		to      string
		out     string
		want    registry.FormatID
		wantErr bool
	}{
		{name: "format id", to: "markdown", want: "markdown"},
		{name: "doclang id", to: "doclang", want: "doclang"},
		{name: "extension md", to: "md", want: "markdown"},
		{name: "dotted ext", to: ".html", want: "html"},
		{name: "from -o extension", out: "/tmp/x.md", want: "markdown"},
		{name: "from -o html", out: "/tmp/x.html", want: "html"},
		{name: "compound .dclg.xml", out: "/tmp/x.dclg.xml", want: "doclang"},
		{name: "unknown --to", to: "nope", wantErr: true},
		{name: "neither", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := app.resolveTargetFormat(tc.to, tc.out)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestConvDocLangToMarkdown: the headline cross-format projection — DocLang →
// clean Markdown, roles → structure, inline from run type.
func TestConvDocLangToMarkdown(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	src := `<?xml version="1.0"?>
<doclang xmlns="https://www.doclang.ai/ns/v0" version="0.6">
  <heading level="2">Overview</heading>
  <text>Delivered <bold>steady growth</bold>.</text>
  <list class="unordered"><ldiv><marker>-</marker></ldiv><text>First</text><ldiv><marker>-</marker></ldiv><text>Second</text></list>
</doclang>`
	path := writeToolboxFile(t, dir, "report.dclg.xml", src)

	out, err := captureStdout(t, func() error {
		return app.runConv(context.Background(), []string{path}, "markdown", "", "")
	})
	require.NoError(t, err)
	assert.Contains(t, out, "## Overview")
	assert.Contains(t, out, "Delivered **steady growth**.")
	assert.Contains(t, out, "- First")
	assert.Contains(t, out, "- Second")
	assert.NotContains(t, out, "<heading") // no source markup leaks
	assert.NotContains(t, out, "<bold>")
}

// TestConvDocLangToHTML: the table reconstructs as a real <table>.
func TestConvDocLangToHTML(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	src := `<?xml version="1.0"?>
<doclang xmlns="https://www.doclang.ai/ns/v0" version="0.6">
  <heading level="1">Title</heading>
  <table><ched/>Region<ched/>Sales<nl/><fcel/>EU<fcel/>100<nl/></table>
</doclang>`
	path := writeToolboxFile(t, dir, "t.dclg.xml", src)

	out, err := captureStdout(t, func() error {
		return app.runConv(context.Background(), []string{path}, "html", "", "")
	})
	require.NoError(t, err)
	// The export is a complete, standalone HTML document, not a bare fragment.
	assert.Contains(t, out, "<!DOCTYPE html>")
	assert.Contains(t, out, "<html")
	assert.Contains(t, out, "<title>Title</title>")
	assert.Contains(t, out, "<body>")
	assert.Contains(t, out, "</html>")
	assert.Contains(t, out, "<h1>Title</h1>")
	assert.Contains(t, out, "<table>")
	assert.Contains(t, out, "<th>Region</th>")
	assert.Contains(t, out, "<td>EU</td>")
}

// TestConvToDocLang: a Markdown source projects to DocLang (heading role +
// inline bold → DocLang elements).
func TestConvMarkdownToDocLang(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := writeToolboxFile(t, dir, "in.md", "# Title\n\nSome **bold** text.\n")

	out, err := captureStdout(t, func() error {
		return app.runConv(context.Background(), []string{path}, "doclang", "", "")
	})
	require.NoError(t, err)
	assert.Contains(t, out, `<doclang xmlns="https://www.doclang.ai/ns/v0"`)
	assert.Contains(t, out, `<heading level="1">Title</heading>`)
	assert.Contains(t, out, "<bold>bold</bold>")
}

// TestConvMarkdownCodeToDocLang: a fenced code block projects to <code> with the
// recommended Linguist language <label> (the producer-path fix), proving the
// structure layer carries code role + language across formats.
func TestConvMarkdownCodeToDocLang(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := writeToolboxFile(t, dir, "in.md", "```go\nfmt.Println(\"hi\")\n```\n")

	out, err := captureStdout(t, func() error {
		return app.runConv(context.Background(), []string{path}, "doclang", "", "")
	})
	require.NoError(t, err)
	assert.Contains(t, out, "<code>")
	assert.Contains(t, out, `<label value="go"/>`)
}

// TestConvToSkeletonTargetFails: a skeleton-driven target (openxml) cannot be
// generated from a foreign content model — it needs the original file. The
// Conversion Lab only offers generative targets; this guards the error path.
func TestConvToSkeletonTargetFails(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := writeToolboxFile(t, dir, "in.md", "# Title\n")
	err := app.runConv(context.Background(), []string{path}, "openxml", "", filepath.Join(dir, "out.docx"))
	require.Error(t, err, "expected openxml (skeleton-driven) to reject generation from markdown")
}

// TestConvToFile: -o writes the converted document to a file, format inferred
// from the extension.
func TestConvToFile(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	src := writeToolboxFile(t, dir, "report.dclg.xml",
		`<doclang xmlns="https://www.doclang.ai/ns/v0" version="0.6"><heading level="1">Hi</heading></doclang>`)
	outPath := filepath.Join(dir, "out.html")

	err := app.runConv(context.Background(), []string{src}, "html", "", outPath)
	require.NoError(t, err)
	got, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Contains(t, string(got), "<h1>Hi</h1>")
}

// TestConvOutputRejectsMultipleInputs: -o is single-input.
func TestConvOutputRejectsMultipleInputs(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	a := writeToolboxFile(t, dir, "a.md", "# A\n")
	b := writeToolboxFile(t, dir, "b.md", "# B\n")
	err := app.runConv(context.Background(), []string{a, b}, "html", "", filepath.Join(dir, "out.html"))
	require.Error(t, err)
}
