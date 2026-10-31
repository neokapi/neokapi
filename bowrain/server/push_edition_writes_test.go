package server

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/auth"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
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
		Basis: enSourceRevision(source), Writer: venue.WriterTool, Origin: "flow:pseudo",
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

// nbDerivations maps each unit of item to the derivation its Norwegian
// translation records, nil for a translation that records none. A unit with
// no Norwegian translation is absent.
func nbDerivations(t *testing.T, srv *Server, pid, item string) map[string]*model.Derivation {
	t.Helper()
	stored, err := srv.ContentStore.GetBlocks(t.Context(), platstore.BlockQuery{ProjectID: pid, Stream: "main", ItemName: item})
	require.NoError(t, err)
	out := map[string]*model.Derivation{}
	for _, row := range stored {
		e, ok := row.Block.Edition(model.EditionKey{Locale: "nb"})
		if !ok {
			continue
		}
		out[row.SourceID] = e.Derived
	}
	return out
}

// A translation a run on a checkout produced reaches the server through the
// push with the basis the run recorded on its edition, and the server grades
// it stale once its source moves, as it grades a draft of its own.
func TestSyncPush_GradesALocalRunsTranslationsAgainstTheirSource(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createProject(t, srv, token)
	ts := httptest.NewServer(srv.GetEcho())
	defer ts.Close()
	client := apiclient.NewProjectBearerClient(ts.URL, pid, token)
	ctx := context.Background()

	// kapi up wrote both translations on the checkout; the push carries them
	// and how they were written.
	blocks := catalog(map[string]string{"greeting": "Hello world", "farewell": "Goodbye now"})
	nb := model.EditionKey{Locale: "nb"}
	var writes []venue.EditionWrite
	for _, b := range blocks["locales/en.json"] {
		b.SetTargetText("nb", "nb:"+b.ID)
		w := produced(b.ID, b.SourceText())
		w.Revision = model.EditionRevision(b, nb)
		writes = append(writes, w)
	}
	_, err := client.Push(ctx, blocks, catalogItems, nil, nil, apiclient.CarryEditionWrites(writes))
	require.NoError(t, err)
	drainWithAuthority(t, srv)

	derived := nbDerivations(t, srv, pid, "locales/en.json")
	require.Len(t, derived, 2, "the venue holds each translation the run produced")
	for unit, d := range derived {
		require.NotNil(t, d, "%s: the translation records the source it was made from", unit)
	}
	assert.Equal(t, enSourceRevision("Hello world"), derived["greeting"].Rev)
	assert.Empty(t, nbLedger(t, srv, pid), "the run decided nothing, and the ledger holds decisions")
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
	// The venue knows the title by a key of its own, and a push names it by
	// that key in its decisions and its writes alike.
	blocks["locales/en.json"][2].Key = "u-title"
	_, err := client.Push(ctx, blocks, catalogItems, nil, nil)
	require.NoError(t, err)
	drainWithAuthority(t, srv)

	approval := func(unit, source, target string) venue.UnitDecision {
		return venue.UnitDecision{
			ItemName: "locales/en.json", Unit: unit, Variant: "nb",
			Status: string(model.TargetStatusEstablished), ReviewState: venue.ReviewStateApproved,
			Revision: textRevision("nb", target), Basis: enSourceRevision(source),
			Updated: time.Now().UTC().Format(time.RFC3339),
		}
	}
	byHand := func(unit, origin string) venue.EditionWrite {
		return venue.EditionWrite{ItemName: "locales/en.json", Unit: unit, Variant: "nb", Revision: "r:" + unit,
			Writer: venue.WriterPerson, Origin: origin}
	}
	resp, err := client.Push(ctx, map[string][]*model.Block{}, nil, nil, []venue.UnitDecision{
		approval("greeting", "Hello world", "Hei, verden"),
		approval("farewell", "Goodbye now", "Ha det"),
		approval("u-title", "Welcome", "Velkommen"),
	}, apiclient.CarryEditionWrites([]venue.EditionWrite{
		byHand("greeting", "apply"), produced("farewell", "Goodbye now"), byHand("u-title", "desktop"),
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
	assert.Equal(t, venue.RefusedSeparationOfDuties, refused["u-title"], "the pusher wrote it in Kapi Desktop")
	assert.NotContains(t, refused, "farewell", "a run produced it")

	records := nbLedger(t, srv, pid)
	assert.Empty(t, records["greeting"].ReviewState)
	assert.Empty(t, records["u-title"].ReviewState)
	assert.Equal(t, venue.ReviewStateApproved, records["farewell"].ReviewState)
}

// bilingualXLIFF is a bilingual document whose translations carry inline codes:
// paired formatting and a placeholder, which a push carries as runs.
const bilingualXLIFF = `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
<file original="messages" source-language="en" target-language="nb" datatype="plaintext"><body>
<trans-unit id="welcome"><source>Welcome to <g id="1">the shop</g>, <x id="2"/>!</source><target>Velkommen til <g id="1">butikken</g>, <x id="2"/>!</target></trans-unit>
<trans-unit id="close"><source>Close</source><target>Lukk</target></trans-unit>
</body></file>
</xliff>
`

// readBilingual reads bilingualXLIFF through the format's reader, as a
// checkout's read does.
func readBilingual(t *testing.T) []*model.Block {
	t.Helper()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	path := filepath.Join(t.TempDir(), "messages.xlf")
	require.NoError(t, os.WriteFile(path, []byte(bilingualXLIFF), 0o644))
	reader, err := reg.NewReader("xliff")
	require.NoError(t, err)
	defer reader.Close()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	require.NoError(t, reader.Open(t.Context(), &model.RawDocument{URI: path, FormatID: "xliff", Reader: f}))
	var blocks []*model.Block
	for pr := range reader.Read(t.Context()) {
		require.NoError(t, pr.Error)
		if b, ok := pr.Part.Resource.(*model.Block); ok && pr.Part.Type == model.PartBlock && b.HasTarget("nb") {
			blocks = append(blocks, b)
		}
	}
	require.Len(t, blocks, 2)
	return blocks
}

// A bilingual document carries its translations in the push, and the venue
// holds them. A checkout's block history names each translation by the
// revision of the runs its read found, and the venue compares that with the
// revision of the runs it stored from the push: inline codes included, the
// two agree, so the venue takes the write as the record of the translation it
// holds rather than as one about another translation.
func TestSyncPush_TakesTheWriteOfABilingualTranslationItHolds(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createProject(t, srv, token)
	ts := httptest.NewServer(srv.GetEcho())
	defer ts.Close()
	client := apiclient.NewProjectBearerClient(ts.URL, pid, token)
	ctx := context.Background()

	blocks := readBilingual(t)
	_, err := client.Push(ctx, map[string][]*model.Block{"messages.xlf": blocks},
		[]apiclient.ItemMeta{{Name: "messages.xlf", Format: "xliff"}}, nil, nil)
	require.NoError(t, err)
	drainWithAuthority(t, srv)

	stored, err := srv.ContentStore.GetBlocks(ctx, platstore.BlockQuery{ProjectID: pid, Stream: "main", ItemName: "messages.xlf"})
	require.NoError(t, err)
	unitOf := map[string]string{}
	for _, row := range stored {
		unitOf[row.Block.Name] = row.SourceID
	}
	nb := model.EditionKey{Locale: "nb"}
	var writes []venue.EditionWrite
	for _, b := range blocks {
		require.NotEmpty(t, unitOf[b.Name], "the venue holds %s", b.Name)
		writes = append(writes, venue.EditionWrite{
			ItemName: "messages.xlf", Unit: unitOf[b.Name], Variant: "nb",
			Revision: model.EditionRevision(b, nb), Basis: venue.SourceRevision(b, "en"),
			Writer: venue.WriterTool, Origin: "flow:up",
		})
	}
	_, err = client.Push(ctx, map[string][]*model.Block{}, nil, nil, nil, apiclient.CarryEditionWrites(writes))
	require.NoError(t, err)
	drainWithAuthority(t, srv)

	derived := nbDerivations(t, srv, pid, "messages.xlf")
	for _, b := range blocks {
		d := derived[unitOf[b.Name]]
		require.NotNil(t, d, "%s: the venue took the write as the record of the translation it holds", b.Name)
		assert.Equal(t, venue.SourceRevision(b, "en"), d.Rev)
	}
}

// The usual order is to write a translation by hand, push, and approve it
// later: the client sends a write once the venue has applied it, so the push
// that carries the approval carries no write. The venue keeps who wrote the
// translation from the earlier push, and the approval is still the author's
// own.
func TestSyncPush_RefusesAnApprovalOfAHandWrittenTranslationInALaterPush(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createProject(t, srv, token)
	ts := httptest.NewServer(srv.GetEcho())
	defer ts.Close()
	require.NoError(t, srv.AuthStore.SetSoDMode(t.Context(), "test-ws", platauth.SoDBlock))
	client := apiclient.NewProjectBearerClient(ts.URL, pid, token)
	ctx := context.Background()

	_, err := client.Push(ctx, catalog(map[string]string{"greeting": "Hello world"}), catalogItems, nil, nil)
	require.NoError(t, err)
	drainWithAuthority(t, srv)

	_, err = client.Push(ctx, map[string][]*model.Block{}, nil, nil, nil, apiclient.CarryEditionWrites([]venue.EditionWrite{
		{ItemName: "locales/en.json", Unit: "greeting", Variant: "nb", Revision: "r:0123456789abcdef", Writer: venue.WriterPerson, Origin: "apply"},
	}))
	require.NoError(t, err)
	drainWithAuthority(t, srv)

	resp, err := client.Push(ctx, map[string][]*model.Block{}, nil, nil, []venue.UnitDecision{{
		ItemName: "locales/en.json", Unit: "greeting", Variant: "nb",
		Status: string(model.TargetStatusEstablished), ReviewState: venue.ReviewStateApproved,
		Revision: textRevision("nb", "Hei, verden"), Basis: enSourceRevision("Hello world"),
		Updated: time.Now().UTC().Add(time.Minute).Format(time.RFC3339),
	}})
	require.NoError(t, err)
	drainWithAuthority(t, srv)

	status, err := client.PushStatus(ctx, resp.PushID)
	require.NoError(t, err)
	require.NotNil(t, status.Governance, "the server reports the approval it did not accept")
	refused := map[string]string{}
	for _, u := range status.Governance.Units {
		refused[u.Unit] = u.Reason
	}
	assert.Equal(t, venue.RefusedSeparationOfDuties, refused["greeting"], "the pusher wrote it by hand one push earlier")
	assert.Empty(t, nbLedger(t, srv, pid)["greeting"].ReviewState)
}
