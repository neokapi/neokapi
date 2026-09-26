package jobs

import (
	"encoding/json"
	"maps"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/core/store"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/venue"
)

// A push's decisions assertion is about the ledger the client observed, so it
// is made against the ledger as it stood before this push wrote anything. A
// committed record can hold a decision for a file the checkout does not have:
// the decision records an item for it, and the next push's declared tree
// removes that item and its decision rows before the ledger is read again. An
// assertion made after that removal compared the client's ref with a ledger the
// push itself had changed, and refused every push that asserted, while a push
// with no assertion applied and put the same decision back.
func TestAPushIsNotRefusedForADecisionItsOwnRemovalTakesAway(t *testing.T) {
	deps := newTestWorkerDeps(t)
	deps.ReviewAuthority = pushAuthority{review: map[string]bool{"nb": true}}
	ctx := t.Context()

	projectID := "decision-order-project"
	require.NoError(t, deps.ContentStore.CreateProject(ctx, &store.Project{ID: projectID, Name: "Decision order"}))

	blocks := []*pb.SyncBlock{{Id: "b1", ItemName: "en.json", SourceText: "Hello", Translatable: true}}
	approval, err := json.Marshal([]map[string]any{{
		"item": "gone.json", "unit": "u1", "variant": "nb",
		"status": "established", "targetHash": "sha256:t1",
		"reviewState": "approved", "by": "ana", "at": "2026-08-06T13:45:44Z", "updated": "2026-08-06T13:45:44Z",
	}})
	require.NoError(t, err)
	declaration := with(declaring([]string{"**"}, treeOf(map[string][]*pb.SyncBlock{"en.json": blocks})),
		map[string]any{"actor_id": "u-reviewer"})

	first := uploadPush(t, deps, "job-decision-order-1", projectID, blocks,
		with(declaration, map[string]any{"decisions": json.RawMessage(approval)}))
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, first.ID))

	ledger, err := deps.ContentStore.(store.DecisionStore).ListUnitDecisions(ctx, projectID, "main")
	require.NoError(t, err)
	observed := venue.DecisionsComponent(ledger)
	require.NotEmpty(t, observed, "the approval is held")

	second := uploadPush(t, deps, "job-decision-order-2", projectID, nil,
		with(declaration, map[string]any{
			"decisions":    json.RawMessage(approval),
			"expected_ref": ref.Ref{Decisions: observed},
		}))
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, second.ID),
		"a client that observed the ledger as it stands is not refused for what its own push removes")
}

// with returns a copy of base carrying the extra manifest fields.
func with(base, extra map[string]any) map[string]any {
	out := maps.Clone(base)
	maps.Copy(out, extra)
	return out
}
