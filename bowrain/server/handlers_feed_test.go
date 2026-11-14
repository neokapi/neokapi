package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	bevent "github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
)

// feedServer is the full server over PostgreSQL with the activity recorder
// subscribed to its bus, as production wires it, and project p-feed in
// workspace "test" (id "test-ws"). It returns the server, the owner's token
// and a stop function that waits for the recorder to file every event
// published so far.
func feedServer(t *testing.T) (*Server, string, func()) {
	t.Helper()
	srv, token := newTestServer(t)
	db := srv.ContentStore.(*bstore.PostgresStore).SQLDB()
	srv.ActivityStore = bstore.NewActivityStore(db)
	recorder := bevent.NewActivityRecorder(srv.ActivityStore, srv.EventBus,
		bevent.WithProjectWorkspaces(&contentStoreWorkspaceResolver{store: srv.ContentStore}))
	require.NoError(t, srv.ContentStore.CreateProject(t.Context(), &store.Project{
		ID: "p-feed", Name: "Docs", WorkspaceID: "test-ws",
		DefaultSourceLanguage: "en", TargetLanguages: []model.LocaleID{"nb"},
	}))
	stopped := false
	stop := func() {
		if !stopped {
			stopped = true
			// Close returns once the handler has finished every event
			// already delivered.
			recorder.Close()
		}
	}
	t.Cleanup(stop)
	return srv, token, stop
}

// getFeed reads one page of the feed through the HTTP route.
func getFeed(t *testing.T, srv *Server, token, project string, q url.Values) (int, FeedResponse) {
	t.Helper()
	target := "/api/v1/test/" + project + "/feed"
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, req)
	var resp FeedResponse
	if rec.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp), rec.Body.String())
	}
	return rec.Code, resp
}

func entriesByKind(resp FeedResponse) map[string][]FeedEntry {
	out := map[string][]FeedEntry{}
	for _, e := range resp.Entries {
		out[e.Kind] = append(out[e.Kind], e)
	}
	return out
}

// Every kind of the feed, filed by the recorder from the events the server
// publishes, through GET /:ws/:id/feed.
func TestProjectFeed_EveryKind(t *testing.T) {
	srv, token, stop := feedServer(t)
	bus := srv.EventBus
	at := time.Date(2026, 11, 14, 9, 10, 0, 0, time.UTC)

	bus.Publish(platev.Event{
		ID: "ev-push", Type: platev.EventPushCompleted, Source: "sync-worker", ProjectID: "p-feed",
		Actor: "test-user", Timestamp: at,
		Data: map[string]string{
			"stream": "main", "files_count": "3", "blocks_count": "40", "push_id": "push-1",
			"workspace_slug": "test",
		},
	})
	bus.Publish(platev.Event{
		ID: "ev-run", Type: platev.EventConvergenceRunCompleted, Source: "convergence", ProjectID: "p-feed",
		Timestamp: at.Add(time.Minute),
		Data: map[string]string{
			"run_id": "run-1", "state": "converged", "passes": "2", "locales": "nb,de",
			"locales_count": "2", "produced_count": "40", "workspace_id": "test-ws", "workspace_slug": "test",
		},
	})
	// Three decisions and a bulk pass in nb in one hour make one row; the
	// redelivered decision is counted once. A decision in de makes its own.
	for _, d := range []struct{ id, decision, locale string }{
		{"ev-r1", "approved", "nb"}, {"ev-r2", "approved", "nb"}, {"ev-r3", "rejected", "nb"},
		{"ev-r1", "approved", "nb"}, {"ev-r4", "approved", "de"},
	} {
		bus.Publish(platev.Event{
			ID: d.id, Type: platev.EventReviewDecided, Source: "server", ProjectID: "p-feed",
			WorkspaceID: "test-ws", Actor: "test-user", Timestamp: at.Add(2 * time.Minute),
			Data: map[string]string{"locale": d.locale, "stream": "main", "decision": d.decision},
		})
	}
	bus.Publish(platev.Event{
		ID: "ev-bulk", Type: platev.EventReviewBulkApproved, Source: "server", ProjectID: "p-feed",
		WorkspaceID: "test-ws", Actor: "test-user", Timestamp: at.Add(20 * time.Minute),
		Data: map[string]string{"locale": "nb", "stream": "main", "approved": "5"},
	})
	bus.Publish(platev.Event{
		ID: "ev-recipe", Type: platev.EventRecipeChanged, Source: "sync-worker", ProjectID: "p-feed",
		Actor: "test-user", Timestamp: at.Add(3 * time.Minute),
		Data: platev.RecipeChange{
			WorkspaceSlug: "test", Source: "push", Stream: "main",
			Collections: []platev.CollectionChange{{
				Name: "docs", Change: platev.CollectionCoordinates,
				Before: map[string]string{"product": "kapi"}, After: map[string]string{"product": "bowrain"},
			}},
		}.Data(),
	})
	counts := platev.ContextCounts{}
	counts.Add(platev.ContextItemTermRule, platev.ContextOpAdd, 6)
	bus.Publish(platev.Event{
		ID: "ev-context", Type: platev.EventContextRecorded, Source: "server", ProjectID: "p-feed",
		WorkspaceID: "test-ws", Actor: "test-user", Timestamp: at.Add(4 * time.Minute),
		Data: platev.ContextChange{
			WorkspaceSlug: "test", ActorKind: platev.ContextActorAgent, ActorName: "claude-code",
			Session: "0ab4e399", Source: "push", Counts: counts,
			Extra: map[string]string{"host": "laptop"},
		}.Data(),
	})
	bus.Publish(platev.Event{
		ID: "ev-cs", Type: knowledge.EventChangeSetMerged, Source: "knowledge", WorkspaceID: "test-ws",
		Actor: "test-user", Timestamp: at.Add(5 * time.Minute),
		Data: map[string]string{"workspace_slug": "test", "changeset_id": "cs-1", "project_id": "p-feed"},
	})
	// A row outside the project feed: listed by the workspace feed only.
	bus.Publish(platev.Event{
		ID: "ev-stream", Type: platev.EventStreamCreated, ProjectID: "p-feed", Actor: "test-user",
		Timestamp: at.Add(6 * time.Minute), Data: map[string]string{"stream": "next", "workspace_slug": "test"},
	})
	stop()
	// No emitter files a milestone yet; the route serves the row as stored.
	require.NoError(t, srv.ActivityStore.Create(t.Context(), &bstore.Activity{
		WorkspaceID: "test", ProjectID: "p-feed", ActorID: "system", Type: bstore.ActivityProgressMilestone,
		EntityType: "locale", EntityID: "nb", Kind: bstore.FeedKindCoverageMilestone, ActorKind: bstore.ActorKindTool,
		Locales: []string{"nb"}, Counts: map[string]int{"percent": 90, "blocks": 36}, CreatedAt: at.Add(7 * time.Minute),
	}))

	code, resp := getFeed(t, srv, token, "p-feed", nil)
	require.Equal(t, http.StatusOK, code)
	by := entriesByKind(resp)
	for _, k := range bstore.FeedKinds {
		assert.NotEmpty(t, by[string(k)], "kind %s", k)
	}
	assert.Len(t, resp.Entries, 8, "the stream row stays out of the project feed")

	push := by["push"][0]
	assert.Equal(t, map[string]int{"files": 3, "blocks": 40}, push.Counts)
	assert.Equal(t, FeedActor{ID: "test-user", Kind: "person", Name: "Test"}, push.Actor)
	assert.Equal(t, FeedLink{Screen: FeedScreenStream, ID: "push-1", Stream: "main"}, push.Link)
	assert.True(t, push.At.Equal(at), "the entry carries the event's own time, got %s", push.At)

	run := by["convergence_run"][0]
	assert.Equal(t, "converged", run.State)
	assert.Equal(t, "tool", run.Actor.Kind)
	assert.Equal(t, []string{"nb", "de"}, run.Locales)
	assert.Equal(t, map[string]int{"passes": 2, "locales": 2, "produced": 40}, run.Counts)
	assert.Equal(t, FeedLink{Screen: FeedScreenRun, ID: "run-1"}, run.Link)

	require.Len(t, by["review_summary"], 2)
	reviews := map[string]FeedEntry{}
	for _, e := range by["review_summary"] {
		reviews[e.Locales[0]] = e
	}
	assert.Equal(t, map[string]int{"approved": 2, "rejected": 1, "bulk_approved": 5}, reviews["nb"].Counts)
	assert.Equal(t, map[string]int{"approved": 1}, reviews["de"].Counts)
	assert.Equal(t, FeedLink{Screen: FeedScreenReviewDeck, Locale: "nb", Stream: "main"}, reviews["nb"].Link)

	recipe := by["recipe_change"][0]
	assert.Equal(t, map[string]int{"coordinates_changed": 1}, recipe.Counts)
	assert.Equal(t, map[string]string{"product": "bowrain"}, recipe.Coordinates)
	require.NotNil(t, recipe.Diff)
	require.Len(t, recipe.Diff.Collections, 1)
	assert.Equal(t, map[string]string{"product": "kapi"}, recipe.Diff.Collections[0].Before)
	assert.Equal(t, FeedLink{Screen: FeedScreenRecipeDiff, ID: recipe.ID}, recipe.Link)

	ctxEntry := by["context_change"][0]
	assert.Equal(t, FeedActor{ID: "test-user", Kind: "agent", Name: "claude-code", Session: "0ab4e399", Host: "laptop"}, ctxEntry.Actor)
	assert.Equal(t, &FeedMember{ID: "test-user", Name: "Test"}, ctxEntry.OnBehalfOf)
	assert.Equal(t, map[string]int{"term_rule": 6, "add": 6}, ctxEntry.Counts)
	assert.Equal(t, FeedLink{Screen: FeedScreenContextDeck, Mode: "pending"}, ctxEntry.Link)

	cs := by["change_set"][0]
	assert.Equal(t, "merged", cs.State)
	assert.Equal(t, FeedLink{Screen: FeedScreenChangeSet, ID: "cs-1"}, cs.Link)

	ms := by["coverage_milestone"][0]
	assert.Equal(t, FeedLink{Screen: FeedScreenLanguage, Locale: "nb"}, ms.Link)

	// Newest first.
	for i := 1; i < len(resp.Entries); i++ {
		assert.False(t, resp.Entries[i].At.After(resp.Entries[i-1].At), "entry %d is newer than the one before it", i)
	}
}

// Rows that share a timestamp are each listed once across page boundaries.
func TestProjectFeed_PagesAcrossSameSecondRows(t *testing.T) {
	srv, token, stop := feedServer(t)
	at := time.Date(2026, 11, 14, 9, 10, 0, 0, time.UTC)
	for i := range 7 {
		srv.EventBus.Publish(platev.Event{
			ID: "ev-push-" + string(rune('a'+i)), Type: platev.EventPushCompleted, ProjectID: "p-feed",
			Actor: "test-user", Timestamp: at,
			Data: map[string]string{"stream": "main", "files_count": "1", "blocks_count": "1", "workspace_slug": "test"},
		})
	}
	stop()

	seen := map[string]int{}
	cursor := ""
	for range 10 {
		q := url.Values{"limit": {"2"}}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		code, resp := getFeed(t, srv, token, "p-feed", q)
		require.Equal(t, http.StatusOK, code)
		for _, e := range resp.Entries {
			seen[e.ID]++
		}
		if resp.NextCursor == "" {
			break
		}
		cursor = resp.NextCursor
	}
	assert.Len(t, seen, 7)
	for id, n := range seen {
		assert.Equal(t, 1, n, "entry %s", id)
	}

	code, _ := getFeed(t, srv, token, "p-feed", url.Values{"cursor": {"2026-11-14T09:10:00Z"}})
	assert.Equal(t, http.StatusBadRequest, code, "a cursor the route did not issue")
}

// Each filter narrows the feed.
func TestProjectFeed_Filters(t *testing.T) {
	srv, token, stop := feedServer(t)
	bus := srv.EventBus
	at := time.Date(2026, 11, 14, 9, 10, 0, 0, time.UTC)

	bus.Publish(platev.Event{
		ID: "f-push", Type: platev.EventPushCompleted, ProjectID: "p-feed", Actor: "test-user", Timestamp: at,
		Data: map[string]string{"stream": "main", "files_count": "1", "blocks_count": "1", "workspace_slug": "test"},
	})
	bus.Publish(platev.Event{
		ID: "f-review", Type: platev.EventReviewDecided, ProjectID: "p-feed", WorkspaceID: "test-ws",
		Actor: "reviewer-1", Timestamp: at.Add(time.Minute),
		Data: map[string]string{"locale": "de", "decision": "approved"},
	})
	counts := platev.ContextCounts{}
	counts.Add(platev.ContextItemNote, platev.ContextOpKeep, 1)
	bus.Publish(platev.Event{
		ID: "f-context", Type: platev.EventContextRecorded, ProjectID: "p-feed", WorkspaceID: "test-ws",
		Actor: "test-user", Timestamp: at.Add(2 * time.Minute),
		Data: platev.ContextChange{
			WorkspaceSlug: "test", ActorKind: platev.ContextActorAgent, Source: "decide", ItemID: "op-9", Counts: counts,
		}.Data(),
	})
	bus.Publish(platev.Event{
		ID: "f-recipe", Type: platev.EventRecipeChanged, ProjectID: "p-feed", Actor: "test-user",
		Timestamp: at.Add(3 * time.Minute),
		Data: platev.RecipeChange{
			WorkspaceSlug: "test", Source: "push",
			Collections: []platev.CollectionChange{{
				Name: "site", Change: platev.CollectionAdded,
				After: map[string]string{"product": "kapi", "channel": "web"},
			}},
		}.Data(),
	})
	stop()

	kinds := func(resp FeedResponse) []string {
		var out []string
		for _, e := range resp.Entries {
			out = append(out, e.Kind)
		}
		return out
	}
	for _, tc := range []struct {
		name string
		q    url.Values
		want []string
	}{
		{"kind", url.Values{"kind": {"push,context_change"}}, []string{"context_change", "push"}},
		{"locale", url.Values{"locale": {"de"}}, []string{"review_summary"}},
		{"coordinate", url.Values{"coordinate": {"product:kapi", "channel:web"}}, []string{"recipe_change"}},
		{"coordinate miss", url.Values{"coordinate": {"product:bowrain"}}, nil},
		{"actor", url.Values{"actor": {"reviewer-1"}}, []string{"review_summary"}},
		{"actor kind", url.Values{"actor_kind": {"agent"}}, []string{"context_change"}},
		{"since", url.Values{"since": {at.Add(90 * time.Second).Format(time.RFC3339)}}, []string{"recipe_change", "context_change"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, resp := getFeed(t, srv, token, "p-feed", tc.q)
			require.Equal(t, http.StatusOK, code)
			assert.Equal(t, tc.want, kinds(resp))
		})
	}

	for _, q := range []url.Values{
		{"kind": {"stream"}}, {"actor_kind": {"robot"}}, {"coordinate": {"product"}}, {"since": {"yesterday"}},
	} {
		code, _ := getFeed(t, srv, token, "p-feed", q)
		assert.Equal(t, http.StatusBadRequest, code, "%v", q)
	}
}

// A member who sets the target languages files a recipe change with the
// languages before and after.
func TestProjectFeed_TargetLanguagesChange(t *testing.T) {
	srv, token, stop := feedServer(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/test/p-feed", strings.NewReader(`{"target_languages":["nb","de","fr"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	stop()

	code, resp := getFeed(t, srv, token, "p-feed", url.Values{"kind": {"recipe_change"}})
	require.Equal(t, http.StatusOK, code)
	require.Len(t, resp.Entries, 1)
	e := resp.Entries[0]
	assert.Equal(t, map[string]int{"targets_added": 2}, e.Counts)
	assert.Equal(t, []string{"de", "fr"}, e.Locales)
	require.NotNil(t, e.Diff)
	require.NotNil(t, e.Diff.Targets)
	assert.Equal(t, []string{"nb"}, e.Diff.Targets.Before)
	assert.Equal(t, []string{"nb", "de", "fr"}, e.Diff.Targets.After)
	assert.Equal(t, "Test", e.Actor.Name)
}

// The feed answers members only, as every project route does.
func TestProjectFeed_Permission(t *testing.T) {
	srv, ownerToken, _ := feedServer(t)

	outsider := &platauth.User{ID: "feed-outsider", Email: "outsider@example.com", Name: "Outsider"}
	require.NoError(t, srv.AuthStore.CreateUser(t.Context(), outsider))
	outsiderToken, err := platauth.GenerateToken(outsider, "test-secret", time.Hour)
	require.NoError(t, err)
	code, _ := getFeed(t, srv, outsiderToken, "p-feed", nil)
	assert.True(t, code == http.StatusForbidden || code == http.StatusNotFound,
		"a non-member must be refused, got %d", code)

	code, _ = getFeed(t, srv, "", "p-feed", nil)
	assert.Equal(t, http.StatusUnauthorized, code)

	code, _ = getFeed(t, srv, ownerToken, "p-feed", nil)
	assert.Equal(t, http.StatusOK, code)
}
