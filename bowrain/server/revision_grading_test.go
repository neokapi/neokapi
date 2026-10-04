package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
)

// A decision names the pairing it blesses by revision on a checkout and on the
// platform alike, so the two grade it the same, and a change to an inline code
// alone retires it on both. These tests drive the real client against the real
// server, landing each push in PostgreSQL, and grade the checkout's side with
// the readers a checkout uses (core/state).

const gradedItem = "docs/guide.json"

var gradedItems = []apiclient.ItemMeta{{Name: gradedItem, Format: "json"}}

// guideRuns is a source that links "the guide" to href.
func guideRuns(href string) []model.Run {
	return []model.Run{
		model.TextR("Read "),
		model.PcOpenR(model.PcOpenRun{ID: "1", Type: "link", Data: `<a href="` + href + `">`}),
		model.TextR("the guide"),
		model.PcCloseR(model.PcCloseRun{ID: "1", Type: "link", Data: "</a>"}),
	}
}

// checkoutBlock is the block a checkout reads from its file: the source
// linking to href, filed under the project's language as a project read files
// it, and the Norwegian translation.
func checkoutBlock(href string) *model.Block {
	b := &model.Block{ID: "guide", Name: "guide", Translatable: true, SourceLocale: "en"}
	b.SetSourceRuns(guideRuns(href))
	b.SetEdition(model.Variant("nb"), model.Edition{
		Runs: []model.Run{model.TextR("Les veiledningen")}, Status: model.Status(model.TargetStatusTranslated),
	})
	return b
}

// checkoutApproval is the record a checkout makes when a person approves the
// Norwegian translation of b, and the wire form a push carries it in.
func checkoutApproval(b *model.Block, at time.Time) (state.UnitState, venue.UnitDecision) {
	read := state.ReadTarget(b, "nb", "en")
	stamp := at.UTC().Format(time.RFC3339)
	u := state.UnitState{
		Scope: gradedItem, Unit: "guide", Variant: model.Variant("nb"),
		Status: model.TargetStatusEstablished, Revision: read.Revision, Basis: read.Basis,
		Decision: state.Decision{ReviewState: venue.ReviewStateApproved, At: stamp}, Updated: stamp,
	}
	return u, venue.UnitDecision{
		ItemName: gradedItem, Unit: u.Unit, Variant: "nb", Status: string(u.Status),
		Revision: u.Revision, Basis: u.Basis, ReviewState: u.Decision.ReviewState,
		DecidedAt: stamp, Updated: stamp,
	}
}

// checkoutFresh is how a checkout holding b grades a record: current while it
// blesses the translation and the source b holds.
func checkoutFresh(record state.UnitState, b *model.Block) bool {
	return record.Fresh(state.ReadTarget(b, "nb", "en"))
}

// platformGrade is how the platform grades the unit's record now: whether the
// tally reads it stale, the rung its translation projects, and the review
// context's stale flag.
type platformGrade struct {
	stale       bool
	established bool
	flagged     bool
}

func gradeOnPlatform(t *testing.T, srv *Server, pid string) platformGrade {
	t.Helper()
	ctx := t.Context()
	ds, ok := srv.ContentStore.(platstore.DecisionStore)
	require.True(t, ok)
	tallies, err := ds.TallyDecisionBasis(ctx, pid, "main")
	require.NoError(t, err)
	stale := 0
	for _, tl := range tallies {
		if tl.Variant == "nb" {
			stale += tl.Stale
		}
	}
	rows, err := srv.ContentStore.GetBlocks(ctx, platstore.BlockQuery{ProjectID: pid, Stream: "main", ItemName: gradedItem})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	sb := rows[0]
	nb, ok := sb.Block.Edition(model.Variant("nb"))
	require.True(t, ok)
	d := srv.unitDecisionFor(ctx, pid, "main", sb, "nb")
	require.NotNil(t, d)
	return platformGrade{
		stale:       stale > 0,
		established: model.TargetStatus(nb.Status) == model.TargetStatusEstablished,
		flagged:     reviewProvenanceOf(d, sb, "nb").Stale,
	}
}

// pushGraded pushes the checkout's block, and the records given, as kapi push
// does: the transfer hash taken under the project's language.
func pushGraded(t *testing.T, srv *Server, client *apiclient.BowrainClient, b *model.Block, decisions []venue.UnitDecision) *apiclient.SyncPushResponse {
	t.Helper()
	resp, err := client.Push(context.Background(), map[string][]*model.Block{gradedItem: {b}}, gradedItems, nil,
		decisions, apiclient.TransferUnder("en"))
	require.NoError(t, err)
	drainWithAuthority(t, srv)
	return resp
}

func newGradedProject(t *testing.T) (*Server, string, *apiclient.BowrainClient) {
	t.Helper()
	srv, token := newTestServer(t)
	pid := createProject(t, srv, token)
	ts := httptest.NewServer(srv.GetEcho())
	t.Cleanup(ts.Close)
	return srv, pid, apiclient.NewProjectBearerClient(ts.URL, pid, token)
}

// A checkout approves a translation and pushes it; the checkout then changes
// the source's link and nothing else, and pushes that. Both sides read the
// approval current before the change and stale after it, and current again
// once the link moves back.
func TestDecisionGrading_AnInlineCodeChangeOnACheckoutGradesTheSameOnThePlatform(t *testing.T) {
	srv, pid, client := newGradedProject(t)

	held := checkoutBlock("/v1/guide")
	record, wire := checkoutApproval(held, time.Now())
	pushGraded(t, srv, client, held, []venue.UnitDecision{wire})

	require.True(t, checkoutFresh(record, held), "the checkout reads its approval current")
	assert.Equal(t, platformGrade{established: true}, gradeOnPlatform(t, srv, pid),
		"the platform reads the approval the checkout pushed current, and projects it")

	moved := checkoutBlock("/v2/guide")
	require.Equal(t, held.SourceText(), moved.SourceText(), "the wording is the same; only the link moved")
	assert.False(t, checkoutFresh(record, moved), "the checkout retires the approval")
	resp := pushGraded(t, srv, client, moved, nil)
	assert.Equal(t, 1, resp.BlocksUploaded, "the push carries a change to an inline code alone")
	assert.Equal(t, platformGrade{stale: true, flagged: true}, gradeOnPlatform(t, srv, pid),
		"the platform retires it too: stale in the tally, off the established rung, flagged in review")

	pushGraded(t, srv, client, checkoutBlock("/v1/guide"), nil)
	assert.True(t, checkoutFresh(record, checkoutBlock("/v1/guide")))
	assert.Equal(t, platformGrade{established: true}, gradeOnPlatform(t, srv, pid),
		"the link moving back restores the approval on both sides")
}

// The platform holds an approval a reviewer made there. A change to the
// source's link alone, made on the platform, retires it there; a checkout
// that pulls the record grades it the same against each source; and the
// reviewer's approval of the new source reads current on a checkout holding
// that source.
func TestDecisionGrading_AnInlineCodeChangeOnThePlatformGradesTheSameOnACheckout(t *testing.T) {
	srv, pid, client := newGradedProject(t)
	pushGraded(t, srv, client, checkoutBlock("/v1/guide"), nil)
	approveOnPlatform(t, srv, pid)

	pulled := pulledRecord(t, client)
	assert.True(t, checkoutFresh(pulled, checkoutBlock("/v1/guide")),
		"a checkout holding the source the reviewer saw reads the platform's approval current")
	assert.False(t, checkoutFresh(pulled, checkoutBlock("/v2/guide")),
		"and stale against a source whose link moved")
	assert.Equal(t, platformGrade{established: true}, gradeOnPlatform(t, srv, pid))

	// The link moves on the platform: a person edits the source there.
	editAsPerson(t, srv, pid, storedGuideID(t, srv, pid), "", change.Content{Runs: guideRuns("/v2/guide")})
	assert.Equal(t, platformGrade{stale: true, flagged: true}, gradeOnPlatform(t, srv, pid),
		"the platform retires the approval made for the old link")
	pulled = pulledRecord(t, client)
	assert.False(t, checkoutFresh(pulled, checkoutBlock("/v2/guide")),
		"a checkout holding the new link grades the pulled approval stale too")

	// The reviewer approves the translation of the new source on the platform.
	approveOnPlatform(t, srv, pid)
	assert.Equal(t, platformGrade{established: true}, gradeOnPlatform(t, srv, pid))
	pulled = pulledRecord(t, client)
	assert.True(t, checkoutFresh(pulled, checkoutBlock("/v2/guide")),
		"the new approval reads current on a checkout holding the new link")
	assert.False(t, checkoutFresh(pulled, checkoutBlock("/v1/guide")))
}

// A format that declares its own language (en-GB here, in an en project) makes
// the change service take the basis under that language, and a checkout reads
// a basis under either as current. The push sends it as the platform takes it
// (venue.Basis, as BowrainSourceConnector does), so the platform grades the
// approval as the checkout does, and retires it on a link change as the
// checkout does.
func TestDecisionGrading_ABasisTakenUnderTheFilesLanguageGradesTheSameOnThePlatform(t *testing.T) {
	srv, pid, client := newGradedProject(t)

	declared := checkoutBlock("/v1/guide")
	declared.SourceLocale = "en-GB"
	record, wire := checkoutApproval(declared, time.Now())
	record.Basis = model.EditionRevision(declared, model.EditionKey{})
	require.NotEqual(t, venue.SourceRevision(declared, "en"), record.Basis,
		"the change service took the basis under the file's language")
	require.True(t, checkoutFresh(record, declared), "the checkout reads its approval current")

	wire.Basis = venue.Basis(declared, "en", record.Basis)
	pushGraded(t, srv, client, declared, []venue.UnitDecision{wire})
	assert.Equal(t, platformGrade{established: true}, gradeOnPlatform(t, srv, pid),
		"the platform reads the approval current, and projects it")

	moved := checkoutBlock("/v2/guide")
	moved.SourceLocale = "en-GB"
	assert.False(t, checkoutFresh(record, moved), "the checkout retires the approval")
	pushGraded(t, srv, client, moved, nil)
	assert.Equal(t, platformGrade{stale: true, flagged: true}, gradeOnPlatform(t, srv, pid),
		"and so does the platform")
}

// storedGuideID is the platform's row id of the guide block.
func storedGuideID(t *testing.T, srv *Server, pid string) string {
	t.Helper()
	rows, err := srv.ContentStore.GetBlocks(t.Context(), platstore.BlockQuery{ProjectID: pid, Stream: "main", ItemName: gradedItem})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	return rows[0].Block.ID
}

// approveOnPlatform approves the Norwegian translation of the guide through
// the stream's changes route, as a reviewer on the web does.
func approveOnPlatform(t *testing.T, srv *Server, pid string) {
	t.Helper()
	bid := storedGuideID(t, srv, pid)
	sb, err := srv.ContentStore.GetBlock(t.Context(), pid, "main", bid)
	require.NoError(t, err)
	rev := model.RunsRevision(model.Variant("nb"), sb.Block.TargetRuns("nb"))
	reviewer := changeCaller{user: "test-user", name: "Test", perms: platauth.PermAll, ws: "test-ws"}
	rec, res := sendChanges(t, srv, pid, reviewer, change.Set{Ops: []change.Op{
		decide(at(gradedItem, bid, "nb"), rev, change.OutcomeEstablish)}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
}

// pulledRecord is the guide's Norwegian record as a pull brings it to a
// checkout (BowrainSourceConnector.recordPulledDecisions).
func pulledRecord(t *testing.T, client *apiclient.BowrainClient) state.UnitState {
	t.Helper()
	resp, err := client.Pull(context.Background(), 0, nil, 0)
	require.NoError(t, err)
	for _, d := range resp.Decisions {
		if d.ItemName == gradedItem && d.Unit == "guide" && d.Variant == "nb" {
			return state.UnitState{
				Scope: d.ItemName, Unit: d.Unit, Variant: model.Variant("nb"),
				Status: model.TargetStatus(d.Status), Revision: d.Revision, Basis: d.Basis,
				Decision: state.Decision{ReviewState: d.ReviewState, By: d.DecidedBy, At: d.DecidedAt},
			}
		}
	}
	t.Fatal("the pull carries no record of the guide")
	return state.UnitState{}
}
