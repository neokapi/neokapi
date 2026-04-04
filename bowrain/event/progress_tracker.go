package event

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// milestones are the percentage thresholds that trigger notifications.
var milestones = []int{25, 50, 75, 100}

// maxSeenEntries caps the in-memory milestone dedup map. When exceeded, the
// map is reset — a few duplicate notifications are acceptable vs unbounded growth.
const maxSeenEntries = 10_000

// ProgressTracker subscribes to batch events (push, flow completion) and
// checks whether any locale has crossed a milestone percentage (25/50/75/100%).
// When a milestone is crossed, it dispatches a progress.milestone notification.
type ProgressTracker struct {
	contentStore store.ContentStore
	dispatcher   *NotificationDispatcher
	bus          platev.EventBus
	sub          *platev.Subscription

	// seen tracks which milestones have already been notified to avoid duplicates.
	// Key: "projectID:locale:milestone". Capped at maxSeenEntries.
	mu   sync.Mutex
	seen map[string]bool
}

// NewProgressTracker creates a tracker that listens for batch content events.
func NewProgressTracker(
	cs store.ContentStore,
	dispatcher *NotificationDispatcher,
	bus platev.EventBus,
) *ProgressTracker {
	pt := &ProgressTracker{
		contentStore: cs,
		dispatcher:   dispatcher,
		bus:          bus,
		seen:         make(map[string]bool),
	}
	pt.sub = bus.SubscribeGroup("progress-tracker", pt.handleEvent)
	return pt
}

// Close unsubscribes from the event bus.
func (pt *ProgressTracker) Close() {
	if pt.sub != nil {
		pt.bus.Unsubscribe(pt.sub)
	}
}

func (pt *ProgressTracker) handleEvent(ev platev.Event) {
	// Only check progress after batch operations that change translation state.
	switch ev.Type {
	case platev.EventPushCompleted, platev.EventFlowCompleted:
		// These are batch operations worth checking.
	default:
		return
	}

	if ev.ProjectID == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pt.checkProject(ctx, ev)
}

func (pt *ProgressTracker) checkProject(ctx context.Context, ev platev.Event) {
	proj, err := pt.contentStore.GetProject(ctx, ev.ProjectID)
	if err != nil {
		slog.Warn("progress tracker failed to get project", "id", ev.ProjectID, "error", err)
		return
	}

	stream := ev.Data["stream"]
	if stream == "" {
		stream = proj.DefaultStream
	}

	stats, err := pt.contentStore.GetBlockStats(ctx, proj.ID, stream)
	if err != nil {
		slog.Warn("progress tracker failed to get block stats for", "id", proj.ID, "error", err)
		return
	}

	// Count total translatable blocks and per-locale translated blocks.
	var totalTranslatable int
	localeTranslated := make(map[string]int)
	for _, row := range stats {
		if !row.Translatable {
			continue
		}
		totalTranslatable++
		for _, loc := range row.TargetLocales {
			localeTranslated[loc]++
		}
	}

	if totalTranslatable == 0 {
		return
	}

	// Check each target locale against milestones.
	for _, locale := range proj.TargetLanguages {
		loc := string(locale)
		translated := localeTranslated[loc]
		pct := (translated * 100) / totalTranslatable

		for _, milestone := range milestones {
			if pct >= milestone {
				key := fmt.Sprintf("%s:%s:%d", proj.ID, loc, milestone)

				pt.mu.Lock()
				alreadySeen := pt.seen[key]
				if !alreadySeen {
					if len(pt.seen) >= maxSeenEntries {
						pt.seen = make(map[string]bool)
					}
					pt.seen[key] = true
				}
				pt.mu.Unlock()

				if alreadySeen {
					continue
				}

				pt.dispatchMilestone(ctx, proj, loc, milestone, ev)
			}
		}
	}
}

func (pt *ProgressTracker) dispatchMilestone(ctx context.Context, proj *store.Project, locale string, milestone int, ev platev.Event) {
	if pt.dispatcher == nil {
		return
	}

	pt.dispatcher.DispatchToProject(ctx, proj.ID, ev.Actor, bstore.Notification{
		Type:      bstore.NotificationProgressMilestone,
		Title:     fmt.Sprintf("%s reached %d%%", locale, milestone),
		Body:      fmt.Sprintf("Translation for %s in project %s has reached %d%% completion.", locale, proj.Name, milestone),
		ProjectID: proj.ID,
		Category:  string(bstore.CategoryProject),
		Priority:  "normal",
	})
}
