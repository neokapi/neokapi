package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCLIJSONContract locks the machine-readable CLI contract: the --json
// result documents of run/extract/merge, the JSON error envelope, and the
// --progress jsonl event shape. These are documented as stable
// (web/docs/reference/cli-contract.md) — fields may be ADDED, but existing
// names, shapes, and the human FormatText renderings must not change.
//
// To regenerate the golden files after an intentional, documented change:
//
//	KAPI_UPDATE_GOLDEN=1 go test ./... -run TestCLIJSONContract -tags fts5
//
// and update web/docs/reference/cli-contract.md in the same commit.
func TestCLIJSONContract(t *testing.T) {
	cases := []struct {
		name string
		data any
	}{
		{
			name: "flow_run",
			data: output.FlowRunOutput{
				FlowName:   "pseudo-translate",
				InputPath:  "src/messages.json",
				OutputPath: "src/messages_qps.json",
			},
		},
		{
			name: "flow_run_batch",
			data: output.FlowRunOutput{FlowName: "translate-qa", FilesProcessed: 3},
		},
		{
			name: "flow_run_process_only",
			data: output.FlowRunOutput{
				FlowName:    "translate",
				InputPath:   "src/messages.json",
				ProcessOnly: true,
			},
		},
		{
			name: "extract",
			data: output.ExtractOutput{
				BatchID:  "0b6be731-3a5c-4a02-9e04-2f79e4c2d1aa",
				Format:   "xliff2",
				Targets:  []string{"fr", "de"},
				Sources:  2,
				Manifest: ".kapi/work/cache/extractions/0b6be731/manifest.yaml",
				Pairs: []output.ExtractPairOutput{
					{TargetLocale: "fr", Files: 2, Blocks: 10, Leverage: output.LeverageOutput{Exact: 4, Fuzzy: 1, New: 5}},
					{TargetLocale: "de", Files: 2, Blocks: 10, Leverage: output.LeverageOutput{New: 10}},
				},
				Reused:   1,
				Leverage: output.LeverageOutput{Exact: 4, Fuzzy: 1, New: 15},
			},
		},
		{
			name: "merge",
			data: output.MergeOutput{
				Files: []output.MergeFileOutput{
					{Input: "out/app.en-to-fr.xliff", Applied: 8, Stale: 1, MemoryNew: 6, MemoryUpdated: 2},
					{Input: "out/bad.xliff", Error: "no extraction batch note found"},
				},
				Applied: 8, Stale: 1, MemoryNew: 6, MemoryUpdated: 2,
				ConflictPolicy: "prefer-incoming",
				Failures:       1,
			},
		},
		{
			name: "merge_store",
			data: output.MergeStoreOutput{Written: 4, FromProjectStore: true},
		},
		{
			name: "error_envelope",
			data: output.Error{Error: "quality gate failed", Code: "gate"},
		},
		{
			name: "progress_event",
			data: FlowRunEvent{
				Type: FlowEventProgress, Flow: "extract", Locale: "fr",
				FileIndex: 1, FileCount: 4, FilePath: "src/messages.json",
			},
		},
		{
			name: "progress_complete_event",
			data: FlowRunEvent{
				Type: FlowEventComplete, Flow: "merge",
				DurationMs: 1200, FilesProcessed: 4, Message: "Merged 4 file(s)",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.MarshalIndent(tc.data, "", "  ")
			require.NoError(t, err)
			raw = append(raw, '\n')
			compareGolden(t, filepath.Join("testdata", "contract", tc.name+".golden.json"), raw)
		})
	}
}

// TestCLIContractText pins the human (FormatText) renderings of the same result
// documents.
//
// Unlike the JSON goldens above, text output is NOT a compatibility contract —
// scripts are expected to use --json, and the renderings are restyled when the
// CLI's presentation improves. This test exists so that a restyle is a
// deliberate, reviewed act rather than an accidental side effect of an unrelated
// change. Update the expectations when you mean to; never to make a test pass.
//
// FormatText writes to a bytes.Buffer here, which is not a terminal, so the
// renderer emits no ANSI and these strings stay deterministic.
func TestCLIContractText(t *testing.T) {
	res := output.ExtractOutput{
		BatchID:  "0b6be731-3a5c-4a02-9e04-2f79e4c2d1aa",
		Format:   "xliff2",
		Targets:  []string{"fr", "de"},
		Sources:  2,
		Manifest: ".kapi/work/cache/extractions/0b6be731/manifest.yaml",
		Pairs: []output.ExtractPairOutput{
			{TargetLocale: "fr", Files: 2, Blocks: 10, Leverage: output.LeverageOutput{Exact: 4, Fuzzy: 1, New: 5}},
		},
		Reused:   1,
		Leverage: output.LeverageOutput{Exact: 4, Fuzzy: 1, New: 15},
	}
	var buf bytes.Buffer
	require.NoError(t, res.FormatText(&buf))
	assert.Equal(t,
		"Extracting batch 0b6be731-3a5c-4a02-9e04-2f79e4c2d1aa (format=xliff2, targets=fr, de, sources=2)\n"+
			"\n"+
			"  LOCALE  FILES  BLOCKS  EXACT  FUZZY  NEW\n"+
			"  fr      2      10      4      1      5\n"+
			"\nBatch 0b6be731-3a5c-4a02-9e04-2f79e4c2d1aa complete. Manifest: .kapi/work/cache/extractions/0b6be731/manifest.yaml\n"+
			"Reused 1 unchanged file(s) from a prior batch (no re-parse).\n"+
			"Aggregate content-memory leverage: exact=4 fuzzy=1 new=15 (total=20)\n",
		buf.String())

	mres := output.MergeOutput{
		Files: []output.MergeFileOutput{
			{Input: "out/app.en-to-fr.xliff", Applied: 8, Stale: 1, MemoryNew: 6, MemoryUpdated: 2},
		},
		Applied: 8, Stale: 1, MemoryNew: 6, MemoryUpdated: 2,
		ConflictPolicy: "prefer-incoming",
	}
	buf.Reset()
	require.NoError(t, mres.FormatText(&buf))
	assert.Equal(t,
		"  FILE                    APPLIED  STALE  SKIPPED  content memory NEW  content memory UPDATED\n"+
			"  out/app.en-to-fr.xliff  8        1      0        6                   2\n"+
			"\nMerge complete. applied=8 stale=1 skipped=0 tm_new=6 tm_updated=2 (conflict_policy=prefer-incoming)\n",
		buf.String())

	sres := output.MergeStoreOutput{Written: 4, FromProjectStore: true}
	buf.Reset()
	require.NoError(t, sres.FormatText(&buf))
	assert.Equal(t, "\nMerge complete. wrote=4 file(s) from the project store.\n", buf.String())

	pres := output.FlowRunOutput{FlowName: "translate", InputPath: "src/messages.json", ProcessOnly: true}
	buf.Reset()
	require.NoError(t, pres.FormatText(&buf))
	assert.Equal(t,
		"Committed overlays for messages.json to the project store. Run `kapi merge` to write target files.\n",
		buf.String())
}

// TestErrorEnvelope verifies the {error, code} envelope replaces the plain
// "Error:" line under --json (stderr), with text mode byte-identical to the
// historical rendering.
func TestErrorEnvelope(t *testing.T) {
	run := func(t *testing.T, jsonMode bool) string {
		root := newTestRootCmd(t, jsonMode)
		var errBuf bytes.Buffer
		root.SetErr(&errBuf)
		executed, err := root.ExecuteC()
		require.Error(t, err)
		require.NotNil(t, executed)
		PrintCommandError(executed, err, ExitCode(executed, err))
		return errBuf.String()
	}

	t.Run("json", func(t *testing.T) {
		got := run(t, true)
		var env output.Error
		require.NoError(t, json.Unmarshal([]byte(got), &env), "stderr must be a JSON envelope, got: %q", got)
		assert.Equal(t, "quality gate failed", env.Error)
		assert.Equal(t, "gate", env.Code)
	})

	t.Run("text unchanged", func(t *testing.T) {
		// Text mode goes straight to os.Stderr in PrintCommandError; assert
		// the code path selects it by checking nothing landed on the
		// command's redirected stderr (the envelope writer).
		got := run(t, false)
		assert.Empty(t, got, "text mode must not emit the JSON envelope")
	})
}

func TestErrorCodeString(t *testing.T) {
	assert.Equal(t, "error", ErrorCodeString(ExitError))
	assert.Equal(t, "usage", ErrorCodeString(ExitUsage))
	assert.Equal(t, "gate", ErrorCodeString(ExitGate))
	assert.Equal(t, "signal", ErrorCodeString(ExitSignal))
	assert.Equal(t, "42", ErrorCodeString(42))
}

// newTestRootCmd builds a root+subcommand pair mirroring the kapi wiring
// (persistent output flags on the root) whose subcommand fails the quality
// gate.
func newTestRootCmd(t *testing.T, jsonMode bool) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "kapi-test", SilenceUsage: true, SilenceErrors: true}
	output.AddPersistentFlags(root.PersistentFlags())
	sub := &cobra.Command{
		Use: "gate-fail",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ErrQualityGate
		},
	}
	root.AddCommand(sub)
	args := []string{"gate-fail"}
	if jsonMode {
		args = append(args, "--json")
	}
	root.SetArgs(args)
	return root
}

func compareGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	if os.Getenv("KAPI_UPDATE_GOLDEN") != "" {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, got, 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "golden file missing — run with KAPI_UPDATE_GOLDEN=1 to create it")
	assert.Equal(t, string(want), string(got),
		"CLI JSON contract drift in %s — if intentional, regenerate with KAPI_UPDATE_GOLDEN=1 and update web/docs/reference/cli-contract.md", path)
}
