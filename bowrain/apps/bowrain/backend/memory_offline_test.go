package backend

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/editorclient"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host/venue/config"
	"github.com/neokapi/neokapi/memory"
)

// memoryServer answers the content-memory routes as the Bowrain server does:
// an entry has a revision the server replaces on every write of its text; an
// update names the revision it was made on and is refused without one (428),
// on an older one (409, with the entry as it stands), or for an entry the
// server does not hold (404).
type memoryServer struct {
	mu      sync.Mutex
	entries map[string]*memoryServerEntry
	next    int
}

type memoryServerEntry struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	Target   string `json:"target"`
	Revision string `json:"revision"`
}

func (m *memoryServer) revise(e *memoryServerEntry) {
	m.next++
	e.Revision = "r" + strings.Repeat("i", m.next)
}

func (m *memoryServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	const prefix = "/api/v1/acme/translation-memory"
	switch {
	case r.URL.Path == "/api/v1/auth/me":
		_, _ = w.Write([]byte(`{"id":"u1","email":"a@example.com","name":"A"}`))
	case r.Method == http.MethodPost && r.URL.Path == prefix:
		var req struct {
			ID     string `json:"id"`
			Source string `json:"source"`
			Target string `json:"target"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		id := req.ID
		if id == "" {
			id = "server-minted"
		}
		if e, ok := m.entries[id]; ok {
			// The same add sent again is answered with the stored entry;
			// other text under the id is refused, as the server does.
			if e.Source != req.Source || e.Target != req.Target {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code": "memory_entry_exists", "error": "a content-memory entry with this id holds other text; the stored entry is attached", "current": e,
				})
				return
			}
			_ = json.NewEncoder(w).Encode(e)
			return
		}
		e := &memoryServerEntry{ID: id, Source: req.Source, Target: req.Target}
		m.revise(e)
		m.entries[id] = e
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(e)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, prefix+"/"):
		id := strings.TrimPrefix(r.URL.Path, prefix+"/")
		base := r.URL.Query().Get("base_revision")
		e, ok := m.entries[id]
		switch {
		case base == "":
			w.WriteHeader(http.StatusPreconditionRequired)
			_, _ = w.Write([]byte(`{"error":"this version of the app cannot save content-memory entries; update it, then save again"}`))
		case !ok:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"this content-memory entry was deleted after it was opened"}`))
		case e.Revision != base:
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": "memory_entry_changed", "error": "someone else saved this content-memory entry after you opened it; the current version is attached", "current": e,
			})
		default:
			delete(m.entries, id)
			w.WriteHeader(http.StatusNoContent)
		}
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, prefix+"/"):
		id := strings.TrimPrefix(r.URL.Path, prefix+"/")
		var req struct {
			Source       string `json:"source"`
			Target       string `json:"target"`
			BaseRevision string `json:"base_revision"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		e, ok := m.entries[id]
		switch {
		case req.BaseRevision == "":
			w.WriteHeader(http.StatusPreconditionRequired)
			_, _ = w.Write([]byte(`{"error":"this version of the app cannot save content-memory entries; update it, then save again"}`))
		case !ok:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"this content-memory entry was deleted after it was opened"}`))
		case e.Revision != req.BaseRevision:
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": "memory_entry_changed", "error": "someone else saved this content-memory entry after you opened it; the current version is attached", "current": e,
			})
		default:
			e.Source, e.Target = req.Source, req.Target
			m.revise(e)
			_ = json.NewEncoder(w).Encode(e)
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// offlineMemoryApp is an app connected to a memoryServer, then cut off.
func offlineMemoryApp(t *testing.T) (*App, *memoryServer, func()) {
	t.Helper()
	ms := &memoryServer{entries: map[string]*memoryServerEntry{}}
	srv := newTestServer(t, ms)
	t.Cleanup(srv.Close)
	app := newTestApp(t)
	q := newTestQueue(t)
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
	}
	app.offlineQueue = q
	app.mu.Lock()
	app.connState = StateOffline
	app.serverURL = srv.URL
	app.activeWS = "acme"
	app.authInfo = &config.StoredAuth{ServerURL: srv.URL, AccessToken: "tok"}
	app.mu.Unlock()
	reconnect := func() {
		app.mu.Lock()
		app.connState = StateConnected
		app.remoteHTTP = editorclient.New(srv.URL, "tok")
		app.mu.Unlock()
		app.replayPendingChanges(t.Context())
	}
	return app, ms, reconnect
}

// An entry added offline and then edited offline reaches the server with the
// edit once the app reconnects.
func TestAnEntryAddedAndEditedOfflineReachesTheServerEdited(t *testing.T) {
	app, ms, reconnect := offlineMemoryApp(t)

	added, err := app.AddMemoryEntry("", "Hello", "Hei", "en", "nb")
	require.NoError(t, err)
	require.NoError(t, app.UpdateMemoryEntry(MemoryUpdateRequest{
		EntryID: added.ID, Source: "Hello", Target: "Hallo", SourceLocale: "en", TargetLocale: "nb",
		BaseRevision: added.Revision,
	}))

	reconnect()

	failed, err := app.GetFailedChanges()
	require.NoError(t, err)
	assert.Empty(t, failed, "no queued change failed")
	ms.mu.Lock()
	defer ms.mu.Unlock()
	require.Len(t, ms.entries, 1)
	for _, e := range ms.entries {
		assert.Equal(t, "Hallo", e.Target, "the server holds the edit made offline")
	}
}

// sentOffline adds an entry offline and sends it, leaving the app offline
// again: the local memory holds the entry with the server's revision.
func sentOffline(t *testing.T) (*App, *memoryServer, func(), *MemoryEntryInfo) {
	t.Helper()
	app, ms, reconnect := offlineMemoryApp(t)
	added, err := app.AddMemoryEntry("", "Hello", "Hei", "en", "nb")
	require.NoError(t, err)
	reconnect()
	app.mu.Lock()
	app.connState = StateOffline
	app.mu.Unlock()

	result, err := app.GetMemoryEntries("", "", "en", "nb", 0, 10)
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)
	require.Equal(t, added.ID, result.Entries[0].ID)
	require.NotEmpty(t, result.Entries[0].Revision, "an offline read carries the revision the server gave the entry")
	return app, ms, reconnect, &result.Entries[0]
}

func serverTarget(ms *memoryServer, id string) string {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if e, ok := ms.entries[id]; ok {
		return e.Target
	}
	return ""
}

// An offline edit of an entry nobody changed meanwhile is saved on replay.
func TestAnOfflineEditOfAnUnchangedEntryIsSaved(t *testing.T) {
	app, ms, reconnect, entry := sentOffline(t)

	require.NoError(t, app.UpdateMemoryEntry(MemoryUpdateRequest{
		EntryID: entry.ID, Source: "Hello", Target: "Hallo", SourceLocale: "en", TargetLocale: "nb",
		BaseRevision: entry.Revision,
	}))
	reconnect()

	failed, err := app.GetFailedChanges()
	require.NoError(t, err)
	assert.Empty(t, failed)
	assert.Equal(t, "Hallo", serverTarget(ms, entry.ID))
}

// An offline edit of an entry someone changed meanwhile is refused as a
// conflict: the other person's text stands on the server, and the person's
// own wording stays readable beside why it was not saved.
func TestAnOfflineEditOfAChangedEntryIsAConflictAndLosesNothing(t *testing.T) {
	app, ms, reconnect, entry := sentOffline(t)
	ms.mu.Lock()
	ms.entries[entry.ID].Target = "Heisann"
	ms.revise(ms.entries[entry.ID])
	ms.mu.Unlock()

	require.NoError(t, app.UpdateMemoryEntry(MemoryUpdateRequest{
		EntryID: entry.ID, Source: "Hello", Target: "Hallo", SourceLocale: "en", TargetLocale: "nb",
		BaseRevision: entry.Revision,
	}))
	reconnect()

	assert.Equal(t, "Heisann", serverTarget(ms, entry.ID), "the other person's text stands")
	failed, err := app.GetFailedChanges()
	require.NoError(t, err)
	require.Len(t, failed, 1)
	assert.Equal(t, "someone else saved this content-memory entry after you opened it; the current version is attached", failed[0].Reason)
	require.Len(t, failed[0].Edits, 1)
	assert.Equal(t, "Hallo", failed[0].Edits[0].Text, "the person's wording is kept with the refusal")
}

// An edit queued by an earlier version, without the revision it was made on,
// is dropped with the notice that says so, never sent to be refused.
func TestAnEditQueuedBeforeRevisionsIsRetired(t *testing.T) {
	app, ms, reconnect, entry := sentOffline(t)
	app.enqueue(updateMemoryEntryOp{MemoryUpdateRequest{
		EntryID: entry.ID, Source: "Hello", Target: "Hallo", SourceLocale: "en", TargetLocale: "nb",
	}})
	reconnect()

	assert.Equal(t, "Hei", serverTarget(ms, entry.ID))
	failed, err := app.GetFailedChanges()
	require.NoError(t, err)
	require.Len(t, failed, 1)
	assert.Equal(t, "dropped", failed[0].Status)
	assert.Equal(t, errRetired.Error(), failed[0].Reason)
}

// An entry an earlier version made offline holds no server revision; an
// offline edit of it is refused at once, with why, rather than when it
// reaches the server.
func TestAnOfflineEditOfAnEntryWithoutARevisionIsRefusedAtOnce(t *testing.T) {
	app, _, _ := offlineMemoryApp(t)
	tm, err := app.getOrCreateMemory()
	require.NoError(t, err)
	require.NoError(t, tm.Add(t.Context(), memory.Entry{
		ID:          "legacy",
		Variants:    map[model.LocaleID][]model.Run{"en": {{Text: &model.TextRun{Text: "Hello"}}}, "nb": {{Text: &model.TextRun{Text: "Hei"}}}},
		HintSrcLang: "en",
	}))

	err = app.UpdateMemoryEntry(MemoryUpdateRequest{
		EntryID: "legacy", Source: "Hello", Target: "Hallo", SourceLocale: "en", TargetLocale: "nb",
	})
	require.ErrorIs(t, err, errUnsentOffline)
	assert.Zero(t, app.offlineQueue.PendingCount(t.Context()), "nothing is queued that can only be refused")
}

func editOffline(t *testing.T, app *App, entry *MemoryEntryInfo, target string) {
	t.Helper()
	require.NoError(t, app.UpdateMemoryEntry(MemoryUpdateRequest{
		EntryID: entry.ID, Source: "Hello", Target: target, SourceLocale: "en", TargetLocale: "nb",
		BaseRevision: entry.Revision,
	}))
}

// Two offline edits of an entry that was already sent: the server ends with
// the person's last wording, and nothing is refused as someone else's.
func TestTwoOfflineEditsOfASentEntryLandAsTheLast(t *testing.T) {
	app, ms, reconnect, entry := sentOffline(t)
	editOffline(t, app, entry, "Hallo")
	editOffline(t, app, entry, "Halloen")
	reconnect()

	failed, err := app.GetFailedChanges()
	require.NoError(t, err)
	assert.Empty(t, failed, "the person's own edits never refuse each other")
	assert.Equal(t, "Halloen", serverTarget(ms, entry.ID))
}

// Two edits of one entry queued as two rows (an earlier version queued every
// edit on its own): the second replays on the revision the first returned.
func TestASecondQueuedEditBuildsOnTheFirst(t *testing.T) {
	app, ms, reconnect, entry := sentOffline(t)
	for _, target := range []string{"Hallo", "Halloen"} {
		app.enqueue(updateMemoryEntryOp{MemoryUpdateRequest{
			EntryID: entry.ID, Source: "Hello", Target: target, SourceLocale: "en", TargetLocale: "nb",
			BaseRevision: entry.Revision,
		}})
	}
	reconnect()

	failed, err := app.GetFailedChanges()
	require.NoError(t, err)
	assert.Empty(t, failed)
	assert.Equal(t, "Halloen", serverTarget(ms, entry.ID))
}

// Edit, edit, delete offline: the entry is gone on the server, and nothing
// failed.
func TestEditEditDeleteOffline(t *testing.T) {
	app, ms, reconnect, entry := sentOffline(t)
	editOffline(t, app, entry, "Hallo")
	editOffline(t, app, entry, "Halloen")
	require.NoError(t, app.DeleteMemoryEntry("", entry.ID, entry.Revision))
	reconnect()

	failed, err := app.GetFailedChanges()
	require.NoError(t, err)
	assert.Empty(t, failed)
	ms.mu.Lock()
	defer ms.mu.Unlock()
	assert.NotContains(t, ms.entries, entry.ID)
}

// An add the server refuses for good, with an edit folded into it, lists the
// person's last wording beside why it was refused.
func TestARefusedAddKeepsThePersonsWording(t *testing.T) {
	app, ms, reconnect := offlineMemoryApp(t)
	added, err := app.AddMemoryEntry("", "Hello", "Hei", "en", "nb")
	require.NoError(t, err)
	require.NoError(t, app.UpdateMemoryEntry(MemoryUpdateRequest{
		EntryID: added.ID, Source: "Hello", Target: "Hallo", SourceLocale: "en", TargetLocale: "nb",
		BaseRevision: added.Revision,
	}))
	// The server already holds other text under the entry's id.
	ms.mu.Lock()
	ms.entries[added.ID] = &memoryServerEntry{ID: added.ID, Source: "Hello", Target: "Something else", Revision: "r0"}
	ms.mu.Unlock()
	reconnect()

	failed, err := app.GetFailedChanges()
	require.NoError(t, err)
	require.Len(t, failed, 1)
	require.Len(t, failed[0].Edits, 1)
	assert.Equal(t, "Hallo", failed[0].Edits[0].Text, "the person's wording is visible with the refusal")
}

// An offline edit refused as a conflict, then an offline delete of the same
// entry: the delete names the version the person saw, so it is refused too,
// and the teammate's text stays on the server.
func TestAnOfflineDeleteOfAChangedEntryIsAConflict(t *testing.T) {
	app, ms, reconnect, entry := sentOffline(t)
	ms.mu.Lock()
	ms.entries[entry.ID].Target = "Heisann"
	ms.revise(ms.entries[entry.ID])
	ms.mu.Unlock()

	editOffline(t, app, entry, "Hallo")
	require.NoError(t, app.DeleteMemoryEntry("", entry.ID, entry.Revision))
	reconnect()

	assert.Equal(t, "Heisann", serverTarget(ms, entry.ID), "the teammate's text is never removed by a delete made over it")
	failed, err := app.GetFailedChanges()
	require.NoError(t, err)
	require.Len(t, failed, 2, "the edit and the delete are both refused, each with why")
}

// A delete of an entry whose add has not been sent sends nothing: the add and
// the edit queued after it go with it.
func TestDeletingAnUnsentEntrySendsNothing(t *testing.T) {
	app, ms, reconnect := offlineMemoryApp(t)
	added, err := app.AddMemoryEntry("", "Hello", "Hei", "en", "nb")
	require.NoError(t, err)
	require.NoError(t, app.UpdateMemoryEntry(MemoryUpdateRequest{
		EntryID: added.ID, Source: "Hello", Target: "Hallo", SourceLocale: "en", TargetLocale: "nb",
	}))
	require.NoError(t, app.DeleteMemoryEntry("", added.ID, ""))
	assert.Zero(t, app.offlineQueue.PendingCount(t.Context()), "nothing is left to send")

	reconnect()
	ms.mu.Lock()
	defer ms.mu.Unlock()
	assert.Empty(t, ms.entries)
}

// A delete queued before deletes named a revision is retired, not sent.
func TestADeleteQueuedBeforeRevisionsIsRetired(t *testing.T) {
	app, ms, reconnect, entry := sentOffline(t)
	app.enqueue(deleteMemoryEntryOp{EntryID: entry.ID})
	reconnect()

	assert.Equal(t, "Hei", serverTarget(ms, entry.ID))
	failed, err := app.GetFailedChanges()
	require.NoError(t, err)
	require.Len(t, failed, 1)
	assert.Equal(t, "dropped", failed[0].Status)
}

// The person edits an entry as their screen showed it. A replay landed after
// that screen was drawn, moving the entry on: the edit names the version the
// person saw, so it is a conflict with their wording kept, never a silent save
// over the version they did not see.
func TestAnEditOfAnEntryThatMovedSinceItWasShownIsAConflict(t *testing.T) {
	app, ms, reconnect, shown := sentOffline(t)

	// An edit queued earlier replays in between, moving the entry on.
	editOffline(t, app, shown, "Hallo")
	reconnect()
	app.mu.Lock()
	app.connState = StateOffline
	app.mu.Unlock()

	// The person's screen still shows the entry as it was.
	editOffline(t, app, shown, "Halloen")
	reconnect()

	assert.Equal(t, "Hallo", serverTarget(ms, shown.ID), "nothing lands over a version the person did not see")
	failed, err := app.GetFailedChanges()
	require.NoError(t, err)
	require.Len(t, failed, 1)
	require.Len(t, failed[0].Edits, 1)
	assert.Equal(t, "Halloen", failed[0].Edits[0].Text, "the person's wording is kept with the conflict")
}

// A change that names no revision is refused at once: the app never supplies
// one from the store.
func TestAChangeThatNamesNoRevisionIsRefused(t *testing.T) {
	app, _, _, entry := sentOffline(t)

	err := app.UpdateMemoryEntry(MemoryUpdateRequest{
		EntryID: entry.ID, Source: "Hello", Target: "Hallo", SourceLocale: "en", TargetLocale: "nb",
	})
	require.ErrorIs(t, err, errRevisionNotSent)
	require.ErrorIs(t, app.DeleteMemoryEntry("", entry.ID, ""), errRevisionNotSent)
	assert.Zero(t, app.offlineQueue.PendingCount(t.Context()), "nothing is queued")
}
