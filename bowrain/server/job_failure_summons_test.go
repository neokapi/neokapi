package server

import (
	"fmt"
	"path/filepath"
	"strconv"
	"testing"
	"time"

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
// the person waiting on it. Unsummoned, a translation that exhausts its retries
// leaves a row with status='failed' and tells no one — indistinguishable, from
// outside, from a job that is still running.

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

	// Mail waits for a group to settle. The tests settle it by hand, so no
	// timer fires in the middle of one.
	srv.failureMail().delay = time.Hour

	return &jobFailureHarness{srv: srv, sends: sends, auth: as}
}

// settle sends the mail the summons is holding, as the settle timer would.
func (h *jobFailureHarness) settle(t *testing.T) {
	t.Helper()
	h.srv.flushJobFailureMail(t.Context())
}

// mailTo counts the messages sent to one address.
func (h *jobFailureHarness) mailTo(addr string) []sentMessage {
	var out []sentMessage
	for _, m := range h.sends.messages() {
		if m.to == addr {
			out = append(out, m)
		}
	}
	return out
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
	h.settle(t)

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
	h.settle(t)

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

// Notify-only: turning the automation category's email channel off leaves the
// in-app notification and stops the mail. It is the escape hatch the
// "email by default" rule is paired with.
func TestJobFailureEmailHonoursThePreference(t *testing.T) {
	h := newJobFailureHarness(t)
	require.NoError(t, h.srv.PreferenceStore.BulkUpsert(t.Context(), []bstore.NotificationPreference{{
		UserID: "translator", WorkspaceID: summonsWSSlug,
		Category: bstore.CategoryAutomation,
		Web:      true, Email: false,
	}}))

	h.srv.summonOnJobFailure(t.Context(), failureEvent("translator"))
	h.settle(t)

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
	h.settle(t)

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

// jobFailed builds the event for one failed job with its own id, item and
// reason, as a fan-out of many jobs produces.
func jobFailed(jobID, initiator, item, reason string) platev.Event {
	ev := failureEvent(initiator)
	ev.Data["job_id"] = jobID
	ev.Data["item"] = item
	ev.Data["error"] = reason
	return ev
}

// burst is n failures of distinct jobs for one reason, the shape of a run
// fanning out into a workspace that has reached its usage limit.
func burst(n int, prefix, initiator, reason string) []platev.Event {
	out := make([]platev.Event, 0, n)
	for i := range n {
		out = append(out, jobFailed(fmt.Sprintf("%s-%d", prefix, i), initiator,
			fmt.Sprintf("docs/page-%d.md", i), reason))
	}
	return out
}

// A systemic failure reaches each recipient once, not once per job. The cases
// here are the incident's shape and its neighbours: one burst, two causes, one
// failure on its own, the email ceiling, and redelivery.
func TestJobFailureSummonsCoalescesByCause(t *testing.T) {
	const quota = "workspace AI quota exceeded"
	const badKey = "openai: API error 401: invalid api key"

	var fiveCauses []platev.Event
	for i := range 5 {
		fiveCauses = append(fiveCauses, jobFailed(fmt.Sprintf("cause-%d", i), "", "en.json",
			fmt.Sprintf("provider error %d: failure kind %c", 500+i, 'a'+i)))
	}

	redelivered := jobFailed("job-a", "", "en.json", quota)

	tests := []struct {
		name   string
		events []platev.Event
		// deliver runs after the first settle, for the redelivery case.
		after []platev.Event
		// notifications each owner/admin holds, and the title of the newest.
		wantNotes int
		wantTitle string
		// emails each owner/admin receives, and what the first one says.
		wantMail     int
		wantMailBody string
	}{
		{
			name:         "a burst of one cause is one summons",
			events:       burst(40, "q", "", quota),
			wantNotes:    1,
			wantTitle:    "40 translation jobs did not finish",
			wantMail:     1,
			wantMailBody: "40",
		},
		{
			name:      "two causes are two summonses",
			events:    append(burst(12, "q", "", quota), burst(7, "k", "", badKey)...),
			wantNotes: 2,
			wantMail:  2,
		},
		{
			name: "reasons that differ only in what is particular to the job coalesce",
			events: []platev.Event{
				jobFailed("r-1", "", "a.json", `openai: request req_01HX7ABCDEF12 for "a.json" failed: 500`),
				jobFailed("r-2", "", "b.json", `openai: request req_01HX7QWERTY98 for "b.json" failed: 500`),
				jobFailed("r-3", "", "c.json", `openai: request req_01HX7ZXCVBN55 for "c.json" failed: 500`),
			},
			wantNotes: 1,
			wantTitle: "3 translation jobs did not finish",
			wantMail:  1,
		},
		{
			name:      "the email ceiling holds whatever the causes",
			events:    fiveCauses,
			wantNotes: 5,
			wantMail:  jobFailureMailCeiling,
		},
		{
			name: "a redelivered event neither recounts nor mails again",
			events: []platev.Event{
				redelivered, redelivered,
				jobFailed("job-b", "", "b.json", quota),
				redelivered,
			},
			after:     []platev.Event{redelivered, jobFailed("job-b", "", "b.json", quota)},
			wantNotes: 1,
			wantTitle: "2 translation jobs did not finish",
			wantMail:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newJobFailureHarness(t)
			for _, ev := range tt.events {
				h.srv.summonOnJobFailure(t.Context(), ev)
			}
			h.settle(t)
			for _, ev := range tt.after {
				h.srv.summonOnJobFailure(t.Context(), ev)
			}
			h.settle(t)

			for _, user := range []string{"owner", "admin"} {
				notes := notificationsFor(t, h, user)
				require.Len(t, notes, tt.wantNotes, user)
				if tt.wantTitle != "" {
					assert.Equal(t, tt.wantTitle, notes[0].Title, user)
				}
			}
			assert.Empty(t, notificationsFor(t, h, "translator"))

			for _, addr := range []string{"owner@acme.test", "admin@acme.test"} {
				mail := h.mailTo(addr)
				require.Len(t, mail, tt.wantMail, addr)
				if tt.wantMailBody != "" {
					assert.Contains(t, mail[0].subject, tt.wantMailBody+" translation jobs did not finish")
					assert.Contains(t, mail[0].body, "<strong>"+tt.wantMailBody+"</strong>")
					assert.Contains(t, mail[0].body, quota)
				}
			}
		})
	}
}

// A single job that fails on its own still reaches the person who asked for
// it, with the per-job wording, while a burst the platform started goes to the
// workspace's owners as one grouped summons.
func TestJobFailureIsolatedFailureReachesTheRequester(t *testing.T) {
	h := newJobFailureHarness(t)
	for _, ev := range burst(30, "q", "", "workspace AI quota exceeded") {
		h.srv.summonOnJobFailure(t.Context(), ev)
	}
	h.srv.summonOnJobFailure(t.Context(), jobFailed("mine", "translator", "en.json", "workspace AI quota exceeded"))
	h.settle(t)

	told := notificationsFor(t, h, "translator")
	require.Len(t, told, 1)
	assert.Equal(t, "A translation job did not finish", told[0].Title)
	assert.Equal(t, "en.json → nb: workspace AI quota exceeded", told[0].Body)

	mine := h.mailTo("t@acme.test")
	require.Len(t, mine, 1)
	assert.Contains(t, mine[0].subject, "A translation job did not finish")

	owners := notificationsFor(t, h, "owner")
	require.Len(t, owners, 1)
	assert.Equal(t, "30 translation jobs did not finish", owners[0].Title,
		"the requester's job is counted in their own group, not the owners'")
}

// A grouped notification that was read comes back unread when the group grows,
// so the badge reflects failures that arrived after the reader looked.
func TestJobFailureGroupGrowingMarksItUnread(t *testing.T) {
	h := newJobFailureHarness(t)
	h.srv.summonOnJobFailure(t.Context(), jobFailed("j-1", "translator", "a.json", "workspace AI quota exceeded"))
	require.NoError(t, h.srv.NotificationStore.MarkAllRead(t.Context(), "translator"))

	h.srv.summonOnJobFailure(t.Context(), jobFailed("j-2", "translator", "b.json", "workspace AI quota exceeded"))

	unread, err := h.srv.NotificationStore.UnreadCount(t.Context(), "translator")
	require.NoError(t, err)
	assert.Equal(t, 1, unread)
}

func TestJobFailureCause(t *testing.T) {
	same := func(a, b platev.Event) bool { return jobFailureCause(a) == jobFailureCause(b) }

	assert.True(t, same(
		jobFailed("1", "", "a.json", "workspace AI quota exceeded"),
		jobFailed("2", "", "b.json", "workspace AI quota exceeded"),
	))
	assert.True(t, same(
		jobFailed("1", "", "a.json", "cannot read a.json: 1843 bytes short"),
		jobFailed("2", "", "b.json", "cannot read b.json: 90211 bytes short"),
	), "the item and a long number are particular to the job")
	assert.False(t, same(
		jobFailed("1", "", "a.json", "openai: API error 401"),
		jobFailed("2", "", "a.json", "openai: API error 429"),
	), "a status code is part of the reason")

	push := jobFailed("1", "", "a.json", "workspace AI quota exceeded")
	push.Data["job_kind"] = "sync"
	assert.False(t, same(push, jobFailed("2", "", "a.json", "workspace AI quota exceeded")),
		"a different kind of work is a different group")
}

// The group key is the same on every instance for a cause in a window, which is
// what lets the store's unique index settle two instances opening it at once.
func TestJobFailureGroupKeyIsSharedWithinAWindow(t *testing.T) {
	at := time.Date(2026, 9, 26, 19, 44, 0, 0, time.UTC)
	p := jobFailureCausePrefix(summonsWSID, "owners", "abc")
	assert.Equal(t, jobFailureGroupKey(p, at), jobFailureGroupKey(p, at.Add(10*time.Minute)))
	assert.NotEqual(t, jobFailureGroupKey(p, at), jobFailureGroupKey(p, at.Add(time.Hour)))
	assert.Equal(t, p+strconv.FormatInt(at.Truncate(time.Hour).Unix(), 10), jobFailureGroupKey(p, at))
}
