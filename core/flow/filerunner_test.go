package flow_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/klf"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileRunner_PseudoTranslate(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.json")
	require.NoError(t, os.WriteFile(inputPath, []byte(`{"greeting": "Hello World"}`), 0o644))

	outputPath := filepath.Join(dir, "output", "qps", "input.json")

	pseudoTool, err := tools.NewPseudoTranslateFromConfig(map[string]any{
		"target_locale": "qps",
	}, "qps")
	require.NoError(t, err)

	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    reg,
		SourceLocale: "en-US",
	})

	err = runner.RunFile(context.Background(), "pseudo-translate", []tool.Tool{pseudoTool}, inputPath, outputPath, "qps")
	require.NoError(t, err)

	output, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.NotEmpty(t, output)
	assert.NotEqual(t, `{"greeting": "Hello World"}`, string(output))
}

// TestFileRunner_BufferedOutputFlushesFully asserts the filerunner-site
// output buffer (#608, S4) is fully flushed to disk — every block's
// pseudo-translated value lands in the output with no truncation, even
// when total output far exceeds the buffer. A skeleton-driven JSON write
// emits many small writes (one per skeleton run), exercising the buffer.
func TestFileRunner_BufferedOutputFlushesFully(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.json")

	// Build a JSON object with enough distinct entries that the output
	// comfortably exceeds the 64 KiB output buffer.
	var sb strings.Builder
	sb.WriteString("{\n")
	const n = 4000
	for i := range n {
		if i > 0 {
			sb.WriteString(",\n")
		}
		fmt.Fprintf(&sb, "  %q: %q", fmt.Sprintf("key%05d", i), fmt.Sprintf("Value number %d here", i))
	}
	sb.WriteString("\n}\n")
	require.NoError(t, os.WriteFile(inputPath, []byte(sb.String()), 0o644))

	outputPath := filepath.Join(dir, "out", "input.json")

	pseudoTool, err := tools.NewPseudoTranslateFromConfig(map[string]any{
		"target_locale": "qps",
	}, "qps")
	require.NoError(t, err)

	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    reg,
		SourceLocale: "en-US",
	})
	require.NoError(t, runner.RunFile(context.Background(), "pseudo-translate",
		[]tool.Tool{pseudoTool}, inputPath, outputPath, "qps"))

	output, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	require.Greater(t, len(output), 64*1024, "test must exceed the output buffer to be meaningful")

	// Output must be valid JSON with all keys present (no truncation).
	var got map[string]string
	require.NoError(t, json.Unmarshal(output, &got), "flushed output must be complete, valid JSON")
	assert.Len(t, got, n, "every key must survive to the flushed output")
	// First and last entries present and pseudo-translated (accented).
	require.Contains(t, got, "key00000")
	require.Contains(t, got, fmt.Sprintf("key%05d", n-1))
	assert.NotEqual(t, "Value number 0 here", got["key00000"], "value should be pseudo-translated")
}

// TestFileRunner_EmitOnCloseWriterFlushes covers writers (like the KLF
// jsx writer) that emit their entire payload in Close() rather than
// Write(). The filerunner must close the writer before flushing the
// output buffer, otherwise the file would be empty.
// TestFileRunner_CrossFormatSemanticExport verifies that a cross-format
// conversion (DocLang → Markdown) does NOT wire the source's byte skeleton into
// the foreign writer, so the Markdown writer reconstructs clean output from the
// content model + structural layer (WS6 role-driven semantic export): headings
// become "## ", list items become "- ". Without the format-match guard on
// skeleton wiring, the markdown writer would receive the DocLang XML skeleton
// and emit angle-bracket garbage.
func TestFileRunner_CrossFormatSemanticExport(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.dclg.xml")
	doclangSrc := `<?xml version="1.0" encoding="UTF-8"?>
<doclang xmlns="https://www.doclang.ai/ns/v0" version="0.6">
  <heading level="2">Overview</heading>
  <text>Intro paragraph.</text>
  <list class="unordered"><ldiv><marker>-</marker></ldiv><text>First</text><ldiv><marker>-</marker></ldiv><text>Second</text></list>
</doclang>`
	require.NoError(t, os.WriteFile(inputPath, []byte(doclangSrc), 0o644))

	reader, err := reg.NewReader("doclang")
	require.NoError(t, err)
	writer, err := reg.NewWriter("markdown")
	require.NoError(t, err)

	outputPath := filepath.Join(dir, "out.md")
	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    reg,
		SourceLocale: "en-US",
	})
	passthrough := &tool.BaseTool{ToolName: "passthrough"}
	require.NoError(t, runner.RunFileWithReaderWriter(context.Background(),
		"convert", []tool.Tool{passthrough}, inputPath, outputPath, "", reader, writer))

	out, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	got := string(out)

	assert.Contains(t, got, "## Overview", "heading should export as a level-2 Markdown heading")
	assert.Contains(t, got, "Intro paragraph.", "paragraph text should be present")
	assert.Contains(t, got, "- First", "list item should export as a Markdown bullet")
	assert.Contains(t, got, "- Second")
	assert.NotContains(t, got, "<heading", "DocLang skeleton markup must NOT leak into the Markdown output")
	assert.NotContains(t, got, "<doclang", "DocLang skeleton markup must NOT leak into the Markdown output")

	// Same source → clean HTML (the other WS6 semantic-export target).
	htmlWriter, err := reg.NewWriter("html")
	require.NoError(t, err)
	htmlOut := filepath.Join(dir, "out.html")
	require.NoError(t, runner.RunFileWithReaderWriter(context.Background(),
		"convert", []tool.Tool{&tool.BaseTool{ToolName: "passthrough"}},
		inputPath, htmlOut, "", mustReader(t, reg, "doclang"), htmlWriter))
	h, err := os.ReadFile(htmlOut)
	require.NoError(t, err)
	hs := string(h)
	assert.Contains(t, hs, "<h2>Overview</h2>", "heading should export as <h2>")
	for _, want := range []string{"<ul>", "<li>First</li>", "<li>Second</li>", "</ul>"} {
		assert.Contains(t, hs, want, "list should export as <ul>/<li>")
	}
	assert.NotContains(t, hs, "<doclang", "DocLang skeleton must NOT leak into the HTML output")
}

func mustReader(t *testing.T, reg *registry.FormatRegistry, name string) format.DataFormatReader {
	t.Helper()
	r, err := reg.NewReader(registry.FormatID(name))
	require.NoError(t, err)
	return r
}

func TestFileRunner_EmitOnCloseWriterFlushes(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.klf")
	klfFile := &klf.File{
		SchemaVersion: klf.SchemaVersion,
		Kind:          klf.Kind,
		Generator:     klf.GeneratorInfo{ID: "test", Version: "0"},
		Project:       klf.ProjectInfo{ID: "p", SourceLocale: "en-US"},
		Documents: []klf.Document{{
			ID:           "doc1",
			DocumentType: klf.DocumentTypeJSX,
			Path:         "a.json",
			Blocks: []klf.Block{{
				ID:           "b1",
				Translatable: true,
				Source:       []klf.Run{{Text: &klf.TextRun{Text: "Hello World"}}},
			}},
		}},
	}
	klfBytes, err := klf.Marshal(klfFile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(inputPath, klfBytes, 0o644))

	outputPath := filepath.Join(dir, "out", "input.klf")

	pseudoTool, err := tools.NewPseudoTranslateFromConfig(map[string]any{
		"target_locale": "qps",
	}, "qps")
	require.NoError(t, err)

	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    reg,
		SourceLocale: "en-US",
	})
	require.NoError(t, runner.RunFile(context.Background(), "pseudo-translate",
		[]tool.Tool{pseudoTool}, inputPath, outputPath, "qps"))

	output, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.NotEmpty(t, output, "emit-on-Close writer output must be flushed, not truncated to empty")
	assert.Contains(t, string(output), "documents", "KLF payload must be present in the flushed file")
}

// TestFileRunner_RunFileProcessOnly_CommitsOverlaysNoFile verifies the
// process-only run path (AD-026 §3): the tool chain runs against a persistent
// store, SessionTools commit `targets/<locale>` overlays, the session is
// committed, and NO output file is produced.
func TestFileRunner_RunFileProcessOnly_CommitsOverlaysNoFile(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.json")
	require.NoError(t, os.WriteFile(inputPath, []byte(`{"greeting":"Hello World"}`), 0o644))

	storePath := filepath.Join(dir, "blocks.db")
	store, err := sqlitestore.New(storePath)
	require.NoError(t, err)
	defer store.Close()
	require.True(t, store.Capabilities().Persistent, "sqlite store must be persistent")

	pseudoTool, err := tools.NewPseudoTranslateFromConfig(map[string]any{"target_locale": "qps"}, "qps")
	require.NoError(t, err)

	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    reg,
		SourceLocale: "en-US",
		Store:        store,
	})
	require.NoError(t, runner.RunFileProcessOnly(context.Background(),
		"pseudo-translate", []tool.Tool{pseudoTool}, inputPath, "qps"))

	// No sibling/output file anywhere in the dir except the input + the store.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), "qps", "process-only run must write no localized file; found %s", e.Name())
	}

	// The store holds at least one targets/qps overlay.
	sess, err := store.Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()
	n := 0
	for ov, err := range sess.ListOverlays("targets/qps") {
		require.NoError(t, err)
		assert.NotEmpty(t, ov.Payload)
		n++
	}
	assert.Positive(t, n, "process-only run must commit targets/qps overlays")
}

// TestFileRunner_RunFileToStore_RequiresPersistentStore verifies that a
// process-only run errors clearly when no store (or an ephemeral one) is
// configured — the work would otherwise be discarded.
func TestFileRunner_RunFileToStore_RequiresPersistentStore(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.json")
	require.NoError(t, os.WriteFile(inputPath, []byte(`{"a":"b"}`), 0o644))

	pseudoTool, err := tools.NewPseudoTranslateFromConfig(map[string]any{"target_locale": "qps"}, "qps")
	require.NoError(t, err)

	t.Run("nil store", func(t *testing.T) {
		err := flow.NewFileRunner(flow.FileRunnerConfig{FormatReg: reg, SourceLocale: "en-US"}).
			RunFileProcessOnly(context.Background(), "pseudo-translate", []tool.Tool{pseudoTool}, inputPath, "qps")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "persistent block store")
	})

	t.Run("ephemeral store", func(t *testing.T) {
		err := flow.NewFileRunner(flow.FileRunnerConfig{
			FormatReg: reg, SourceLocale: "en-US", Store: blockstore.NewMemoryStore(),
		}).RunFileProcessOnly(context.Background(), "pseudo-translate", []tool.Tool{pseudoTool}, inputPath, "qps")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "persistent block store")
	})
}

// erroringTool returns an error after passing through n parts. Used to
// exercise the filerunner's concurrent error-propagation path.
type erroringTool struct {
	*tool.BaseTool
	afterN int
}

func newErroringTool(afterN int) *erroringTool {
	return &erroringTool{BaseTool: &tool.BaseTool{ToolName: "boom"}, afterN: afterN}
}

func (e *erroringTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	seen := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case p, ok := <-in:
			if !ok {
				return nil
			}
			if seen >= e.afterN {
				return errBoom
			}
			seen++
			select {
			case out <- p:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

var errBoom = errors.New("boom: simulated tool failure")

// TestFileRunner_ToolErrorAbortsWithoutPartialOutput verifies that a tool
// error aborts the run, surfaces the error, and leaves NO output file at
// the destination — matching the pre-S1 contract even though the
// pipeline now streams concurrently (#608, S1).
func TestFileRunner_ToolErrorAbortsWithoutPartialOutput(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.json")
	require.NoError(t, os.WriteFile(inputPath,
		[]byte(`{"a":"one","b":"two","c":"three","d":"four"}`), 0o644))

	outputPath := filepath.Join(dir, "out", "input.json")

	err := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    reg,
		SourceLocale: "en-US",
	}).RunFile(context.Background(), "boom", []tool.Tool{newErroringTool(2)},
		inputPath, outputPath, "qps")

	require.Error(t, err, "tool error must propagate")
	require.ErrorIs(t, err, errBoom)

	_, statErr := os.Stat(outputPath)
	assert.True(t, os.IsNotExist(statErr),
		"a tool error must leave no output file at the destination; got statErr=%v", statErr)

	// No leftover temp files in the output dir either.
	entries, _ := os.ReadDir(filepath.Dir(outputPath))
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".kapi-out-", "temp file must be cleaned up on error")
	}
}

// TestFileRunner_ContextCancellationAborts verifies that cancelling the
// context aborts the run promptly without deadlock and without producing
// a destination file.
func TestFileRunner_ContextCancellationAborts(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.json")

	var sb strings.Builder
	sb.WriteString("{\n")
	for i := range 2000 {
		if i > 0 {
			sb.WriteString(",\n")
		}
		fmt.Fprintf(&sb, "  %q: %q", fmt.Sprintf("k%05d", i), "value here")
	}
	sb.WriteString("\n}\n")
	require.NoError(t, os.WriteFile(inputPath, []byte(sb.String()), 0o644))

	outputPath := filepath.Join(dir, "out", "input.json")

	pseudoTool, err := tools.NewPseudoTranslateFromConfig(map[string]any{
		"target_locale": "qps",
	}, "qps")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the run starts so the pipeline observes Done

	runErr := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    reg,
		SourceLocale: "en-US",
	}).RunFile(ctx, "pseudo-translate", []tool.Tool{pseudoTool},
		inputPath, outputPath, "qps")

	require.Error(t, runErr, "a cancelled context must abort the run")
	require.ErrorIs(t, runErr, context.Canceled)

	_, statErr := os.Stat(outputPath)
	assert.True(t, os.IsNotExist(statErr),
		"a cancelled run must leave no output file; got statErr=%v", statErr)
}

// TestFileRunner_XLSXGridToTable verifies that a cross-format export of a
// spreadsheet (whose translatable text lives in the deduplicated shared-string
// table, with no per-cell structure) reconstructs a real table from the cells'
// grid geometry — rendering a GFM table in Markdown and <table> markup in HTML,
// not a flat list of cell values, and without the shared strings duplicated as
// loose paragraphs.
func TestFileRunner_XLSXGridToTable(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	src, err := os.ReadFile("../formats/openxml/testdata/EksempelFiltrering.xlsx")
	require.NoError(t, err)
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.xlsx")
	require.NoError(t, os.WriteFile(inputPath, src, 0o644))

	runner := flow.NewFileRunner(flow.FileRunnerConfig{FormatReg: reg, SourceLocale: "en-US"})

	// → Markdown: a GFM table with the header row and separator.
	mdOut := filepath.Join(dir, "out.md")
	require.NoError(t, runner.RunFileWithReaderWriter(context.Background(), "convert",
		[]tool.Tool{&tool.BaseTool{ToolName: "passthrough"}}, inputPath, mdOut, "",
		mustReader(t, reg, "openxml"), mustWriter(t, reg, "markdown")))
	md, err := os.ReadFile(mdOut)
	require.NoError(t, err)
	gotMD := string(md)
	t.Logf("markdown:\n%s", firstLines(gotMD, 6))
	assert.Contains(t, gotMD, "| ID | Artist | Tittel |", "the header row should render as a GFM table row")
	assert.Contains(t, gotMD, "| --- |", "a GFM header separator row should be present")
	assert.Contains(t, gotMD, "| Nordlys |", "a data cell value should render inside the table")
	assert.NotContains(t, gotMD, "<worksheet", "openxml skeleton must not leak")
	// The shared-string source blocks and the Excel table-column names are
	// represented by the grid, so they must NOT also appear as loose paragraphs.
	assert.NotContains(t, gotMD, "\n\nID\n", "header text must not duplicate as a loose paragraph")
	assert.NotContains(t, gotMD, "\n\nRPM\n", "table-column names must not duplicate as loose paragraphs")

	// → HTML: real <table>/<tr>/<td> markup.
	htmlOut := filepath.Join(dir, "out.html")
	require.NoError(t, runner.RunFileWithReaderWriter(context.Background(), "convert",
		[]tool.Tool{&tool.BaseTool{ToolName: "passthrough"}}, inputPath, htmlOut, "",
		mustReader(t, reg, "openxml"), mustWriter(t, reg, "html")))
	h, err := os.ReadFile(htmlOut)
	require.NoError(t, err)
	gotHTML := string(h)
	assert.Contains(t, gotHTML, "<table>", "should render an HTML table")
	assert.Contains(t, gotHTML, "Nordlys", "a data cell value should be present in the HTML table")
}

func mustWriter(t *testing.T, reg *registry.FormatRegistry, name string) format.DataFormatWriter {
	t.Helper()
	w, err := reg.NewWriter(registry.FormatID(name))
	require.NoError(t, err)
	return w
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

// TestFileRunner_XLSXMergedCellToHTML proves the merged-cell chain end-to-end:
// the reader records a merge as cell-grid span, the structural transform maps it
// to ColSpan, and the HTML writer renders colspan. The fixture has no merges, so
// we inject one (A1:B1) by rebuilding the workbook zip.
func TestFileRunner_XLSXMergedCellToHTML(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	src, err := os.ReadFile("../formats/openxml/testdata/EksempelFiltrering.xlsx")
	require.NoError(t, err)
	merged := injectMerge(t, src, "A1:B1")
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "merged.xlsx")
	require.NoError(t, os.WriteFile(inputPath, merged, 0o644))

	runner := flow.NewFileRunner(flow.FileRunnerConfig{FormatReg: reg, SourceLocale: "en-US"})
	htmlOut := filepath.Join(dir, "out.html")
	require.NoError(t, runner.RunFileWithReaderWriter(context.Background(), "convert",
		[]tool.Tool{&tool.BaseTool{ToolName: "passthrough"}}, inputPath, htmlOut, "",
		mustReader(t, reg, "openxml"), mustWriter(t, reg, "html")))
	got, err := os.ReadFile(htmlOut)
	require.NoError(t, err)
	assert.Contains(t, string(got), `colspan="2"`, "the merged A1:B1 header should render with colspan=2")
}

func injectMerge(t *testing.T, src []byte, ref string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(src), int64(len(src)))
	require.NoError(t, err)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		rc, err := f.Open()
		require.NoError(t, err)
		body, err := io.ReadAll(rc)
		rc.Close()
		require.NoError(t, err)
		if f.Name == "xl/worksheets/sheet1.xml" {
			body = []byte(strings.Replace(string(body), "</sheetData>",
				`</sheetData><mergeCells count="1"><mergeCell ref="`+ref+`"/></mergeCells>`, 1))
		}
		w, err := zw.Create(f.Name)
		require.NoError(t, err)
		_, err = w.Write(body)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// memPartCache is an in-memory streaming flow.PartCache: a record() captures the
// parts (+ a memory-backed skeleton) one at a time, and an OpenDocument replays
// them. It tracks records so a test can prove a re-run is served from the cache.
type memPartCache struct {
	docs    map[string]*memDoc
	records int
}

type memDoc struct {
	parts   []*model.Part
	skel    []byte
	hasSkel bool
}

func newMemPartCache() *memPartCache { return &memPartCache{docs: map[string]*memDoc{}} }

// seed pre-populates a document so a test can prove the run is driven by the
// cached parts (not the file).
func (c *memPartCache) seed(path, key string, parts []*model.Part) {
	c.docs[path+"\x00"+key] = &memDoc{parts: parts}
}

func (c *memPartCache) OpenDocument(path, key string) flow.CachedDocument {
	d, ok := c.docs[path+"\x00"+key]
	if !ok {
		return nil
	}
	return &memCachedDoc{d: d}
}

func (c *memPartCache) RecordDocument(path, key, _ string) flow.DocumentRecorder {
	c.records++
	return &memRecorder{c: c, k: path + "\x00" + key, skel: format.NewMemorySkeletonStore()}
}

type memCachedDoc struct{ d *memDoc }

func (m *memCachedDoc) Feed(ctx context.Context, inCh chan<- *model.Part) error {
	defer close(inCh)
	for _, p := range m.d.parts {
		select {
		case inCh <- p:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (m *memCachedDoc) OpenSkeleton() *format.SkeletonStore {
	if !m.d.hasSkel {
		return nil
	}
	return format.NewSkeletonStoreFromBytes(m.d.skel)
}

func (m *memCachedDoc) Close() error { return nil }

type memRecorder struct {
	c     *memPartCache
	k     string
	parts []*model.Part
	skel  *format.SkeletonStore
}

func (r *memRecorder) SkeletonStore() *format.SkeletonStore { return r.skel }
func (r *memRecorder) Add(p *model.Part) error              { r.parts = append(r.parts, p); return nil }
func (r *memRecorder) Abort()                               {}
func (r *memRecorder) Commit() error {
	b, _ := r.skel.Bytes()
	r.c.docs[r.k] = &memDoc{parts: r.parts, skel: b, hasSkel: r.skel.EntriesWritten() > 0}
	return nil
}

// TestFileRunner_ProcessOnly_UsesPartCache proves the process-only runner reads
// through the document cache: a first run parses and stores the file's parts; a
// second run is served from the cache (no put). And when the cache is pre-seeded
// with sentinel parts, the run is driven by THOSE parts, not the file — decisive
// evidence the reader is bypassed on a hit.
func TestFileRunner_ProcessOnly_UsesPartCache(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.json")
	require.NoError(t, os.WriteFile(inputPath, []byte(`{"greeting":"Hello World"}`), 0o644))

	newStore := func(name string) blockstore.Store {
		s, err := sqlitestore.New(filepath.Join(dir, name))
		require.NoError(t, err)
		t.Cleanup(func() { s.Close() })
		return s
	}
	pseudo := func() tool.Tool {
		pt, err := tools.NewPseudoTranslateFromConfig(map[string]any{"target_locale": "qps"}, "qps")
		require.NoError(t, err)
		return pt
	}
	overlayHashes := func(s blockstore.Store) []string {
		sess, err := s.Begin(context.Background())
		require.NoError(t, err)
		defer sess.Close()
		var out []string
		for ov, err := range sess.ListOverlays("targets/qps") {
			require.NoError(t, err)
			out = append(out, ov.BlockHash)
		}
		return out
	}

	cache := newMemPartCache()

	// First run: cache miss → parse the file once and record its parts.
	store1 := newStore("blocks1.db")
	r1 := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg: reg, SourceLocale: "en-US", Store: store1,
		PartCache: cache, PartCacheKey: "k",
	})
	require.NoError(t, r1.RunFileProcessOnly(context.Background(),
		"pseudo-translate", []tool.Tool{pseudo()}, inputPath, "qps"))
	assert.Equal(t, 1, cache.records, "a cache miss records the parsed document once")
	require.Len(t, cache.docs, 1, "exactly one document cached")

	// Second run: cache hit → served from the cache (no new record).
	store2 := newStore("blocks2.db")
	r2 := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg: reg, SourceLocale: "en-US", Store: store2,
		PartCache: cache, PartCacheKey: "k",
	})
	require.NoError(t, r2.RunFileProcessOnly(context.Background(),
		"pseudo-translate", []tool.Tool{pseudo()}, inputPath, "qps"))
	assert.Equal(t, 1, cache.records, "the second run is served from the cache; no re-parse, no record")
	assert.Equal(t, overlayHashes(store1), overlayHashes(store2),
		"a cache-served run produces the identical work as the parsed run")

	// Decisive: pre-seed the cache for a fresh key with a SENTINEL block that is
	// not in the file. The run must commit an overlay for the sentinel's hash —
	// proving cached parts, not the file, drove it.
	sentinel := model.NewBlock("sentinel-hash", "Only In Cache")
	sentinel.Translatable = true
	cache.seed(inputPath, "json|run|seed", []*model.Part{{Type: model.PartBlock, Resource: sentinel}})
	store3 := newStore("blocks3.db")
	r3 := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg: reg, SourceLocale: "en-US", Store: store3,
		PartCache: cache, PartCacheKey: "seed",
	})
	require.NoError(t, r3.RunFileProcessOnly(context.Background(),
		"pseudo-translate", []tool.Tool{pseudo()}, inputPath, "qps"))
	assert.Equal(t, []string{"sentinel-hash"}, overlayHashes(store3),
		"the run was driven by the cached parts, not by re-parsing the file")
}

// TestFileRunner_CachedWrite_ByteIdenticalToLive is the byte-fidelity gate for
// the streaming file-writing document cache: for each format, the cached miss
// (parse → record parts + skeleton, then replay) and the cached hit (replay,
// reader never runs) must each produce output byte-identical to the live,
// uncached round-trip. This proves the streamed record/replay reconstruction
// matches the live skeleton-wired path.
func TestFileRunner_CachedWrite_ByteIdenticalToLive(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	cases := []struct{ name, file, content string }{
		{"json", "m.json", `{"greeting":"Hello World","bye":"Goodbye"}`},
		{"markdown", "m.md", "# Title\n\nHello World and more text.\n"},
		{"properties", "m.properties", "greeting = Hello World\nbye = Goodbye\n"},
		{"yaml", "m.yaml", "greeting: Hello World\nbye: Goodbye\n"},
	}
	pseudo := func(t *testing.T) tool.Tool {
		pt, err := tools.NewPseudoTranslateFromConfig(map[string]any{"target_locale": "qps"}, "qps")
		require.NoError(t, err)
		return pt
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, tc.file)
			require.NoError(t, os.WriteFile(src, []byte(tc.content), 0o644))

			run := func(cache flow.PartCache, out string) {
				r := flow.NewFileRunner(flow.FileRunnerConfig{
					FormatReg: reg, SourceLocale: "en-US",
					PartCache: cache, PartCacheKey: "k",
				})
				require.NoError(t, r.RunFile(context.Background(), "pseudo-translate", []tool.Tool{pseudo(t)}, src, out, "qps"))
			}

			liveOut := filepath.Join(dir, "live."+tc.name)
			run(nil, liveOut) // no cache → live skeleton-wired path
			live, err := os.ReadFile(liveOut)
			require.NoError(t, err)

			cache := newMemPartCache()
			missOut := filepath.Join(dir, "miss."+tc.name)
			run(cache, missOut) // cache miss → record then replay
			hitOut := filepath.Join(dir, "hit."+tc.name)
			run(cache, hitOut) // cache hit → replay, no reader

			miss, err := os.ReadFile(missOut)
			require.NoError(t, err)
			hit, err := os.ReadFile(hitOut)
			require.NoError(t, err)

			assert.Equal(t, string(live), string(miss), "cached miss (record→replay) must equal the live output")
			assert.Equal(t, string(live), string(hit), "cached hit (replay) must equal the live output")
			assert.NotEqual(t, tc.content, string(hit), "pseudo must have altered the source")
			assert.Equal(t, 1, cache.records, "the source is parsed/recorded exactly once across both runs")
			require.Len(t, cache.docs, 1, "exactly one document cached")
		})
	}
}
