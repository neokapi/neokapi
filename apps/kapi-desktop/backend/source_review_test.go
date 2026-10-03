package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// readCatalog reads a flat JSON catalog written by the fixture.
func readCatalog(t *testing.T, path string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	out := map[string]string{}
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

// The queue is empty under the default `written` gate and lists everything not
// yet established under `established`; approving one takes it out.
func TestReviewQueue_SourceRowsAndApprove(t *testing.T) {
	app := newAIReviewApp(t, aiprovider.NewMockProvider())
	tab, root := newReviewProject(t, app)

	queue, err := app.ReviewQueue(tab.ID, ProjectFilter{})
	require.NoError(t, err)
	assert.Empty(t, sourceRows(queue.Pending),
		"the default gate asks for checks, not an approval")

	// Raise the gate, reopen so the recipe is re-read.
	recipe := filepath.Join(root, "project.kapi")
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.TranslateAfter = string(model.TranslateAfterEstablished)
	require.NoError(t, project.Save(recipe, proj))

	tab2, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab2.ID) })

	queue, err = app.ReviewQueue(tab2.ID, ProjectFilter{})
	require.NoError(t, err)
	rows := sourceRows(queue.Pending)
	require.NotEmpty(t, rows, "an established gate asks a person to approve every unit")
	for _, it := range rows {
		assert.True(t, it.Held)
		assert.NotEqual(t, string(model.SourceStatusEstablished), it.Status)
		assert.Equal(t, "en-US", it.Language, "a source row belongs to the source language")
		assert.Nil(t, it.HasFindings, "a source row has no translation to check")
	}
	before := len(rows)

	// The source language carries its own pending count in the summary, so the
	// language selector can offer source review beside the targets.
	var srcLang *host.ReviewLanguage
	for i, l := range queue.Languages {
		if l.Source {
			srcLang = &queue.Languages[i]
		}
	}
	require.NotNil(t, srcLang, "the summary marks the source language")
	assert.Equal(t, before, srcLang.Pending)

	src := blockVia(t, app, tab2.ID, filepath.ToSlash(rows[0].File), rows[0].Key)
	res := applyVia(t, app, tab2.ID, change.Set{Ops: []change.Op{decideOp(src.Ref, src.Rev, change.OutcomeEstablish)}})
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	after, err := app.ReviewQueue(tab2.ID, ProjectFilter{})
	require.NoError(t, err)
	assert.Len(t, sourceRows(after.Pending), before-1, "the approved unit leaves the queue")
}

// sourceRows is the source half of a unified queue.
func sourceRows(items []host.ReviewQueueItem) []host.ReviewQueueItem {
	var out []host.ReviewQueueItem
	for _, it := range items {
		if it.IsSource {
			out = append(out, it)
		}
	}
	return out
}
