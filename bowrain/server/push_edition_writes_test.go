package server

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/auth"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
)

// The server half of the edition writes a push carries beside its decisions:
// the real client against the real server, and the worker landing each push in
// PostgreSQL with the review authority a deployed worker resolves permissions
// and the workspace policy through. The client half (what a checkout reads
// from its block history and sends) is tested in host/venue/source.

// drainWithAuthority processes every queued push as a deployed worker does.
func drainWithAuthority(t *testing.T, srv *Server) {
	t.Helper()
	for {
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		jobID, ack, _, err := srv.JobQueue.Dequeue(ctx)
		cancel()
		if err != nil || jobID == "" {
			return
		}
		deps := &jobs.WorkerDeps{
			JobStore: srv.JobStore, ContentStore: srv.ContentStore, BlobStore: srv.BlobStore, Queue: srv.JobQueue,
			ReviewAuthority: auth.NewReviewAuthority(srv.AuthStore),
		}
		require.NoError(t, jobs.ProcessSyncPushJobForTest(t.Context(), deps, jobID))
		ack()
	}
}

// catalog is the source document a checkout pushes: one block per key.
func catalog(texts map[string]string) map[string][]*model.Block {
	var blocks []*model.Block
	for _, key := range []string{"greeting", "farewell", "title"} {
		text, ok := texts[key]
		if !ok {
			continue
		}
		b := &model.Block{ID: key, Name: key, Translatable: true}
		b.SetSourceText(text)
		blocks = append(blocks, b)
	}
	return map[string][]*model.Block{"locales/en.json": blocks}
}

var catalogItems = []apiclient.ItemMeta{{Name: "locales/en.json", Format: "json"}}

// produced is a run's write of the Norwegian translation of unit, made from
// source.
func produced(unit, source string) venue.EditionWrite {
	return venue.EditionWrite{
		ItemName: "locales/en.json", Unit: unit, Variant: "nb", Revision: "r:" + unit + "-nb",
		Basis: state.SourceHash(source), Writer: venue.WriterTool, Origin: "flow:pseudo",
	}
}

func nbTallyOf(t *testing.T, srv *Server, pid string) platstore.DecisionBasisTally {
	t.Helper()
	ds, ok := srv.ContentStore.(platstore.DecisionStore)
	require.True(t, ok)
	tallies, err := ds.TallyDecisionBasis(t.Context(), pid, "main")
	require.NoError(t, err)
	var out platstore.DecisionBasisTally
	for _, tl := range tallies {
		if tl.Variant == "nb" {
			out.Stale += tl.Stale
			out.BasisUnknown += tl.BasisUnknown
		}
	}
	return out
}

func nbLedger(t *testing.T, srv *Server, pid string) map[string]venue.UnitDecision {
	t.Helper()
	ds, ok := srv.ContentStore.(platstore.DecisionStore)
	require.True(t, ok)
	records, err := ds.ListUnitDecisions(t.Context(), pid, "main")
	require.NoError(t, err)
	out := map[string]venue.UnitDecision{}
	for _, d := range records {
		if d.Variant == "nb" {
			out[d.Unit] = d
		}
	}
	return out
}

// A translation a run on a checkout produced reaches the server through the
// push as the basis the run recorded, and the server grades it stale once its
// source moves, as it grades a draft of its own.
func TestSyncPush_GradesALocalRunsTranslationsAgainstTheirSource(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createProject(t, srv, token)
	ts := httptest.NewServer(srv.GetEcho())
	defer ts.Close()
	client := apiclient.NewProjectBearerClient(ts.URL, pid, token)
	ctx := context.Background()

	_, err := client.Push(ctx, catalog(map[string]string{"greeting": "Hello world", "farewell": "Goodbye now"}), catalogItems, nil, nil)
	require.NoError(t, err)
	drainWithAuthority(t, srv)

	// kapi up wrote both translations on the checkout, and changed no source
	// block: the push carries only how they were written.
	_, err = client.Push(ctx, map[string][]*model.Block{}, nil, nil, nil, apiclient.CarryEditionWrites([]venue.EditionWrite{
		produced("greeting", "Hello world"), produced("farewell", "Goodbye now"),
	}))
	require.NoError(t, err)
	drainWithAuthority(t, srv)

	records := nbLedger(t, srv, pid)
	require.Len(t, records, 2, "a record of each translation the run produced")
	for unit, d := range records {
		assert.False(t, d.IsDecision(), "%s: the run decided nothing", unit)
	}
	assert.Zero(t, nbTallyOf(t, srv, pid).Stale)

	// The source of one moves on the server.
	_, err = client.Push(ctx, catalog(map[string]string{"farewell": "Goodbye for now"}), catalogItems, nil, nil)
	require.NoError(t, err)
	drainWithAuthority(t, srv)
	assert.Equal(t, 1, nbTallyOf(t, srv, pid).Stale,
		"the translation the local run made of the old wording reads stale")
}

// Separation of duties holds a reviewer to the translations they wrote, by
// whichever surface they wrote them. A push says, beside the approvals it
// carries, which translations the pusher wrote by hand; an approval of one of
// those is not accepted under a workspace policy that blocks it, and an
// approval of a translation a run produced is.
func TestSyncPush_RefusesAnApprovalOfTheTranslationThePusherWrote(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createProject(t, srv, token)
	ts := httptest.NewServer(srv.GetEcho())
	defer ts.Close()
	require.NoError(t, srv.AuthStore.SetSoDMode(t.Context(), "test-ws", platauth.SoDBlock))
	client := apiclient.NewProjectBearerClient(ts.URL, pid, token)
	ctx := context.Background()

	blocks := catalog(map[string]string{"greeting": "Hello world", "farewell": "Goodbye now", "title": "Welcome"})
	// The venue knows the title by a key of its own; the checkout's decisions
	// name it by the key its reader gives it.
	blocks["locales/en.json"][2].Unit = "u-title"
	_, err := client.Push(ctx, blocks, catalogItems, nil, nil)
	require.NoError(t, err)
	drainWithAuthority(t, srv)

	approval := func(unit, source, target string) venue.UnitDecision {
		return venue.UnitDecision{
			ItemName: "locales/en.json", Unit: unit, Variant: "nb",
			Status: string(model.TargetStatusEstablished), ReviewState: venue.ReviewStateApproved,
			TargetHash: state.TargetHash(target), ContentHash: state.SourceHash(source),
			Updated: time.Now().UTC().Format(time.RFC3339),
		}
	}
	byHand := func(unit, origin string) venue.EditionWrite {
		return venue.EditionWrite{ItemName: "locales/en.json", Unit: unit, Variant: "nb", Revision: "r:" + unit,
			Writer: venue.WriterPerson, Origin: origin}
	}
	title := byHand("u-title", "desktop")
	title.Block = "title"

	resp, err := client.Push(ctx, map[string][]*model.Block{}, nil, nil, []venue.UnitDecision{
		approval("greeting", "Hello world", "Hei, verden"),
		approval("farewell", "Goodbye now", "Ha det"),
		approval("title", "Welcome", "Velkommen"),
	}, apiclient.CarryEditionWrites([]venue.EditionWrite{
		byHand("greeting", "apply"), produced("farewell", "Goodbye now"), title,
	}))
	require.NoError(t, err)
	drainWithAuthority(t, srv)

	status, err := client.PushStatus(ctx, resp.PushID)
	require.NoError(t, err)
	require.NotNil(t, status.Governance, "the server reports the approvals it did not accept")
	refused := map[string]string{}
	for _, u := range status.Governance.Units {
		refused[u.Unit] = u.Reason
	}
	assert.Equal(t, venue.RefusedSeparationOfDuties, refused["greeting"], "the pusher wrote it in kapi apply")
	assert.Equal(t, venue.RefusedSeparationOfDuties, refused["title"], "the pusher wrote it in Kapi Desktop")
	assert.NotContains(t, refused, "farewell", "a run produced it")

	records := nbLedger(t, srv, pid)
	assert.Empty(t, records["greeting"].ReviewState)
	assert.Empty(t, records["title"].ReviewState)
	assert.Equal(t, venue.ReviewStateApproved, records["farewell"].ReviewState)
}
