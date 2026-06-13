package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestMain lets the test binary double as a worker subprocess so the driver's
// one-file-one-subprocess isolation can be exercised without building a
// separate binary. The driver (in tests) sets WorkerArgv=[testbin] and
// WorkerEnv=[CORPUS_SWEEP_TEST_WORKER=1]; the re-exec'd binary lands here before
// the test runner parses any flags.
func TestMain(m *testing.M) {
	if os.Getenv("CORPUS_SWEEP_TEST_WORKER") == "1" {
		if os.Getenv("CORPUS_SWEEP_TEST_HANG") == "1" {
			// A real timer keeps the runtime from flagging a deadlock; the
			// driver's wall-clock kill must terminate us.
			time.Sleep(time.Hour)
		}
		os.Exit(runWorkerArgs(os.Args[1:]))
	}
	os.Exit(m.Run())
}

func testDriver(t *testing.T, formats []string) *Driver {
	t.Helper()
	repoRoot, err := findRepoRoot()
	require.NoError(t, err)
	return &Driver{
		RepoRoot:   repoRoot,
		Formats:    formats,
		Timeout:    30 * time.Second,
		RSSCap:     0, // disabled in tests: the worker's baseline RSS varies by host
		WorkerArgv: []string{os.Args[0]},
		WorkerEnv:  []string{"CORPUS_SWEEP_TEST_WORKER=1"},
		Promote:    false,
		PromoteDir: t.TempDir(),
		Stderr:     io.Discard,
	}
}

// TestDriverSmoke runs the driver over a few formats' committed Tier A testdata
// (no Tier B fetched), spawning one real worker subprocess per file, and
// asserts the taxonomy is populated with no crashers on the clean exemplars.
func TestDriverSmoke(t *testing.T) {
	d := testDriver(t, []string{"json", "plaintext", "po"})
	rep, err := d.Run()
	require.NoError(t, err)

	require.True(t, rep.TierBEmptyAll, "no Tier B corpus is fetched in CI/tests")

	valid := map[string]bool{}
	for _, c := range allClasses {
		valid[string(c)] = true
	}
	var totalFiles int
	for _, fr := range rep.Formats {
		require.Contains(t, []string{"json", "plaintext", "po"}, fr.Format)
		for _, f := range fr.Files {
			totalFiles++
			require.True(t, valid[f.Class], "unexpected class %q for %s", f.Class, f.File)
		}
	}
	require.Positive(t, totalFiles, "expected at least one Tier A file to be swept")

	// Committed exemplars must not crash/hang/OOM/drift.
	require.Zero(t, rep.Totals[string(Crash)], "no crashes on clean Tier A files")
	require.Zero(t, rep.Totals[string(Hang)])
	require.Zero(t, rep.Totals[string(OOM)])
	require.Zero(t, rep.Totals[string(RoundtripDrift)])

	// output_sha is a stable evidence hash.
	require.NotEmpty(t, rep.OutputSHA)
	rep2, err := d.Run()
	require.NoError(t, err)
	require.Equal(t, rep.OutputSHA, rep2.OutputSHA, "output_sha must be reproducible")
}

// TestDriverHangKill verifies the wall-clock guard: a worker that never returns
// is killed and classified HANG (not left to block the sweep).
func TestDriverHangKill(t *testing.T) {
	d := testDriver(t, []string{"json"})
	d.Timeout = 300 * time.Millisecond
	d.WorkerEnv = append(d.WorkerEnv, "CORPUS_SWEEP_TEST_HANG=1")

	rep, err := d.Run()
	require.NoError(t, err)
	require.Positive(t, rep.Totals[string(Hang)], "a permahang worker must be killed and counted HANG")
	require.Zero(t, rep.Totals[string(OKRoundTrip)]+rep.Totals[string(OK)])
}

// TestDriverPromotion verifies a crasher is promoted into the fuzz seed dir as
// a valid `go test fuzz v1` seed with a suggested origin:bug manifest line —
// driven by a forced HANG so we don't depend on a real corpus crasher.
func TestDriverPromotion(t *testing.T) {
	d := testDriver(t, []string{"json"})
	d.Timeout = 300 * time.Millisecond
	d.WorkerEnv = append(d.WorkerEnv, "CORPUS_SWEEP_TEST_HANG=1")
	d.Promote = true

	rep, err := d.Run()
	require.NoError(t, err)

	var proms []promotion
	for _, fr := range rep.Formats {
		proms = append(proms, fr.Promotions...)
	}
	require.NotEmpty(t, proms, "a HANG must be promoted")
	p := proms[0]
	require.Equal(t, string(Hang), p.Class)
	require.Contains(t, p.SeedFile, filepath.ToSlash("testdata/fuzz/FuzzReadJson/"))
	require.Contains(t, p.ManifestYAML, "origin: bug")

	// The seed file exists under the temp promote dir and is a valid fuzz seed.
	seedAbs := filepath.Join(d.PromoteDir, filepath.FromSlash(p.SeedFile))
	body, err := os.ReadFile(seedAbs)
	require.NoError(t, err)
	require.Contains(t, string(body), "go test fuzz v1")
	require.Contains(t, string(body), "[]byte(")
}

// TestEnumerateFallsBackToTierA confirms the Tier-B-empty smoke fallback: with
// no fetched corpus, enumerate yields the committed Tier A testdata and flags
// tierBEmpty.
func TestEnumerateFallsBackToTierA(t *testing.T) {
	repoRoot, err := findRepoRoot()
	require.NoError(t, err)
	files, tierBEmpty, err := enumerate(repoRoot, "json")
	require.NoError(t, err)
	require.True(t, tierBEmpty)
	require.NotEmpty(t, files)
	for _, f := range files {
		require.Equal(t, "A", f.Tier)
		require.FileExists(t, f.AbsPath)
	}
}
