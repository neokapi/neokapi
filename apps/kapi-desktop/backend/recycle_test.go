package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSyncProjectStore_RecyclesExactMatches opens a project whose content
// memory holds the French for one of its three strings. The first sync fills
// that target and nothing else, writes it to the target file, and the plan
// reports what it filled.
func TestSyncProjectStore_RecyclesExactMatches(t *testing.T) {
	app := NewApp()
	tab, root := newCoverageProject(t, app)
	db, err := app.projectStore(app.getOpenProject(tab.ID))
	require.NoError(t, err)
	require.NoError(t, db.Memory().Add(t.Context(), memory.Entry{
		ID:          "farewell",
		HintSrcLang: "en-US",
		Variants: map[model.LocaleID][]model.Run{
			"en-US": {{Text: &model.TextRun{Text: "Goodbye"}}},
			"fr-FR": {{Text: &model.TextRun{Text: "Au revoir"}}},
		},
	}))
	var events []string
	InjectEventSink(app, func(name string, _ any) { events = append(events, name) })

	res, err := app.SyncProjectStore(tab.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Recycled)
	assert.Contains(t, events, "project:recycled")

	fr, err := os.ReadFile(filepath.Join(root, "locales", "fr-FR.json"))
	require.NoError(t, err)
	assert.Contains(t, string(fr), "Au revoir")

	plan, err := app.GetConvergePlan(tab.ID)
	require.NoError(t, err)
	require.NotNil(t, plan.Recycled)
	assert.Equal(t, 1, plan.Recycled.Blocks)
	assert.Zero(t, plan.Plan.Totals.MemoryExact, "nothing exact is left for a run")

	again, err := app.SyncProjectStore(tab.ID)
	require.NoError(t, err)
	assert.Zero(t, again.Recycled, "a current cache is not recycled again")
}

// TestSyncProjectStore_RecyclesNothingWithoutAMatch keeps the sync's events
// to the extraction alone when the content memory answers nothing.
func TestSyncProjectStore_RecyclesNothingWithoutAMatch(t *testing.T) {
	app := NewApp()
	tab, root := newCoverageProject(t, app)
	res, err := app.SyncProjectStore(tab.ID)
	require.NoError(t, err)
	assert.Zero(t, res.Recycled)
	_, err = os.Stat(filepath.Join(root, "locales", "fr-FR.json"))
	assert.True(t, os.IsNotExist(err), "no target file is written")
	plan, err := app.GetConvergePlan(tab.ID)
	require.NoError(t, err)
	assert.Nil(t, plan.Recycled)
}
