package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	bevent "github.com/neokapi/neokapi/bowrain/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	voicepg "github.com/neokapi/neokapi/bowrain/voice"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/host/screens"
)

// contextEventsServer is the full server over PostgreSQL with the audit
// logger, the activity recorder and a voice store wired as production wires
// them, served on a local listener. It returns the server, its URL and the
// owner's token for workspace "test" (id "test-ws").
func contextEventsServer(t *testing.T) (*Server, string, string) {
	t.Helper()
	srv, token := newTestServer(t)
	db := auditTestDB(t, srv)
	srv.AuditLogger = bevent.NewAuditLogger(db, srv.EventBus)
	t.Cleanup(srv.AuditLogger.Close)
	srv.ActivityStore = bstore.NewActivityStore(db)
	recorder := bevent.NewActivityRecorder(srv.ActivityStore, srv.EventBus)
	t.Cleanup(recorder.Close)
	vs, err := voicepg.NewPostgresVoiceStore(pgtest.NewTestDB(t))
	require.NoError(t, err)
	srv.VoiceStore = vs
	hs := httptest.NewServer(srv.GetEcho())
	t.Cleanup(hs.Close)
	return srv, hs.URL, token
}

// contextAudit waits for the audit entry of type evType about resourceID and
// returns it with its data decoded.
func contextAudit(t *testing.T, srv *Server, evType platev.EventType, resourceID string) (bevent.AuditEntry, map[string]string) {
	t.Helper()
	var found bevent.AuditEntry
	require.Eventually(t, func() bool {
		entries, err := srv.AuditLogger.QueryAuditLog(t.Context(), bevent.AuditQuery{WorkspaceID: "test-ws", EventType: string(evType), Limit: 100})
		if err != nil {
			return false
		}
		for _, e := range entries {
			if e.EventType == string(evType) && e.ResourceID == resourceID {
				found = e
				return true
			}
		}
		return false
	}, 5*time.Second, 20*time.Millisecond, "no %s audit entry about %s", evType, resourceID)
	data := map[string]string{}
	require.NoError(t, json.Unmarshal([]byte(found.Data), &data))
	return found, data
}

// contextActivity waits for the context_change activity row about entityID
// in workspace "test".
func contextActivity(t *testing.T, srv *Server, entityID string) bstore.Activity {
	t.Helper()
	var found bstore.Activity
	require.Eventually(t, func() bool {
		res, err := srv.ActivityStore.List(t.Context(), bstore.ActivityQuery{WorkspaceID: "test", Type: string(bstore.ActivityContextChanged), Limit: 100})
		if err != nil {
			return false
		}
		for _, a := range res.Activities {
			if a.EntityID == entityID {
				found = a
				return true
			}
		}
		return false
	}, 5*time.Second, 20*time.Millisecond, "no context activity about %s", entityID)
	assert.Equal(t, string(bstore.FeedKindContextChange), found.Data["feed_kind"])
	return found
}

// verifyAuditChain asks the server to verify the workspace's audit chain.
func verifyAuditChain(t *testing.T, base, token string) {
	t.Helper()
	status, body := rawRequest(t, http.MethodGet, base+"/api/v1/test/audit-log/verify", token, nil)
	require.Equal(t, http.StatusOK, status, string(body))
	var v bevent.ChainVerification
	require.NoError(t, json.Unmarshal(body, &v))
	assert.True(t, v.Valid, "the audit chain verifies: %+v", v)
	assert.Positive(t, v.Rows)
}

// A push of context objects publishes one context.recorded event for the
// operations the project did not hold: the audit log records it with the
// counts by item kind and operation, the activity recorder files one row,
// and a push of the same segment again changes nothing and publishes
// nothing.
func TestContextEvents_PushIsRecorded_Postgres(t *testing.T) {
	srv, base, token := contextEventsServer(t)
	pid := seedDecideParityIn(t, srv.ContentStore, "test-ws", func(pid string) workspace.Remote {
		return contextRemote(t, base, "test", pid, token)
	})

	entry, data := contextAudit(t, srv, platev.EventContextRecorded, pid)
	assert.Equal(t, "test-user", entry.Actor)
	assert.Equal(t, pid, entry.ProjectID)
	assert.Equal(t, contextResourceType, entry.ResourceType)
	assert.Equal(t, "push", data["source"])
	assert.Equal(t, "agent", data["actor_kind"], "a push holding an agent's operations is an agent's")
	assert.Equal(t, "s-parity", data["session"])
	assert.Equal(t, "Test", data["actor_name"])
	assert.Equal(t, "7", data["count"])
	counts := platev.ParseContextChange(data).Counts
	assert.Equal(t, platev.ContextCounts{
		platev.ContextItemTermRule: {platev.ContextOpAdd: 4, platev.ContextOpKeep: 1},
		platev.ContextItemNote:     {platev.ContextOpAdd: 1},
		platev.ContextItemBinding:  {platev.ContextOpAdd: 1},
	}, counts)

	a := contextActivity(t, srv, "")
	assert.Equal(t, pid, a.ProjectID)
	assert.Equal(t, "test-user", a.ActorID)
	assert.Equal(t, "context", a.EntityType, "a change of several kinds names none")
	assert.Equal(t, "added 1 binding · added 1 note · added 4 term rules · kept 1 term rule", a.Summary)

	// The same segment again holds nothing new.
	cos, ok := srv.contextObjects()
	require.True(t, ok)
	names, err := cos.ListContextObjects(t.Context(), pid, workspace.RemoteLogDir)
	require.NoError(t, err)
	require.Len(t, names, 1)
	obj, err := cos.GetContextObject(t.Context(), pid, names[0])
	require.NoError(t, err)
	require.NoError(t, contextRemote(t, base, "test", pid, token).Put(t.Context(), workspace.Object{Name: names[0], Data: obj.Data}))
	verifyAuditChain(t, base, token)
	entries, err := srv.AuditLogger.QueryAuditLog(t.Context(), bevent.AuditQuery{WorkspaceID: "test-ws", EventType: string(platev.EventContextRecorded)})
	require.NoError(t, err)
	assert.Len(t, entries, 1, "a push of held operations records nothing")
}

// Each Decide answer publishes one context.recorded event about the
// suggestion it answered, with what the answer recorded counted.
func TestContextEvents_DecideIsRecorded_Postgres(t *testing.T) {
	srv, base, token := contextEventsServer(t)
	pid := seedDecideParityIn(t, srv.ContentStore, "test-ws", func(pid string) workspace.Remote {
		return contextRemote(t, base, "test", pid, token)
	})
	route := base + "/api/v1/test/" + pid + "/rules/main/decide"
	status, body := rawRequest(t, http.MethodGet, route, token, nil)
	require.Equal(t, http.StatusOK, status, string(body))
	var view screens.RulesDecideViewDTO
	require.NoError(t, json.Unmarshal(body, &view))
	var name, note, side screens.RuleDecisionDTO
	for _, d := range view.Decisions {
		switch d.Kind {
		case "name":
			name = d
		case "note":
			note = d
		case "disagreement":
			side = d
		}
	}
	require.NotEmpty(t, name.ID, "%+v", view.Decisions)
	require.NotEmpty(t, note.ID, "%+v", view.Decisions)
	require.NotEmpty(t, side.Options, "%+v", view.Decisions)

	decide := func(id, answer, form string) {
		raw, err := json.Marshal(ruleDecideRequest{ID: id, Answer: answer, Form: form})
		require.NoError(t, err)
		status, body := rawRequest(t, http.MethodPost, route, token, raw)
		require.Equal(t, http.StatusOK, status, string(body))
	}

	decide(name.ID, "keep", "Sign In")
	_, data := contextAudit(t, srv, platev.EventContextRecorded, name.ID)
	assert.Equal(t, "decide", data["source"])
	assert.Equal(t, "person", data["actor_kind"])
	assert.Equal(t, "Test", data["actor_name"])
	assert.Equal(t, "term_rule", data["item_kind"])
	assert.Equal(t, "keep", data["operation"])
	assert.Equal(t, name.ID, data["item_id"])
	assert.Equal(t, "1", data["count"])
	a := contextActivity(t, srv, name.ID)
	assert.Equal(t, pid, a.ProjectID)
	assert.Equal(t, "term_rule", a.EntityType)
	assert.Equal(t, "kept 1 term rule", a.Summary)

	employ := side.Options[len(side.Options)-1]
	decide(employ.ID, "keep", "")
	_, data = contextAudit(t, srv, platev.EventContextRecorded, employ.ID)
	assert.Empty(t, data["operation"], "an answer that keeps one side and drops another names no single operation")
	assert.Equal(t, platev.ContextCounts{
		platev.ContextItemTermRule: {platev.ContextOpKeep: 1, platev.ContextOpDrop: 1},
	}, platev.ParseContextChange(data).Counts)
	assert.Equal(t, "kept 1 term rule · dropped 1 term rule", contextActivity(t, srv, employ.ID).Summary)

	decide(note.ID, "drop", "")
	_, data = contextAudit(t, srv, platev.EventContextRecorded, note.ID)
	assert.Equal(t, "note", data["item_kind"])
	assert.Equal(t, "drop", data["operation"])
	assert.Equal(t, "dropped 1 note", contextActivity(t, srv, note.ID).Summary)

	verifyAuditChain(t, base, token)
}

// Decisions about voice rules and edits of a voice profile carry the context
// change they make, so the audit log records who decided what and the
// activity recorder files a context_change row for each.
func TestContextEvents_VoiceDecisionsAreRecorded_Postgres(t *testing.T) {
	srv, base, token := contextEventsServer(t)
	profile := &coreprofile.VoiceProfile{ID: "p-context-events", Scope: "test-ws", Name: "Docs"}
	require.NoError(t, srv.VoiceStore.CreateProfile(t.Context(), profile))
	route := base + "/api/v1/test/voice-profiles/" + profile.ID

	status, body := rawRequest(t, http.MethodPost, route+"/reject-rule", token, []byte(`{"term":"utilize","replacement":"use"}`))
	require.Equal(t, http.StatusOK, status, string(body))
	status, body = rawRequest(t, http.MethodPost, route+"/promote-rule", token, []byte(`{"term":"leverage","replacement":"use"}`))
	require.Equal(t, http.StatusOK, status, string(body))
	status, body = rawRequest(t, http.MethodPost, route+"/demote-rule", token, []byte(`{"term":"leverage"}`))
	require.Equal(t, http.StatusOK, status, string(body))
	status, body = rawRequest(t, http.MethodPut, route, token, []byte(`{"name":"Docs voice"}`))
	require.Equal(t, http.StatusOK, status, string(body))

	cases := []struct {
		evType  platev.EventType
		item    string
		kind    string
		op      string
		summary string
	}{
		{platev.EventVoiceRuleRejected, "utilize", "voice_rule", "drop", "dropped 1 voice rule"},
		{platev.EventVoiceRulePromoted, "leverage", "voice_rule", "keep", "kept 1 voice rule"},
		{platev.EventVoiceRuleDemoted, "leverage", "voice_rule", "retire", "retired 1 voice rule"},
		{platev.EventVoiceProfileUpdated, profile.ID, "voice_profile", "edit", "edited 1 voice profile"},
	}
	for _, tc := range cases {
		t.Run(string(tc.evType), func(t *testing.T) {
			entry, data := contextAudit(t, srv, tc.evType, profile.ID)
			assert.Equal(t, "test-user", entry.Actor)
			assert.Equal(t, "voice", data["source"])
			assert.Equal(t, "person", data["actor_kind"])
			assert.Equal(t, "Test", data["actor_name"])
			assert.Equal(t, tc.kind, data["item_kind"])
			assert.Equal(t, tc.op, data["operation"])
			assert.Equal(t, tc.item, data["item_id"])

			var found *bstore.Activity
			require.Eventually(t, func() bool {
				res, err := srv.ActivityStore.List(t.Context(), bstore.ActivityQuery{WorkspaceID: "test", Type: string(bstore.ActivityContextChanged), Limit: 100})
				if err != nil {
					return false
				}
				for i, a := range res.Activities {
					if a.EntityID == tc.item && a.Summary == tc.summary {
						found = &res.Activities[i]
						return true
					}
				}
				return false
			}, 5*time.Second, 20*time.Millisecond)
			assert.Empty(t, found.ProjectID, "a voice profile belongs to the workspace")
			assert.Equal(t, tc.kind, found.EntityType)
			assert.Equal(t, string(bstore.FeedKindContextChange), found.Data["feed_kind"])
		})
	}
	verifyAuditChain(t, base, token)
}
