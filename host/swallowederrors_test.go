package host

import (
	"context"
	"errors"
	"iter"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file guards the "an operation failed and the system reported success"
// family that #1449/#1459 opened. Each test below fails on the pre-fix code for
// the defect's own reason — see the PR body for the per-site revert evidence.

// ─── the read-path document recorder (host/verify.go) ────────────────────────

// recordingSpy is a flow.DocumentRecorder that fails on the nth Add and records
// whether it was aborted or committed. The interface is the real seam the doc
// cache satisfies; only the failure is arranged.
type recordingSpy struct {
	failAddAt   int // 1-based; 0 = never fail
	failCommit  error
	added       int
	aborted     bool
	committed   bool
	addFailures int
}

func (r *recordingSpy) SkeletonStore() *format.SkeletonStore { return nil }

func (r *recordingSpy) Add(*model.Part) error {
	r.added++
	if r.failAddAt > 0 && r.added == r.failAddAt {
		r.addFailures++
		return errors.New("no space left on device")
	}
	return nil
}

func (r *recordingSpy) Commit() error { r.committed = true; return r.failCommit }
func (r *recordingSpy) Abort()        { r.aborted = true }

// partsReader is a minimal DataFormatReader that replays a fixed part list.
type partsReader struct {
	format.BaseFormatReader
	parts []*model.Part
}

func (p *partsReader) Open(context.Context, *model.RawDocument) error { return nil }
func (p *partsReader) Close() error                                   { return nil }
func (p *partsReader) Signature() format.FormatSignature              { return format.FormatSignature{} }

func (p *partsReader) Read(context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, len(p.parts))
	for _, part := range p.parts {
		ch <- model.PartResult{Part: part}
	}
	close(ch)
	return ch
}

func threeBlockReader() *partsReader {
	return &partsReader{parts: []*model.Part{
		{Type: model.PartBlock, Resource: mkBlock("a", "Apple")},
		{Type: model.PartBlock, Resource: mkBlock("b", "Banana")},
		{Type: model.PartBlock, Resource: mkBlock("c", "Cherry")},
	}}
}

// TestStreamIntoRecorder_AddFailureAbortsRatherThanCommitting is the core
// regression for the discarded `_ = rec.Add(...)`. A part that never reached the
// append log used to be followed by a Commit anyway, so the index row claimed a
// COMPLETE document that was in fact short — and every later read replayed the
// short version, shrinking coverage's denominator until the source changed or
// `.kapi/work/cache` was deleted. A document that cannot be fully recorded must not be
// recorded at all.
func TestStreamIntoRecorder_AddFailureAbortsRatherThanCommitting(t *testing.T) {
	spy := &recordingSpy{failAddAt: 2}
	blocks, err := streamIntoRecorder(context.Background(), threeBlockReader(), spy, "m.json")
	require.NoError(t, err, "the READ is unaffected — only the cache write failed")

	assert.True(t, spy.aborted, "a partially recorded document must be aborted")
	assert.False(t, spy.committed, "committing after a failed Add is what published a truncated document as complete")
	assert.Equal(t, 2, spy.added, "recording stops at the first failure instead of writing into an aborted log")

	// The caller's own result is complete: blocks come from the reader, which is
	// why this degrades to "no cache entry" rather than failing the command.
	var texts []string
	for _, b := range blocks {
		texts = append(texts, b.SourceText())
	}
	assert.Equal(t, []string{"Apple", "Banana", "Cherry"}, texts,
		"every block is still returned — the cache is an optimization, not the source of truth")
}

// TestStreamIntoRecorder_CommitFailureDegradesToNoEntry: Commit aborts internally
// on failure, so no index row is written and the next read is an honest miss. The
// read still succeeds and returns everything.
func TestStreamIntoRecorder_CommitFailureDegradesToNoEntry(t *testing.T) {
	spy := &recordingSpy{failCommit: errors.New("disk I/O error")}
	blocks, err := streamIntoRecorder(context.Background(), threeBlockReader(), spy, "m.json")
	require.NoError(t, err)
	assert.Len(t, blocks, 3)
	assert.True(t, spy.committed)
}

// TestStreamIntoRecorder_HealthyRecordingCommits is the negative control: with no
// induced failure the document is committed and nothing is aborted, so the fix
// cannot be satisfied by simply never caching.
func TestStreamIntoRecorder_HealthyRecordingCommits(t *testing.T) {
	spy := &recordingSpy{}
	blocks, err := streamIntoRecorder(context.Background(), threeBlockReader(), spy, "m.json")
	require.NoError(t, err)
	assert.Len(t, blocks, 3)
	assert.Equal(t, 3, spy.added)
	assert.True(t, spy.committed, "a clean recording still commits")
	assert.False(t, spy.aborted)
}

// TestStreamIntoRecorder_NoRecorderStillReads covers rec == nil (no cache, or
// another worker already recording this document).
func TestStreamIntoRecorder_NoRecorderStillReads(t *testing.T) {
	blocks, err := streamIntoRecorder(context.Background(), threeBlockReader(), nil, "m.json")
	require.NoError(t, err)
	assert.Len(t, blocks, 3)
}

// ─── check tools (host/checkservice.go) ──────────────────────────────────────

// failingChecker is a BlockProcessor whose Process fails — the shape a check
// plugin takes when its daemon is down.
type failingChecker struct{ err error }

func (f failingChecker) Process(_ context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	for range in { //nolint:revive // drain
	}
	return f.err
}

// TestRunCheckTool_ReturnsToolError: a checker that could not run is not a
// checker that passed. The error used to be read off the channel and dropped, so
// an un-annotated block was indistinguishable from a clean one.
func TestRunCheckTool_ReturnsToolError(t *testing.T) {
	b := mkBlock("a", "Apple")
	err := RunCheckTool(context.Background(), failingChecker{err: errors.New("check plugin: connection refused")}, b)
	require.Error(t, err, "the tool's Process error must reach the caller")
	assert.Contains(t, err.Error(), "connection refused")
	assert.Contains(t, err.Error(), blockKey(b), "the message names the block it failed on")
}

// TestRunFamily_ReportsCheckerFailure: the per-family driver stops and reports.
// Continuing would report the remaining blocks as a complete, clean result.
func TestRunFamily_ReportsCheckerFailure(t *testing.T) {
	a := newTestApp()
	blocks := []*model.Block{mkBlock("a", "Apple"), mkBlock("b", "Banana")}
	err := a.runFamily(context.Background(), blocks, failingChecker{err: errors.New("boom")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

// TestRunCheckTool_CleanToolReturnsNil is the negative control.
func TestRunCheckTool_CleanToolReturnsNil(t *testing.T) {
	require.NoError(t, RunCheckTool(context.Background(), failingChecker{err: nil}, mkBlock("a", "Apple")))
}

// ─── block-store overlay reads (host/merge.go, host/kpzhydrate.go) ───────────

// brokenOverlayStore is a blockstore.Store whose overlay reads fail with a real
// I/O-class error — distinct from the ErrNotFound that means "no translation for
// this block yet". Conflating the two is what let a short absorb, and a
// source-text deliverable, be reported as success.
type brokenOverlayStore struct{ readErr error }

type brokenOverlaySession struct{ readErr error }

func (s brokenOverlayStore) Begin(context.Context) (blockstore.Session, error) {
	return brokenOverlaySession(s), nil
}
func (s brokenOverlayStore) Capabilities() blockstore.Capabilities {
	return blockstore.Capabilities{RandomAccess: true}
}
func (s brokenOverlayStore) Close() error { return nil }

func (s brokenOverlaySession) Blocks(blockstore.BlockFilter) iter.Seq2[*blockstore.Block, error] {
	return func(func(*blockstore.Block, error) bool) {}
}
func (s brokenOverlaySession) Capabilities() blockstore.Capabilities {
	return blockstore.Capabilities{RandomAccess: true}
}
func (s brokenOverlaySession) GetBlock(string) (*blockstore.Block, error) {
	return nil, blockstore.ErrNotFound
}
func (s brokenOverlaySession) PutBlock(string, *blockstore.Block) error { return nil }
func (s brokenOverlaySession) GetOverlay(string, string) (blockstore.Overlay, error) {
	return blockstore.Overlay{}, s.readErr
}
func (s brokenOverlaySession) PutOverlay(blockstore.Overlay) error { return nil }
func (s brokenOverlaySession) ListOverlays(string) iter.Seq2[blockstore.Overlay, error] {
	return func(func(blockstore.Overlay, error) bool) {}
}
func (s brokenOverlaySession) Commit() error   { return nil }
func (s brokenOverlaySession) Rollback() error { return nil }
func (s brokenOverlaySession) Close() error    { return nil }
func (s brokenOverlaySession) Clear() error    { return nil }

func writeJSONSource(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "en.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"a":"Apple","b":"Banana"}`), 0o644))
	return src
}

// TestAbsorbStoreTargets_StoreReadFailureFails: `if oerr != nil || len(payload) ==
// 0 { continue }` treated a failed store read exactly like "no target overlay".
// Every stored translation therefore went unabsorbed while `kapi merge` printed
// tm_new=0 / tm_updated=0 and exited 0 — the next run re-translated (and re-paid
// for) content the store already held.
func TestAbsorbStoreTargets_StoreReadFailureFails(t *testing.T) {
	a := &App{}
	a.InitRegistries()
	src := writeJSONSource(t)

	_, _, err := absorbStoreTargets(context.Background(), a.FormatReg, "json", src,
		model.LocaleID("en"), model.LocaleID("fr"),
		brokenOverlayStore{readErr: errors.New("database disk image is malformed")}, nil, "en.json")
	require.Error(t, err, "a store read failure must not read as 'no translation stored'")
	assert.Contains(t, err.Error(), "malformed")
	assert.Contains(t, err.Error(), "targets/fr", "the message names the overlay kind")
	assert.Contains(t, err.Error(), "en.json", "and the source it was absorbing")
}

// TestAbsorbStoreTargets_NotFoundIsAbsenceNotFailure is the discriminating
// control: ErrNotFound is the documented absence sentinel, so it must still skip
// the block quietly. Without this, "fail on any error" would break every ordinary
// partially-translated project.
func TestAbsorbStoreTargets_NotFoundIsAbsenceNotFailure(t *testing.T) {
	a := &App{}
	a.InitRegistries()
	src := writeJSONSource(t)

	n, u, err := absorbStoreTargets(context.Background(), a.FormatReg, "json", src,
		model.LocaleID("en"), model.LocaleID("fr"),
		brokenOverlayStore{readErr: blockstore.ErrNotFound}, nil, "en.json")
	require.NoError(t, err, "no overlay for a block is ordinary pending work")
	assert.Zero(t, n)
	assert.Zero(t, u)
}

// TestHydrateTargets_StoreReadFailureFailsTheRun: the same conflation on the
// output-writing side. A locked or corrupt block store made the merge write the
// SOURCE text into the target file and report it applied — a silently
// untranslated deliverable.
func TestHydrateTargets_StoreReadFailureFailsTheRun(t *testing.T) {
	tl := newHydrateTargetsTool(model.LocaleID("fr"))
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	b := mkBlock("a", "Apple")
	b.Translatable = true
	in <- &model.Part{Type: model.PartBlock, Resource: b}
	close(in)

	sess := brokenOverlaySession{readErr: errors.New("database is locked")}
	errc := make(chan error, 1)
	go func() {
		defer close(out)
		errc <- tl.SessionProcess(context.Background(), sess, in, out)
	}()
	for range out { //nolint:revive // drain
	}
	err := <-errc
	require.Error(t, err, "a store read failure must fail the merge, not silently ship source text")
	assert.Contains(t, err.Error(), "locked")
}

// TestHydrateTargets_NotFoundPassesThrough is the control: an untranslated block
// flows through untouched.
func TestHydrateTargets_NotFoundPassesThrough(t *testing.T) {
	tl := newHydrateTargetsTool(model.LocaleID("fr"))
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	b := mkBlock("a", "Apple")
	b.Translatable = true
	in <- &model.Part{Type: model.PartBlock, Resource: b}
	close(in)

	sess := brokenOverlaySession{readErr: blockstore.ErrNotFound}
	errc := make(chan error, 1)
	go func() {
		defer close(out)
		errc <- tl.SessionProcess(context.Background(), sess, in, out)
	}()
	got := 0
	for range out {
		got++
	}
	require.NoError(t, <-errc)
	assert.Equal(t, 1, got, "an untranslated block still passes through")
}

// TestApplyTargetOverlay_MalformedPayloadIsAnError: returning silently left the
// block with no target at all, so the merged file shipped that segment as source
// text while the run still counted it applied.
func TestApplyTargetOverlay_MalformedPayloadIsAnError(t *testing.T) {
	b := mkBlock("a", "Apple")
	err := applyTargetOverlay(b, model.LocaleID("fr"), []byte(`{"text":`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode target overlay")
	assert.Nil(t, b.Target(model.LocaleID("fr")), "nothing is applied from an undecodable payload")
}

// TestApplyTargetOverlay_WellFormedPayloadApplies is the control.
func TestApplyTargetOverlay_WellFormedPayloadApplies(t *testing.T) {
	b := mkBlock("a", "Apple")
	require.NoError(t, applyTargetOverlay(b, model.LocaleID("fr"), []byte(`{"text":"Pomme"}`)))
	assert.Equal(t, "Pomme", b.TargetText(model.LocaleID("fr")))
}

// ─── the voice gate (host/verify.go) ─────────────────────────────────────────

// TestRunVoiceVocabOnBlock_MalformedFindingsPropertyIsAnError: the voice gate
// scores whatever findings this returns, so folding a decode failure into "no
// findings" produced an empty set, a perfect score, and `brand: PASS` — the gate
// silently disabled while CI went green. (The Process-error half now shares
// RunCheckTool's driver, covered by TestRunCheckTool_ReturnsToolError.)
func TestRunVoiceVocabOnBlock_MalformedFindingsPropertyIsAnError(t *testing.T) {
	b := mkBlock("a", "Apple")
	if b.Properties == nil {
		b.Properties = map[string]string{}
	}
	b.Properties["voice-vocab-findings"] = `[{"severity":`

	findings, err := runVoiceVocabOnBlock(context.Background(), coretools.NewVoiceVocabCheckTool(&profile.VoiceProfile{}, nil), b)
	require.Error(t, err, "an undecodable findings payload is a fault, not a clean block")
	assert.Contains(t, err.Error(), "voice vocabulary findings")
	assert.Empty(t, findings)
}

// TestRunVoiceVocabOnBlock_CleanBlockScoresEmpty is the control: a block with
// nothing recorded returns no findings and no error.
func TestRunVoiceVocabOnBlock_CleanBlockScoresEmpty(t *testing.T) {
	b := mkBlock("a", "Apple")
	findings, err := runVoiceVocabOnBlock(context.Background(), coretools.NewVoiceVocabCheckTool(&profile.VoiceProfile{}, nil), b)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

// ─── --trace (host/flow.go) ──────────────────────────────────────────────────

// TestWriteTraceFile_ReportsAnUnwritableDestination: `--trace` is an explicit
// request for a file. `_ = os.MkdirAll` / `_ = os.WriteFile` meant a run could
// write no trace at all and still exit 0, leaving the user to open a visualizer
// on nothing. The batch path has always propagated these; the single-file path
// did not.
func TestWriteTraceFile_ReportsAnUnwritableDestination(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "en.json")
	require.NoError(t, os.WriteFile(input, []byte(`{"a":"Apple"}`), 0o644))

	// A FILE stands where the trace's parent directory would have to be, so
	// MkdirAll cannot create it.
	blocker := filepath.Join(dir, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	tracePath := filepath.Join(blocker, "sub", "trace.json")

	a := &App{}
	a.InitRegistries()
	rec := flow.NewTraceRecorder()
	err := a.writeTraceFile(tracePath, "translate", "json", input, input, rec, []string{"translate"})
	require.Error(t, err, "an unwritable --trace destination must fail the run")
	assert.Contains(t, err.Error(), "trace dir")
	assert.Contains(t, err.Error(), blocker, "the message names the path that could not be created")
}

// TestWriteTraceFile_WritesAFreePath is the control: a writable destination is
// written and reported clean, so the guard rejects only real failures.
func TestWriteTraceFile_WritesAFreePath(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "en.json")
	require.NoError(t, os.WriteFile(input, []byte(`{"a":"Apple"}`), 0o644))
	tracePath := filepath.Join(dir, "nested", "trace.json")

	a := &App{}
	a.InitRegistries()
	rec := flow.NewTraceRecorder()
	require.NoError(t, a.writeTraceFile(tracePath, "translate", "json", input, input, rec, []string{"translate"}))
	st, err := os.Stat(tracePath)
	require.NoError(t, err)
	assert.Positive(t, st.Size())
}
