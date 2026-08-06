package server

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/bowrain/mailer"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
)

// These tests pin the other half of the summons: a job that fails must reach
// the person waiting on it. Before this, flow.failed was published by nothing
// and consumed by nobody, so a translation that exhausted its retries left a row
// with status='failed' and told no one — indistinguishable, from outside, from a
// job that was still running.

// jobFailureHarness wires everything the summons touches — a SQLite
// notification and preference store, a scripted auth store, a recording mail
// sender — with no Postgres, no bus, and no network.
type jobFailureHarness struct {
	srv   *Server
	sends *recordingSender
	auth  *summonsAuthStore
}

func newJobFailureHarness(t *testing.T) *jobFailureHarness {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "jobfail.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	srv := shutdownOnCleanup(t, NewServer(DefaultConfig()))
	srv.Config.AppPublicURL = "https://app.example.test"
	srv.NotificationStore = bstore.NewNotificationStore(cs.DB())
	srv.PreferenceStore = bstore.NewPreferenceStore(cs.DB())

	as := &summonsAuthStore{
		workspace: &platauth.Workspace{ID: summonsWSID, Slug: summonsWSSlug, Name: "Acme"},
		members: []*platauth.Membership{
			{UserID: "owner", WorkspaceID: summonsWSID, Role: platauth.RoleOwner},
			{UserID: "admin", WorkspaceID: summonsWSID, Role: platauth.RoleAdmin},
			{UserID: "translator", WorkspaceID: summonsWSID, Role: platauth.RoleMember},
		},
		users: map[string]*platauth.User{
			"owner":      {ID: "owner", Email: "owner@acme.test", Name: "Otto"},
			"admin":      {ID: "admin", Email: "admin@acme.test", Name: "Ada"},
			"translator": {ID: "translator", Email: "t@acme.test", Name: "Tan"},
		},
	}
	srv.AuthStore = as

	sends := &recordingSender{}
	m, err := mailer.New(sends)
	require.NoError(t, err)
	srv.Mailer = m

	// The dispatcher persists and pushes; the summons resolves who and mails.
	srv.NotificationDispatcher = event.NewNotificationDispatcher(
		srv.EventBus, srv.NotificationStore, srv.PreferenceStore, srv, nil)
	t.Cleanup(srv.NotificationDispatcher.Close)

	return &jobFailureHarness{srv: srv, sends: sends, auth: as}
}

// failureEvent builds the event a worker publishes for a failed translation.
func failureEvent(initiator string) platev.Event {
	data := map[string]string{
		"workspace_slug": summonsWSSlug,
		"workspace_id":   summonsWSID,
		"job_id":         "job-1",
		"job_kind":       "translation",
		"item":           "en.json",
		"target_locale":  "nb",
		"stream":         "main",
		"error":          "openai: API error 401: invalid api key",
	}
	if initiator != "" {
		data["initiator"] = initiator
	}
	return platev.Event{
		Type:      platev.EventFlowFailed,
		ProjectID: "proj-1",
		Data:      data,
	}
}

func notificationsFor(t *testing.T, h *jobFailureHarness, userID string) []bstore.Notification {
	t.Helper()
	list, err := h.srv.NotificationStore.List(t.Context(), userID, 50, false)
	require.NoError(t, err)
	return list
}

// The person who asked for the work is the person who hears about it. Not every
// member of the project: a translation failure is not news for a translator who
// never asked for it.
func TestJobFailureSummonsTheInitiator(t *testing.T) {
	h := newJobFailureHarness(t)
	h.srv.summonOnJobFailure(t.Context(), failureEvent("translator"))

	told := notificationsFor(t, h, "translator")
	require.Len(t, told, 1)
	assert.Equal(t, bstore.NotificationFlowFailed, told[0].Type)
	assert.Equal(t, string(bstore.CategoryAutomation), told[0].Category)
	assert.Equal(t, "high", told[0].Priority)
	assert.Contains(t, told[0].Title, "translation job did not finish")
	assert.Contains(t, told[0].Body, "en.json → nb")
	assert.Contains(t, told[0].Body, "invalid api key")
	assert.Equal(t, "/acme/p/proj-1/s/main/runs", told[0].LinkURL)

	assert.Empty(t, notificationsFor(t, h, "owner"), "the owners are the fallback, not an addition")

	// Mail by default: the automation category ships with email off, and the
	// founder's rule for failures overrides it.
	require.Len(t, h.sends.messages(), 1)
	msg := h.sends.messages()[0]
	assert.Equal(t, "t@acme.test", msg.to)
	assert.Contains(t, msg.subject, "translation job did not finish")
	assert.Contains(t, msg.body, "https://app.example.test/acme/p/proj-1/s/main/runs")
	assert.Contains(t, msg.body, "invalid api key")
}

// A job the platform started itself names no initiator, and the failure belongs
// to whoever is responsible for the workspace.
func TestJobFailureFallsBackToWorkspaceOwners(t *testing.T) {
	h := newJobFailureHarness(t)
	h.srv.summonOnJobFailure(t.Context(), failureEvent(""))

	assert.Len(t, notificationsFor(t, h, "owner"), 1)
	assert.Len(t, notificationsFor(t, h, "admin"), 1)
	assert.Empty(t, notificationsFor(t, h, "translator"),
		"a plain member is not responsible for a job nobody asked for")

	var to []string
	for _, m := range h.sends.messages() {
		to = append(to, m.to)
	}
	assert.ElementsMatch(t, []string{"owner@acme.test", "admin@acme.test"}, to)
}

// Once per job, not once per delivery. The bus is at-least-once and the server
// may run on more than one instance, so the guard has to survive both.
func TestJobFailureSummonsOncePerJob(t *testing.T) {
	h := newJobFailureHarness(t)
	ev := failureEvent("translator")

	h.srv.summonOnJobFailure(t.Context(), ev)
	h.srv.summonOnJobFailure(t.Context(), ev)
	h.srv.summonOnJobFailure(t.Context(), ev)

	assert.Len(t, notificationsFor(t, h, "translator"), 1)
	assert.Len(t, h.sends.messages(), 1)
}

// A second, different job is a second summons — the guard is per job, not a
// blanket mute on the category.
func TestJobFailureSummonsAgainForADifferentJob(t *testing.T) {
	h := newJobFailureHarness(t)
	h.srv.summonOnJobFailure(t.Context(), failureEvent("translator"))

	second := failureEvent("translator")
	second.Data["job_id"] = "job-2"
	h.srv.summonOnJobFailure(t.Context(), second)

	assert.Len(t, notificationsFor(t, h, "translator"), 2)
	assert.Len(t, h.sends.messages(), 2)
}

// Notify-only: turning the automation category's email channel off leaves the
// in-app notification and stops the mail. This is the escape hatch the founder's
// "email by default" rule is paired with.
func TestJobFailureEmailHonoursThePreference(t *testing.T) {
	h := newJobFailureHarness(t)
	require.NoError(t, h.srv.PreferenceStore.BulkUpsert(t.Context(), []bstore.NotificationPreference{{
		UserID: "translator", WorkspaceID: summonsWSSlug,
		Category: bstore.CategoryAutomation,
		Web:      true, Email: false,
	}}))

	h.srv.summonOnJobFailure(t.Context(), failureEvent("translator"))

	assert.Len(t, notificationsFor(t, h, "translator"), 1, "the badge still lights")
	assert.Empty(t, h.sends.messages(), "and no mail goes out")
}

// A deployment that has not been told its own origin cannot build a link an
// email can follow. The in-app summons still lands — its link is a path the web
// hub resolves — and the mail is skipped rather than sent broken.
func TestJobFailureWithoutAnAppOriginStillNotifies(t *testing.T) {
	h := newJobFailureHarness(t)
	h.srv.Config.AppPublicURL = ""

	h.srv.summonOnJobFailure(t.Context(), failureEvent("translator"))

	assert.Len(t, notificationsFor(t, h, "translator"), 1)
	assert.Empty(t, h.sends.messages())
}

// The extraction queue's table has no workspace id, so its events carry the slug
// alone. The summons resolves the id from it rather than falling silent.
func TestJobFailureResolvesTheWorkspaceFromTheSlugAlone(t *testing.T) {
	h := newJobFailureHarness(t)
	ev := failureEvent("")
	delete(ev.Data, "workspace_id")
	ev.Data["job_kind"] = "extraction"

	h.srv.summonOnJobFailure(t.Context(), ev)

	assert.Len(t, notificationsFor(t, h, "owner"), 1)
}

func TestJobFailureIgnoresAnEventWithNoJob(t *testing.T) {
	h := newJobFailureHarness(t)
	ev := failureEvent("translator")
	delete(ev.Data, "job_id")

	assert.NotPanics(t, func() { h.srv.summonOnJobFailure(t.Context(), ev) })
	assert.Empty(t, notificationsFor(t, h, "translator"))
}

func TestJobKindProseAndSubject(t *testing.T) {
	// "sync" is what the protocol calls it; "push" is what the user ran.
	assert.Equal(t, "push", jobKindProse("sync"))
	assert.Equal(t, "translation", jobKindProse("translation"))
	assert.Equal(t, "background", jobKindProse(""))

	assert.Equal(t, "en.json → nb", jobFailureSubject(failureEvent("")))

	noItem := failureEvent("")
	delete(noItem.Data, "item")
	delete(noItem.Data, "target_locale")
	assert.Equal(t, "project proj-1", jobFailureSubject(noItem))
}
