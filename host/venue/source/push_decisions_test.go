package source

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/venue"
	bowrainconn "github.com/neokapi/neokapi/core/venue/connector"
	"github.com/neokapi/neokapi/host"
)

// What the venue holds decides whether the record goes.
//
// A client that only remembered sending it told the venue once and never again:
// a stream it had not pushed to, and a venue that lost its ledger, both stayed
// without decisions this project holds, and a removal on the venue's side was
// permanent because the next push had nothing to say.
//
// The tree a push already fetches carries that stream's ref, and its decisions
// component is the venue's answer about what it holds. A drafting run moves the
// record's full fold but not that component, so this compares the two halves
// that mean something: the record changed here, or the venue lacks what it says.

// heldHere is the decisions component of the project's committed record.
func heldHere(t *testing.T, conn *BowrainSourceConnector) string {
	t.Helper()
	records, err := conn.projectDecisions(t.Context())
	require.NoError(t, err)
	return venue.DecisionsComponent(records)
}

// committedApproval scaffolds a project whose committed record holds one
// approval, pointed at srv.
func committedApproval(t *testing.T, srv *refServer) *BowrainSourceConnector {
	t.Helper()
	conn := newRefConnector(t, srv.Server, "proj1")
	a := &host.App{}
	t.Cleanup(a.Shutdown)
	st, err := a.OpenProjectState(t.Context(), conn.project.Root)
	require.NoError(t, err)
	require.NoError(t, st.Put(t.Context(), approvedUnit("greeting", "fr")))
	require.NoError(t, st.Commit(t.Context()))
	return conn
}

func TestPush_SendsTheRecordUntilTheVenueHoldsIt(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{Content: 99, Context: "ctx-server", Decisions: "dec-server"})
	conn := committedApproval(t, srv)

	_, err := conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, srv.decisionsSent, "the first push sends the committed record")

	_, err = conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	assert.Equal(t, 2, srv.decisionsSent,
		"the venue holds none of these decisions, so the record goes again")

	// The venue now answers with exactly what this project holds.
	srv.published.Decisions = heldHere(t, conn)
	before := srv.decisionsSent
	_, err = conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	assert.Equal(t, before, srv.decisionsSent, "a record the venue holds is not sent again")
}

func TestPush_SendsTheRecordAgainWhenTheVenueLosesIt(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{Content: 99})
	conn := committedApproval(t, srv)

	_, err := conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	srv.published.Decisions = heldHere(t, conn)

	_, err = conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	held := srv.decisionsSent

	// The venue's ledger is gone, which is what a data reset that keeps the
	// project id looks like from here.
	srv.published.Decisions = ""
	_, err = conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	assert.Greater(t, srv.decisionsSent, held,
		"a venue that no longer holds the record is told again")
}

func TestPush_TellsEachStreamWhatItHolds(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{Content: 5})
	conn := committedApproval(t, srv)

	_, err := conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	srv.published.Decisions = heldHere(t, conn)

	_, err = conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	onMain := srv.decisionsSent

	// The same checkout, pushed to a stream that holds nothing.
	srv.byStream = map[string]ref.Ref{"feature": {}}
	conn.stream = "feature"
	conn.client.SetStream("feature")

	_, err = conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	assert.Greater(t, srv.decisionsSent, onMain,
		"a stream holding none of these decisions is told, whatever another stream holds")
}

func TestPush_SendsTheRecordWhenItCannotAskWhatTheVenueHolds(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{Content: 5})
	conn := committedApproval(t, srv)

	_, err := conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	srv.published.Decisions = heldHere(t, conn)

	_, err = conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	quiet := srv.decisionsSent

	// No tree, so no answer about what the venue holds. The push says what it
	// has rather than assuming it has been heard.
	srv.treeUnavailable = true
	_, err = conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	assert.Greater(t, srv.decisionsSent, quiet,
		"a push that cannot ask sends the record")
}
