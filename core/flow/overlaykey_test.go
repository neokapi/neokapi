package flow_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1846. Overlays are addressed by ONE key whichever run mode wrote them:
// blockstore.StoreKey(sourceRel, blockID, sourceText) in project scope, the bare
// block id outside it. The file-writing path (which convergence takes, so each
// pass's coverage sees real files) used to skip the project namespace entirely,
// so the overlays it committed were invisible to the process-only path, to merge
// and to materialize — and invisibility is indistinguishable from "not translated
// yet", which is why merge wrote the source text back over translated files.
//
// These tests pin the key per path rather than the symptom, so a future run mode
// that forgets to name its file fails here rather than in a deliverable.

// overlayKeyFixture writes a one-block JSON source under `src/` in a fresh
// project root and returns the root, the source path and an open store.
func overlayKeyFixture(t *testing.T) (root, srcPath string, store blockstore.Store, reg *registry.FormatRegistry) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	srcPath = filepath.Join(root, "src", "en.json")
	require.NoError(t, os.WriteFile(srcPath, []byte(`{"greeting":"Hello world"}`), 0o644))

	reg = registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	s, err := sqlitestore.New(filepath.Join(root, "blocks.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	require.True(t, s.Capabilities().Persistent)
	return root, srcPath, s, reg
}

// overlayKeys returns every key holding a `targets/<locale>` overlay.
func overlayKeys(t *testing.T, store blockstore.Store, locale string) []string {
	t.Helper()
	sess, err := store.Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()
	var keys []string
	for ov, err := range sess.ListOverlays("targets/" + locale) {
		require.NoError(t, err)
		require.NotEmpty(t, ov.Payload)
		keys = append(keys, ov.BlockHash)
	}
	return keys
}

func pseudoTools(t *testing.T, locale string) []tool.Tool {
	t.Helper()
	pseudo, err := tools.NewPseudoTranslateFromConfig(map[string]any{"target_locale": locale}, locale)
	require.NoError(t, err)
	return []tool.Tool{pseudo}
}

// TestOverlayKey_EveryWritePathAgrees is the core pin: a file-writing run and a
// process-only run over the same project source commit their overlays under the
// same key, and that key is the source-file-namespaced StoreKey the readers ask
// for.
func TestOverlayKey_EveryWritePathAgrees(t *testing.T) {
	want := ""
	for _, tc := range []struct {
		name string
		run  func(t *testing.T, r *flow.FileRunner, root, srcPath string) error
	}{
		{
			name: "file-writing (what convergence runs)",
			run: func(t *testing.T, r *flow.FileRunner, root, srcPath string) error {
				return r.RunFile(context.Background(), "pseudo-translate", pseudoTools(t, "qps"),
					srcPath, filepath.Join(root, "out", "qps.json"), "qps")
			},
		},
		{
			name: "process-only",
			run: func(t *testing.T, r *flow.FileRunner, root, srcPath string) error {
				return r.RunFileProcessOnly(context.Background(), "pseudo-translate",
					pseudoTools(t, "qps"), srcPath, "qps")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, srcPath, store, reg := overlayKeyFixture(t)
			runner := flow.NewFileRunner(flow.FileRunnerConfig{
				FormatReg:    reg,
				SourceLocale: "en",
				Store:        store,
				ProjectRoot:  root,
			})
			require.NoError(t, tc.run(t, runner, root, srcPath))

			keys := overlayKeys(t, store, "qps")
			require.Len(t, keys, 1, "one translatable block, one overlay")
			assert.Equal(t, blockstore.StoreKey(filepath.Join("src", "en.json"), "tu1", "Hello world"), keys[0],
				"an overlay must be addressed by the source file's namespaced key")

			if want == "" {
				want = keys[0]
				return
			}
			assert.Equal(t, want, keys[0], "the two run modes must key one block identically")
		})
	}
}

// TestOverlayKey_ReadPathAddressesTheWrittenKey closes the loop the defect broke:
// what a file-writing run commits, a reader tagged the way merge and materialize
// tag their context finds again.
func TestOverlayKey_ReadPathAddressesTheWrittenKey(t *testing.T) {
	root, srcPath, store, reg := overlayKeyFixture(t)
	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    reg,
		SourceLocale: "en",
		Store:        store,
		ProjectRoot:  root,
	})
	require.NoError(t, runner.RunFile(context.Background(), "pseudo-translate", pseudoTools(t, "qps"),
		srcPath, filepath.Join(root, "out", "qps.json"), "qps"))

	// The reader's half: a context carrying the recipe's spelling of the file,
	// exactly as host/merge.go sets it before hydrating and absorbing.
	readCtx := blockstore.WithSourceRel(context.Background(), filepath.Join("src", "en.json"))
	sess, err := store.Begin(readCtx)
	require.NoError(t, err)
	defer sess.Close()

	ov, err := sess.GetOverlay("targets/qps", blockstore.OverlayKey(readCtx, "tu1", "Hello world"))
	require.NoError(t, err, "merge asked for a key the run never wrote, and a miss reads as untranslated")
	assert.NotEmpty(t, ov.Payload)

	_, err = sess.GetOverlay("targets/qps", "tu1")
	require.ErrorIs(t, err, blockstore.ErrNotFound,
		"the collision-prone file-local id is not a key in project scope")
}

// TestOverlayKey_TwoFilesSharingABlockIDDoNotCollide is why the namespace exists:
// format readers number blocks per file, so `tu1` means a different string in
// every source.
func TestOverlayKey_TwoFilesSharingABlockIDDoNotCollide(t *testing.T) {
	root, srcPath, store, reg := overlayKeyFixture(t)
	second := filepath.Join(root, "src", "more.json")
	require.NoError(t, os.WriteFile(second, []byte(`{"greeting":"Another string entirely"}`), 0o644))

	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    reg,
		SourceLocale: "en",
		Store:        store,
		ProjectRoot:  root,
	})
	for i, in := range []string{srcPath, second} {
		require.NoError(t, runner.RunFile(context.Background(), "pseudo-translate", pseudoTools(t, "qps"),
			in, filepath.Join(root, "out", string(rune('a'+i))+".json"), "qps"))
	}

	keys := overlayKeys(t, store, "qps")
	assert.Len(t, keys, 2, "two files each holding a block called tu1 must hold two overlays")
}

// TestOverlayKey_OutsideTheProjectKeepsItsOwnNamespace states the constraint on
// the write path: an input the project root cannot name still writes its file, and
// still gets a namespace of its own rather than sharing the bare id space. The
// process-only path, whose only deliverable is the overlays, refuses that input
// instead (blockstore.ProjectRel) — the asymmetry is the difference between a run
// that has produced something and one that has not.
func TestOverlayKey_OutsideTheProjectKeepsItsOwnNamespace(t *testing.T) {
	root, _, store, reg := overlayKeyFixture(t)
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "elsewhere.json")
	require.NoError(t, os.WriteFile(outside, []byte(`{"greeting":"Hello world"}`), 0o644))

	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    reg,
		SourceLocale: "en",
		Store:        store,
		ProjectRoot:  root,
	})
	out := filepath.Join(outsideDir, "qps.json")
	require.NoError(t, runner.RunFile(context.Background(), "pseudo-translate", pseudoTools(t, "qps"),
		outside, out, "qps"), "a run that writes a file must not fail over where its cache lands")
	assert.FileExists(t, out)

	keys := overlayKeys(t, store, "qps")
	require.Len(t, keys, 1)
	assert.Equal(t, blockstore.StoreKey(outside, "tu1", "Hello world"), keys[0])
	assert.NotEqual(t, "tu1", keys[0], "the raw id would collide with every other outside file")

	err := runner.RunFileProcessOnly(context.Background(), "pseudo-translate",
		pseudoTools(t, "qps"), outside, "qps")
	require.Error(t, err, "a process-only run has no deliverable but the overlays")
	assert.Contains(t, err.Error(), "could never be collected")
}

// TestOverlayKey_ChannelProducerReachesTheStore covers the tool shape `recycle`
// has — the first step of the built-in convergence flow. It fills the target from
// the content memory through a plain Produce handler and implements no
// SessionTool, so its work reached the written file and nothing else: the store
// still read "never translated", and the next merge wrote the source back over it.
func TestOverlayKey_ChannelProducerReachesTheStore(t *testing.T) {
	root, srcPath, store, reg := overlayKeyFixture(t)
	producer := &tool.BaseTool{
		ToolName: "channel-producer",
		Produce: func(v tool.VariantView) error {
			v.SetTargetText("qps", "Recycled from the content memory")
			return nil
		},
	}

	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    reg,
		SourceLocale: "en",
		Store:        store,
		ProjectRoot:  root,
	})
	out := filepath.Join(root, "out", "qps.json")
	require.NoError(t, runner.RunFile(context.Background(), "recycle-shaped",
		[]tool.Tool{producer}, srcPath, out, "qps"))

	written, err := os.ReadFile(out)
	require.NoError(t, err)
	require.Contains(t, string(written), "Recycled from the content memory")

	keys := overlayKeys(t, store, "qps")
	require.Len(t, keys, 1, "the produced target must be persisted, not only written to the file")
	assert.Equal(t, blockstore.StoreKey(filepath.Join("src", "en.json"), "tu1", "Hello world"), keys[0])
}

// TestSourceNamespace names one file one way whichever spelling reaches it.
func TestSourceNamespace(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "src", "en.json")
	outside := filepath.Join(t.TempDir(), "elsewhere.json")

	assert.Equal(t, filepath.Join("src", "en.json"), blockstore.SourceNamespace(root, inside))
	assert.Equal(t, outside, blockstore.SourceNamespace(root, outside),
		"a file the project cannot name is named by its absolute path")
	assert.NotEqual(t, blockstore.SourceNamespace(root, outside),
		blockstore.SourceNamespace(root, filepath.Join(t.TempDir(), "elsewhere.json")),
		"two out-of-project files must not share a namespace")
}
