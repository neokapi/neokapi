package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
)

func requireNoReaderIn(t *testing.T, warnings []check.Warning, file, format string) {
	t.Helper()
	for _, w := range warnings {
		if w.Code == check.WarningFormatNoReader && w.Source == file {
			assert.Contains(t, w.Message, `"`+format+`"`)
			assert.Contains(t, w.Message, "kapi plugins install "+format)
			return
		}
	}
	t.Fatalf("no format.no_reader warning names %s: %+v", file, warnings)
}

// The Review page and the convergence report read the project's translations.
// A translated collection in a plugin format the machine lacks is left out and
// named in the data both return, and the readable translation is still listed.
func TestReviewQueueSkipsTranslatedContentWithNoReader(t *testing.T) {
	isolateCheckPlugins(t)
	app := NewApp()
	tabID := unreadCheckProject(t, app, `{"greeting":"Hello world"}`, true)

	q, err := app.ReviewQueue(tabID, ProjectFilter{})
	require.NoError(t, err, "a missing plugin leaves the rest of the queue listable")
	require.NotEmpty(t, q.Pending, "the readable translation awaits review")
	for _, it := range q.Pending {
		assert.NotContains(t, it.File, "doc", "a set-aside file lists no unit")
	}
	requireNoReaderIn(t, q.Warnings, "pkg/doc.idml", "okf_idml")

	rep, err := app.GetConvergence(tabID)
	require.NoError(t, err)
	require.NotEmpty(t, rep.Review)
	requireNoReaderIn(t, rep.Warnings, "pkg/doc.idml", "okf_idml")
}
