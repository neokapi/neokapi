package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/storage"
)

// defaultUnclaimedPurgeInterval is how often the purge job runs by default.
const defaultUnclaimedPurgeInterval = time.Hour

// UnclaimedAuthPurger purges expired unclaimed (anonymous) auth records.
// Satisfied by bowrain/auth.AuthStore.
type UnclaimedAuthPurger interface {
	PurgeExpiredUnclaimed(ctx context.Context) (int, error)
}

// ExpiredUnclaimedLister lists the project IDs of anonymous projects whose
// claim window has expired. bowrain/auth owns the unclaimed_projects table and
// its PurgeExpiredUnclaimed returns only a row count; the purge job needs the
// IDs to clean the matching content-store projects, so it reads them from the
// shared database (see PgUnclaimedLister).
type ExpiredUnclaimedLister interface {
	ListExpiredUnclaimedProjectIDs(ctx context.Context) ([]string, error)
}

// OrphanProjectStore is the slice of the content store the purge job needs to
// delete orphaned anonymous projects. Satisfied by store.ContentStore.
type OrphanProjectStore interface {
	GetProject(ctx context.Context, id string) (*store.Project, error)
	DeleteProject(ctx context.Context, id string) error
}

// PgUnclaimedLister reads expired unclaimed project IDs directly from the
// shared PostgreSQL database. It reads the bowrain/auth-owned unclaimed_projects
// table on purpose: auth exposes no ID-returning lister and is out of scope to
// change here, and the content-project cleanup needs the exact IDs so it never
// touches a legitimate WorkspaceID-less project created by another path.
type PgUnclaimedLister struct{ db *storage.PgDB }

// NewPgUnclaimedLister binds a lister to the shared database.
func NewPgUnclaimedLister(db *storage.PgDB) *PgUnclaimedLister {
	return &PgUnclaimedLister{db: db}
}

// ListExpiredUnclaimedProjectIDs returns the project IDs whose claim window has
// elapsed (expires_at < now).
func (l *PgUnclaimedLister) ListExpiredUnclaimedProjectIDs(ctx context.Context) ([]string, error) {
	rows, err := l.db.QueryContext(ctx,
		`SELECT project_id FROM unclaimed_projects WHERE expires_at < NOW()`)
	if err != nil {
		return nil, fmt.Errorf("query expired unclaimed projects: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan unclaimed project id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// UnclaimedPurgeConfig configures the periodic purge job.
type UnclaimedPurgeConfig struct {
	Lister   ExpiredUnclaimedLister
	Auth     UnclaimedAuthPurger
	Projects OrphanProjectStore
	Interval time.Duration
}

// UnclaimedProjectPurger periodically removes expired anonymous projects: it
// purges the expired unclaimed auth records (auth.PurgeExpiredUnclaimed, which
// nothing else calls) and deletes the orphaned content-store projects the
// anonymous-create flow leaves behind (server/handlers_claim.go creates them
// but claims are the only thing that ever cleaned them). It guards claimed
// projects by refusing to delete any content project whose WorkspaceID is set.
type UnclaimedProjectPurger struct {
	cfg  UnclaimedPurgeConfig
	done chan struct{}
}

// NewUnclaimedProjectPurger starts the purge loop and returns the handle; Close
// stops it. Mirrors the RunRetentionCleaner lifecycle used by the server's
// other periodic cleaners.
func NewUnclaimedProjectPurger(cfg UnclaimedPurgeConfig) *UnclaimedProjectPurger {
	if cfg.Interval <= 0 {
		cfg.Interval = defaultUnclaimedPurgeInterval
	}
	p := &UnclaimedProjectPurger{cfg: cfg, done: make(chan struct{})}
	go p.loop()
	return p
}

// Close stops the purge loop.
func (p *UnclaimedProjectPurger) Close() { close(p.done) }

func (p *UnclaimedProjectPurger) loop() {
	ticker := time.NewTicker(p.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
			purged, deleted, err := p.purgeOnce(context.Background())
			if err != nil {
				slog.Warn("unclaimed-purge: error", "error", err)
			} else if purged > 0 || deleted > 0 {
				slog.Info("unclaimed-purge: cleaned expired anonymous projects",
					"auth_records", purged, "content_projects", deleted)
			}
		}
	}
}

// purgeOnce runs a single purge cycle. It (1) reads the expired unclaimed
// project IDs, (2) purges the expired unclaimed auth records, then (3) deletes
// the orphaned content-store projects — skipping any that have since been
// claimed. Ordering matters: the IDs are read before the auth purge deletes the
// records, and the WorkspaceID re-check makes a concurrent claim safe (a claim
// sets WorkspaceID and deletes the auth row, so a racing purge sees the claimed
// project and leaves it). It returns the count of purged auth records and
// deleted content projects.
func (p *UnclaimedProjectPurger) purgeOnce(ctx context.Context) (purgedAuth, deletedProjects int, err error) {
	var ids []string
	if p.cfg.Lister != nil {
		ids, err = p.cfg.Lister.ListExpiredUnclaimedProjectIDs(ctx)
		if err != nil {
			return 0, 0, fmt.Errorf("list expired unclaimed: %w", err)
		}
	}

	if p.cfg.Auth != nil {
		purgedAuth, err = p.cfg.Auth.PurgeExpiredUnclaimed(ctx)
		if err != nil {
			return 0, 0, fmt.Errorf("purge unclaimed auth records: %w", err)
		}
	}

	if p.cfg.Projects != nil {
		for _, id := range ids {
			proj, gerr := p.cfg.Projects.GetProject(ctx, id)
			if gerr != nil {
				// Already deleted, or a transient store error — skip; the next
				// cycle retries anything that still exists.
				continue
			}
			if proj.WorkspaceID != "" {
				// Claimed between the ID read and now: never delete a claimed
				// project. (Content projects created by non-anonymous paths are
				// never in `ids` — those come only from unclaimed auth records.)
				continue
			}
			if derr := p.cfg.Projects.DeleteProject(ctx, id); derr != nil {
				slog.Warn("unclaimed-purge: delete orphaned project failed",
					"project_id", id, "error", derr)
				continue
			}
			deletedProjects++
		}
	}
	return purgedAuth, deletedProjects, nil
}
