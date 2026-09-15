package jobs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
)

// A push counts as in flight while it is queued or being applied, on its own
// project's stream. Translation jobs a push fans out, finished pushes, and
// pushes to another project or stream are not in flight for it.
func TestCountActivePushApplies(t *testing.T) {
	js, err := NewJobStore(pgtest.NewTestDB(t))
	require.NoError(t, err)
	ctx := t.Context()

	create := func(id, projectID, itemName, stream string) {
		t.Helper()
		require.NoError(t, js.CreateJob(ctx, &TranslationJob{
			ID: id, ProjectID: projectID, ItemName: itemName, Stream: stream,
			PushID: "push-" + id, Model: "manifest", Status: StatusQueued,
		}))
	}
	create("queued", "p1", SyncPushItemName, "")
	create("applying", "p1", SyncPushItemName, "main")
	claimed, _, err := js.ClaimJob(ctx, "applying")
	require.NoError(t, err)
	require.True(t, claimed)
	create("applied", "p1", SyncPushItemName, "")
	require.NoError(t, js.UpdateJobStatus(ctx, "applied", StatusCompleted, ""))
	create("translation", "p1", "en.json", "")
	create("other-project", "p2", SyncPushItemName, "")
	create("other-stream", "p1", SyncPushItemName, "feature")

	n, err := js.CountActivePushApplies(ctx, "p1", "main")
	require.NoError(t, err)
	assert.Equal(t, 2, n, "a queued push and a push being applied, on the project's main stream")

	n, err = js.CountActivePushApplies(ctx, "p1", "")
	require.NoError(t, err)
	assert.Equal(t, 2, n, "an empty stream is main")

	n, err = js.CountActivePushApplies(ctx, "p1", "feature")
	require.NoError(t, err)
	assert.Equal(t, 1, n, "a push to another stream is in flight only for that stream")
}
