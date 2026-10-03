package host

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/change"
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
// success. An in-place edit is the worst of them, because the original bytes
// are gone: the change service's file home (core/change/filehome) refuses an
// edit it cannot write through the skeleton.
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
		// The lock files of an edit outside a project live in the data
		// directory, which a test binary otherwise puts under TMPDIR.
		{"KAPI_DATA_DIR", filepath.Join(base, "data-dir")},
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

// skeletonEditFrom and skeletonEditTo are the greeting's wording before and
// after the edit the change-service tests below make.
const (
	skeletonEditFrom = "Hello world"
	skeletonEditTo   = "Hello there"
)

// yamlGreetingEdit reads doc through a change service over dir, outside a
// project, and returns the service and the change set that rewrites the
// greeting from the revision the read saw.
func yamlGreetingEdit(t *testing.T, app *App, dir, doc string) (*change.Service, change.Set) {
	t.Helper()
	svc, err := app.ChangeService(context.Background(), ChangeServiceOptions{Root: dir, Origin: "test"})
	require.NoError(t, err)
	page, err := svc.Read(context.Background(), change.ReadRequest{Doc: doc})
	require.NoError(t, err)
	b := blockWith(t, page, skeletonEditFrom)
	return svc, change.Set{Ops: []change.Op{setTo(b.Ref, b.Rev, skeletonEditTo)}}
}

// refusedEdit applies set through svc, requires the service to refuse it, and
// returns the refusal's message: an edit the file home cannot write through
// the skeleton is refused, and nothing is written.
func refusedEdit(t *testing.T, svc *change.Service, set change.Set) string {
	t.Helper()
	res, err := svc.Apply(context.Background(), set, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetRefused, res.Status, "an edit that cannot preserve the document must be refused, not reconstructed: %+v", res.Ops)
	require.NotNil(t, res.Ops[0].Error)
	assert.Nil(t, res.Record)
	return res.Ops[0].Error.Message
}

// ─── core/change/filehome: an edit through the change service ───────────────

// TestChangeService_AnEditKeepsTheFormattingOnlyTheSkeletonHolds is the
// control for the test below and the property the swallowed error destroyed:
// with the skeleton store available, an edit changes the edited value and
// leaves every other byte as it was, comments, quoting and indentation
// included.
func TestChangeService_AnEditKeepsTheFormattingOnlyTheSkeletonHolds(t *testing.T) {
	isolateKapiEnv(t)
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "messages.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yamlWithFormattingOnlyASkeletonPreserves), 0o644))

	svc, set := yamlGreetingEdit(t, app, dir, "messages.yaml")
	res, err := svc.Apply(context.Background(), set, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, strings.Replace(yamlWithFormattingOnlyASkeletonPreserves, skeletonEditFrom, skeletonEditTo, 1), string(got),
		"an in-place edit writes through the skeleton, so only the edited value changes")
}

// TestChangeService_AnEditIsRefusedWhenTheSkeletonStoreCannotBeCreated is the
// core regression. The skeleton could not be created, so the writer would
// have rewritten the file from the content model, losing the comments and
// the exact layout. The user would see a `kapi apply` or `ksed -i` that exits
// 0 while their document has been silently reformatted, with no copy of the
// original left anywhere.
func TestChangeService_AnEditIsRefusedWhenTheSkeletonStoreCannotBeCreated(t *testing.T) {
	isolateKapiEnv(t)
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "messages.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yamlWithFormattingOnlyASkeletonPreserves), 0o644))
	svc, set := yamlGreetingEdit(t, app, dir, "messages.yaml")

	breakTempDir(t)

	msg := refusedEdit(t, svc, set)
	assert.Contains(t, msg, "messages.yaml", "the refusal must name the file it refused to rewrite")
	assert.Contains(t, msg, "formatting", "the refusal must say what would have been lost")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, yamlWithFormattingOnlyASkeletonPreserves, string(got),
		"and the file on disk must be untouched: a refused edit leaves no degraded artefact")
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

// ─── core/change/filehome: an edit to one archive member ─────────────────────

// TestChangeService_AnArchiveMemberEditIsRefusedWhenTheSkeletonStoreCannotBeCreated:
// a repack that silently substitutes a reconstruction of the member for the
// member is worse than a single-file edit, because the archive is rewritten
// around it.
func TestChangeService_AnArchiveMemberEditIsRefusedWhenTheSkeletonStoreCannotBeCreated(t *testing.T) {
	isolateKapiEnv(t)
	app := newToolboxApp(t)
	dir := t.TempDir()
	archive := writeZipFixture(t, dir, "messages.yaml", yamlWithFormattingOnlyASkeletonPreserves)
	before, err := os.ReadFile(archive)
	require.NoError(t, err)
	svc, set := yamlGreetingEdit(t, app, dir, "bundle.zip!messages.yaml")

	breakTempDir(t)

	msg := refusedEdit(t, svc, set)
	assert.Contains(t, msg, "messages.yaml", "the refusal must name the member it refused to rewrite")
	assert.Contains(t, msg, "formatting", "the refusal must say what would have been lost")

	after, rerr := os.ReadFile(archive)
	require.NoError(t, rerr)
	assert.Equal(t, before, after, "and the archive must not have been repacked")
}

// TestChangeService_AnArchiveMemberEditKeepsTheMembersFormatting is the
// control: with a working temp filesystem the member is repacked with only the
// edited value changed.
func TestChangeService_AnArchiveMemberEditKeepsTheMembersFormatting(t *testing.T) {
	isolateKapiEnv(t)
	app := newToolboxApp(t)
	dir := t.TempDir()
	archive := writeZipFixture(t, dir, "messages.yaml", yamlWithFormattingOnlyASkeletonPreserves)

	svc, set := yamlGreetingEdit(t, app, dir, "bundle.zip!messages.yaml")
	res, err := svc.Apply(context.Background(), set, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	assert.Equal(t, strings.Replace(yamlWithFormattingOnlyASkeletonPreserves, skeletonEditFrom, skeletonEditTo, 1),
		readZipEntry(t, archive, "messages.yaml"))
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
