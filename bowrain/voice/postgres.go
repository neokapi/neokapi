package voice

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// PostgresVoiceStore implements profile.Store using PostgreSQL.
type PostgresVoiceStore struct {
	db *storage.PgDB
	// tx is the transition this store was bound into, or nil for the pool. A
	// push writes voice profiles and content in one transaction, so the same
	// store has to be able to run either way — see storage.PgDB.Transition.
	tx storage.Runner
}

// Bind returns this store with its statements directed at tx, for a caller that
// owns the transaction and is putting more than one store in it. The receiver
// is left alone: the pooled store stays usable beside the bound one.
func (s *PostgresVoiceStore) Bind(tx storage.Runner) *PostgresVoiceStore {
	bound := *s
	bound.tx = tx
	return &bound
}

// run is where this store's statements go.
func (s *PostgresVoiceStore) run() storage.Runner {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

// NewPostgresVoiceStore creates a new PostgreSQL-backed voice store.
func NewPostgresVoiceStore(db *storage.PgDB) (*PostgresVoiceStore, error) {
	if err := storage.MigratePostgresNS(db, "voice_schema_migrations", Migrations); err != nil {
		return nil, fmt.Errorf("brand migration: %w", err)
	}
	return &PostgresVoiceStore{db: db}, nil
}

// Close is a no-op; the caller owns the database connection.
func (s *PostgresVoiceStore) Close() error {
	return nil
}

// ---------------------------------------------------------------------------
// Profile CRUD
// ---------------------------------------------------------------------------

// profileColumns is the one column list every profile read selects, in the
// order scanProfile scans. Spelled once because a column added to the table and
// forgotten in one of the reads is invisible — the row still scans, the field
// just comes back zero, which is exactly how min_score reached the API, the
// wire and four UI surfaces while the store silently dropped it.
const profileColumns = `id, workspace_id, name, description, tone, style, vocabulary, examples,
	locales, channels, personas, autonomy, min_score, version, created_at, updated_at, created_by`

func (s *PostgresVoiceStore) CreateProfile(ctx context.Context, profile *coreprofile.VoiceProfile) error {
	if profile.ID == "" {
		profile.ID = id.New()
	}
	now := time.Now().UTC()
	profile.CreatedAt = now
	profile.UpdatedAt = now
	profile.Version = 1

	tone, err := json.Marshal(profile.Tone)
	if err != nil {
		return fmt.Errorf("marshal tone: %w", err)
	}
	style, err := json.Marshal(profile.Style)
	if err != nil {
		return fmt.Errorf("marshal style: %w", err)
	}
	vocab, err := json.Marshal(profile.Vocabulary)
	if err != nil {
		return fmt.Errorf("marshal vocabulary: %w", err)
	}
	examples, err := json.Marshal(profile.Examples)
	if err != nil {
		return fmt.Errorf("marshal examples: %w", err)
	}
	locales, err := json.Marshal(profile.Locales)
	if err != nil {
		return fmt.Errorf("marshal locales: %w", err)
	}
	channels, err := json.Marshal(profile.Channels)
	if err != nil {
		return fmt.Errorf("marshal channels: %w", err)
	}
	personas, err := json.Marshal(profile.Personas)
	if err != nil {
		return fmt.Errorf("marshal personas: %w", err)
	}
	autonomy, err := json.Marshal(profile.Autonomy)
	if err != nil {
		return fmt.Errorf("marshal autonomy: %w", err)
	}

	_, err = s.run().ExecContext(ctx,
		`INSERT INTO voice_profiles (`+profileColumns+`)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
		profile.ID, profile.Scope, profile.Name, profile.Description,
		string(tone), string(style), string(vocab), string(examples),
		string(locales), string(channels), string(personas), string(autonomy),
		profile.MinScore, profile.Version, now, now, profile.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert voice profile: %w", err)
	}
	return nil
}

func (s *PostgresVoiceStore) GetProfile(ctx context.Context, profileID string) (*coreprofile.VoiceProfile, error) {
	row := s.run().QueryRowContext(ctx,
		`SELECT `+profileColumns+`
		 FROM voice_profiles WHERE id = $1`, profileID)
	return scanProfile(row)
}

func (s *PostgresVoiceStore) UpdateProfile(ctx context.Context, profile *coreprofile.VoiceProfile) error {
	// Archive the current state as an immutable ProfileVersion before applying the edit.
	existing, err := s.GetProfile(ctx, profile.ID)
	if err != nil {
		return fmt.Errorf("get existing profile for versioning: %w", err)
	}

	snapshotJSON, _ := json.Marshal(existing)
	_, _ = s.run().ExecContext(ctx,
		`INSERT INTO voice_profile_versions (profile_id, version, snapshot, note, created_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, NOW())
		 ON CONFLICT DO NOTHING`,
		existing.ID, existing.Version, string(snapshotJSON),
		profile.VersionNote, existing.CreatedBy)

	now := time.Now().UTC()
	profile.UpdatedAt = now
	profile.Version = existing.Version + 1

	tone, err := json.Marshal(profile.Tone)
	if err != nil {
		return fmt.Errorf("marshal tone: %w", err)
	}
	style, err := json.Marshal(profile.Style)
	if err != nil {
		return fmt.Errorf("marshal style: %w", err)
	}
	vocab, err := json.Marshal(profile.Vocabulary)
	if err != nil {
		return fmt.Errorf("marshal vocabulary: %w", err)
	}
	examples, err := json.Marshal(profile.Examples)
	if err != nil {
		return fmt.Errorf("marshal examples: %w", err)
	}
	locales, err := json.Marshal(profile.Locales)
	if err != nil {
		return fmt.Errorf("marshal locales: %w", err)
	}
	channels, err := json.Marshal(profile.Channels)
	if err != nil {
		return fmt.Errorf("marshal channels: %w", err)
	}
	personas, err := json.Marshal(profile.Personas)
	if err != nil {
		return fmt.Errorf("marshal personas: %w", err)
	}
	autonomy, err := json.Marshal(profile.Autonomy)
	if err != nil {
		return fmt.Errorf("marshal autonomy: %w", err)
	}

	res, err := s.run().ExecContext(ctx,
		`UPDATE voice_profiles
		 SET name=$1, description=$2, tone=$3, style=$4, vocabulary=$5, examples=$6,
		     locales=$7, channels=$8, personas=$9, autonomy=$10, min_score=$11,
		     version=$12, updated_at=$13
		 WHERE id=$14`,
		profile.Name, profile.Description,
		string(tone), string(style), string(vocab), string(examples),
		string(locales), string(channels), string(personas), string(autonomy),
		profile.MinScore, profile.Version, now, profile.ID)
	if err != nil {
		return fmt.Errorf("update voice profile: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("voice profile %s not found", profile.ID)
	}
	return nil
}

func (s *PostgresVoiceStore) DeleteProfile(ctx context.Context, profileID string) error {
	res, err := s.run().ExecContext(ctx, `DELETE FROM voice_profiles WHERE id=$1`, profileID)
	if err != nil {
		return fmt.Errorf("delete voice profile: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("voice profile %s not found", profileID)
	}
	return nil
}

func (s *PostgresVoiceStore) ListProfiles(ctx context.Context, workspaceID string) ([]*coreprofile.VoiceProfile, error) {
	rows, err := s.run().QueryContext(ctx,
		`SELECT `+profileColumns+`
		 FROM voice_profiles WHERE workspace_id = $1 ORDER BY name`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list brand profiles: %w", err)
	}
	defer rows.Close()

	var result []*coreprofile.VoiceProfile
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// ---------------------------------------------------------------------------
// Score storage
// ---------------------------------------------------------------------------

func (s *PostgresVoiceStore) StoreScore(ctx context.Context, score *coreprofile.StoredScore) error {
	if score.ID == "" {
		score.ID = id.New()
	}
	if score.CheckedAt.IsZero() {
		score.CheckedAt = time.Now().UTC()
	}

	dims, err := json.Marshal(score.Dimensions)
	if err != nil {
		return fmt.Errorf("marshal dimensions: %w", err)
	}
	findings, err := json.Marshal(score.Findings)
	if err != nil {
		return fmt.Errorf("marshal findings: %w", err)
	}

	stream := score.Stream
	if stream == "" {
		stream = "main"
	}

	_, err = s.run().ExecContext(ctx,
		`INSERT INTO voice_scores (id, project_id, stream, block_id, profile_id, profile_version, locale, score, dimensions, findings, checked_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		score.ID, score.ProjectID, stream, score.BlockID, score.ProfileID,
		score.ProfileVersion, string(locale.Normalize(score.Locale)), score.Score, string(dims), string(findings), score.CheckedAt)
	if err != nil {
		return fmt.Errorf("insert voice score: %w", err)
	}
	return nil
}

// GetScores returns the persisted scores for a project, newest first. An empty
// locale means ALL locales — the project-wide read the score endpoints and the
// voice rollup use — not "rows stored with an empty locale".
func (s *PostgresVoiceStore) GetScores(ctx context.Context, projectID string, loc model.LocaleID) ([]*coreprofile.StoredScore, error) {
	query := `SELECT id, project_id, stream, block_id, profile_id, profile_version, locale, score, dimensions, findings, checked_at
		 FROM voice_scores WHERE project_id = $1 AND locale = $2
		 ORDER BY checked_at DESC`
	args := []any{projectID, string(locale.Normalize(loc))}
	if loc == "" {
		query = `SELECT id, project_id, stream, block_id, profile_id, profile_version, locale, score, dimensions, findings, checked_at
		 FROM voice_scores WHERE project_id = $1
		 ORDER BY checked_at DESC`
		args = []any{projectID}
	}
	rows, err := s.run().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get brand scores: %w", err)
	}
	defer rows.Close()

	var result []*coreprofile.StoredScore
	for rows.Next() {
		sc, err := scanScore(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, sc)
	}
	return result, rows.Err()
}

func (s *PostgresVoiceStore) GetScoreTrends(ctx context.Context, projectID string, days int) ([]*coreprofile.ScoreTrend, error) {
	rows, err := s.run().QueryContext(ctx,
		`SELECT DATE(checked_at) AS d, AVG(score)::int, COUNT(*)
		 FROM voice_scores
		 WHERE project_id = $1 AND checked_at >= NOW() - MAKE_INTERVAL(days => $2)
		 GROUP BY d ORDER BY d`, projectID, days)
	if err != nil {
		return nil, fmt.Errorf("get score trends: %w", err)
	}
	defer rows.Close()

	var result []*coreprofile.ScoreTrend
	for rows.Next() {
		var t coreprofile.ScoreTrend
		if err := rows.Scan(&t.Date, &t.AvgScore, &t.Count); err != nil {
			return nil, fmt.Errorf("scan score trend: %w", err)
		}
		result = append(result, &t)
	}
	return result, rows.Err()
}

// ---------------------------------------------------------------------------
// Correction storage
// ---------------------------------------------------------------------------

func (s *PostgresVoiceStore) StoreCorrection(ctx context.Context, correction *coreprofile.Correction) error {
	if correction.ID == "" {
		correction.ID = id.New()
	}
	if correction.CorrectedAt.IsZero() {
		correction.CorrectedAt = time.Now().UTC()
	}

	_, err := s.run().ExecContext(ctx,
		`INSERT INTO voice_corrections (id, profile_id, block_id, dimension, original_text, corrected_text, finding_id, corrected_by, corrected_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		correction.ID, correction.ProfileID, correction.BlockID,
		string(correction.Dimension), correction.OriginalText, correction.CorrectedText,
		correction.FindingID, correction.CorrectedBy, correction.CorrectedAt)
	if err != nil {
		return fmt.Errorf("insert brand correction: %w", err)
	}
	return nil
}

func (s *PostgresVoiceStore) GetSuggestedRules(ctx context.Context, workspaceID string, minCount int) ([]*coreprofile.SuggestedRule, error) {
	rows, err := s.run().QueryContext(ctx,
		`SELECT c.original_text, c.corrected_text, COUNT(*) AS cnt, c.dimension
		 FROM voice_corrections c
		 JOIN voice_profiles p ON p.id = c.profile_id
		 WHERE p.workspace_id = $1
		 GROUP BY c.original_text, c.corrected_text, c.dimension
		 HAVING COUNT(*) >= $2
		 ORDER BY cnt DESC`, workspaceID, minCount)
	if err != nil {
		return nil, fmt.Errorf("get suggested rules: %w", err)
	}
	defer rows.Close()

	var result []*coreprofile.SuggestedRule
	for rows.Next() {
		var r coreprofile.SuggestedRule
		var dim string
		if err := rows.Scan(&r.Term, &r.Replacement, &r.CorrectionCount, &dim); err != nil {
			return nil, fmt.Errorf("scan suggested rule: %w", err)
		}
		r.Dimension = coreprofile.Dimension(dim)
		result = append(result, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Back-fill the knowledge-graph concept each suggested term already denotes, so
	// the concept-backed suggestion story is visible in the candidate list (AD-021).
	// A correction aggregate carries no concept_id of its own; the authoritative
	// link lives on the promoted TermRule (the live profile vocabulary) and, durably
	// across a later demote, on the rule decision.
	if len(result) > 0 {
		byTerm, err := s.conceptIDsByTerm(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		for _, r := range result {
			if r.ConceptID == "" {
				if cid := byTerm[strings.ToLower(strings.TrimSpace(r.Term))]; cid != "" {
					r.ConceptID = cid
				}
			}
		}
	}
	return result, nil
}

// conceptIDsByTerm builds a lower-cased term → knowledge-graph concept ID map for
// a workspace, so correction-derived suggestions can surface the concept a term
// already denotes. It draws from two authoritative sources: the durable rule
// decisions (which retain a promoted term's concept even after it is demoted and
// the live profile no longer carries it) and the live profiles' enforced
// vocabulary (the current truth, which wins on conflict).
func (s *PostgresVoiceStore) conceptIDsByTerm(ctx context.Context, workspaceID string) (map[string]string, error) {
	byTerm := map[string]string{}
	if err := s.collectDecisionConcepts(ctx, workspaceID, byTerm); err != nil {
		return nil, err
	}
	if err := s.collectVocabConcepts(ctx, workspaceID, byTerm); err != nil {
		return nil, err
	}
	return byTerm, nil
}

// collectDecisionConcepts records each promoted term's concept from the durable
// rule-decision log into byTerm (keyed lower-cased), covering terms that were
// later demoted out of the live profile.
func (s *PostgresVoiceStore) collectDecisionConcepts(ctx context.Context, workspaceID string, byTerm map[string]string) error {
	rows, err := s.run().QueryContext(ctx,
		`SELECT d.term, d.concept_id
		 FROM voice_rule_decisions d
		 JOIN voice_profiles p ON p.id = d.profile_id
		 WHERE p.workspace_id = $1 AND d.concept_id <> ''`, workspaceID)
	if err != nil {
		return fmt.Errorf("load rule-decision concepts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var term, conceptID string
		if err := rows.Scan(&term, &conceptID); err != nil {
			return fmt.Errorf("scan rule-decision concept: %w", err)
		}
		byTerm[strings.ToLower(strings.TrimSpace(term))] = conceptID
	}
	return rows.Err()
}

// collectVocabConcepts overlays the concept IDs carried by the live profiles'
// forbidden and competitor terms — the current, authoritative link — onto byTerm.
func (s *PostgresVoiceStore) collectVocabConcepts(ctx context.Context, workspaceID string, byTerm map[string]string) error {
	rows, err := s.run().QueryContext(ctx,
		`SELECT vocabulary FROM voice_profiles WHERE workspace_id = $1`, workspaceID)
	if err != nil {
		return fmt.Errorf("load profile vocabularies: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var vocabJSON string
		if err := rows.Scan(&vocabJSON); err != nil {
			return fmt.Errorf("scan profile vocabulary: %w", err)
		}
		var v coreprofile.VocabularyRules
		if err := json.Unmarshal([]byte(vocabJSON), &v); err != nil {
			continue
		}
		for _, group := range [][]coreprofile.TermRule{v.ForbiddenTerms, v.CompetitorTerms} {
			for _, rule := range group {
				if rule.ConceptID != "" {
					byTerm[strings.ToLower(strings.TrimSpace(rule.Term))] = rule.ConceptID
				}
			}
		}
	}
	return rows.Err()
}

// ---------------------------------------------------------------------------
// Rule decisions (review/approve/reject/promote of candidate rules)
// ---------------------------------------------------------------------------

func (s *PostgresVoiceStore) RecordRuleDecision(ctx context.Context, d *coreprofile.RuleDecision) error {
	if d.DecidedAt.IsZero() {
		d.DecidedAt = time.Now().UTC()
	}
	_, err := s.run().ExecContext(ctx,
		`INSERT INTO voice_rule_decisions
		   (profile_id, term, replacement, dimension, status, correction_count, promoted_version, auto, concept_id, decided_by, decided_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT (profile_id, term) DO UPDATE SET
		   replacement = EXCLUDED.replacement,
		   dimension = EXCLUDED.dimension,
		   status = EXCLUDED.status,
		   correction_count = EXCLUDED.correction_count,
		   promoted_version = EXCLUDED.promoted_version,
		   auto = EXCLUDED.auto,
		   concept_id = EXCLUDED.concept_id,
		   decided_by = EXCLUDED.decided_by,
		   decided_at = EXCLUDED.decided_at`,
		d.ProfileID, d.Term, d.Replacement, string(d.Dimension), string(d.Status),
		d.CorrectionCount, d.PromotedVersion, d.Auto, d.ConceptID, d.DecidedBy, d.DecidedAt)
	if err != nil {
		return fmt.Errorf("record rule decision: %w", err)
	}
	return nil
}

func (s *PostgresVoiceStore) GetRuleDecision(ctx context.Context, profileID, term string) (*coreprofile.RuleDecision, error) {
	row := s.run().QueryRowContext(ctx,
		`SELECT profile_id, term, replacement, dimension, status, correction_count, promoted_version, auto, concept_id, decided_by, decided_at
		 FROM voice_rule_decisions WHERE profile_id = $1 AND LOWER(term) = LOWER($2)`, profileID, term)
	var d coreprofile.RuleDecision
	var dim, status string
	if err := row.Scan(&d.ProfileID, &d.Term, &d.Replacement, &dim, &status,
		&d.CorrectionCount, &d.PromotedVersion, &d.Auto, &d.ConceptID, &d.DecidedBy, &d.DecidedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get rule decision: %w", err)
	}
	d.Dimension = coreprofile.Dimension(dim)
	d.Status = coreprofile.RuleDecisionStatus(status)
	return &d, nil
}

func (s *PostgresVoiceStore) ListRuleDecisions(ctx context.Context, profileID string) ([]*coreprofile.RuleDecision, error) {
	rows, err := s.run().QueryContext(ctx,
		`SELECT profile_id, term, replacement, dimension, status, correction_count, promoted_version, auto, concept_id, decided_by, decided_at
		 FROM voice_rule_decisions WHERE profile_id = $1 ORDER BY decided_at DESC`, profileID)
	if err != nil {
		return nil, fmt.Errorf("list rule decisions: %w", err)
	}
	defer rows.Close()
	var out []*coreprofile.RuleDecision
	for rows.Next() {
		var d coreprofile.RuleDecision
		var dim, status string
		if err := rows.Scan(&d.ProfileID, &d.Term, &d.Replacement, &dim, &status,
			&d.CorrectionCount, &d.PromotedVersion, &d.Auto, &d.ConceptID, &d.DecidedBy, &d.DecidedAt); err != nil {
			return nil, fmt.Errorf("scan rule decision: %w", err)
		}
		d.Dimension = coreprofile.Dimension(dim)
		d.Status = coreprofile.RuleDecisionStatus(status)
		out = append(out, &d)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Profile versioning
// ---------------------------------------------------------------------------

func (s *PostgresVoiceStore) ListProfileVersions(ctx context.Context, profileID string) ([]*coreprofile.ProfileVersion, error) {
	rows, err := s.run().QueryContext(ctx,
		`SELECT profile_id, version, snapshot, note, created_by, created_at
		 FROM voice_profile_versions WHERE profile_id = $1 ORDER BY version DESC`, profileID)
	if err != nil {
		return nil, fmt.Errorf("list profile versions: %w", err)
	}
	defer rows.Close()

	var versions []*coreprofile.ProfileVersion
	for rows.Next() {
		var v coreprofile.ProfileVersion
		var snapshotJSON string
		if err := rows.Scan(&v.ProfileID, &v.Version, &snapshotJSON, &v.Note, &v.CreatedBy, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan profile version: %w", err)
		}
		if err := json.Unmarshal([]byte(snapshotJSON), &v.Snapshot); err != nil {
			return nil, fmt.Errorf("profile %s v%d: unmarshal snapshot: %w", v.ProfileID, v.Version, err)
		}
		versions = append(versions, &v)
	}
	return versions, rows.Err()
}

func (s *PostgresVoiceStore) GetProfileVersion(ctx context.Context, profileID string, version int) (*coreprofile.ProfileVersion, error) {
	var v coreprofile.ProfileVersion
	var snapshotJSON string
	err := s.run().QueryRowContext(ctx,
		`SELECT profile_id, version, snapshot, note, created_by, created_at
		 FROM voice_profile_versions WHERE profile_id = $1 AND version = $2`, profileID, version).
		Scan(&v.ProfileID, &v.Version, &snapshotJSON, &v.Note, &v.CreatedBy, &v.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("profile version not found: %s v%d", profileID, version)
	}
	if err != nil {
		return nil, fmt.Errorf("get profile version: %w", err)
	}
	if err := json.Unmarshal([]byte(snapshotJSON), &v.Snapshot); err != nil {
		return nil, fmt.Errorf("profile %s v%d: unmarshal snapshot: %w", v.ProfileID, v.Version, err)
	}
	return &v, nil
}

func (s *PostgresVoiceStore) GetProfileAtTag(ctx context.Context, profileID, tagName string) (*coreprofile.VoiceProfile, error) {
	var version int
	err := s.run().QueryRowContext(ctx,
		`SELECT version FROM voice_profile_tags WHERE profile_id = $1 AND name = $2`, profileID, tagName).
		Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("profile tag not found: %s/%s", profileID, tagName)
	}
	if err != nil {
		return nil, fmt.Errorf("get profile tag: %w", err)
	}

	v, err := s.GetProfileVersion(ctx, profileID, version)
	if err != nil {
		return nil, err
	}
	return &v.Snapshot, nil
}

// ---------------------------------------------------------------------------
// Profile tags
// ---------------------------------------------------------------------------

func (s *PostgresVoiceStore) CreateProfileTag(ctx context.Context, tag *coreprofile.ProfileTag) error {
	if tag.CreatedAt.IsZero() {
		tag.CreatedAt = time.Now().UTC()
	}
	_, err := s.run().ExecContext(ctx,
		`INSERT INTO voice_profile_tags (profile_id, name, version, created_by, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		tag.ProfileID, tag.Name, tag.Version, tag.CreatedBy, tag.CreatedAt)
	if err != nil {
		return fmt.Errorf("create profile tag: %w", err)
	}
	return nil
}

func (s *PostgresVoiceStore) ListProfileTags(ctx context.Context, profileID string) ([]*coreprofile.ProfileTag, error) {
	rows, err := s.run().QueryContext(ctx,
		`SELECT profile_id, name, version, created_by, created_at
		 FROM voice_profile_tags WHERE profile_id = $1 ORDER BY name`, profileID)
	if err != nil {
		return nil, fmt.Errorf("list profile tags: %w", err)
	}
	defer rows.Close()

	var tags []*coreprofile.ProfileTag
	for rows.Next() {
		var t coreprofile.ProfileTag
		if err := rows.Scan(&t.ProfileID, &t.Name, &t.Version, &t.CreatedBy, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan profile tag: %w", err)
		}
		tags = append(tags, &t)
	}
	return tags, rows.Err()
}

func (s *PostgresVoiceStore) DeleteProfileTag(ctx context.Context, profileID, tagName string) error {
	res, err := s.run().ExecContext(ctx,
		`DELETE FROM voice_profile_tags WHERE profile_id = $1 AND name = $2`, profileID, tagName)
	if err != nil {
		return fmt.Errorf("delete profile tag: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("profile tag not found: %s/%s", profileID, tagName)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Scores by stream
// ---------------------------------------------------------------------------

func (s *PostgresVoiceStore) GetScoresByStream(ctx context.Context, projectID, stream string) ([]*coreprofile.StoredScore, error) {
	rows, err := s.run().QueryContext(ctx,
		`SELECT id, project_id, stream, block_id, profile_id, profile_version, locale, score, dimensions, findings, checked_at
		 FROM voice_scores WHERE project_id = $1 AND stream = $2
		 ORDER BY checked_at DESC`, projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("query scores by stream: %w", err)
	}
	defer rows.Close()

	var result []*coreprofile.StoredScore
	for rows.Next() {
		sc, err := scanScore(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, sc)
	}
	return result, rows.Err()
}

// GetBlockScore implements coreprofile.BlockScoreReader. The locale is
// normalized to match how StoreScore writes it.
func (s *PostgresVoiceStore) GetBlockScore(ctx context.Context, projectID, stream, blockID string, loc model.LocaleID) (*coreprofile.StoredScore, error) {
	if stream == "" {
		stream = "main"
	}
	row := s.run().QueryRowContext(ctx,
		`SELECT id, project_id, stream, block_id, profile_id, profile_version, locale, score, dimensions, findings, checked_at
		 FROM voice_scores
		 WHERE project_id = $1 AND stream = $2 AND block_id = $3 AND locale = $4
		 ORDER BY checked_at DESC LIMIT 1`,
		projectID, stream, blockID, string(locale.Normalize(loc)))
	sc, err := scanScore(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return sc, nil
}

// ---------------------------------------------------------------------------
// Scan helpers
// ---------------------------------------------------------------------------

// scanner is an alias for storage.Scanner, satisfied by *sql.Row and *sql.Rows.
type scanner = storage.Scanner

func scanProfile(row scanner) (*coreprofile.VoiceProfile, error) {
	var p coreprofile.VoiceProfile
	var toneJSON, styleJSON, vocabJSON, examplesJSON, localesJSON, channelsJSON, personasJSON, autonomyJSON string

	err := row.Scan(
		&p.ID, &p.Scope, &p.Name, &p.Description,
		&toneJSON, &styleJSON, &vocabJSON, &examplesJSON,
		&localesJSON, &channelsJSON, &personasJSON, &autonomyJSON,
		&p.MinScore, &p.Version, &p.CreatedAt, &p.UpdatedAt, &p.CreatedBy)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("voice profile not found")
		}
		return nil, fmt.Errorf("scan voice profile: %w", err)
	}

	if err := json.Unmarshal([]byte(toneJSON), &p.Tone); err != nil {
		return nil, fmt.Errorf("unmarshal tone: %w", err)
	}
	if err := json.Unmarshal([]byte(styleJSON), &p.Style); err != nil {
		return nil, fmt.Errorf("unmarshal style: %w", err)
	}
	if err := json.Unmarshal([]byte(vocabJSON), &p.Vocabulary); err != nil {
		return nil, fmt.Errorf("unmarshal vocabulary: %w", err)
	}
	if err := json.Unmarshal([]byte(examplesJSON), &p.Examples); err != nil {
		return nil, fmt.Errorf("unmarshal examples: %w", err)
	}
	if err := json.Unmarshal([]byte(localesJSON), &p.Locales); err != nil {
		p.Locales = map[model.LocaleID]coreprofile.LocaleOverride{}
	}
	if err := json.Unmarshal([]byte(channelsJSON), &p.Channels); err != nil {
		p.Channels = map[string]coreprofile.ChannelOverride{}
	}
	if err := json.Unmarshal([]byte(personasJSON), &p.Personas); err != nil {
		p.Personas = map[string]coreprofile.PersonaOverride{}
	}
	if err := json.Unmarshal([]byte(autonomyJSON), &p.Autonomy); err != nil {
		p.Autonomy = coreprofile.AutonomyConfig{}
	}

	return &p, nil
}

func scanScore(row scanner) (*coreprofile.StoredScore, error) {
	var sc coreprofile.StoredScore
	var dimsJSON, findingsJSON string

	err := row.Scan(
		&sc.ID, &sc.ProjectID, &sc.Stream, &sc.BlockID, &sc.ProfileID,
		&sc.ProfileVersion, &sc.Locale, &sc.Score, &dimsJSON, &findingsJSON, &sc.CheckedAt)
	if err != nil {
		return nil, fmt.Errorf("scan brand score: %w", err)
	}

	if err := json.Unmarshal([]byte(dimsJSON), &sc.Dimensions); err != nil {
		return nil, fmt.Errorf("unmarshal dimensions: %w", err)
	}
	if err := json.Unmarshal([]byte(findingsJSON), &sc.Findings); err != nil {
		return nil, fmt.Errorf("unmarshal findings: %w", err)
	}

	return &sc, nil
}
