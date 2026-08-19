package server

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingStore counts the block reads a dashboard load issues, so a test can
// assert what the request asked the database for rather than how long it took.
// Wall clock on a shared runner measures the runner; the number of blocks a
// request hydrates measures the defect.
type countingStore struct {
	platstore.ContentStore
	getBlocks      atomic.Int64 // GetBlocks calls
	hydratedBlocks atomic.Int64 // blocks returned by those calls
}

// Unwrap keeps the optional capabilities of the store underneath reachable, so
// the counter measures the request the server makes rather than a degraded one.
func (c *countingStore) Unwrap() platstore.ContentStore { return c.ContentStore }

func (c *countingStore) GetBlocks(ctx context.Context, q platstore.BlockQuery) ([]*venue.StoredBlock, error) {
	c.getBlocks.Add(1)
	out, err := c.ContentStore.GetBlocks(ctx, q)
	c.hydratedBlocks.Add(int64(len(out)))
	return out, err
}

// reset zeroes the counters so one load can be measured on its own.
func (c *countingStore) reset() {
	c.getBlocks.Store(0)
	c.hydratedBlocks.Store(0)
}

const scaleParagraph = "The reader parses the document into the one content model, and the writer " +
	"puts every byte it did not change back where it found it. A block carries its runs, " +
	"its properties and the overlays that stand off from them, so a surface can project " +
	"the content it shows without walking the sequence by hand. This paragraph exists to " +
	"give the corpus the shape of real prose rather than a three-word fixture."

// scaleBlock builds one paragraph-sized translated block, so a corpus has the
// weight of real content: a dashboard over three-word fixtures cannot show what
// hydrating a project costs.
func scaleBlock(name string, item, index int, locales []model.LocaleID, revision string) *model.Block {
	b := &model.Block{
		ID:           fmt.Sprintf("%s-b-%04d-%04d", name, item, index),
		Translatable: true,
		Properties:   map[string]string{"index": strconv.Itoa(index)},
	}
	b.SetSourceText(fmt.Sprintf("%s (paragraph %d of item %d%s)", scaleParagraph, index, item, revision))
	for _, loc := range locales {
		b.SetTargetText(loc, fmt.Sprintf("%s (avsnitt %d av %d%s) [%s]", scaleParagraph, index, item, revision, loc))
		b.StampTargetProvenance(loc, model.TargetStatusReviewed, model.Origin{Kind: model.OriginHuman})
	}
	return b
}

func scaleItemName(name string, item int) string {
	return fmt.Sprintf("%s-item-%04d.md", name, item)
}

// seedScaleProject stores items × blocksPerItem translatable blocks, each with
// a paragraph-sized source and a reviewed target in every locale.
func seedScaleProject(t *testing.T, cs *bstore.PostgresStore, name string, items, blocksPerItem int, locales []model.LocaleID) string {
	t.Helper()
	ctx := t.Context()
	proj := &platstore.Project{
		Name:                  name,
		DefaultSourceLanguage: "en",
		TargetLanguages:       locales,
		WorkspaceID:           "ws-scale",
		Properties:            map[string]string{},
	}
	require.NoError(t, cs.CreateProject(ctx, proj))

	for i := range items {
		itemName := scaleItemName(name, i)
		require.NoError(t, cs.StoreItem(ctx, proj.ID, "main", &platstore.Item{
			Name: itemName, Format: "markdown", ItemType: "file",
			CollectionID: fmt.Sprintf("col-%d", i%2),
		}))
		blocks := make([]*model.Block, 0, blocksPerItem)
		for j := range blocksPerItem {
			blocks = append(blocks, scaleBlock(name, i, j, locales, ""))
		}
		require.NoError(t, cs.StoreBlocksForItem(ctx, proj.ID, "main", itemName, blocks))
	}
	return proj.ID
}

// dashboardLoad runs one dashboard derivation end to end — the two halves the
// endpoint runs on a cache miss — and returns the blocks it hydrated.
func dashboardLoad(t *testing.T, cs *countingStore, projectID string) (*platstore.TranslationDashboardStats, int64) {
	t.Helper()
	ctx := t.Context()
	cs.reset()
	proj, err := cs.GetProject(ctx, projectID)
	require.NoError(t, err)
	stats, err := editorGetDashboardStats(ctx, cs, proj, "main")
	require.NoError(t, err)
	require.NoError(t, applyShipStates(ctx, cs, nil, projectID, "main", nil, stats))
	return stats, cs.hydratedBlocks.Load()
}

// TestDashboardWorkDoesNotGrowWithProjectSize is the guard on the defect: a
// dashboard load must cost what the answer costs, not what the customer's
// corpus costs.
//
// It asserts a complexity, not a duration — how many blocks the request
// hydrated, over two projects one of which is four times the other. A wall-clock
// threshold would measure the runner and would pass on a fast one while the read
// was unbounded again; the block count cannot.
//
// The first load over a project judges every translated pair, because no verdict
// for them exists yet; that is the one load that is allowed to scale, and it is
// asserted to scale so the test says plainly which cost is which. Every load
// after it reads the blocks that CHANGED and no others — none at all when
// nothing has.
func TestDashboardWorkDoesNotGrowWithProjectSize(t *testing.T) {
	db := pgtest.NewTestDB(t)
	base, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	locales := []model.LocaleID{"nb", "de"}

	const perItem = 20
	sizes := []struct {
		name  string
		items int
	}{
		{"small", 5},
		{"large", 20},
	}

	cs := &countingStore{ContentStore: base}
	warm := map[string]int64{}
	delta := map[string]int64{}
	for _, size := range sizes {
		pid := seedScaleProject(t, base, size.name, size.items, perItem, locales)

		_, cold := dashboardLoad(t, cs, pid)
		assert.EqualValues(t, size.items*perItem, cold,
			"%s: the first load judges every translated pair", size.name)

		_, warm[size.name] = dashboardLoad(t, cs, pid)

		// One item rewritten: the delta an ordinary push leaves behind.
		blocks := make([]*model.Block, 0, perItem)
		for j := range perItem {
			blocks = append(blocks, scaleBlock(size.name, 0, j, locales, " rewritten"))
		}
		require.NoError(t, base.StoreBlocksForItem(t.Context(), pid, "main", scaleItemName(size.name, 0), blocks))
		_, delta[size.name] = dashboardLoad(t, cs, pid)
	}

	assert.Zero(t, warm["small"], "a load over an unchanged project must read no blocks")
	assert.Zero(t, warm["large"], "a load over an unchanged project must read no blocks")
	assert.EqualValues(t, perItem, delta["small"], "a load after one item changed reads that item")
	assert.Equal(t, delta["small"], delta["large"],
		"the same edit must cost the same, whatever else the project holds")
}

// TestDashboardVerdictsFollowTheContentTheyJudged asserts the other half of the
// bargain: a stored verdict is an optimisation only while it is true. A target
// rewritten into one that fails a check must be reported as failing on the very
// next load, with no cache to expire and nothing to invalidate by hand.
func TestDashboardVerdictsFollowTheContentTheyJudged(t *testing.T) {
	db := pgtest.NewTestDB(t)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	ctx := t.Context()

	proj := &platstore.Project{
		Name: "verdict-proj", DefaultSourceLanguage: "en",
		TargetLanguages: []model.LocaleID{"fr"}, WorkspaceID: "ws-1",
		Properties: map[string]string{},
	}
	require.NoError(t, cs.CreateProject(ctx, proj))
	require.NoError(t, cs.StoreItem(ctx, proj.ID, "main", &platstore.Item{
		Name: "a.json", Format: "json", ItemType: "file", CollectionID: "col-a",
	}))

	clean := &model.Block{ID: "b1", Translatable: true, Source: []model.Run{textRun("Hello "), phRun()}}
	clean.SetTargetRuns("fr", []model.Run{textRun("Bonjour "), phRun()})
	clean.StampTargetProvenance("fr", model.TargetStatusReviewed, model.Origin{Kind: model.OriginHuman})
	require.NoError(t, cs.StoreBlocksForItem(ctx, proj.ID, "main", "a.json", []*model.Block{clean}))

	load := func() platstore.LocaleTranslationStats {
		stats, err := editorGetDashboardStats(ctx, cs, proj, "main")
		require.NoError(t, err)
		require.NoError(t, applyShipStates(ctx, cs, nil, proj.ID, "main", nil, stats))
		return localeByCode(t, stats.LocaleStats, "fr")
	}

	require.Zero(t, load().FailingChecks, "a target that keeps its placeholder passes")
	require.Zero(t, load().FailingChecks, "and still passes when the verdict is read rather than recomputed")

	// The same block, its target rewritten to drop the placeholder — the
	// error-severity finding the ship gate exists to catch.
	broken := &model.Block{ID: "b1", Translatable: true, Source: []model.Run{textRun("Hello "), phRun()}}
	broken.SetTargetText("fr", "Bonjour")
	broken.StampTargetProvenance("fr", model.TargetStatusReviewed, model.Origin{Kind: model.OriginHuman})
	require.NoError(t, cs.StoreBlocksForItem(ctx, proj.ID, "main", "a.json", []*model.Block{broken}))

	assert.Equal(t, 1, load().FailingChecks, "a rewritten target is judged again, not remembered")
}

// TestDashboardScaleMeasure reproduces the production defect at a realistic
// corpus size and prints where a load's time, block reads and allocations go.
// It is a measurement, not an assertion, and it seeds tens of thousands of
// blocks, so it runs on request:
//
//	DASHBOARD_SCALE=1 go test ./bowrain/server/ -run TestDashboardScaleMeasure -v
func TestDashboardScaleMeasure(t *testing.T) {
	if os.Getenv("DASHBOARD_SCALE") == "" {
		t.Skip("set DASHBOARD_SCALE=1 to measure a realistic corpus")
	}
	db := pgtest.NewTestDB(t)
	base, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	locales := []model.LocaleID{"nb", "de", "fr"}
	cs := &countingStore{ContentStore: base}

	for _, size := range []struct {
		name  string
		items int
		per   int
	}{{"small", 50, 100}, {"large", 200, 100}} {
		seedStart := time.Now()
		pid := seedScaleProject(t, base, size.name, size.items, size.per, locales)
		t.Logf("[%s] seeded %d blocks × %d locales in %s",
			size.name, size.items*size.per, len(locales), time.Since(seedStart))

		load := func(label string) {
			var m0, m1 runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&m0)
			start := time.Now()
			_, hydrated := dashboardLoad(t, cs, pid)
			elapsed := time.Since(start)
			runtime.ReadMemStats(&m1)
			t.Logf("[%s/%s] %s | GetBlocks=%d blocks hydrated=%d | allocated %d MiB",
				size.name, label, elapsed, cs.getBlocks.Load(), hydrated,
				(m1.TotalAlloc-m0.TotalAlloc)/(1<<20))
		}

		load("cold")
		// Bulk-loaded tables carry no statistics, so a load measured straight
		// after the seed measures the planner guessing. Production has been
		// analyzed long before anyone opens a dashboard.
		_, err = db.ExecContext(t.Context(), "ANALYZE")
		require.NoError(t, err)
		load("warm")

		blocks := make([]*model.Block, 0, size.per)
		for j := range size.per {
			blocks = append(blocks, scaleBlock(size.name, 0, j, locales, " rewritten"))
		}
		require.NoError(t, base.StoreBlocksForItem(t.Context(), pid, "main", scaleItemName(size.name, 0), blocks))
		load("after one item changed")
	}
}
