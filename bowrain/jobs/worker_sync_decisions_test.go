package jobs

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/core/store"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
)

// A push whose decision records say only what was produced asserts nothing
// about the decisions component, in the worker as in the commit handler. The
// records merge by record time, and a server run writes its own between any
// client's pull and push, so an assertion over them would refuse a push that
// overwrites no decision.
func TestAPushCarryingNoDecisionAssertsNothingInTheWorker(t *testing.T) {
	deps := newTestWorkerDeps(t)
	ctx := t.Context()

	projectID := "bases-only-project"
	require.NoError(t, deps.ContentStore.CreateProject(ctx, &store.Project{ID: projectID, Name: "Bases"}))

	bases, err := json.Marshal([]map[string]any{{
		"item": "en.json", "unit": "b1", "variant": "nb",
		"targetHash": "sha256:t1", "contentHash": "sha256:s1", "updated": "2026-09-15T00:00:00Z",
	}})
	require.NoError(t, err)

	job := uploadPush(t, deps, "job-bases-only", projectID, []*pb.SyncBlock{
		{Id: "b1", ItemName: "en.json", SourceText: "Hello", Translatable: true},
	}, map[string]any{
		"decisions":    json.RawMessage(bases),
		"expected_ref": map[string]string{"decisions": "sha256:0000000000000000000000000000000000000000000000000000000000000000"},
	})
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, job.ID),
		"records of what was produced assert nothing about the decisions component")

	ledger, err := deps.ContentStore.(store.DecisionStore).ListUnitDecisions(ctx, projectID, "main")
	require.NoError(t, err)
	require.Len(t, ledger, 1, "the basis is recorded")
}
