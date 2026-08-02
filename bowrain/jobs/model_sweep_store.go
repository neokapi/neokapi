package jobs

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
)

// ModelSweepResult is one persisted measurement: how a candidate model scored
// on one project+locale fixture set, with and without the project's brand
// context. Keyed (project, locale, model, fixture_digest) so results are tied
// to exactly what was measured; re-running the same sweep upserts the row.
type ModelSweepResult struct {
	ProjectID     string    `json:"project_id"`
	Locale        string    `json:"locale"`
	Model         string    `json:"model"`
	FixtureDigest string    `json:"fixture_digest"`
	FixtureCount  int       `json:"fixture_count"`
	Adherence     float64   `json:"adherence"`      // 0..1, with the project's full context
	AdherenceBare float64   `json:"adherence_bare"` // 0..1, bare (no context)
	Lift          float64   `json:"lift"`           // adherence − adherence_bare
	TokensUsed    int       `json:"tokens_used"`    // both arms combined
	MeasuredAt    time.Time `json:"measured_at"`
}

// ModelSweepStore persists sweep measurements and answers the recommendation
// query. Postgres-backed like every other jobs-layer store.
type ModelSweepStore interface {
	// UpsertModelSweepResult inserts or refreshes the row for
	// (project, locale, model, fixture_digest).
	UpsertModelSweepResult(ctx context.Context, r *ModelSweepResult) error
	// LatestModelSweepResults returns, per model, the most recent measurement
	// for the given project+locale (across digests), newest first by lift.
	LatestModelSweepResults(ctx context.Context, projectID, locale string) ([]*ModelSweepResult, error)
	// ListModelSweepResults returns the latest measurement per (locale, model)
	// for the whole project — the project-settings panel's dataset.
	ListModelSweepResults(ctx context.Context, projectID string) ([]*ModelSweepResult, error)
}

// Recommendation policy: the recommended model per (project, locale) is the
// one with the highest measured lift, subject to an absolute-adherence floor
// (a model that wins on lift but still fails the fixtures outright must not be
// recommended) and a minimum fixture count (a two-segment fixture set is not a
// measurement). Freshness is a consumption concern (SweepModelRecommender),
// not a persistence one.
const (
	// RecommendMinFixtures is the minimum fixture count a measurement needs to
	// back a recommendation.
	RecommendMinFixtures = 8
	// RecommendAdherenceFloor is the minimum absolute (with-context) adherence
	// a model must reach to be recommendable.
	RecommendAdherenceFloor = 0.7
)

// RecommendModel picks the recommended model from per-model latest
// measurements: highest lift above the adherence floor with enough fixtures.
// Deterministic tie-break: higher adherence, then lexicographic model id.
// Returns nil when no measurement qualifies.
func RecommendModel(rows []*ModelSweepResult) *ModelSweepResult {
	var best *ModelSweepResult
	for _, r := range rows {
		if r == nil || r.FixtureCount < RecommendMinFixtures || r.Adherence < RecommendAdherenceFloor {
			continue
		}
		if best == nil {
			best = r
			continue
		}
		switch {
		case r.Lift > best.Lift:
			best = r
		case r.Lift == best.Lift && r.Adherence > best.Adherence:
			best = r
		case r.Lift == best.Lift && r.Adherence == best.Adherence && r.Model < best.Model:
			best = r
		}
	}
	return best
}

// ModelSweepMigrations is the sweep-measurement schema as a single consolidated
// baseline.
//
// LEDGER — every version this subsystem has ever issued, now folded in:
//
//	1  create model_sweep_results table (measured steerability)
//
// Baseline is version 2 — above every number issued, so an existing database
// applies it once and any drift between its schema and its bookkeeping is
// repaired. Retired numbers are never reused; the next migration is version 3.
var ModelSweepMigrations = []storage.Migration{
	{
		Version:     2,
		Description: "model sweep baseline (folds 1)",
		SQL: `
			CREATE TABLE IF NOT EXISTS model_sweep_results (
				project_id     TEXT NOT NULL,
				locale         TEXT NOT NULL,
				model          TEXT NOT NULL,
				fixture_digest TEXT NOT NULL,
				fixture_count  INTEGER NOT NULL DEFAULT 0,
				adherence      DOUBLE PRECISION NOT NULL DEFAULT 0,
				adherence_bare DOUBLE PRECISION NOT NULL DEFAULT 0,
				lift           DOUBLE PRECISION NOT NULL DEFAULT 0,
				tokens_used    INTEGER NOT NULL DEFAULT 0,
				measured_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, locale, model, fixture_digest)
			);
			-- The recommendation query scans a project (+locale) for the latest
			-- measurement per model.
			CREATE INDEX IF NOT EXISTS idx_model_sweep_project_locale
				ON model_sweep_results(project_id, locale, measured_at DESC);
		`,
	},
}

// modelSweepStore implements ModelSweepStore using PostgreSQL.
type modelSweepStore struct {
	db *storage.PgDB
}

// NewModelSweepStore creates a PostgreSQL-backed ModelSweepStore, running its
// (namespaced, idempotent) migrations first.
func NewModelSweepStore(db *storage.PgDB) (ModelSweepStore, error) {
	if err := storage.MigratePostgresNS(db, "model_sweep_schema_migrations", ModelSweepMigrations); err != nil {
		return nil, fmt.Errorf("migrate model-sweep schema: %w", err)
	}
	return &modelSweepStore{db: db}, nil
}

func (s *modelSweepStore) UpsertModelSweepResult(ctx context.Context, r *ModelSweepResult) error {
	if r.MeasuredAt.IsZero() {
		r.MeasuredAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO model_sweep_results
			(project_id, locale, model, fixture_digest, fixture_count,
			 adherence, adherence_bare, lift, tokens_used, measured_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 ON CONFLICT (project_id, locale, model, fixture_digest) DO UPDATE SET
			fixture_count  = EXCLUDED.fixture_count,
			adherence      = EXCLUDED.adherence,
			adherence_bare = EXCLUDED.adherence_bare,
			lift           = EXCLUDED.lift,
			tokens_used    = EXCLUDED.tokens_used,
			measured_at    = EXCLUDED.measured_at`,
		r.ProjectID, r.Locale, r.Model, r.FixtureDigest, r.FixtureCount,
		r.Adherence, r.AdherenceBare, r.Lift, r.TokensUsed, r.MeasuredAt)
	if err != nil {
		return fmt.Errorf("upsert model-sweep result: %w", err)
	}
	return nil
}

const modelSweepColumns = `project_id, locale, model, fixture_digest, fixture_count,
	adherence, adherence_bare, lift, tokens_used, measured_at`

func (s *modelSweepStore) LatestModelSweepResults(ctx context.Context, projectID, locale string) ([]*ModelSweepResult, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT ON (model) `+modelSweepColumns+`
		 FROM model_sweep_results
		 WHERE project_id = $1 AND locale = $2
		 ORDER BY model, measured_at DESC`,
		projectID, locale)
	if err != nil {
		return nil, fmt.Errorf("latest model-sweep results: %w", err)
	}
	defer rows.Close()
	out, err := scanModelSweepResults(rows)
	if err != nil {
		return nil, err
	}
	sortModelSweepResults(out)
	return out, nil
}

func (s *modelSweepStore) ListModelSweepResults(ctx context.Context, projectID string) ([]*ModelSweepResult, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT ON (locale, model) `+modelSweepColumns+`
		 FROM model_sweep_results
		 WHERE project_id = $1
		 ORDER BY locale, model, measured_at DESC`,
		projectID)
	if err != nil {
		return nil, fmt.Errorf("list model-sweep results: %w", err)
	}
	defer rows.Close()
	out, err := scanModelSweepResults(rows)
	if err != nil {
		return nil, err
	}
	sortModelSweepResults(out)
	return out, nil
}

// sortModelSweepResults orders rows for presentation: locale asc, then lift
// desc, then adherence desc, then model asc — the same deterministic order the
// recommendation tie-break uses.
func sortModelSweepResults(rows []*ModelSweepResult) {
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Locale != b.Locale {
			return a.Locale < b.Locale
		}
		if a.Lift != b.Lift {
			return a.Lift > b.Lift
		}
		if a.Adherence != b.Adherence {
			return a.Adherence > b.Adherence
		}
		return a.Model < b.Model
	})
}

// scanModelSweepResults scans query rows into results.
func scanModelSweepResults(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]*ModelSweepResult, error) {
	var out []*ModelSweepResult
	for rows.Next() {
		var r ModelSweepResult
		if err := rows.Scan(&r.ProjectID, &r.Locale, &r.Model, &r.FixtureDigest, &r.FixtureCount,
			&r.Adherence, &r.AdherenceBare, &r.Lift, &r.TokensUsed, &r.MeasuredAt); err != nil {
			return nil, fmt.Errorf("scan model-sweep result: %w", err)
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

// ModelRecommender supplies the measured model recommendation for a
// project+locale (EV-4 consumption). Implemented by *SweepModelRecommender; a
// fake serves unit tests.
type ModelRecommender interface {
	RecommendedModel(ctx context.Context, projectID, locale string) (string, bool)
}

// SweepModelRecommender is the consumption side of measured steerability
// (EV-4): the worker's platform model resolution consults it for jobs whose
// workspace has not pinned a model. It applies the recommendation only when
//
//   - the instance flag (model_sweeps.enabled) is ON,
//   - the measurement is fresh enough (MaxAge; default RecommendMaxAge),
//   - the recommendation clears the floor/min-fixture policy, and
//   - the recommended model is still in the ctrl-enabled set.
//
// Strictly best-effort: any store error yields "no recommendation" so the
// platform default always remains the fallback.
type SweepModelRecommender struct {
	Store    ModelSweepStore
	Settings ModelSweepSettings
	// MaxAge bounds how old a measurement may be and still steer live jobs.
	// Zero uses RecommendMaxAge.
	MaxAge time.Duration
}

// RecommendMaxAge is the default consumption freshness bound: twice the
// weekly sweep cadence, so a single missed cron never flaps the model back to
// the platform default.
const RecommendMaxAge = 14 * 24 * time.Hour

// RecommendedModel returns the model to use for a project+locale, and whether
// a qualifying recommendation exists.
func (r *SweepModelRecommender) RecommendedModel(ctx context.Context, projectID, locale string) (string, bool) {
	if r == nil || r.Store == nil || r.Settings == nil || !r.Settings.ModelSweepsEnabled() {
		return "", false
	}
	rows, err := r.Store.LatestModelSweepResults(ctx, projectID, locale)
	if err != nil {
		return "", false
	}
	best := RecommendModel(rows)
	if best == nil {
		return "", false
	}
	maxAge := r.MaxAge
	if maxAge <= 0 {
		maxAge = RecommendMaxAge
	}
	if time.Since(best.MeasuredAt) > maxAge {
		return "", false
	}
	if !r.Settings.IsModelEnabled(best.Model) {
		return "", false
	}
	return best.Model, true
}
