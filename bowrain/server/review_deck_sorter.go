package server

import (
	"context"
	"database/sql"
	"log/slog"
	"slices"
	"sync"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
)

const (
	// deckSortInterval is how often the sorter looks at every deck. Facts
	// expire after a day (reviewDeckFresh), so a sweep also renews them.
	deckSortInterval = 5 * time.Minute
	// deckSortSettle is how long the sorter waits after a push or a run
	// before it sweeps, so a burst of events is one sweep.
	deckSortSettle = 20 * time.Second
	// deckSortTimeout bounds one sweep; what is left waits for the next.
	deckSortTimeout = 10 * time.Minute
	// deckSortLockKey names the advisory lock that lets one server instance
	// sweep at a time.
	deckSortLockKey = "bowrain_deck_sorter"
)

// deckSorter sorts every project's review deck in the background: it
// classifies each translation waiting for review into the deck's groups, as
// a deck request does for the part it reaches, so the counts a reviewer
// sees settle without anyone opening the deck. It sweeps on a timer and
// shortly after a push or a convergence run completes. Deck requests still
// classify what the sorter has not reached.
type deckSorter struct {
	s    *Server
	db   *sql.DB
	kick chan struct{}
	stop chan struct{}
	done chan struct{}
	subs []*platev.Subscription
	once sync.Once
}

// startDeckSorter starts the sorter over the server's PostgreSQL database.
func (s *Server) startDeckSorter(db *sql.DB) *deckSorter {
	d := &deckSorter{
		s:    s,
		db:   db,
		kick: make(chan struct{}, 1),
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	if s.EventBus != nil {
		for _, t := range []platev.EventType{platev.EventPushCompleted, platev.EventConvergenceRunCompleted} {
			d.subs = append(d.subs, s.EventBus.Subscribe(t, func(platev.Event) { d.nudge() }))
		}
	}
	go d.loop()
	return d
}

// nudge asks for a sweep after deckSortSettle.
func (d *deckSorter) nudge() {
	select {
	case d.kick <- struct{}{}:
	default:
	}
}

func (d *deckSorter) loop() {
	defer close(d.done)
	ticker := time.NewTicker(deckSortInterval)
	defer ticker.Stop()
	var settle <-chan time.Time
	// The first sweep waits for the server to settle after start.
	first := time.After(deckSortSettle)
	for {
		select {
		case <-d.stop:
			return
		case <-first:
			first = nil
			d.sweep()
		case <-ticker.C:
			d.sweep()
		case <-d.kick:
			if settle == nil {
				settle = time.After(deckSortSettle)
			}
		case <-settle:
			settle = nil
			d.sweep()
		}
	}
}

// Close stops the sorter and waits for a running sweep to stop.
func (d *deckSorter) Close() {
	d.once.Do(func() {
		if d.s.EventBus != nil {
			for _, sub := range d.subs {
				d.s.EventBus.Unsubscribe(sub)
			}
		}
		close(d.stop)
		<-d.done
	})
}

// sweep classifies every unclassified translation of every project's
// languages, under the advisory lock so one instance sweeps at a time.
func (d *deckSorter) sweep() {
	ctx, cancel := context.WithTimeout(context.Background(), deckSortTimeout)
	defer cancel()
	go func() {
		select {
		case <-d.stop:
			cancel()
		case <-ctx.Done():
		}
	}()

	conn, err := d.db.Conn(ctx)
	if err != nil {
		slog.Warn("deck sorter: no database connection", "error", err)
		return
	}
	defer conn.Close()
	var locked bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtext($1))`, deckSortLockKey).Scan(&locked); err != nil || !locked {
		return
	}
	defer func() {
		_, _ = conn.ExecContext(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock(hashtext($1))`, deckSortLockKey)
	}()

	sorted, err := d.s.sortDecks(ctx)
	if err != nil && ctx.Err() == nil {
		slog.Warn("deck sorter: sweep failed", "sorted", sorted, "error", err)
		return
	}
	if sorted > 0 {
		slog.Info("deck sorter: sorted translations", "count", sorted)
	}
}

// sortDecks classifies what waits unclassified in every language of every
// project that is not archived, and returns how many it classified.
func (s *Server) sortDecks(ctx context.Context) (int, error) {
	if s.ContentStore == nil {
		return 0, nil
	}
	decks, ok := store.ReviewDeckStores(s.ContentStore)
	if !ok {
		return 0, nil
	}
	projects, err := s.ContentStore.ListProjects(ctx)
	if err != nil {
		return 0, err
	}
	policy := s.deckPolicy()
	total := 0
	for _, p := range projects {
		if p.Archived {
			continue
		}
		streams := []string{reviewInboxStream}
		if p.DefaultStream != "" && !slices.Contains(streams, p.DefaultStream) {
			streams = append(streams, p.DefaultStream)
		}
		for _, stream := range streams {
			scope, err := deckScopeOf(ctx, s.ContentStore, p.ID, stream)
			if err != nil {
				return total, err
			}
			for _, loc := range p.TargetLanguages {
				q := store.ReviewDeckQuery{ProjectID: p.ID, Stream: stream, Locale: string(loc), Policy: deckPolicyKey(policy)}
				scope.apply(&q)
				for {
					if err := ctx.Err(); err != nil {
						return total, err
					}
					n, err := s.classifyDeck(ctx, p.WorkspaceID, decks, p, q, policy, deckClassifyBudget)
					total += n
					if err != nil {
						return total, err
					}
					if n < deckClassifyBudget {
						break
					}
				}
			}
		}
	}
	return total, nil
}
