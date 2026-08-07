package event

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func countAuditRows(t *testing.T, db *sql.DB, ws string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM audit_log WHERE workspace_id = $1`, ws).Scan(&n))
	return n
}

// TestAudit_FailedAppendIsReported is the half of the durability fix that lives
// in the handler: a write that did not happen must be reported, or the bus has
// no reason to redeliver it.
//
// A dropped audit record is invisible after the fact. Each row links to the
// previous SURVIVING row, so the chain over a gap verifies exactly as cleanly
// as one with nothing missing — there is no later check that can find it.
func TestAudit_FailedAppendIsReported(t *testing.T) {
	al, _, db := newAuditTestLogger(t)

	ev := auditEvent("ws-blip", platev.EventType("member.role_changed"), "alice", time.Now())
	ev.ID = "evt-blip"

	// The store is down for this delivery.
	down := &AuditLogger{db: closedDB(t), bus: al.bus}
	require.Error(t, down.handleEvent(ev), "an append that failed must say so")
	assert.Equal(t, 0, countAuditRows(t, db, "ws-blip"))

	// The same event, redelivered once the store is back, lands.
	require.NoError(t, al.handleEvent(ev))
	assert.Equal(t, 1, countAuditRows(t, db, "ws-blip"))
}

// closedDB is a database handle that refuses every statement — the shape a
// connection failure takes from the handler's side.
func closedDB(t *testing.T) *sql.DB {
	t.Helper()
	cfg, err := pgx.ParseConfig("postgres://127.0.0.1:1/none")
	require.NoError(t, err)
	db := stdlib.OpenDB(*cfg)
	require.NoError(t, db.Close())
	return db
}

// TestAudit_RedeliveredEventAppendsOnce is the other half. The error return
// above enables redelivery, and the reclaim sweep redelivers anyway to handlers
// whose consumer died after finishing — so an event that arrives twice must
// append one row, not a second copy of the same fact.
func TestAudit_RedeliveredEventAppendsOnce(t *testing.T) {
	al, _, db := newAuditTestLogger(t)

	ev := auditEvent("ws-dup", platev.EventType("member.role_changed"), "bob", time.Now())
	ev.ID = "evt-1"

	for range 3 {
		require.NoError(t, al.handleEvent(ev))
	}
	assert.Equal(t, 1, countAuditRows(t, db, "ws-dup"))

	// A different event with the same content is a different fact and appends.
	other := ev
	other.ID = "evt-2"
	require.NoError(t, al.handleEvent(other))
	assert.Equal(t, 2, countAuditRows(t, db, "ws-dup"))

	// The chain the two rows form still verifies.
	entries, err := al.QueryAuditLog(t.Context(), AuditQuery{WorkspaceID: "ws-dup", Limit: 10})
	require.NoError(t, err)
	require.Len(t, entries, 2)
	report, err := al.VerifyChain(t.Context(), "ws-dup")
	require.NoError(t, err)
	assert.True(t, report.Valid, "%+v", report)
}

// TestAudit_EventsWithoutIDsStillAppend keeps the key optional: an event
// published without an id (a direct call, a test, an older publisher) is still
// recorded rather than colliding with every other id-less event.
func TestAudit_EventsWithoutIDsStillAppend(t *testing.T) {
	al, _, db := newAuditTestLogger(t)

	for i := range 3 {
		ev := auditEvent("ws-anon", platev.EventType("member.role_changed"), "carol", time.Now().Add(time.Duration(i)*time.Second))
		ev.ID = ""
		require.NoError(t, al.handleEvent(ev))
	}
	assert.Equal(t, 3, countAuditRows(t, db, "ws-anon"))
}
