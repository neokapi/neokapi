package cli

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestZip(t *testing.T, dir string, entries map[string][]byte) string {
	t.Helper()
	path := filepath.Join(dir, "bundle.zip")
	f, err := os.Create(path)
	require.NoError(t, err)
	zw := zip.NewWriter(f)
	// Deterministic order.
	for _, name := range []string{"m.json", "r.md", "data.bin"} {
		data, ok := entries[name]
		if !ok {
			continue
		}
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())
	return path
}

func zipEntry(t *testing.T, path, name string) []byte {
	t.Helper()
	zr, err := zip.OpenReader(path)
	require.NoError(t, err)
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			require.NoError(t, err)
			defer rc.Close()
			b, err := io.ReadAll(rc)
			require.NoError(t, err)
			return b
		}
	}
	t.Fatalf("entry %q not found in %s", name, path)
	return nil
}

func TestStreamBlocksArchiveProvenance(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := writeTestZip(t, dir, map[string][]byte{
		"m.json": []byte(`{"a":"Hello"}`),
		"r.md":   []byte("# Title\n"),
	})

	var labels []string
	_, err := app.streamBlocks(context.Background(), path, func(_ int, b *model.Block) error {
		labels = append(labels, entryLabel(displayName(path), b))
		return nil
	})
	require.NoError(t, err)
	assert.Contains(t, labels, path+"!m.json")
	assert.Contains(t, labels, path+"!r.md")
}

func TestStreamBlocksEntryLocator(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := writeTestZip(t, dir, map[string][]byte{
		"m.json": []byte(`{"a":"Hello","b":"World"}`),
		"r.md":   []byte("# Title\n"),
	})

	var texts []string
	fmtName, err := app.streamBlocks(context.Background(), path+"!m.json", func(_ int, b *model.Block) error {
		texts = append(texts, b.SourceText())
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, "json", fmtName)
	assert.ElementsMatch(t, []string{"Hello", "World"}, texts)
}

func TestEditArchiveAllInPlace(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	bin := []byte("\x00\x01world-bytes")
	path := writeTestZip(t, dir, map[string][]byte{
		"m.json": []byte(`{"a":"Hello world"}`),
		"r.md":   []byte("# Title world\n"),
		"data.bin": bin,
	})

	prog, err := parseSedProgram([]string{"s/world/MOON/g"})
	require.NoError(t, err)
	tool := newSedTool(prog, "", true)

	require.NoError(t, app.editDocument(context.Background(), path, tool, "", true, "", nil))

	assert.Contains(t, string(zipEntry(t, path, "m.json")), "Hello MOON")
	assert.Contains(t, string(zipEntry(t, path, "r.md")), "Title MOON")
	// Binary entry is copied byte-for-byte — its "world" bytes are NOT edited.
	assert.Equal(t, bin, zipEntry(t, path, "data.bin"))
}

func TestEditArchiveEntryInPlaceLeavesOthers(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := writeTestZip(t, dir, map[string][]byte{
		"m.json": []byte(`{"a":"keep world"}`),
		"r.md":   []byte("# Title world\n"),
	})

	prog, err := parseSedProgram([]string{"s/world/STAR/g"})
	require.NoError(t, err)
	tool := newSedTool(prog, "", true)

	require.NoError(t, app.editDocument(context.Background(), path+"!r.md", tool, "", true, "", nil))

	assert.Contains(t, string(zipEntry(t, path, "r.md")), "Title STAR")
	// The other entry is untouched.
	assert.Contains(t, string(zipEntry(t, path, "m.json")), "keep world")
}

func TestEditArchiveAllToStdout(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := writeTestZip(t, dir, map[string][]byte{
		"m.json": []byte(`{"a":"Hello world"}`),
	})

	prog, err := parseSedProgram([]string{"s/world/SUN/g"})
	require.NoError(t, err)
	tool := newSedTool(prog, "", true)

	var buf bytes.Buffer
	require.NoError(t, app.editDocument(context.Background(), path, tool, "", false, "", &buf))

	// Output is a valid repacked archive with the edit applied.
	out := filepath.Join(dir, "out.zip")
	require.NoError(t, os.WriteFile(out, buf.Bytes(), 0o644))
	assert.Contains(t, string(zipEntry(t, out, "m.json")), "Hello SUN")
	// Source archive untouched (stdout mode).
	assert.Contains(t, string(zipEntry(t, path, "m.json")), "Hello world")
}
