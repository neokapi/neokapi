package source

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/venue"
	bowrainconn "github.com/neokapi/neokapi/core/venue/connector"
)

// A checkout's records name a block by the key its reader gives it, and a push
// resolves every block to the key the venue files it under (Block.Key),
// minting one for content a venue has never seen. These tests drive a real
// checkout (a flow, the change service, and the connector's push and pull)
// against a venue holding nothing, and read the keys on the wire. What the
// server joins on those keys is tested in bowrain/server.

const keyedItem = "locales/en.json"

func keyedCheckout(t *testing.T, srv *refServer) *BowrainSourceConnector {
	t.Helper()
	return checkoutOf(t, srv, "en-US", map[string]string{
		keyedItem: `{"greeting": "Hello there", "farewell": "Goodbye now"}` + "\n",
	}, coreproj.Collection{Name: "site", Path: keyedItem, Target: "locales/{lang}.json"})
}

// A checkout translates its catalog and approves one translation before the
// project's first push. The push sends the approval under the key it filed the
// block by, and under no other key.
func TestPush_SendsACheckoutDecisionUnderTheKeyItFiledTheBlockBy(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{Content: 5})
	conn := keyedCheckout(t, srv)
	up(t, conn)
	record := approveFrench(t, conn, keyedItem, "greeting")

	_, err := conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)

	filed := filedAs(t, conn, keyedItem, "greeting")
	require.NotEqual(t, "greeting", filed, "a venue holding nothing mints the block a key of its own")
	var units []string
	for _, d := range srv.decisions {
		if d.Variant == "fr" && d.ReviewState != "" {
			units = append(units, d.ItemName+"|"+d.Unit)
		}
	}
	assert.Equal(t, []string{keyedItem + "|" + filed}, units,
		"the approval goes under the key the venue files the block by")

	d := sentFor(t, srv, conn, keyedItem, "greeting")
	assert.Equal(t, venue.ReviewStateApproved, d.ReviewState)
	assert.Equal(t, record.Revision, d.Revision)

	for _, w := range srv.writes {
		assert.NotEqual(t, "greeting", w.Unit, "an edition write names its unit by the venue's key too")
		assert.NotEqual(t, "farewell", w.Unit, "an edition write names its unit by the venue's key too")
	}
}

// A decision the venue holds comes back under the key the venue files its unit
// by, and the pull records it under the key the checkout's reader gives the
// block that resolves to that unit, so the change service reads it on the
// block it judges.
func TestPull_FilesAVenueDecisionUnderTheCheckoutsKey(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{Content: 5})
	conn := keyedCheckout(t, srv)
	filed := filedAs(t, conn, keyedItem, "greeting")
	require.NotEqual(t, "greeting", filed, "a venue holding nothing mints the block a key of its own")
	srv.pulled = []venue.UnitDecision{{
		ItemName: keyedItem, Unit: filed, Variant: "fr",
		Status: string(model.TargetStatusEstablished), ReviewState: venue.ReviewStateApproved,
		DecidedBy: "reviewer@example.test", Updated: "2026-10-06T10:00:00Z",
	}}

	res, err := conn.Pull(context.Background(), bowrainconn.PullOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, res.DecisionsStaged)

	st, err := conn.workingStore(t.Context())
	require.NoError(t, err)
	units, err := st.All(t.Context())
	require.NoError(t, err)
	var keys []string
	for _, u := range units {
		if u.Variant.Locale == "fr" && u.Decision.ReviewState != "" {
			keys = append(keys, u.Unit)
		}
	}
	assert.Equal(t, []string{"greeting"}, keys,
		"the checkout holds the approval under its reader's key alone")
}
