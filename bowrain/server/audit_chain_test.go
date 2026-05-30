package server

import (
	"database/sql"
	"net/http"
	"testing"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	bevent "github.com/neokapi/neokapi/bowrain/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func auditTestDB(t *testing.T, s *Server) *sql.DB {
	t.Helper()
	pg, ok := s.ContentStore.(*bstore.PostgresStore)
	require.True(t, ok, "content store should be PostgresStore in tests")
	return pg.SQLDB()
}

// TestPhase2_AuditChain_PersistAndVerify covers the hash-chained audit log
// end-to-end: events persist with enriched fields, the chain verifies, the
// table is append-only, tampering is detectable, and retention prunes safely.
func TestPhase2_AuditChain_PersistAndVerify(t *testing.T) {
	s, _ := newTestServer(t)
	db := auditTestDB(t, s)
	al := bevent.NewAuditLogger(db, s.EventBus)
	defer al.Close()

	ws := "ws-audit"
	for i, et := range []platev.EventType{platev.EventMemberAdded, platev.EventRoleTemplateCreated, platev.EventTokenCreated} {
		s.EventBus.Publish(platev.Event{
			Type:         et,
			Source:       "test",
			WorkspaceID:  ws,
			Actor:        "actor-1",
			ResourceType: "thing",
			ResourceID:   string(rune('a' + i)),
			Data:         map[string]string{"k": "v", "i": string(rune('0' + i))},
		})
	}

	ctx := t.Context()
	require.Eventually(t, func() bool {
		entries, err := al.QueryAuditLog(ctx, bevent.AuditQuery{WorkspaceID: ws})
		return err == nil && len(entries) == 3
	}, 3*time.Second, 20*time.Millisecond, "3 audit rows should persist")

	entries, err := al.QueryAuditLog(ctx, bevent.AuditQuery{WorkspaceID: ws})
	require.NoError(t, err)
	require.Len(t, entries, 3)
	// Enriched fields are populated and the chain hash is present.
	for _, e := range entries {
		assert.Equal(t, "actor-1", e.Actor)
		assert.Equal(t, ws, e.WorkspaceID)
		assert.Equal(t, "thing", e.ResourceType)
		assert.NotEmpty(t, e.Hash)
	}

	// The chain verifies.
	v, err := al.VerifyChain(ctx, ws)
	require.NoError(t, err)
	assert.True(t, v.Valid, "chain should verify: %+v", v)
	assert.Equal(t, 3, v.Rows)

	// Append-only: direct UPDATE and DELETE are rejected by the trigger.
	_, err = db.ExecContext(ctx, `UPDATE audit_log SET actor = 'evil' WHERE chain_key = $1`, ws)
	require.Error(t, err, "UPDATE must be blocked")
	assert.Contains(t, err.Error(), "append-only")
	_, err = db.ExecContext(ctx, `DELETE FROM audit_log WHERE chain_key = $1`, ws)
	require.Error(t, err, "DELETE must be blocked")
	assert.Contains(t, err.Error(), "append-only")
}

// TestPhase2_AuditChain_DetectsTampering simulates a privileged tamper (trigger
// disabled) and confirms the verifier catches it.
func TestPhase2_AuditChain_DetectsTampering(t *testing.T) {
	s, _ := newTestServer(t)
	db := auditTestDB(t, s)
	al := bevent.NewAuditLogger(db, s.EventBus)
	defer al.Close()

	ws := "ws-tamper"
	for i := 0; i < 3; i++ {
		s.EventBus.Publish(platev.Event{Type: platev.EventMemberAdded, WorkspaceID: ws, Actor: "a", Data: map[string]string{"n": string(rune('0' + i))}})
	}
	ctx := t.Context()
	require.Eventually(t, func() bool {
		e, err := al.QueryAuditLog(ctx, bevent.AuditQuery{WorkspaceID: ws})
		return err == nil && len(e) == 3
	}, 3*time.Second, 20*time.Millisecond)

	// Tamper with a row by bypassing the append-only trigger (DB-level attacker).
	_, err := db.ExecContext(ctx, `ALTER TABLE audit_log DISABLE TRIGGER audit_log_append_only`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE audit_log SET actor = 'mallory' WHERE chain_key = $1 AND id = (SELECT MIN(id) FROM audit_log WHERE chain_key = $1)`, ws)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `ALTER TABLE audit_log ENABLE TRIGGER audit_log_append_only`)
	require.NoError(t, err)

	v, err := al.VerifyChain(ctx, ws)
	require.NoError(t, err)
	assert.False(t, v.Valid, "verifier must detect the tampered actor")
	assert.NotZero(t, v.BrokenAt)
}

// TestPhase2_AuditRetention prunes old rows and confirms the remaining window
// still verifies.
func TestPhase2_AuditRetention(t *testing.T) {
	s, _ := newTestServer(t)
	db := auditTestDB(t, s)
	al := bevent.NewAuditLogger(db, s.EventBus)
	defer al.Close()

	ws := "ws-retain"
	old := time.Now().Add(-60 * 24 * time.Hour)
	// One old event (explicit timestamp is preserved by the bus) then two recent.
	s.EventBus.Publish(platev.Event{Type: platev.EventMemberAdded, WorkspaceID: ws, Actor: "a", Timestamp: old, Data: map[string]string{"n": "old"}})
	s.EventBus.Publish(platev.Event{Type: platev.EventMemberAdded, WorkspaceID: ws, Actor: "a", Data: map[string]string{"n": "new1"}})
	s.EventBus.Publish(platev.Event{Type: platev.EventMemberAdded, WorkspaceID: ws, Actor: "a", Data: map[string]string{"n": "new2"}})

	ctx := t.Context()
	require.Eventually(t, func() bool {
		e, err := al.QueryAuditLog(ctx, bevent.AuditQuery{WorkspaceID: ws})
		return err == nil && len(e) == 3
	}, 3*time.Second, 20*time.Millisecond)

	n, err := al.PruneOlderThan(ctx, 30*24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n, "the 60-day-old row should be pruned")

	entries, err := al.QueryAuditLog(ctx, bevent.AuditQuery{WorkspaceID: ws})
	require.NoError(t, err)
	assert.Len(t, entries, 2)

	v, err := al.VerifyChain(ctx, ws)
	require.NoError(t, err)
	assert.True(t, v.Valid, "remaining window should still verify after prune: %+v", v)
}

// TestPhase2_AuditPersistsWorkspaceEvent proves the end-to-end path: an HTTP
// governance action persists a workspace-scoped (non-project) audit row, which
// the old project-filtered logger would have dropped.
func TestPhase2_AuditPersistsWorkspaceEvent(t *testing.T) {
	s, ownerToken := newTestServer(t)
	db := auditTestDB(t, s)
	al := bevent.NewAuditLogger(db, s.EventBus)
	defer al.Close()

	code := do(t, s, http.MethodPost, "/api/v1/test/roles", ownerToken,
		`{"name":"auditor","display_name":"Auditor","permissions":["view_content","audit_read"]}`)
	require.Equal(t, http.StatusCreated, code)

	ctx := t.Context()
	require.Eventually(t, func() bool {
		entries, err := al.QueryAuditLog(ctx, bevent.AuditQuery{WorkspaceID: "test-ws", EventType: "role.template"})
		return err == nil && len(entries) >= 1
	}, 3*time.Second, 20*time.Millisecond, "role.template.created should persist as a workspace-scoped audit row")
}
