package event

import (
	"context"
	"sync"
	"testing"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DispatchTaskNotification never called a mailer, so an urgent assignment
// waited for its assignee to open the queue. These tests pin the rule the
// founder set: mail at high and urgent, badge for everything else.

// recordingTaskMailer records the assignment emails a dispatcher asks for,
// standing in for MailerAdapter's half of the work.
type recordingTaskMailer struct {
	mu    sync.Mutex
	sent  []*bstore.Task
	slugs []string
}

func (r *recordingTaskMailer) SendImmediate(context.Context, string, *bstore.Notification) error {
	return nil
}

func (r *recordingTaskMailer) SendTaskAssigned(_ context.Context, _ string, task *bstore.Task, slug string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, task)
	r.slugs = append(r.slugs, slug)
	return nil
}

func (r *recordingTaskMailer) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.sent)
}

// dispatcherWithTaskMailer builds a dispatcher whose only wired outputs are the
// task mailer and the slug resolver — no store, so nothing needs a database.
func dispatcherWithTaskMailer(t *testing.T) (*NotificationDispatcher, *recordingTaskMailer) {
	t.Helper()
	tm := &recordingTaskMailer{}
	d := &NotificationDispatcher{}
	d.SetMailer(tm)
	d.SetWorkspaceSlugResolver(func(context.Context, string) (string, error) {
		return "acme", nil
	})
	return d, tm
}

func urgentTask() *bstore.Task {
	return &bstore.Task{
		ID:          "task-1",
		WorkspaceID: "ws-1",
		Type:        bstore.TaskFixTerminology,
		Priority:    bstore.TaskPriorityUrgent,
		Title:       "Fix terminology in the release notes",
		Description: "Three forbidden terms shipped.",
		AssigneeID:  "assignee",
		CreatedBy:   "dana",
	}
}

func TestTaskAssignmentEmailsAtHighAndUrgentOnly(t *testing.T) {
	tests := []struct {
		priority bstore.TaskPriority
		wantMail bool
	}{
		{priority: bstore.TaskPriorityUrgent, wantMail: true},
		{priority: bstore.TaskPriorityHigh, wantMail: true},
		{priority: bstore.TaskPriorityNormal, wantMail: false},
		{priority: bstore.TaskPriorityLow, wantMail: false},
		{priority: "", wantMail: false},
	}
	for _, tt := range tests {
		t.Run(string(tt.priority), func(t *testing.T) {
			d, tm := dispatcherWithTaskMailer(t)
			task := urgentTask()
			task.Priority = tt.priority

			d.mailTaskAssignment(t.Context(), task)

			if tt.wantMail {
				require.Equal(t, 1, tm.count())
				assert.Equal(t, "acme", tm.slugs[0])
			} else {
				assert.Zero(t, tm.count(), "routine assignments stay in the queue")
			}
		})
	}
}

// The change-set summons already sends its reviewers the dedicated
// review-request email. Its task is opened at high priority, so without a guard
// the same reviewer would get two messages about one change-set.
func TestTaskAssignmentDoesNotDoubleSendForAChangeSetReview(t *testing.T) {
	d, tm := dispatcherWithTaskMailer(t)
	task := urgentTask()
	task.Type = bstore.TaskReviewTerms
	task.Priority = bstore.TaskPriorityHigh
	task.Data = map[string]string{"changeset_id": "cs-1"}

	d.mailTaskAssignment(t.Context(), task)

	assert.Zero(t, tm.count())
}

// The guard reads the change-set id, not the task type, so a terms-review task
// opened by any other route still mails.
func TestTaskAssignmentMailsATermsReviewWithNoChangeSet(t *testing.T) {
	d, tm := dispatcherWithTaskMailer(t)
	task := urgentTask()
	task.Type = bstore.TaskReviewTerms
	task.Priority = bstore.TaskPriorityHigh

	d.mailTaskAssignment(t.Context(), task)

	assert.Equal(t, 1, tm.count())
}

// Without the id→slug lookup a preference cannot be read, and mailing anyway
// would take the choice away from the person it belongs to.
func TestTaskAssignmentWithoutASlugResolverStaysSilent(t *testing.T) {
	tm := &recordingTaskMailer{}
	d := &NotificationDispatcher{}
	d.SetMailer(tm)

	d.mailTaskAssignment(t.Context(), urgentTask())

	assert.Zero(t, tm.count())
}

func TestTaskAssignmentWithNoAssigneeStaysSilent(t *testing.T) {
	d, tm := dispatcherWithTaskMailer(t)
	task := urgentTask()
	task.AssigneeID = ""

	d.mailTaskAssignment(t.Context(), task)

	assert.Zero(t, tm.count())
}

// A bare DigestEmailer is still a valid mailer — it just cannot send the
// assignment template, and must not make the dispatcher pretend it can.
func TestBareDigestEmailerSendsNoAssignmentEmail(t *testing.T) {
	d := &NotificationDispatcher{}
	d.SetMailer(&bareEmailer{})
	d.SetWorkspaceSlugResolver(func(context.Context, string) (string, error) { return "acme", nil })

	assert.NotPanics(t, func() { d.mailTaskAssignment(t.Context(), urgentTask()) })
}

type bareEmailer struct{}

func (bareEmailer) SendImmediate(context.Context, string, *bstore.Notification) error { return nil }

// MailerAdapter resolves the people and the link; the dispatcher only decides
// whether to send.
func TestMailerAdapterSendTaskAssigned(t *testing.T) {
	m, sender := newTestMailer(t)
	store := newMockAuthStore()
	store.users["assignee"] = &platauth.User{ID: "assignee", Email: "a@acme.test", Name: "Ann"}
	store.users["dana"] = &platauth.User{ID: "dana", Email: "d@acme.test", Name: "Dana"}
	store.workspace = &platauth.Workspace{ID: "ws-1", Slug: "acme", Name: "Acme Corp"}

	adapter := NewMailerAdapter(m, store, "https://app.example.test/")
	require.NoError(t, adapter.SendTaskAssigned(t.Context(), "assignee", urgentTask(), "acme"))

	require.Equal(t, 1, sender.count())
	msg := sender.last()
	assert.Equal(t, "a@acme.test", msg.To)
	assert.Contains(t, msg.Subject, "Fix terminology in the release notes")
	assert.Contains(t, msg.Subject, "urgent")
	assert.Contains(t, msg.Body, "Dana")
	assert.Contains(t, msg.Body, "Acme Corp", "the workspace's name, not its slug")
	assert.Contains(t, msg.Body, "Three forbidden terms shipped.")
	// The trailing slash on the origin must not survive into the link.
	assert.Contains(t, msg.Body, "https://app.example.test/acme/tasks")
	assert.NotContains(t, msg.Body, "example.test//acme")
}

// A task the platform opened for itself names Bowrain rather than the literal
// "system" or a bare id.
func TestMailerAdapterSendTaskAssignedFromSystem(t *testing.T) {
	m, sender := newTestMailer(t)
	store := newMockAuthStore()
	store.users["assignee"] = &platauth.User{ID: "assignee", Email: "a@acme.test", Name: "Ann"}

	adapter := NewMailerAdapter(m, store, "https://app.example.test")
	task := urgentTask()
	task.CreatedBy = "system"
	require.NoError(t, adapter.SendTaskAssigned(t.Context(), "assignee", task, "acme"))

	assert.Contains(t, sender.last().Body, "Bowrain")
}

func TestMailerAdapterSendTaskAssignedNeedsAnAppOrigin(t *testing.T) {
	m, sender := newTestMailer(t)
	store := newMockAuthStore()
	store.users["assignee"] = &platauth.User{ID: "assignee", Email: "a@acme.test", Name: "Ann"}

	adapter := NewMailerAdapter(m, store, "")
	err := adapter.SendTaskAssigned(t.Context(), "assignee", urgentTask(), "acme")

	require.ErrorIs(t, err, errNoAppOrigin)
	assert.Zero(t, sender.count())
}
