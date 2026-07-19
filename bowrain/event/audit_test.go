package event

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newAuditTestLogger provisions a real PostgreSQL-backed AuditLogger: an
// isolated schema migrated to create the append-only, hash-chained audit_log
// table (plus its tamper-evidence trigger) and an in-process event bus. It
// returns the logger, the bus, and the raw *sql.DB used to simulate direct
// tampering and to inspect stored rows.
func newAuditTestLogger(t *testing.T) (*AuditLogger, *ChannelEventBus, *sql.DB) {
	t.Helper()
	pdb := pgtest.NewTestDB(t)
	if _, err := bstore.NewPostgresStoreFromDB(pdb); err != nil {
		t.Fatalf("migrate store schema: %v", err)
	}

	bus := NewChannelEventBus()
	t.Cleanup(bus.Close)
	al := NewAuditLogger(pdb.DB, bus)
	t.Cleanup(al.Close)
	return al, bus, pdb.DB
}

// auditEvent builds a workspace-scoped auditable event. Workspace scope makes
// chainKeyFor partition every event onto the ws chain, so a test controls which
// entries share a hash chain.
func auditEvent(ws string, typ platev.EventType, actor string, ts time.Time) platev.Event {
	return platev.Event{
		Type:         typ,
		Source:       "test",
		WorkspaceID:  ws,
		Actor:        actor,
		ResourceType: "member",
		ResourceID:   "u-" + actor,
		Effect:       "allow",
		Data:         map[string]string{"event": string(typ)},
		Timestamp:    ts.UTC().Truncate(time.Microsecond),
	}
}

// appendAudit mirrors AuditLogger.handleEvent's field marshaling and writes one
// row synchronously via insertChained. Going straight to the (same-package)
// insert keeps chain assertions deterministic instead of racing the bus
// subscriber goroutine.
func appendAudit(t *testing.T, al *AuditLogger, ev platev.Event) {
	t.Helper()
	dataJSON, err := json.Marshal(ev.Data)
	if err != nil || len(ev.Data) == 0 {
		dataJSON = []byte("{}")
	}
	before := marshalOrEmpty(ev.Before)
	after := marshalOrEmpty(ev.After)
	require.NoError(t, al.insertChained(t.Context(), ev, chainKeyFor(ev), string(dataJSON), before, after))
}

type chainRow struct {
	id       int64
	prevHash string
	hash     string
}

// fetchChain returns a chain's rows in insertion (id) order.
func fetchChain(t *testing.T, db *sql.DB, chainKey string) []chainRow {
	t.Helper()
	rows, err := db.QueryContext(t.Context(),
		`SELECT id, prev_hash, hash FROM audit_log WHERE chain_key = $1 ORDER BY id ASC`, chainKey)
	require.NoError(t, err)
	defer rows.Close()
	var out []chainRow
	for rows.Next() {
		var r chainRow
		require.NoError(t, rows.Scan(&r.id, &r.prevHash, &r.hash))
		out = append(out, r)
	}
	require.NoError(t, rows.Err())
	return out
}

// countChain is safe to call from an Eventually goroutine (no require).
func countChain(db *sql.DB, chainKey string) int {
	var n int
	_ = db.QueryRowContext(context.Background(), `SELECT count(*) FROM audit_log WHERE chain_key = $1`, chainKey).Scan(&n)
	return n
}

// bypassAppendOnly temporarily disables the append-only trigger, runs fn (a
// direct UPDATE/DELETE an attacker with DB access could perform), then restores
// the guard. The hash chain, not the trigger, is what must still catch this.
func bypassAppendOnly(t *testing.T, db *sql.DB, fn func()) {
	t.Helper()
	ctx := t.Context()
	_, err := db.ExecContext(ctx, `ALTER TABLE audit_log DISABLE TRIGGER audit_log_append_only`)
	require.NoError(t, err)
	fn()
	_, err = db.ExecContext(ctx, `ALTER TABLE audit_log ENABLE TRIGGER audit_log_append_only`)
	require.NoError(t, err)
}

// TestAuditChainAppendLinksAndVerifies pins the hash-chain construction:
// genesis links to an empty prev_hash, every later row links to its
// predecessor's hash, and VerifyChain accepts the untampered chain.
func TestAuditChainAppendLinksAndVerifies(t *testing.T) {
	al, _, db := newAuditTestLogger(t)
	const ws = "ws-chain"
	base := time.Now().UTC().Truncate(time.Microsecond)
	types := []platev.EventType{
		platev.EventProjectCreated,
		platev.EventBlockUpdated,
		platev.EventVersionCreated,
		platev.EventProjectUpdated,
	}
	for i, ty := range types {
		appendAudit(t, al, auditEvent(ws, ty, "alice", base.Add(time.Duration(i)*time.Second)))
	}

	rows := fetchChain(t, db, ws)
	require.Len(t, rows, len(types))
	assert.Empty(t, rows[0].prevHash, "genesis row must link to empty prev_hash")
	assert.NotEmpty(t, rows[0].hash)
	for i := 1; i < len(rows); i++ {
		assert.Equal(t, rows[i-1].hash, rows[i].prevHash, "row %d must link to row %d's hash", i, i-1)
		assert.NotEqual(t, rows[i-1].hash, rows[i].hash, "each row must produce a distinct hash")
	}

	v, err := al.VerifyChain(t.Context(), ws)
	require.NoError(t, err)
	assert.True(t, v.Valid, v.BrokenMsg)
	assert.Equal(t, len(types), v.Rows)
	assert.Zero(t, v.BrokenAt)
}

// TestAuditLoggerPersistsFromBus exercises the real subscription path: events
// published on the bus are persisted asynchronously by the audit-logger
// subscriber into a verifiable chain.
func TestAuditLoggerPersistsFromBus(t *testing.T) {
	al, bus, db := newAuditTestLogger(t)
	const ws = "ws-bus"
	base := time.Now().UTC()
	for i := range 3 {
		bus.Publish(auditEvent(ws, platev.EventProjectUpdated, "carol", base.Add(time.Duration(i)*time.Millisecond)))
	}

	require.Eventually(t, func() bool {
		return countChain(db, ws) == 3
	}, 5*time.Second, 20*time.Millisecond, "audit logger should persist all published events")

	v, err := al.VerifyChain(t.Context(), ws)
	require.NoError(t, err)
	assert.True(t, v.Valid, v.BrokenMsg)
	assert.Equal(t, 3, v.Rows)
}

// TestAuditVerifyChainDetectsTamper mutates a stored row's payload behind the
// append-only guard and asserts VerifyChain flags exactly that row.
func TestAuditVerifyChainDetectsTamper(t *testing.T) {
	al, _, db := newAuditTestLogger(t)
	const ws = "ws-tamper"
	base := time.Now().UTC().Truncate(time.Microsecond)
	for i := range 4 {
		appendAudit(t, al, auditEvent(ws, platev.EventProjectUpdated, "mallory", base.Add(time.Duration(i)*time.Second)))
	}

	rows := fetchChain(t, db, ws)
	require.Len(t, rows, 4)

	clean, err := al.VerifyChain(t.Context(), ws)
	require.NoError(t, err)
	require.True(t, clean.Valid, "chain must verify before tampering")

	target := rows[2].id // third entry
	bypassAppendOnly(t, db, func() {
		_, err := db.ExecContext(t.Context(),
			`UPDATE audit_log SET data = '{"tampered":"true"}'::jsonb WHERE id = $1`, target)
		require.NoError(t, err)
	})

	v, err := al.VerifyChain(t.Context(), ws)
	require.NoError(t, err)
	assert.False(t, v.Valid, "tampered payload must fail verification")
	assert.Equal(t, target, v.BrokenAt, "verification must point at the tampered row")
	assert.Contains(t, v.BrokenMsg, "row hash does not match")
}

// TestAuditVerifyChainDetectsMiddleDeletion distinguishes an illicit middle
// deletion (chain break, must be caught) from the sanctioned oldest-row prune
// tested separately. Deleting an interior row severs the prev_hash link at the
// following row.
func TestAuditVerifyChainDetectsMiddleDeletion(t *testing.T) {
	al, _, db := newAuditTestLogger(t)
	const ws = "ws-del"
	base := time.Now().UTC().Truncate(time.Microsecond)
	for i := range 4 {
		appendAudit(t, al, auditEvent(ws, platev.EventProjectUpdated, "eve", base.Add(time.Duration(i)*time.Second)))
	}

	rows := fetchChain(t, db, ws)
	require.Len(t, rows, 4)

	bypassAppendOnly(t, db, func() {
		_, err := db.ExecContext(t.Context(), `DELETE FROM audit_log WHERE id = $1`, rows[1].id)
		require.NoError(t, err)
	})

	v, err := al.VerifyChain(t.Context(), ws)
	require.NoError(t, err)
	assert.False(t, v.Valid, "interior deletion must fail verification")
	assert.Equal(t, rows[2].id, v.BrokenAt, "the row after the gap must report the broken link")
	assert.Contains(t, v.BrokenMsg, "prev_hash does not match preceding row")
}

// TestAuditVerifyAllChains sweeps every chain and returns only broken ones,
// leaving an untampered chain out of the report.
func TestAuditVerifyAllChains(t *testing.T) {
	al, _, db := newAuditTestLogger(t)
	base := time.Now().UTC().Truncate(time.Microsecond)
	for i := range 3 {
		appendAudit(t, al, auditEvent("ws-clean", platev.EventProjectUpdated, "alice", base.Add(time.Duration(i)*time.Second)))
		appendAudit(t, al, auditEvent("ws-bad", platev.EventBlockUpdated, "bob", base.Add(time.Duration(i)*time.Second)))
	}

	bad := fetchChain(t, db, "ws-bad")
	require.Len(t, bad, 3)
	bypassAppendOnly(t, db, func() {
		_, err := db.ExecContext(t.Context(),
			`UPDATE audit_log SET actor = 'attacker' WHERE id = $1`, bad[1].id)
		require.NoError(t, err)
	})

	keys, err := al.ListChainKeys(t.Context())
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"ws-clean", "ws-bad"}, keys)

	broken, err := al.VerifyAllChains(t.Context())
	require.NoError(t, err)
	require.Len(t, broken, 1, "only the tampered chain should be reported")
	assert.Equal(t, "ws-bad", broken[0].ChainKey)
	assert.False(t, broken[0].Valid)
	assert.Equal(t, bad[1].id, broken[0].BrokenAt)
}

// TestAuditLogIsAppendOnly pins the database-layer tamper-evidence: UPDATE and
// DELETE are rejected while the trigger is enabled, and the row survives.
func TestAuditLogIsAppendOnly(t *testing.T) {
	al, _, db := newAuditTestLogger(t)
	const ws = "ws-ao"
	appendAudit(t, al, auditEvent(ws, platev.EventProjectCreated, "alice", time.Now()))

	rows := fetchChain(t, db, ws)
	require.Len(t, rows, 1)
	id := rows[0].id

	_, err := db.ExecContext(t.Context(), `UPDATE audit_log SET actor = 'x' WHERE id = $1`, id)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "append-only")

	_, err = db.ExecContext(t.Context(), `DELETE FROM audit_log WHERE id = $1`, id)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "append-only")

	assert.Len(t, fetchChain(t, db, ws), 1, "row must survive rejected mutations")
}

// TestAuditRetentionPrunesOldestChainStaysValid pins the retention contract:
// PruneOlderThan removes only rows past the window (boundary-inside rows are
// kept), and because VerifyChain anchors on the oldest retained row, the
// remaining window still verifies clean even though its predecessor is gone.
func TestAuditRetentionPrunesOldestChainStaysValid(t *testing.T) {
	al, _, db := newAuditTestLogger(t)
	const ws = "ws-ret"
	const maxAge = 24 * time.Hour
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Insertion order == chronological order == chain order.
	offsets := []time.Duration{
		-48 * time.Hour,  // pruned (outside window)
		-36 * time.Hour,  // pruned (outside window)
		-23 * time.Hour,  // kept (just inside the window boundary)
		-1 * time.Hour,   // kept
		-1 * time.Minute, // kept
	}
	for i, off := range offsets {
		appendAudit(t, al, auditEvent(ws, platev.EventProjectUpdated, "alice", now.Add(off).Add(time.Duration(i)*time.Millisecond)))
	}
	require.Len(t, fetchChain(t, db, ws), len(offsets))

	n, err := al.PruneOlderThan(t.Context(), maxAge)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n, "only rows older than the window should be pruned")

	remaining := fetchChain(t, db, ws)
	assert.Len(t, remaining, 3, "boundary-inside and newer rows must be kept")

	v, err := al.VerifyChain(t.Context(), ws)
	require.NoError(t, err)
	assert.True(t, v.Valid, v.BrokenMsg)
	assert.Equal(t, 3, v.Rows)
}

// TestNewAuditRetentionCleanerGuards pins the constructor guards that keep a
// no-op cleaner from starting a goroutine.
func TestNewAuditRetentionCleanerGuards(t *testing.T) {
	assert.Nil(t, NewAuditRetentionCleaner(nil, time.Hour, time.Hour), "nil logger disables the cleaner")

	al := &AuditLogger{}
	assert.Nil(t, NewAuditRetentionCleaner(al, 0, time.Hour), "non-positive maxAge disables the cleaner")
	assert.Nil(t, NewAuditRetentionCleaner(al, -time.Hour, time.Hour), "negative maxAge disables the cleaner")

	var c *AuditRetentionCleaner
	assert.NotPanics(t, c.Close, "Close on a nil cleaner is a no-op")
}

// captureSink records every NDJSON line the exporter forwards.
type captureSink struct {
	mu    sync.Mutex
	lines [][]byte
}

func (s *captureSink) Export(_ context.Context, ndjson []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lines = append(s.lines, append([]byte(nil), ndjson...))
	return nil
}

func (s *captureSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.lines)
}

func (s *captureSink) snapshot() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([][]byte(nil), s.lines...)
}

// TestSIEMExporterForwardsEventsAsNDJSON asserts the exporter's output shape:
// one newline-delimited JSON line per event, each round-tripping back into the
// published event's fields.
func TestSIEMExporterForwardsEventsAsNDJSON(t *testing.T) {
	bus := NewChannelEventBus()
	t.Cleanup(bus.Close)
	sink := &captureSink{}
	exp := NewSIEMExporter(bus, sink)
	require.NotNil(t, exp)
	t.Cleanup(exp.Close)

	bus.Publish(platev.Event{Type: platev.EventProjectCreated, ProjectID: "p1", Actor: "alice", Data: map[string]string{"name": "Site"}})
	bus.Publish(platev.Event{Type: platev.EventBlockUpdated, ProjectID: "p1", Data: map[string]string{"block_id": "b1"}})

	require.Eventually(t, func() bool { return sink.count() >= 2 }, 5*time.Second, 20*time.Millisecond)

	byType := make(map[platev.EventType]platev.Event)
	for _, ln := range sink.snapshot() {
		require.NotEmpty(t, ln)
		assert.Equal(t, byte('\n'), ln[len(ln)-1], "each SIEM record must be newline-delimited")
		var got platev.Event
		require.NoError(t, json.Unmarshal(ln, &got))
		byType[got.Type] = got
	}

	require.Contains(t, byType, platev.EventProjectCreated)
	assert.Equal(t, "alice", byType[platev.EventProjectCreated].Actor)
	assert.Equal(t, "Site", byType[platev.EventProjectCreated].Data["name"])
	require.Contains(t, byType, platev.EventBlockUpdated)
	assert.Equal(t, "b1", byType[platev.EventBlockUpdated].Data["block_id"])
}

// TestNewSIEMExporterNilGuards pins that a nil sink or bus disables export and
// that Close is nil-safe.
func TestNewSIEMExporterNilGuards(t *testing.T) {
	assert.Nil(t, NewSIEMExporter(NewChannelEventBus(), nil))
	assert.Nil(t, NewSIEMExporter(nil, &captureSink{}))

	var e *SIEMExporter
	assert.NotPanics(t, e.Close, "Close on a nil exporter is a no-op")
}

// TestHTTPSinkPostsNDJSON asserts the webhook sink posts the raw NDJSON body
// with the ndjson content type and configured headers.
func TestHTTPSinkPostsNDJSON(t *testing.T) {
	var (
		gotBody []byte
		gotCT   string
		gotHdr  string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		gotHdr = r.Header.Get("X-Token")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sink := &HTTPSink{URL: srv.URL, Headers: map[string]string{"X-Token": "s3cret"}}
	payload := []byte(`{"type":"project.created"}` + "\n")
	require.NoError(t, sink.Export(t.Context(), payload))

	assert.Equal(t, "application/x-ndjson", gotCT)
	assert.Equal(t, "s3cret", gotHdr)
	assert.Equal(t, payload, gotBody)
}

// TestHTTPSinkErrorsOnNon2xx asserts a non-success response surfaces an error
// so the exporter can log the failed forward.
func TestHTTPSinkErrorsOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	sink := &HTTPSink{URL: srv.URL}
	err := sink.Export(t.Context(), []byte("{}\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}
