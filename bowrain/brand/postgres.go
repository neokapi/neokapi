package brand

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
)

// PostgresBrandStore implements brand.BrandStore using PostgreSQL.
type PostgresBrandStore struct {
	db *storage.PgDB
}

// NewPostgresBrandStore creates a new PostgreSQL-backed brand store.
func NewPostgresBrandStore(db *storage.PgDB) (*PostgresBrandStore, error) {
	if err := storage.MigratePostgresNS(db, "brand_schema_migrations", brandMigrations); err != nil {
		return nil, fmt.Errorf("brand migration: %w", err)
	}
	return &PostgresBrandStore{db: db}, nil
}

// Close is a no-op; the caller owns the database connection.
func (s *PostgresBrandStore) Close() error {
	return nil
}

// ---------------------------------------------------------------------------
// Profile CRUD
// ---------------------------------------------------------------------------

func (s *PostgresBrandStore) CreateProfile(ctx context.Context, profile *corebrand.VoiceProfile) error {
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
	autonomy, err := json.Marshal(profile.Autonomy)
	if err != nil {
		return fmt.Errorf("marshal autonomy: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO brand_profiles (id, workspace_id, name, description, tone, style, vocabulary, examples, locales, channels, autonomy, version, created_at, updated_at, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		profile.ID, profile.WorkspaceID, profile.Name, profile.Description,
		string(tone), string(style), string(vocab), string(examples),
		string(locales), string(channels), string(autonomy),
		profile.Version, now, now, profile.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert brand profile: %w", err)
	}
	return nil
}

func (s *PostgresBrandStore) GetProfile(ctx context.Context, profileID string) (*corebrand.VoiceProfile, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, name, description, tone, style, vocabulary, examples, locales, channels, autonomy, version, created_at, updated_at, created_by
		 FROM brand_profiles WHERE id = $1`, profileID)
	return scanProfile(row)
}

func (s *PostgresBrandStore) UpdateProfile(ctx context.Context, profile *corebrand.VoiceProfile) error {
	// Archive the current state as an immutable ProfileVersion before applying the edit.
	existing, err := s.GetProfile(ctx, profile.ID)
	if err != nil {
		return fmt.Errorf("get existing profile for versioning: %w", err)
	}

	snapshotJSON, _ := json.Marshal(existing)
	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO brand_profile_versions (profile_id, version, snapshot, note, created_by, created_at)
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
	autonomy, err := json.Marshal(profile.Autonomy)
	if err != nil {
		return fmt.Errorf("marshal autonomy: %w", err)
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE brand_profiles
		 SET name=$1, description=$2, tone=$3, style=$4, vocabulary=$5, examples=$6,
		     locales=$7, channels=$8, autonomy=$9, version=$10, updated_at=$11
		 WHERE id=$12`,
		profile.Name, profile.Description,
		string(tone), string(style), string(vocab), string(examples),
		string(locales), string(channels), string(autonomy),
		profile.Version, now, profile.ID)
	if err != nil {
		return fmt.Errorf("update brand profile: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("brand profile %s not found", profile.ID)
	}
	return nil
}

func (s *PostgresBrandStore) DeleteProfile(ctx context.Context, profileID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM brand_profiles WHERE id=$1`, profileID)
	if err != nil {
		return fmt.Errorf("delete brand profile: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("brand profile %s not found", profileID)
	}
	return nil
}

func (s *PostgresBrandStore) ListProfiles(ctx context.Context, workspaceID string) ([]*corebrand.VoiceProfile, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_id, name, description, tone, style, vocabulary, examples, locales, channels, autonomy, version, created_at, updated_at, created_by
		 FROM brand_profiles WHERE workspace_id = $1 ORDER BY name`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list brand profiles: %w", err)
	}
	defer rows.Close()

	var result []*corebrand.VoiceProfile
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

func (s *PostgresBrandStore) StoreScore(ctx context.Context, score *corebrand.StoredScore) error {
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

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO brand_voice_scores (id, project_id, stream, block_id, profile_id, profile_version, locale, score, dimensions, findings, checked_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		score.ID, score.ProjectID, stream, score.BlockID, score.ProfileID,
		score.ProfileVersion, string(locale.Normalize(score.Locale)), score.Score, string(dims), string(findings), score.CheckedAt)
	if err != nil {
		return fmt.Errorf("insert brand voice score: %w", err)
	}
	return nil
}

func (s *PostgresBrandStore) GetScores(ctx context.Context, projectID string, loc model.LocaleID) ([]*corebrand.StoredScore, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, stream, block_id, profile_id, profile_version, locale, score, dimensions, findings, checked_at
		 FROM brand_voice_scores WHERE project_id = $1 AND locale = $2
		 ORDER BY checked_at DESC`, projectID, string(locale.Normalize(loc)))
	if err != nil {
		return nil, fmt.Errorf("get brand scores: %w", err)
	}
	defer rows.Close()

	var result []*corebrand.StoredScore
	for rows.Next() {
		sc, err := scanScore(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, sc)
	}
	return result, rows.Err()
}

func (s *PostgresBrandStore) GetScoreTrends(ctx context.Context, projectID string, days int) ([]*corebrand.ScoreTrend, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DATE(checked_at) AS d, AVG(score)::int, COUNT(*)
		 FROM brand_voice_scores
		 WHERE project_id = $1 AND checked_at >= NOW() - MAKE_INTERVAL(days => $2)
		 GROUP BY d ORDER BY d`, projectID, days)
	if err != nil {
		return nil, fmt.Errorf("get score trends: %w", err)
	}
	defer rows.Close()

	var result []*corebrand.ScoreTrend
	for rows.Next() {
		var t corebrand.ScoreTrend
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

func (s *PostgresBrandStore) StoreCorrection(ctx context.Context, correction *corebrand.Correction) error {
	if correction.ID == "" {
		correction.ID = id.New()
	}
	if correction.CorrectedAt.IsZero() {
		correction.CorrectedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO brand_voice_corrections (id, profile_id, block_id, dimension, original_text, corrected_text, finding_id, corrected_by, corrected_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		correction.ID, correction.ProfileID, correction.BlockID,
		string(correction.Dimension), correction.OriginalText, correction.CorrectedText,
		correction.FindingID, correction.CorrectedBy, correction.CorrectedAt)
	if err != nil {
		return fmt.Errorf("insert brand correction: %w", err)
	}
	return nil
}

func (s *PostgresBrandStore) GetSuggestedRules(ctx context.Context, workspaceID string, minCount int) ([]*corebrand.SuggestedRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.original_text, c.corrected_text, COUNT(*) AS cnt, c.dimension
		 FROM brand_voice_corrections c
		 JOIN brand_profiles p ON p.id = c.profile_id
		 WHERE p.workspace_id = $1
		 GROUP BY c.original_text, c.corrected_text, c.dimension
		 HAVING COUNT(*) >= $2
		 ORDER BY cnt DESC`, workspaceID, minCount)
	if err != nil {
		return nil, fmt.Errorf("get suggested rules: %w", err)
	}
	defer rows.Close()

	var result []*corebrand.SuggestedRule
	for rows.Next() {
		var r corebrand.SuggestedRule
		var dim string
		if err := rows.Scan(&r.Term, &r.Replacement, &r.CorrectionCount, &dim); err != nil {
			return nil, fmt.Errorf("scan suggested rule: %w", err)
		}
		r.Dimension = corebrand.Dimension(dim)
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
func (s *PostgresBrandStore) conceptIDsByTerm(ctx context.Context, workspaceID string) (map[string]string, error) {
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
func (s *PostgresBrandStore) collectDecisionConcepts(ctx context.Context, workspaceID string, byTerm map[string]string) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT d.term, d.concept_id
		 FROM brand_rule_decisions d
		 JOIN brand_profiles p ON p.id = d.profile_id
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
func (s *PostgresBrandStore) collectVocabConcepts(ctx context.Context, workspaceID string, byTerm map[string]string) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT vocabulary FROM brand_profiles WHERE workspace_id = $1`, workspaceID)
	if err != nil {
		return fmt.Errorf("load profile vocabularies: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var vocabJSON string
		if err := rows.Scan(&vocabJSON); err != nil {
			return fmt.Errorf("scan profile vocabulary: %w", err)
		}
		var v corebrand.VocabularyRules
		if err := json.Unmarshal([]byte(vocabJSON), &v); err != nil {
			continue
		}
		for _, group := range [][]corebrand.TermRule{v.ForbiddenTerms, v.CompetitorTerms} {
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

func (s *PostgresBrandStore) RecordRuleDecision(ctx context.Context, d *corebrand.RuleDecision) error {
	if d.DecidedAt.IsZero() {
		d.DecidedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO brand_rule_decisions
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

func (s *PostgresBrandStore) GetRuleDecision(ctx context.Context, profileID, term string) (*corebrand.RuleDecision, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT profile_id, term, replacement, dimension, status, correction_count, promoted_version, auto, concept_id, decided_by, decided_at
		 FROM brand_rule_decisions WHERE profile_id = $1 AND LOWER(term) = LOWER($2)`, profileID, term)
	var d corebrand.RuleDecision
	var dim, status string
	if err := row.Scan(&d.ProfileID, &d.Term, &d.Replacement, &dim, &status,
		&d.CorrectionCount, &d.PromotedVersion, &d.Auto, &d.ConceptID, &d.DecidedBy, &d.DecidedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get rule decision: %w", err)
	}
	d.Dimension = corebrand.Dimension(dim)
	d.Status = corebrand.RuleDecisionStatus(status)
	return &d, nil
}

func (s *PostgresBrandStore) ListRuleDecisions(ctx context.Context, profileID string) ([]*corebrand.RuleDecision, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT profile_id, term, replacement, dimension, status, correction_count, promoted_version, auto, concept_id, decided_by, decided_at
		 FROM brand_rule_decisions WHERE profile_id = $1 ORDER BY decided_at DESC`, profileID)
	if err != nil {
		return nil, fmt.Errorf("list rule decisions: %w", err)
	}
	defer rows.Close()
	var out []*corebrand.RuleDecision
	for rows.Next() {
		var d corebrand.RuleDecision
		var dim, status string
		if err := rows.Scan(&d.ProfileID, &d.Term, &d.Replacement, &dim, &status,
			&d.CorrectionCount, &d.PromotedVersion, &d.Auto, &d.ConceptID, &d.DecidedBy, &d.DecidedAt); err != nil {
			return nil, fmt.Errorf("scan rule decision: %w", err)
		}
		d.Dimension = corebrand.Dimension(dim)
		d.Status = corebrand.RuleDecisionStatus(status)
		out = append(out, &d)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Profile versioning
// ---------------------------------------------------------------------------

func (s *PostgresBrandStore) ListProfileVersions(ctx context.Context, profileID string) ([]*corebrand.ProfileVersion, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT profile_id, version, snapshot, note, created_by, created_at
		 FROM brand_profile_versions WHERE profile_id = $1 ORDER BY version DESC`, profileID)
	if err != nil {
		return nil, fmt.Errorf("list profile versions: %w", err)
	}
	defer rows.Close()

	var versions []*corebrand.ProfileVersion
	for rows.Next() {
		var v corebrand.ProfileVersion
		var snapshotJSON string
		if err := rows.Scan(&v.ProfileID, &v.Version, &snapshotJSON, &v.Note, &v.CreatedBy, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan profile version: %w", err)
		}
		_ = json.Unmarshal([]byte(snapshotJSON), &v.Snapshot)
		versions = append(versions, &v)
	}
	return versions, rows.Err()
}

func (s *PostgresBrandStore) GetProfileVersion(ctx context.Context, profileID string, version int) (*corebrand.ProfileVersion, error) {
	var v corebrand.ProfileVersion
	var snapshotJSON string
	err := s.db.QueryRowContext(ctx,
		`SELECT profile_id, version, snapshot, note, created_by, created_at
		 FROM brand_profile_versions WHERE profile_id = $1 AND version = $2`, profileID, version).
		Scan(&v.ProfileID, &v.Version, &snapshotJSON, &v.Note, &v.CreatedBy, &v.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("profile version not found: %s v%d", profileID, version)
	}
	if err != nil {
		return nil, fmt.Errorf("get profile version: %w", err)
	}
	_ = json.Unmarshal([]byte(snapshotJSON), &v.Snapshot)
	return &v, nil
}

func (s *PostgresBrandStore) GetProfileAtTag(ctx context.Context, profileID, tagName string) (*corebrand.VoiceProfile, error) {
	var version int
	err := s.db.QueryRowContext(ctx,
		`SELECT version FROM brand_profile_tags WHERE profile_id = $1 AND name = $2`, profileID, tagName).
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

func (s *PostgresBrandStore) CreateProfileTag(ctx context.Context, tag *corebrand.ProfileTag) error {
	if tag.CreatedAt.IsZero() {
		tag.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO brand_profile_tags (profile_id, name, version, created_by, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		tag.ProfileID, tag.Name, tag.Version, tag.CreatedBy, tag.CreatedAt)
	if err != nil {
		return fmt.Errorf("create profile tag: %w", err)
	}
	return nil
}

func (s *PostgresBrandStore) ListProfileTags(ctx context.Context, profileID string) ([]*corebrand.ProfileTag, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT profile_id, name, version, created_by, created_at
		 FROM brand_profile_tags WHERE profile_id = $1 ORDER BY name`, profileID)
	if err != nil {
		return nil, fmt.Errorf("list profile tags: %w", err)
	}
	defer rows.Close()

	var tags []*corebrand.ProfileTag
	for rows.Next() {
		var t corebrand.ProfileTag
		if err := rows.Scan(&t.ProfileID, &t.Name, &t.Version, &t.CreatedBy, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan profile tag: %w", err)
		}
		tags = append(tags, &t)
	}
	return tags, rows.Err()
}

func (s *PostgresBrandStore) DeleteProfileTag(ctx context.Context, profileID, tagName string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM brand_profile_tags WHERE profile_id = $1 AND name = $2`, profileID, tagName)
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

func (s *PostgresBrandStore) GetScoresByStream(ctx context.Context, projectID, stream string) ([]*corebrand.StoredScore, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, stream, block_id, profile_id, profile_version, locale, score, dimensions, findings, checked_at
		 FROM brand_voice_scores WHERE project_id = $1 AND stream = $2
		 ORDER BY checked_at DESC`, projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("query scores by stream: %w", err)
	}
	defer rows.Close()

	var result []*corebrand.StoredScore
	for rows.Next() {
		sc, err := scanScore(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, sc)
	}
	return result, rows.Err()
}

// ---------------------------------------------------------------------------
// Scan helpers
// ---------------------------------------------------------------------------

// scanner is an alias for storage.Scanner, satisfied by *sql.Row and *sql.Rows.
type scanner = storage.Scanner

func scanProfile(row scanner) (*corebrand.VoiceProfile, error) {
	var p corebrand.VoiceProfile
	var toneJSON, styleJSON, vocabJSON, examplesJSON, localesJSON, channelsJSON, autonomyJSON string

	err := row.Scan(
		&p.ID, &p.WorkspaceID, &p.Name, &p.Description,
		&toneJSON, &styleJSON, &vocabJSON, &examplesJSON,
		&localesJSON, &channelsJSON, &autonomyJSON,
		&p.Version, &p.CreatedAt, &p.UpdatedAt, &p.CreatedBy)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("brand profile not found")
		}
		return nil, fmt.Errorf("scan brand profile: %w", err)
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
		p.Locales = map[model.LocaleID]corebrand.LocaleOverride{}
	}
	if err := json.Unmarshal([]byte(channelsJSON), &p.Channels); err != nil {
		p.Channels = map[string]corebrand.ChannelOverride{}
	}
	if err := json.Unmarshal([]byte(autonomyJSON), &p.Autonomy); err != nil {
		p.Autonomy = corebrand.AutonomyConfig{}
	}

	return &p, nil
}

func scanScore(row scanner) (*corebrand.StoredScore, error) {
	var sc corebrand.StoredScore
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
