package event

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	bstore "github.com/neokapi/neokapi/bowrain/store"
	platev "github.com/neokapi/neokapi/platform/event"
	"github.com/neokapi/neokapi/platform/store"
)

// milestones are the percentage thresholds that trigger notifications.
var milestones = []int{25, 50, 75, 100}

// ProgressTracker subscribes to batch events (push, flow completion) and
// checks whether any locale has crossed a milestone percentage (25/50/75/100%).
// When a milestone is crossed, it dispatches a progress.milestone notification.
type ProgressTracker struct {
	contentStore store.ContentStore
	dispatcher   *NotificationDispatcher
	bus          platev.EventBus
	sub          *platev.Subscription

	// seen tracks which milestones have already been notified to avoid duplicates.
	// Key: "projectID:locale:milestone"
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
	pt.sub = bus.SubscribeAll(pt.handleEvent)
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
		log.Printf("WARNING: progress tracker failed to get project %s: %v", ev.ProjectID, err)
		return
	}

	stream := ev.Data["stream"]
	if stream == "" {
		stream = proj.DefaultStream
	}

	stats, err := pt.contentStore.GetBlockStats(ctx, proj.ID, stream)
	if err != nil {
		log.Printf("WARNING: progress tracker failed to get block stats for %s: %v", proj.ID, err)
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
	if pt.dispatcher == nil || pt.dispatcher.store == nil {
		return
	}

	title := fmt.Sprintf("%s reached %d%%", locale, milestone)
	body := fmt.Sprintf("Translation for %s in project %s has reached %d%% completion.", locale, proj.Name, milestone)

	// Determine target users via the dispatcher's targetFn.
	var userIDs []string
	if pt.dispatcher.targetFn != nil && proj.ID != "" {
		var err error
		userIDs, err = pt.dispatcher.targetFn(ctx, proj.ID, ev.Actor)
		if err != nil {
			log.Printf("WARNING: progress tracker failed to resolve targets for %s: %v", proj.ID, err)
			return
		}
	}

	for _, userID := range userIDs {
		n := &bstore.Notification{
			UserID:    userID,
			Type:      bstore.NotificationProgressMilestone,
			Title:     title,
			Body:      body,
			ProjectID: proj.ID,
			Category:  string(bstore.CategoryProject),
			Priority:  "normal",
		}
		if err := pt.dispatcher.store.Create(ctx, n); err != nil {
			log.Printf("WARNING: progress tracker failed to create milestone notification: %v", err)
			continue
		}
		if pt.dispatcher.sender != nil {
			pt.dispatcher.sender.NotifyUser(userID, n)
		}
	}
}
