package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"syscall"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/segment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSatTransport struct {
	boundaries []int
	err        error
	gotText    string
	gotModel   string
	gotThresh  float64
}

func (f *fakeSatTransport) segment(text, modelName, _ string, threshold float64) ([]int, error) {
	f.gotText, f.gotModel, f.gotThresh = text, modelName, threshold
	return f.boundaries, f.err
}

func TestSatEngine_MapsBoundariesToSpans(t *testing.T) {
	ft := &fakeSatTransport{boundaries: []int{13, 26}}
	eng := &satEngine{cfg: segment.Config{SatModel: "sat-3l-sm", Threshold: 0.25}, transport: ft}

	runs := []model.Run{{Text: &model.TextRun{Text: "Hello world. How are you? I am fine."}}}
	spans, err := eng.Segment(context.Background(), runs, "en")
	require.NoError(t, err)
	require.Len(t, spans, 3)
	assert.Equal(t, "Hello world. ", model.RunsText(spans[0].Range.ExtractRuns(runs)))
	assert.Equal(t, "How are you? ", model.RunsText(spans[1].Range.ExtractRuns(runs)))
	assert.Equal(t, "I am fine.", model.RunsText(spans[2].Range.ExtractRuns(runs)))
	// Config is forwarded to the plugin.
	assert.Equal(t, "sat-3l-sm", ft.gotModel)
	assert.InDelta(t, 0.25, ft.gotThresh, 1e-9)
	assert.Equal(t, segment.LayerSentence, eng.Layer())
}

func TestSatEngine_EmptyTextSkipsTransport(t *testing.T) {
	ft := &fakeSatTransport{err: errors.New("should not be called")}
	eng := &satEngine{cfg: segment.Config{}, transport: ft}
	spans, err := eng.Segment(context.Background(), nil, "en")
	require.NoError(t, err)
	assert.Nil(t, spans)
	assert.Empty(t, ft.gotText, "transport not invoked for empty content")
}

func TestSatEngine_TransportErrorWrapped(t *testing.T) {
	ft := &fakeSatTransport{err: errors.New("model.onnx not found: build with -tags onnx")}
	eng := &satEngine{cfg: segment.Config{}, transport: ft}
	runs := []model.Run{{Text: &model.TextRun{Text: "One. Two."}}}
	_, err := eng.Segment(context.Background(), runs, "en")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sat:")
	assert.Contains(t, err.Error(), "model.onnx")
}

func TestSatEngine_RegisteredAndDiscoverable(t *testing.T) {
	assert.True(t, segment.HasEngine("sat"), "cli registers the sat engine at init")
}

func TestFindSatPlugin_NotInstalled(t *testing.T) {
	// Point discovery at an empty dir and disable system roots via the env
	// override; with no sat plugin present, the error guides installation.
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	_, err := findSatPlugin()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugins install sat")
}

// ── #48: host scanner buffer must match the plugin's line cap ────────────────

func TestSatScannerBufferMatchesPluginLineCap(t *testing.T) {
	// The host read-buffer cap must match plugins/sat/satproto.MaxLineBytes
	// (64 MiB). A smaller host cap silently truncates a large but valid
	// response into a decode error. Duplicated as a constant (not imported) so
	// the CLI keeps no dependency on the plugin module — this test guards the
	// duplication against drift.
	assert.Equal(t, 64<<20, satMaxLineBytes,
		"satMaxLineBytes must equal satproto.MaxLineBytes (64 MiB)")
}

func TestSatProcess_ReadsLineLargerThanDefaultCap(t *testing.T) {
	// Drive a real satProcess (built by startSatProcess) against a fake plugin
	// that emits a response far larger than bufio's default 64 KiB line cap,
	// proving the buffer is sized to satMaxLineBytes and the response is not
	// truncated into a decode error.
	useSatHelper(t)
	t.Setenv("GO_SAT_BOUNDARY_COUNT", "200000")
	proc, err := startSatProcess(context.Background(), selfBinary(t))
	require.NoError(t, err)
	t.Cleanup(proc.close)

	bounds, err := proc.segment("anything", "sat-3l-sm", "en", 0.25)
	require.NoError(t, err)
	require.Len(t, bounds, 200_000,
		"a >64 KiB valid response must be read whole, not truncated")
}

// ── #18: subprocess lifetime is decoupled from any per-call context ──────────

func TestSatEngine_SurvivesPerItemContextCancel(t *testing.T) {
	// Mirrors the desktop runner reusing one tool (and thus one engine) across
	// files: file 1 runs under ctx1 and completes (cancel), then file 2 runs
	// under ctx2 on the SAME engine. The plugin process must outlive ctx1.
	useSatHelper(t)
	eng := &satEngine{cfg: segment.Config{PluginPath: selfBinary(t), SatModel: "sat-3l-sm"}}
	t.Cleanup(func() { _ = eng.Close() })

	runs := []model.Run{{Text: &model.TextRun{Text: "Hello world. How are you?"}}}

	// File 1: a per-run context that is cancelled once the file completes.
	ctx1, cancel1 := context.WithCancel(context.Background())
	spans1, err := eng.Segment(ctx1, runs, "en")
	require.NoError(t, err)
	require.NotEmpty(t, spans1)
	cancel1() // file 1 done — under the old bug this killed the subprocess

	// File 2: a fresh per-run context on the SAME engine — must still work.
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	spans2, err := eng.Segment(ctx2, runs, "en")
	require.NoError(t, err, "subprocess must outlive file 1's cancelled context")
	assert.Equal(t, spans1, spans2)
}

func TestSatEngine_ClosePerItemCtxAlreadyCancelled(t *testing.T) {
	// A per-call context that is already cancelled is reported promptly without
	// tearing the engine down (the process is engine-scoped, not call-scoped).
	useSatHelper(t)
	eng := &satEngine{cfg: segment.Config{PluginPath: selfBinary(t), SatModel: "sat-3l-sm"}}
	t.Cleanup(func() { _ = eng.Close() })

	// Warm the process with a good call so dial() runs.
	runs := []model.Run{{Text: &model.TextRun{Text: "One. Two."}}}
	_, err := eng.Segment(context.Background(), runs, "en")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = eng.Segment(ctx, runs, "en")
	require.ErrorIs(t, err, context.Canceled)

	// Engine still usable afterwards.
	_, err = eng.Segment(context.Background(), runs, "en")
	require.NoError(t, err)
}

// ── #18: Close reaps the child (Kill + Wait), idempotently ───────────────────

func TestSatEngine_CloseReapsChild(t *testing.T) {
	useSatHelper(t)
	eng := &satEngine{cfg: segment.Config{PluginPath: selfBinary(t), SatModel: "sat-3l-sm"}}

	runs := []model.Run{{Text: &model.TextRun{Text: "Hello. World."}}}
	_, err := eng.Segment(context.Background(), runs, "en")
	require.NoError(t, err)

	proc := eng.closer.(*satProcess)
	require.NotNil(t, proc.cmd.Process)
	pid := proc.cmd.Process.Pid

	require.NoError(t, eng.Close())
	// After Kill + Wait the child is reaped: signalling the finished process
	// errors with "process already finished".
	require.Error(t, proc.cmd.Process.Signal(syscall.Signal(0)),
		"child pid %d should be reaped after Close", pid)

	// Idempotent: a second Close and the finalizer-equivalent close are safe.
	require.NoError(t, eng.Close())
	proc.close()
}

func TestSatEngine_CloseWithoutDialIsSafe(t *testing.T) {
	// An engine backed by a fake transport (never dialed) has no child to reap;
	// Close must be a no-op, not a nil panic.
	eng := &satEngine{cfg: segment.Config{}, transport: &fakeSatTransport{}}
	require.NoError(t, eng.Close())
	require.NoError(t, eng.Close())
}

func TestSatProcess_CloseIsIdempotent(t *testing.T) {
	useSatHelper(t)
	proc, err := startSatProcess(context.Background(), selfBinary(t))
	require.NoError(t, err)
	proc.close()
	// Second and third closes must not panic or block (Kill/Wait guarded once).
	proc.close()
	require.NoError(t, proc.Close())
}

// ── fake sat plugin (helper-process pattern) ─────────────────────────────────
//
// The test binary doubles as a fake kapi-sat plugin: when GO_SAT_HELPER=1 the
// TestHelperSatPlugin entry point runs the line-delimited sat serve loop on
// stdin/stdout, so startSatProcess(self) yields a real subprocess that speaks
// the protocol without the ONNX model. useSatHelper sets the env on the parent
// test (t.Setenv) so children spawned by startSatProcess inherit it.

// useSatHelper marks the current (non-parallel) test so that subprocesses it
// spawns via startSatProcess behave as the fake sat plugin.
func useSatHelper(t *testing.T) {
	t.Helper()
	t.Setenv("GO_SAT_HELPER", "1")
}

// selfBinary returns the path of the running test binary, used as the fake
// plugin's binary path.
func selfBinary(t *testing.T) string {
	t.Helper()
	self, err := os.Executable()
	require.NoError(t, err)
	return self
}

// TestMain lets the test binary double as the fake sat plugin. When
// GO_SAT_HELPER=1 the process runs ONLY the serve loop (and exits) so nothing
// but protocol bytes reach stdout — running the full suite in the child would
// corrupt the stream. Otherwise it runs the tests normally.
func TestMain(m *testing.M) {
	if os.Getenv("GO_SAT_HELPER") == "1" {
		runFakeSatPlugin()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runFakeSatPlugin speaks the line-delimited sat protocol on stdin/stdout,
// segmenting after each "." or, when GO_SAT_BOUNDARY_COUNT is set, returning
// exactly that many boundaries (to exercise the large-response buffer cap, #48).
func runFakeSatPlugin() {
	var fixed int
	if n := os.Getenv("GO_SAT_BOUNDARY_COUNT"); n != "" {
		v, err := json.Number(n).Int64()
		if err != nil {
			panic(err)
		}
		fixed = int(v)
	}
	enc := json.NewEncoder(os.Stdout)
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), satMaxLineBytes)
	for sc.Scan() {
		var req satRequest
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			_ = enc.Encode(satResponse{Error: "bad request: " + err.Error()})
			continue
		}
		var bounds []int
		if fixed > 0 {
			bounds = make([]int, fixed)
			for i := range bounds {
				bounds[i] = i + 1
			}
		} else {
			// Interior boundaries: one after each "." that is not the last rune.
			runes := []rune(req.Text)
			for i, r := range runes {
				if r == '.' && i+1 < len(runes) {
					bounds = append(bounds, i+1)
				}
			}
		}
		_ = enc.Encode(satResponse{ID: req.ID, Boundaries: bounds, OK: true})
	}
}
