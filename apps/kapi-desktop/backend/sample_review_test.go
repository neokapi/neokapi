package backend

import (
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/kapi-desktop/backend/sample"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A scaffolded KapiMart opens with work in the review queue. The message
// catalogue ships translated and unapproved, so the queue lists it, the detail
// pane has a unit to draw, and Approve and Send back have something to act on.
// Every other committed target ships approved, so the queue holds that one
// source and nothing else.
func TestSampleOpensWithAReviewQueue(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, sample.Scaffold("kapimart", dir))

	app := NewApp()
	tab, err := app.OpenProject(filepath.Join(dir, "kapi.yaml"))
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	queue, err := app.ReviewQueue(tab.ID, ProjectFilter{})
	require.NoError(t, err)
	require.NotEmpty(t, queue.Pending, "the sample must open with a review queue")

	perLocale := map[string]int{}
	for _, it := range queue.Pending {
		assert.Equal(t, "src/en/error-messages.properties", it.Relative,
			"only the message catalogue ships unreviewed")
		assert.Equal(t, "Online Store", it.Collection)
		assert.NotEmpty(t, it.Source)
		assert.NotEmpty(t, it.Target)
		perLocale[it.Locale]++
	}
	assert.Equal(t, map[string]int{"ar": 4, "de": 4, "fr": 4, "ja": 4, "nb": 4}, perLocale,
		"every target language must have the catalogue's four units pending")
}
