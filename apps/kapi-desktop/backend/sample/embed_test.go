package sample

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestList(t *testing.T) {
	names := List()
	assert.Equal(t, []string{"kapimart"}, names)
}

func TestDisplayName(t *testing.T) {
	assert.Equal(t, "KapiMart", DisplayName["kapimart"])
}

func TestScaffoldKapiMart(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Scaffold("kapimart", dir))

	// Validate project file.
	proj, err := project.Load(filepath.Join(dir, "kapi.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "KapiMart", proj.Name)
	assert.Equal(t, model.LocaleID("en"), proj.Defaults.SourceLanguage)
	assert.Equal(t, []model.LocaleID{"de", "fr", "ja", "nb", "ar"}, proj.Defaults.TargetLanguages)

	// 4 named content collections.
	require.Len(t, proj.Collections, 4)
	assert.Equal(t, "Website", proj.Collections[0].Name)
	assert.Equal(t, "Online Store", proj.Collections[1].Name)
	assert.Equal(t, "Contracts", proj.Collections[2].Name)
	assert.Equal(t, "Templates", proj.Collections[3].Name)

	// 3 flows.
	assert.NotEmpty(t, proj.Flows)

	// Source file counts per area (natural layout: <area>/en/…).
	assertDirCount(t, filepath.Join(dir, "web", "en"), 7)
	assertDirCount(t, filepath.Join(dir, "src", "en"), 5)
	assertDirCount(t, filepath.Join(dir, "legal", "en"), 2)
	assertDirCount(t, filepath.Join(dir, "marketing", "en"), 2)

	// No separate output/ tree — localized files land beside source in locale dirs.
	_, err = os.Stat(filepath.Join(dir, "output"))
	require.True(t, os.IsNotExist(err), "KapiMart must not scaffold an output/ dir")

	// The seed landed in the project's one store; both subsystems come off the
	// same handle, and closing it releases both.
	db, err := projectdb.Open(t.Context(), project.Layout{
		Root: dir, StateDir: filepath.Join(dir, project.StateDirName),
	})
	require.NoError(t, err)
	defer db.Close()

	// The memory is a projection of approvals rather than an imported corpus, so
	// it is small on purpose: every entry is one the record taught it.
	memoryCount, err := db.Memory().Count(t.Context())
	require.NoError(t, err)
	assert.Positive(t, memoryCount, "the sample must scaffold with a content memory")

	// Terms should have 100+ concepts.
	tbCount, err := db.Terms().Count(t.Context())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, tbCount, 100, "terms should have at least 100 concepts")

	entries, err := db.Memory().Entries(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	// Every entry names the block it was approved for. Without it a re-seed
	// returns the corpus as answers approved for nothing, which reads to the
	// matcher as a pile of ad-hoc additions.
	for _, e := range entries {
		assert.NotEmpty(t, e.Unit, "entry %s must name the unit it was approved for", e.ID)
		assert.NotEmpty(t, e.Origins, "entry %s must say where it came from", e.ID)
	}

	// A point is recorded only where one source was answered differently at two
	// of them. The sample carries such a disagreement so the field is exercised.
	var contested int
	for _, e := range entries {
		if e.Point != "" {
			contested++
		}
	}
	assert.Positive(t, contested,
		"the sample must carry a contested source, or nothing exercises a point")

	// Entries arrive through a bulk load, which skips the per-row FTS5 inserts
	// and leaves search returning nothing while exact lookup still works.
	hits, _, err := db.Memory().SearchEntries(t.Context(), memory.SearchParams{
		Query: "Warenkorb", Limit: 50,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, hits, "the seeded content memory must be searchable")
}

// The sample models a project part-way through its work: some of it approved
// and committed, most of it still to do. Both halves have to be true, or the
// convergence hero, the plan and Review open with nothing to show.
func TestScaffoldShipsPartialCoverage(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Scaffold("kapimart", dir))

	shipped, err := filepath.Glob(filepath.Join(dir, "src", "de", "*"))
	require.NoError(t, err)
	assert.NotEmpty(t, shipped, "the sample must ship some translated content")

	// A committed target carries the target language alone. Convergence fills the
	// units it cannot answer with the source text, and a German file that is
	// three-quarters English reads as broken rather than as unfinished.
	source, err := os.ReadFile(filepath.Join(dir, "src", "en", "store-ui.json"))
	require.NoError(t, err)
	target, err := os.ReadFile(filepath.Join(dir, "src", "de", "store-ui.json"))
	require.NoError(t, err)
	assert.Less(t, len(target), len(source),
		"the German catalogue must hold only the keys it has answers for")

	// The documentation is untranslated on purpose: half a translated page is
	// not a page, so prose waits for a translator.
	_, err = os.Stat(filepath.Join(dir, "web", "de"))
	assert.True(t, os.IsNotExist(err), "prose must not ship part-translated")
}

// The approvals that blessed the memory travel with it. Without them the
// entries are answers nobody agreed to.
func TestScaffoldShipsTheUnitStateLedger(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Scaffold("kapimart", dir))

	shards, err := filepath.Glob(filepath.Join(dir, project.StateDirName, "state", "*.jsonl"))
	require.NoError(t, err)
	require.NotEmpty(t, shards, "the sample must ship its unit-state record")

	var rows, approved int
	for _, shard := range shards {
		data, rerr := os.ReadFile(shard)
		require.NoError(t, rerr)
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if line == "" {
				continue
			}
			var row struct {
				Status   string `json:"status"`
				Decision struct {
					ReviewState string `json:"reviewState"`
				} `json:"decision"`
			}
			require.NoError(t, json.Unmarshal([]byte(line), &row))
			rows++
			if row.Decision.ReviewState == "approved" {
				approved++
			}
		}
	}
	assert.Positive(t, rows)
	assert.Positive(t, approved, "the ledger must record approvals, not just states")
}

func TestScaffoldUnknown(t *testing.T) {
	err := Scaffold("unknown", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown sample project")
}

func assertDirCount(t *testing.T, dir string, expectedCount int) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "directory should exist: %s", dir)
	assert.Len(t, entries, expectedCount, "file count in %s", dir)
}
