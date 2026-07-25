package host

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1468 item 1: five (in fact seven) sites opened a skeleton store with
// `if store, err := format.NewSkeletonStore(); err == nil` and, on failure,
// simply ran on without one. The skeleton is what preserves the structure the
// reader could not model, so running on turns a byte-exact same-format pass into
// a re-serialization from the content model — and the command still reports
// success. host/toolbox.go's EditDocument is the worst of them because it
// rewrites the file IN PLACE: the original bytes are gone.
//
// Failure is induced by a real filesystem condition — TMPDIR pointed at a
// directory that does not exist, which is what os.CreateTemp consults — never by
// a production hook or a test-only branch.

// yamlWithFormattingOnlyASkeletonPreserves is a document whose comments, quoting
// style, blank lines and indentation exist only in the source bytes. The content
// model does not carry them, so a writer running without the skeleton produces a
// different file: that difference is exactly what the swallowed error hid.
const yamlWithFormattingOnlyASkeletonPreserves = `# Release notes shown in the app
greeting:    "Hello world"          # keep this trailing comment

nested:
      deep:   'single quoted'
`

// isolateKapiEnv applies the repo's dogfood isolation contract so an in-process
// App can never bind to the repository's own kapi.yaml, the developer's config,
// or Homebrew-installed plugins. Call it before breakTempDir — the throwaway
// directories it creates need a working TMPDIR.
func isolateKapiEnv(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	for _, kv := range [][2]string{
		{"KAPI_NO_PROJECT", "1"},
		{"KAPI_CONFIG_DIR", filepath.Join(base, "config")},
		{"XDG_DATA_HOME", filepath.Join(base, "data")},
		{"XDG_CACHE_HOME", filepath.Join(base, "cache")},
		{"KAPI_PLUGINS_DIR_ONLY", "1"},
		{"KAPI_PLUGINS_DIR", filepath.Join(base, "plugins")},
	} {
		t.Setenv(kv[0], kv[1])
	}
}

// breakTempDir points TMPDIR at a path that does not exist, so os.CreateTemp("")
// fails the way a missing, read-only or full TMPDIR makes it fail in the field.
// Everything that needs a real temp directory must be created before this call.
func breakTempDir(t *testing.T) string {
	t.Helper()
	missing := filepath.Join(t.TempDir(), "no-such-tmpdir")
	t.Setenv("TMPDIR", missing)
	t.Setenv("TMP", missing)
	t.Setenv("TEMP", missing)
	require.Equal(t, missing, os.TempDir(), "sanity: os.TempDir must follow the swapped TMPDIR")
	return missing
}

// noopTool passes every part through untouched, so the only thing under test is
// whether the document survives the reader→writer round-trip intact.
func noopTool() *tool.BaseTool { return &tool.BaseTool{} }

// ─── host/toolbox.go: EditDocument (in place) ────────────────────────────────

// TestEditDocument_InPlaceRoundTripIsByteIdentical is the control for the test
// below and the property the swallowed error destroyed: with the skeleton store
// available, an in-place edit that changes nothing leaves the file byte-for-byte
// as it was — comments, quoting and indentation included.
func TestEditDocument_InPlaceRoundTripIsByteIdentical(t *testing.T) {
	isolateKapiEnv(t)
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "messages.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yamlWithFormattingOnlyASkeletonPreserves), 0o644))

	require.NoError(t, app.EditDocument(context.Background(), path, noopTool(), "", true, "", nil))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, yamlWithFormattingOnlyASkeletonPreserves, string(got),
		"a same-format in-place edit round-trips through the skeleton byte-for-byte")
}

// TestEditDocument_InPlaceFailsWhenTheSkeletonStoreCannotBeCreated is the core
// regression. The skeleton could not be created, so the writer would have
// rewritten the file from the content model — losing the comments and the exact
// layout — and returned nil. The user sees a `kapi ksed -i` / `kapi apply` that
// exits 0 while their document has been silently reformatted, with no copy of
// the original left anywhere.
func TestEditDocument_InPlaceFailsWhenTheSkeletonStoreCannotBeCreated(t *testing.T) {
	isolateKapiEnv(t)
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "messages.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yamlWithFormattingOnlyASkeletonPreserves), 0o644))

	breakTempDir(t)

	err := app.EditDocument(context.Background(), path, noopTool(), "", true, "", nil)
	require.Error(t, err, "an in-place edit that cannot preserve the document must fail, not reconstruct it")
	assert.Contains(t, err.Error(), "messages.yaml", "the error must name the file it refused to rewrite")
	assert.Contains(t, err.Error(), "formatting", "the error must say what would have been lost")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, yamlWithFormattingOnlyASkeletonPreserves, string(got),
		"and the file on disk must be untouched — a refused edit leaves no degraded artefact")
}

// ─── host/toolbox_conv.go: convertDocument ───────────────────────────────────

// markdownWithFormattingOnlyASkeletonPreserves uses a setext heading, a
// reference link and irregular spacing — none of which survive a re-serialization
// from the content model.
const markdownWithFormattingOnlyASkeletonPreserves = `Release notes
=============

Some  *emphasis*   and a [reference][1].

[1]: https://example.com
`

// TestConvertDocument_SameFormatFailsWhenTheSkeletonStoreCannotBeCreated:
// `kapi convert doc.md --to markdown -o copy.md` is a faithful round-trip.
// Without the skeleton it silently became a re-serialization, so the copy
// differed from its source while the command reported success.
func TestConvertDocument_SameFormatFailsWhenTheSkeletonStoreCannotBeCreated(t *testing.T) {
	isolateKapiEnv(t)
	app := newToolboxApp(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "notes.md")
	out := filepath.Join(dir, "copy.md")
	require.NoError(t, os.WriteFile(src, []byte(markdownWithFormattingOnlyASkeletonPreserves), 0o644))

	breakTempDir(t)

	err := app.convertDocument(context.Background(), src, registry.FormatID("markdown"), "", out)
	require.Error(t, err, "a same-format convert that cannot preserve the source must fail")
	assert.Contains(t, err.Error(), "notes.md")
	assert.NoFileExists(t, out, "and it must not leave a degraded copy behind")
}

// TestConvertDocument_CrossFormatSucceedsWithoutASkeleton is the discriminating
// control: absent and present-but-unobtainable are different facts. A
// cross-format conversion reconstructs the target from the content model **by
// design** — it never wires a skeleton — so it must keep working with the very
// same broken TMPDIR that fails the same-format case above. "Fail whenever
// TMPDIR is broken" is therefore not an acceptable substitute for the fix.
func TestConvertDocument_CrossFormatSucceedsWithoutASkeleton(t *testing.T) {
	isolateKapiEnv(t)
	app := newToolboxApp(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "notes.md")
	out := filepath.Join(dir, "notes.json")
	require.NoError(t, os.WriteFile(src, []byte(markdownWithFormattingOnlyASkeletonPreserves), 0o644))

	breakTempDir(t)

	require.NoError(t, app.convertDocument(context.Background(), src, registry.FormatID("json"), "", out),
		"a cross-format conversion needs no skeleton, so a broken TMPDIR must not fail it")
	got, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Contains(t, string(got), "emphasis")
}

// TestConvertDocument_SameFormatIsByteIdentical is the control: on a working
// temp filesystem the same call reproduces the source exactly.
func TestConvertDocument_SameFormatIsByteIdentical(t *testing.T) {
	isolateKapiEnv(t)
	app := newToolboxApp(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "notes.md")
	out := filepath.Join(dir, "copy.md")
	require.NoError(t, os.WriteFile(src, []byte(markdownWithFormattingOnlyASkeletonPreserves), 0o644))

	require.NoError(t, app.convertDocument(context.Background(), src, registry.FormatID("markdown"), "", out))

	got, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, markdownWithFormattingOnlyASkeletonPreserves, string(got))
}

// ─── host/toolbox_archive.go: editBytes (one archive member) ─────────────────

// TestEditArchiveEntry_FailsWhenTheSkeletonStoreCannotBeCreated: a repack that
// silently substitutes a reconstruction of the member for the member is worse
// than a single-file edit, because the archive is rewritten around it.
func TestEditArchiveEntry_FailsWhenTheSkeletonStoreCannotBeCreated(t *testing.T) {
	isolateKapiEnv(t)
	app := newToolboxApp(t)
	dir := t.TempDir()
	archive := writeZipFixture(t, dir, "messages.yaml", yamlWithFormattingOnlyASkeletonPreserves)
	before, err := os.ReadFile(archive)
	require.NoError(t, err)

	breakTempDir(t)

	err = app.EditDocument(context.Background(), archive+"!messages.yaml", noopTool(), "", true, "", nil)
	require.Error(t, err, "an archive member that cannot be preserved must fail the edit")
	assert.Contains(t, err.Error(), "messages.yaml")

	after, rerr := os.ReadFile(archive)
	require.NoError(t, rerr)
	assert.Equal(t, before, after, "and the archive must not have been repacked")
}

// TestEditArchiveEntry_RoundTripIsByteIdentical is the control: with a working
// temp filesystem the member survives the repack byte-for-byte.
func TestEditArchiveEntry_RoundTripIsByteIdentical(t *testing.T) {
	isolateKapiEnv(t)
	app := newToolboxApp(t)
	dir := t.TempDir()
	archive := writeZipFixture(t, dir, "messages.yaml", yamlWithFormattingOnlyASkeletonPreserves)

	require.NoError(t, app.EditDocument(context.Background(), archive+"!messages.yaml", noopTool(), "", true, "", nil))

	assert.Equal(t, yamlWithFormattingOnlyASkeletonPreserves, readZipEntry(t, archive, "messages.yaml"))
}

// ─── host/toolrun.go: processOneFile (the `kapi <tool>` file runner) ─────────

// TestRunToolOnFiles_FailsWhenTheSkeletonStoreCannotBeCreated: this is the path
// behind every top-level tool command (`kapi pseudo-translate notes.md -o …`).
// Its swallow shipped an output file reconstructed from the content model while
// the run reported the file written.
func TestRunToolOnFiles_FailsWhenTheSkeletonStoreCannotBeCreated(t *testing.T) {
	isolateKapiEnv(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "notes.md")
	out := filepath.Join(dir, "out.md")
	require.NoError(t, os.WriteFile(src, []byte(markdownWithFormattingOnlyASkeletonPreserves), 0o644))

	a := &App{SourceLang: "en"}
	a.InitRegistries()

	breakTempDir(t)

	err := a.RunToolOnFiles(context.Background(), ToolRunConfig{
		ToolName:       "noop",
		Files:          []string{src},
		OutputTemplate: out,
		NewTool:        func() (tool.Tool, error) { return &tool.BaseTool{ToolName: "noop"}, nil },
	})
	require.Error(t, err, "a tool run that cannot preserve the document must fail, not write a reconstruction")
	assert.Contains(t, err.Error(), "notes.md")
	assert.NoFileExists(t, out, "and no degraded output may be left behind")
}

// TestRunToolOnFiles_RoundTripIsByteIdentical is the control: with a working
// temp filesystem the same run reproduces the source exactly.
func TestRunToolOnFiles_RoundTripIsByteIdentical(t *testing.T) {
	isolateKapiEnv(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "notes.md")
	out := filepath.Join(dir, "out.md")
	require.NoError(t, os.WriteFile(src, []byte(markdownWithFormattingOnlyASkeletonPreserves), 0o644))

	a := &App{SourceLang: "en"}
	a.InitRegistries()

	require.NoError(t, a.RunToolOnFiles(context.Background(), ToolRunConfig{
		ToolName:       "noop",
		Files:          []string{src},
		OutputTemplate: out,
		NewTool:        func() (tool.Tool, error) { return &tool.BaseTool{ToolName: "noop"}, nil },
	}))

	got, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, markdownWithFormattingOnlyASkeletonPreserves, string(got))
}

func writeZipFixture(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, "bundle.zip")
	f, err := os.Create(p)
	require.NoError(t, err)
	zw := zip.NewWriter(f)
	w, err := zw.Create(name)
	require.NoError(t, err)
	_, err = w.Write([]byte(body))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())
	return p
}

func readZipEntry(t *testing.T, archive, name string) string {
	t.Helper()
	zr, err := zip.OpenReader(archive)
	require.NoError(t, err)
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, oerr := f.Open()
		require.NoError(t, oerr)
		defer rc.Close()
		var buf bytes.Buffer
		_, cerr := buf.ReadFrom(rc)
		require.NoError(t, cerr)
		return buf.String()
	}
	t.Fatalf("entry %q not found in %s", name, archive)
	return ""
}
