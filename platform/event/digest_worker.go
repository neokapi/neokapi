package event

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/mailer"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// UserEmailResolver resolves a user ID to their email address.
type UserEmailResolver func(ctx context.Context, userID string) (email string, err error)

// DigestWorker periodically assembles and sends notification digest emails.
type DigestWorker struct {
	notifStore   *bstore.NotificationStore
	digestStore  *bstore.DigestStore
	mailer       *mailer.Mailer
	resolveEmail UserEmailResolver
	frequency    bstore.DigestFrequency
	interval     time.Duration
	stop         chan struct{}
	done         chan struct{}
}

// NewDigestWorker creates a digest worker for the given frequency.
func NewDigestWorker(
	notifStore *bstore.NotificationStore,
	digestStore *bstore.DigestStore,
	m *mailer.Mailer,
	resolveEmail UserEmailResolver,
	frequency bstore.DigestFrequency,
	interval time.Duration,
) *DigestWorker {
	return &DigestWorker{
		notifStore:   notifStore,
		digestStore:  digestStore,
		mailer:       m,
		resolveEmail: resolveEmail,
		frequency:    frequency,
		interval:     interval,
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
	}
}

// Start begins the periodic digest loop. Call Close to stop.
func (w *DigestWorker) Start() {
	go w.loop()
}

// Close stops the digest worker and waits for it to finish.
func (w *DigestWorker) Close() {
	close(w.stop)
	<-w.done
}

func (w *DigestWorker) loop() {
	defer close(w.done)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stop:
			return
		case <-ticker.C:
			w.tick()
		}
	}
}

func (w *DigestWorker) tick() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	users, err := w.digestStore.ListUsersWithFrequency(ctx, w.frequency)
	if err != nil {
		log.Printf("WARNING: digest worker (%s) failed to list users: %v", w.frequency, err)
		return
	}

	for _, ds := range users {
		if err := w.sendDigestForUser(ctx, ds); err != nil {
			log.Printf("WARNING: digest worker (%s) failed for user %s: %v", w.frequency, ds.UserID, err)
		}
	}
}

func (w *DigestWorker) sendDigestForUser(ctx context.Context, ds bstore.DigestSettings) error {
	// Check quiet hours.
	if w.digestStore.IsInQuietHours(&ds, time.Now().UTC()) {
		return nil
	}

	// Get last sent time.
	state, err := w.digestStore.GetState(ctx, ds.UserID, ds.WorkspaceID, string(w.frequency))
	if err != nil {
		return fmt.Errorf("get digest state: %w", err)
	}

	// Fetch unread notifications since last digest.
	notifications, err := w.notifStore.ListUnreadSince(ctx, ds.UserID, state.LastSentAt)
	if err != nil {
		return fmt.Errorf("list unread: %w", err)
	}

	if len(notifications) == 0 {
		return nil
	}

	// Resolve user email.
	email, err := w.resolveEmail(ctx, ds.UserID)
	if err != nil || email == "" {
		return fmt.Errorf("resolve email for %s: %w", ds.UserID, err)
	}

	// Group notifications by category.
	groups := groupByCategory(notifications)

	// Build and send the digest email.
	subject, body := w.renderDigest(groups, ds)
	if body == "" {
		return nil
	}

	if w.mailer != nil && w.mailer.Sender != nil {
		if err := w.mailer.Sender.Send(ctx, email, subject, body); err != nil {
			return fmt.Errorf("send digest email: %w", err)
		}
	}

	// Update state.
	return w.digestStore.UpdateState(ctx, ds.UserID, ds.WorkspaceID, string(w.frequency), time.Now().UTC())
}

// categoryGroup holds notifications for a single category.
type categoryGroup struct {
	Category string
	Items    []bstore.Notification
}

func groupByCategory(notifications []bstore.Notification) []categoryGroup {
	order := make([]string, 0)
	groups := make(map[string][]bstore.Notification)
	for _, n := range notifications {
		cat := n.Category
		if cat == "" {
			cat = "general"
		}
		if _, ok := groups[cat]; !ok {
			order = append(order, cat)
		}
		groups[cat] = append(groups[cat], n)
	}

	result := make([]categoryGroup, 0, len(order))
	for _, cat := range order {
		result = append(result, categoryGroup{Category: cat, Items: groups[cat]})
	}
	return result
}

func (w *DigestWorker) renderDigest(groups []categoryGroup, ds bstore.DigestSettings) (subject, body string) {
	var total int
	for _, g := range groups {
		total += len(g.Items)
	}

	if w.frequency == bstore.DigestWeekly {
		subject = fmt.Sprintf("Your weekly summary — %d updates", total)
	} else {
		subject = fmt.Sprintf("Your daily digest — %d updates", total)
	}

	// Build a simple, clean HTML digest.
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html><head><meta charset="UTF-8"></head>`)
	b.WriteString(`<body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:#f8fafc;padding:20px;">`)
	b.WriteString(`<div style="max-width:600px;margin:0 auto;background:#fff;border-radius:8px;border:1px solid #e2e8f0;overflow:hidden;">`)

	// Header
	b.WriteString(`<div style="background:#0f172a;color:#fff;padding:24px 32px;">`)
	if w.frequency == bstore.DigestWeekly {
		b.WriteString(`<h1 style="margin:0;font-size:20px;font-weight:600;">Weekly Summary</h1>`)
	} else {
		b.WriteString(`<h1 style="margin:0;font-size:20px;font-weight:600;">Daily Digest</h1>`)
	}
	b.WriteString(fmt.Sprintf(`<p style="margin:4px 0 0;color:#94a3b8;font-size:14px;">%d new updates</p>`, total))
	b.WriteString(`</div>`)

	// Body
	b.WriteString(`<div style="padding:24px 32px;">`)

	categoryLabels := map[string]string{
		"task":       "Tasks",
		"review":     "Reviews",
		"quality":    "Quality",
		"automation": "Automation",
		"mention":    "Mentions",
		"project":    "Project",
		"system":     "System",
		"general":    "General",
	}

	for _, g := range groups {
		label := categoryLabels[g.Category]
		if label == "" {
			label = g.Category
		}

		b.WriteString(fmt.Sprintf(`<div style="margin-bottom:20px;">
			<h2 style="font-size:14px;font-weight:600;color:#64748b;text-transform:uppercase;letter-spacing:0.05em;margin:0 0 12px;border-bottom:1px solid #f1f5f9;padding-bottom:8px;">%s (%d)</h2>`, label, len(g.Items)))

		for _, n := range g.Items {
			priorityStyle := ""
			if n.Priority == "high" {
				priorityStyle = "border-left:3px solid #ef4444;padding-left:12px;"
			}
			b.WriteString(fmt.Sprintf(`<div style="margin-bottom:12px;%s">
				<p style="margin:0;font-size:14px;font-weight:500;color:#0f172a;">%s</p>
				<p style="margin:2px 0 0;font-size:13px;color:#64748b;">%s</p>
			</div>`, priorityStyle, n.Title, n.Body))
		}

		b.WriteString(`</div>`)
	}

	b.WriteString(`</div>`)

	// Footer
	b.WriteString(`<div style="padding:16px 32px;background:#f8fafc;border-top:1px solid #e2e8f0;text-align:center;">`)
	b.WriteString(`<p style="margin:0;font-size:12px;color:#94a3b8;">You can change your digest frequency in notification settings.</p>`)
	b.WriteString(`</div>`)

	b.WriteString(`</div></body></html>`)

	return subject, b.String()
}
