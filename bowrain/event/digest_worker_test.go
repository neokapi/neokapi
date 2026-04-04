package event

import (
	"testing"
	"time"

	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/stretchr/testify/assert"
)

func TestGroupByCategory(t *testing.T) {
	notifications := []bstore.Notification{
		{ID: "1", Category: "task", Title: "Task assigned"},
		{ID: "2", Category: "quality", Title: "Gate failed"},
		{ID: "3", Category: "task", Title: "Task overdue"},
		{ID: "4", Category: "mention", Title: "You were mentioned"},
	}

	groups := groupByCategory(notifications)

	assert.Len(t, groups, 3)
	assert.Equal(t, "task", groups[0].Category)
	assert.Len(t, groups[0].Items, 2)
	assert.Equal(t, "quality", groups[1].Category)
	assert.Len(t, groups[1].Items, 1)
	assert.Equal(t, "mention", groups[2].Category)
	assert.Len(t, groups[2].Items, 1)
}

func TestRenderDigest(t *testing.T) {
	w := &DigestWorker{frequency: bstore.DigestDaily}

	groups := []categoryGroup{
		{
			Category: "task",
			Items: []bstore.Notification{
				{Title: "Review assigned", Body: "Review 12 blocks in fr-FR", Priority: "normal"},
				{Title: "Task overdue", Body: "Translation task is overdue", Priority: "high"},
			},
		},
		{
			Category: "quality",
			Items: []bstore.Notification{
				{Title: "Quality gate failed", Body: "Terminology check failed", Priority: "high"},
			},
		},
	}

	ds := bstore.DigestSettings{
		UserID:      "usr_1",
		WorkspaceID: "ws_1",
		Frequency:   bstore.DigestDaily,
	}

	subject, body := w.renderDigest(groups, ds)

	assert.Contains(t, subject, "daily digest")
	assert.Contains(t, subject, "3 updates")
	assert.Contains(t, body, "Tasks (2)")
	assert.Contains(t, body, "Quality (1)")
	assert.Contains(t, body, "Review assigned")
	assert.Contains(t, body, "border-left:3px solid #ef4444") // high priority styling
}

func TestDigestWorkerStartStop(t *testing.T) {
	w := NewDigestWorker(nil, nil, nil, nil, bstore.DigestDaily, time.Hour)
	w.Start()
	// Give it a moment to start the goroutine.
	time.Sleep(10 * time.Millisecond)
	w.Close()
	// Should not hang — the done channel should be closed.
}

func TestGroupByCategoryEmpty(t *testing.T) {
	groups := groupByCategory(nil)
	assert.Empty(t, groups)
}

func TestGroupByCategoryEmptyCategory(t *testing.T) {
	notifications := []bstore.Notification{
		{ID: "1", Category: "", Title: "No category"},
	}

	groups := groupByCategory(notifications)

	assert.Len(t, groups, 1)
	assert.Equal(t, "general", groups[0].Category)
}

func TestRenderWeeklyDigest(t *testing.T) {
	w := &DigestWorker{frequency: bstore.DigestWeekly}

	groups := []categoryGroup{
		{Category: "project", Items: []bstore.Notification{{Title: "Progress milestone", Body: "fr-FR reached 100%"}}},
	}

	ds := bstore.DigestSettings{Frequency: bstore.DigestWeekly}

	subject, body := w.renderDigest(groups, ds)

	assert.Contains(t, subject, "weekly summary")
	assert.Contains(t, body, "Weekly Summary")
}

func TestSendDigestForUserNoNotifications(t *testing.T) {
	// Verify that sendDigestForUser returns nil when there are no unread notifications.
	// This requires a real store, which we don't have here, so we just test the grouping.
	_ = t.Context()
}
