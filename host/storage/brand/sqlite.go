package brand

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/storage"
)

// SQLiteBrandStore implements brand.BrandStore using SQLite.
type SQLiteBrandStore struct {
	db *storage.DB
}

// migrations holds the brand voice store's schema. Version 1 is the launch
// baseline; user machines carry long-lived local databases, so every schema
// change after it MUST be an incremental migration — existing databases only
// ever run new versions, never a re-run of the baseline.
var migrations = []storage.Migration{
	{
		Version:     1,
		Description: "brand voice store schema (baseline)",
		SQL: `
		CREATE TABLE IF NOT EXISTS brand_profiles (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT,
			tone TEXT NOT NULL DEFAULT '{}',
			style TEXT NOT NULL DEFAULT '{}',
			vocabulary TEXT NOT NULL DEFAULT '{}',
			examples TEXT NOT NULL DEFAULT '[]',
			locales TEXT NOT NULL DEFAULT '{}',
			channels TEXT NOT NULL DEFAULT '{}',
			autonomy TEXT NOT NULL DEFAULT '{}',
			version INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			created_by TEXT NOT NULL DEFAULT '',
			UNIQUE (workspace_id, name)
		);

		CREATE TABLE IF NOT EXISTS brand_voice_scores (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			stream TEXT NOT NULL DEFAULT 'main',
			block_id TEXT NOT NULL,
			profile_id TEXT NOT NULL,
			profile_version INTEGER NOT NULL DEFAULT 0,
			locale TEXT NOT NULL,
			score INTEGER NOT NULL,
			dimensions TEXT NOT NULL,
			findings TEXT NOT NULL,
			checked_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS brand_voice_corrections (
			id TEXT PRIMARY KEY,
			profile_id TEXT NOT NULL,
			block_id TEXT NOT NULL,
			dimension TEXT NOT NULL,
			original_text TEXT NOT NULL,
			corrected_text TEXT NOT NULL,
			finding_id TEXT,
			corrected_by TEXT NOT NULL,
			corrected_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS brand_profile_versions (
			profile_id TEXT NOT NULL,
			version INTEGER NOT NULL,
			snapshot TEXT NOT NULL,
			note TEXT NOT NULL DEFAULT '',
			created_by TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			PRIMARY KEY (profile_id, version)
		);

		CREATE TABLE IF NOT EXISTS brand_profile_tags (
			profile_id TEXT NOT NULL,
			name TEXT NOT NULL,
			version INTEGER NOT NULL,
			created_by TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			PRIMARY KEY (profile_id, name)
		);

		CREATE TABLE IF NOT EXISTS brand_rule_decisions (
			profile_id TEXT NOT NULL,
			term TEXT NOT NULL,
			replacement TEXT NOT NULL DEFAULT '',
			dimension TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			correction_count INTEGER NOT NULL DEFAULT 0,
			promoted_version INTEGER NOT NULL DEFAULT 0,
			auto INTEGER NOT NULL DEFAULT 0,
			decided_by TEXT NOT NULL DEFAULT '',
			decided_at TEXT NOT NULL,
			PRIMARY KEY (profile_id, term)
		);
		`,
	},
	{
		Version:     2,
		Description: "author personas on brand profiles",
		SQL: `
		ALTER TABLE brand_profiles ADD COLUMN personas TEXT NOT NULL DEFAULT '{}';
		`,
	},
}

// NewSQLiteBrandStore creates a new SQLite-backed brand store.
func NewSQLiteBrandStore(db *storage.DB) (*SQLiteBrandStore, error) {
	if err := storage.Migrate(db, "brand", migrations); err != nil {
		return nil, fmt.Errorf("brand migration: %w", err)
	}
	return &SQLiteBrandStore{db: db}, nil
}

func (s *SQLiteBrandStore) CreateProfile(ctx context.Context, profile *corebrand.VoiceProfile) error {
	now := time.Now()
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = now
	}
	if profile.UpdatedAt.IsZero() {
		profile.UpdatedAt = now
	}
	if profile.Version == 0 {
		profile.Version = 1
	}
	tone, _ := json.Marshal(profile.Tone)
	style, _ := json.Marshal(profile.Style)
	vocab, _ := json.Marshal(profile.Vocabulary)
	examples, _ := json.Marshal(profile.Examples)
	locales, _ := json.Marshal(profile.Locales)
	channels, _ := json.Marshal(profile.Channels)
	personas, _ := json.Marshal(profile.Personas)
	autonomy, _ := json.Marshal(profile.Autonomy)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO brand_profiles (id, workspace_id, name, description, tone, style, vocabulary, examples, locales, channels, personas, autonomy, version, created_at, updated_at, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		profile.ID, profile.WorkspaceID, profile.Name, profile.Description,
		string(tone), string(style), string(vocab), string(examples),
		string(locales), string(channels), string(personas), string(autonomy), profile.Version,
		profile.CreatedAt.Format(time.RFC3339), profile.UpdatedAt.Format(time.RFC3339),
		profile.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert profile: %w", err)
	}
	return nil
}

func (s *SQLiteBrandStore) GetProfile(ctx context.Context, id string) (*corebrand.VoiceProfile, error) {
	var p corebrand.VoiceProfile
	var desc *string
	var toneJSON, styleJSON, vocabJSON, examplesJSON, localesJSON, channelsJSON, personasJSON, autonomyJSON string
	var createdStr, updatedStr string

	err := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, name, description, tone, style, vocabulary, examples, locales, channels, personas, autonomy, version, created_at, updated_at, created_by
		 FROM brand_profiles WHERE id = ?`, id).
		Scan(&p.ID, &p.WorkspaceID, &p.Name, &desc,
			&toneJSON, &styleJSON, &vocabJSON, &examplesJSON,
			&localesJSON, &channelsJSON, &personasJSON, &autonomyJSON, &p.Version,
			&createdStr, &updatedStr, &p.CreatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("profile not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get profile: %w", err)
	}
	if desc != nil {
		p.Description = *desc
	}
	// Mirror the Postgres sibling (bowrain/brand.scanProfile): treat malformed
	// JSON in the core descriptor fields as a hard error, but tolerate the
	// optional override maps by falling back to their zero values.
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
	if err := json.Unmarshal([]byte(personasJSON), &p.Personas); err != nil {
		p.Personas = map[string]corebrand.PersonaOverride{}
	}
	if err := json.Unmarshal([]byte(autonomyJSON), &p.Autonomy); err != nil {
		p.Autonomy = corebrand.AutonomyConfig{}
	}
	if p.CreatedAt, err = parseStoredTime(createdStr); err != nil {
		return nil, fmt.Errorf("profile %s: parse created_at: %w", p.ID, err)
	}
	if p.UpdatedAt, err = parseStoredTime(updatedStr); err != nil {
		return nil, fmt.Errorf("profile %s: parse updated_at: %w", p.ID, err)
	}
	return &p, nil
}

func (s *SQLiteBrandStore) UpdateProfile(ctx context.Context, profile *corebrand.VoiceProfile) error {
	// Archive the current state as an immutable ProfileVersion before applying the edit.
	existing, err := s.GetProfile(ctx, profile.ID)
	if err != nil {
		return fmt.Errorf("get existing profile for versioning: %w", err)
	}

	snapshotJSON, _ := json.Marshal(existing)
	now := time.Now()
	_, _ = s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO brand_profile_versions (profile_id, version, snapshot, note, created_by, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		existing.ID, existing.Version, string(snapshotJSON),
		profile.VersionNote, existing.CreatedBy, now.Format(time.RFC3339))

	profile.UpdatedAt = now
	profile.Version = existing.Version + 1
	tone, _ := json.Marshal(profile.Tone)
	style, _ := json.Marshal(profile.Style)
	vocab, _ := json.Marshal(profile.Vocabulary)
	examples, _ := json.Marshal(profile.Examples)
	locales, _ := json.Marshal(profile.Locales)
	channels, _ := json.Marshal(profile.Channels)
	personas, _ := json.Marshal(profile.Personas)
	autonomy, _ := json.Marshal(profile.Autonomy)

	result, err := s.db.ExecContext(ctx,
		`UPDATE brand_profiles SET name = ?, description = ?, tone = ?, style = ?, vocabulary = ?, examples = ?, locales = ?, channels = ?, personas = ?, autonomy = ?, version = ?, updated_at = ?
		 WHERE id = ?`,
		profile.Name, profile.Description,
		string(tone), string(style), string(vocab), string(examples),
		string(locales), string(channels), string(personas), string(autonomy), profile.Version,
		profile.UpdatedAt.Format(time.RFC3339), profile.ID)
	if err != nil {
		return fmt.Errorf("update profile: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("profile not found: %s", profile.ID)
	}
	return nil
}

func (s *SQLiteBrandStore) DeleteProfile(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM brand_profiles WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete profile: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("profile not found: %s", id)
	}
	return nil
}

func (s *SQLiteBrandStore) ListProfiles(ctx context.Context, workspaceID string) ([]*corebrand.VoiceProfile, error) {
	// Collect IDs first, then close the cursor before querying individual profiles.
	// SQLite :memory: databases use a single connection, so a nested query
	// while rows are open would deadlock.
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM brand_profiles WHERE workspace_id = ? ORDER BY name`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list profiles: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate profiles: %w", err)
	}
	rows.Close()

	var profiles []*corebrand.VoiceProfile
	for _, id := range ids {
		p, err := s.GetProfile(ctx, id)
		if err == nil {
			profiles = append(profiles, p)
		}
	}
	return profiles, nil
}

func (s *SQLiteBrandStore) StoreScore(ctx context.Context, score *corebrand.StoredScore) error {
	dims, _ := json.Marshal(score.Dimensions)
	findings, _ := json.Marshal(score.Findings)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO brand_voice_scores (id, project_id, stream, block_id, profile_id, profile_version, locale, score, dimensions, findings, checked_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		score.ID, score.ProjectID, score.Stream, score.BlockID,
		score.ProfileID, score.ProfileVersion, string(locale.Normalize(score.Locale)), score.Score,
		string(dims), string(findings),
		score.CheckedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert score: %w", err)
	}
	return nil
}

// GetScores returns the persisted scores for a project, newest first. An empty
// locale means ALL locales — the project-wide read the score endpoints and the
// brand rollup use — not "rows stored with an empty locale".
func (s *SQLiteBrandStore) GetScores(ctx context.Context, projectID string, loc model.LocaleID) ([]*corebrand.StoredScore, error) {
	query := `SELECT id, project_id, stream, block_id, profile_id, locale, score, dimensions, findings, checked_at
		 FROM brand_voice_scores WHERE project_id = ? AND locale = ? ORDER BY checked_at DESC`
	args := []any{projectID, string(locale.Normalize(loc))}
	if loc == "" {
		query = `SELECT id, project_id, stream, block_id, profile_id, locale, score, dimensions, findings, checked_at
		 FROM brand_voice_scores WHERE project_id = ? ORDER BY checked_at DESC`
		args = []any{projectID}
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query scores: %w", err)
	}
	defer rows.Close()

	var scores []*corebrand.StoredScore
	for rows.Next() {
		var sc corebrand.StoredScore
		var dimsJSON, findingsJSON, checkedStr string
		if err := rows.Scan(&sc.ID, &sc.ProjectID, &sc.Stream, &sc.BlockID,
			&sc.ProfileID, &sc.Locale, &sc.Score,
			&dimsJSON, &findingsJSON, &checkedStr); err != nil {
			continue
		}
		if err := json.Unmarshal([]byte(dimsJSON), &sc.Dimensions); err != nil {
			return nil, fmt.Errorf("score %s: unmarshal dimensions: %w", sc.ID, err)
		}
		if err := json.Unmarshal([]byte(findingsJSON), &sc.Findings); err != nil {
			return nil, fmt.Errorf("score %s: unmarshal findings: %w", sc.ID, err)
		}
		var perr error
		if sc.CheckedAt, perr = parseStoredTime(checkedStr); perr != nil {
			return nil, fmt.Errorf("score %s: parse checked_at: %w", sc.ID, perr)
		}
		scores = append(scores, &sc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate scores: %w", err)
	}
	return scores, nil
}

func (s *SQLiteBrandStore) GetScoreTrends(ctx context.Context, projectID string, days int) ([]*corebrand.ScoreTrend, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DATE(checked_at) as date, AVG(score) as avg_score, COUNT(*) as count
		 FROM brand_voice_scores
		 WHERE project_id = ? AND checked_at >= DATE('now', '-' || ? || ' days')
		 GROUP BY DATE(checked_at) ORDER BY date`, projectID, days)
	if err != nil {
		return nil, fmt.Errorf("query score trends: %w", err)
	}
	defer rows.Close()

	var trends []*corebrand.ScoreTrend
	for rows.Next() {
		var t corebrand.ScoreTrend
		if err := rows.Scan(&t.Date, &t.AvgScore, &t.Count); err != nil {
			continue
		}
		trends = append(trends, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trends: %w", err)
	}
	return trends, nil
}

func (s *SQLiteBrandStore) StoreCorrection(ctx context.Context, correction *corebrand.Correction) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO brand_voice_corrections (id, profile_id, block_id, dimension, original_text, corrected_text, finding_id, corrected_by, corrected_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		correction.ID, correction.ProfileID, correction.BlockID,
		string(correction.Dimension), correction.OriginalText, correction.CorrectedText,
		correction.FindingID, correction.CorrectedBy,
		correction.CorrectedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert correction: %w", err)
	}
	return nil
}

func (s *SQLiteBrandStore) GetSuggestedRules(ctx context.Context, workspaceID string, minCount int) ([]*corebrand.SuggestedRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.original_text, c.corrected_text, COUNT(*) as cnt, c.dimension
		 FROM brand_voice_corrections c
		 JOIN brand_profiles p ON c.profile_id = p.id
		 WHERE p.workspace_id = ?
		 GROUP BY c.original_text, c.corrected_text, c.dimension
		 HAVING cnt >= ?
		 ORDER BY cnt DESC`, workspaceID, minCount)
	if err != nil {
		return nil, fmt.Errorf("query suggested rules: %w", err)
	}
	defer rows.Close()

	var rules []*corebrand.SuggestedRule
	for rows.Next() {
		var r corebrand.SuggestedRule
		var dim string
		if err := rows.Scan(&r.Term, &r.Replacement, &r.CorrectionCount, &dim); err != nil {
			continue
		}
		r.Dimension = corebrand.Dimension(dim)
		rules = append(rules, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rules: %w", err)
	}
	return rules, nil
}

func (s *SQLiteBrandStore) RecordRuleDecision(ctx context.Context, d *corebrand.RuleDecision) error {
	auto := 0
	if d.Auto {
		auto = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO brand_rule_decisions
		   (profile_id, term, replacement, dimension, status, correction_count, promoted_version, auto, decided_by, decided_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(profile_id, term) DO UPDATE SET
		   replacement = excluded.replacement,
		   dimension = excluded.dimension,
		   status = excluded.status,
		   correction_count = excluded.correction_count,
		   promoted_version = excluded.promoted_version,
		   auto = excluded.auto,
		   decided_by = excluded.decided_by,
		   decided_at = excluded.decided_at`,
		d.ProfileID, d.Term, d.Replacement, string(d.Dimension), string(d.Status),
		d.CorrectionCount, d.PromotedVersion, auto, d.DecidedBy, d.DecidedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("record rule decision: %w", err)
	}
	return nil
}

func (s *SQLiteBrandStore) GetRuleDecision(ctx context.Context, profileID, term string) (*corebrand.RuleDecision, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT profile_id, term, replacement, dimension, status, correction_count, promoted_version, auto, decided_by, decided_at
		 FROM brand_rule_decisions WHERE profile_id = ? AND term = ? COLLATE NOCASE`, profileID, term)
	d, err := scanRuleDecision(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get rule decision: %w", err)
	}
	return d, nil
}

func (s *SQLiteBrandStore) ListRuleDecisions(ctx context.Context, profileID string) ([]*corebrand.RuleDecision, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT profile_id, term, replacement, dimension, status, correction_count, promoted_version, auto, decided_by, decided_at
		 FROM brand_rule_decisions WHERE profile_id = ? ORDER BY decided_at DESC`, profileID)
	if err != nil {
		return nil, fmt.Errorf("list rule decisions: %w", err)
	}
	defer rows.Close()
	var out []*corebrand.RuleDecision
	for rows.Next() {
		d, err := scanRuleDecision(rows)
		if err != nil {
			continue
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rule decisions: %w", err)
	}
	return out, nil
}

// scanner abstracts *sql.Row and *sql.Rows for the shared decision scan.
type scanner interface {
	Scan(dest ...any) error
}

func scanRuleDecision(sc scanner) (*corebrand.RuleDecision, error) {
	var d corebrand.RuleDecision
	var dim, status, decidedAt string
	var auto int
	if err := sc.Scan(&d.ProfileID, &d.Term, &d.Replacement, &dim, &status,
		&d.CorrectionCount, &d.PromotedVersion, &auto, &d.DecidedBy, &decidedAt); err != nil {
		return nil, err
	}
	d.Dimension = corebrand.Dimension(dim)
	d.Status = corebrand.RuleDecisionStatus(status)
	d.Auto = auto != 0
	if t, err := time.Parse(time.RFC3339, decidedAt); err == nil {
		d.DecidedAt = t
	}
	return &d, nil
}

func (s *SQLiteBrandStore) ListProfileVersions(ctx context.Context, profileID string) ([]*corebrand.ProfileVersion, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT profile_id, version, snapshot, note, created_by, created_at
		 FROM brand_profile_versions WHERE profile_id = ? ORDER BY version DESC`, profileID)
	if err != nil {
		return nil, fmt.Errorf("list profile versions: %w", err)
	}
	defer rows.Close()

	var versions []*corebrand.ProfileVersion
	for rows.Next() {
		var v corebrand.ProfileVersion
		var snapshotJSON, createdStr string
		if err := rows.Scan(&v.ProfileID, &v.Version, &snapshotJSON, &v.Note, &v.CreatedBy, &createdStr); err != nil {
			continue
		}
		if err := json.Unmarshal([]byte(snapshotJSON), &v.Snapshot); err != nil {
			return nil, fmt.Errorf("profile %s v%d: unmarshal snapshot: %w", v.ProfileID, v.Version, err)
		}
		var perr error
		if v.CreatedAt, perr = parseStoredTime(createdStr); perr != nil {
			return nil, fmt.Errorf("profile %s v%d: parse created_at: %w", v.ProfileID, v.Version, perr)
		}
		versions = append(versions, &v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate versions: %w", err)
	}
	return versions, nil
}

func (s *SQLiteBrandStore) GetProfileVersion(ctx context.Context, profileID string, version int) (*corebrand.ProfileVersion, error) {
	var v corebrand.ProfileVersion
	var snapshotJSON, createdStr string
	err := s.db.QueryRowContext(ctx,
		`SELECT profile_id, version, snapshot, note, created_by, created_at
		 FROM brand_profile_versions WHERE profile_id = ? AND version = ?`, profileID, version).
		Scan(&v.ProfileID, &v.Version, &snapshotJSON, &v.Note, &v.CreatedBy, &createdStr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("profile version not found: %s v%d", profileID, version)
	}
	if err != nil {
		return nil, fmt.Errorf("get profile version: %w", err)
	}
	if err := json.Unmarshal([]byte(snapshotJSON), &v.Snapshot); err != nil {
		return nil, fmt.Errorf("profile %s v%d: unmarshal snapshot: %w", v.ProfileID, v.Version, err)
	}
	if v.CreatedAt, err = parseStoredTime(createdStr); err != nil {
		return nil, fmt.Errorf("profile %s v%d: parse created_at: %w", v.ProfileID, v.Version, err)
	}
	return &v, nil
}

func (s *SQLiteBrandStore) GetProfileAtTag(ctx context.Context, profileID, tagName string) (*corebrand.VoiceProfile, error) {
	var version int
	err := s.db.QueryRowContext(ctx,
		`SELECT version FROM brand_profile_tags WHERE profile_id = ? AND name = ?`, profileID, tagName).
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

func (s *SQLiteBrandStore) CreateProfileTag(ctx context.Context, tag *corebrand.ProfileTag) error {
	if tag.CreatedAt.IsZero() {
		tag.CreatedAt = time.Now()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO brand_profile_tags (profile_id, name, version, created_by, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		tag.ProfileID, tag.Name, tag.Version, tag.CreatedBy,
		tag.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("create profile tag: %w", err)
	}
	return nil
}

func (s *SQLiteBrandStore) ListProfileTags(ctx context.Context, profileID string) ([]*corebrand.ProfileTag, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT profile_id, name, version, created_by, created_at
		 FROM brand_profile_tags WHERE profile_id = ? ORDER BY name`, profileID)
	if err != nil {
		return nil, fmt.Errorf("list profile tags: %w", err)
	}
	defer rows.Close()

	var tags []*corebrand.ProfileTag
	for rows.Next() {
		var t corebrand.ProfileTag
		var createdStr string
		if err := rows.Scan(&t.ProfileID, &t.Name, &t.Version, &t.CreatedBy, &createdStr); err != nil {
			continue
		}
		var perr error
		if t.CreatedAt, perr = parseStoredTime(createdStr); perr != nil {
			return nil, fmt.Errorf("profile tag %s/%s: parse created_at: %w", t.ProfileID, t.Name, perr)
		}
		tags = append(tags, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tags: %w", err)
	}
	return tags, nil
}

func (s *SQLiteBrandStore) DeleteProfileTag(ctx context.Context, profileID, tagName string) error {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM brand_profile_tags WHERE profile_id = ? AND name = ?`, profileID, tagName)
	if err != nil {
		return fmt.Errorf("delete profile tag: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("profile tag not found: %s/%s", profileID, tagName)
	}
	return nil
}

func (s *SQLiteBrandStore) GetScoresByStream(ctx context.Context, projectID, stream string) ([]*corebrand.StoredScore, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, stream, block_id, profile_id, profile_version, locale, score, dimensions, findings, checked_at
		 FROM brand_voice_scores WHERE project_id = ? AND stream = ? ORDER BY checked_at DESC`, projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("query scores by stream: %w", err)
	}
	defer rows.Close()

	var scores []*corebrand.StoredScore
	for rows.Next() {
		var sc corebrand.StoredScore
		var dimsJSON, findingsJSON, checkedStr string
		if err := rows.Scan(&sc.ID, &sc.ProjectID, &sc.Stream, &sc.BlockID,
			&sc.ProfileID, &sc.ProfileVersion, &sc.Locale, &sc.Score,
			&dimsJSON, &findingsJSON, &checkedStr); err != nil {
			continue
		}
		if err := json.Unmarshal([]byte(dimsJSON), &sc.Dimensions); err != nil {
			return nil, fmt.Errorf("score %s: unmarshal dimensions: %w", sc.ID, err)
		}
		if err := json.Unmarshal([]byte(findingsJSON), &sc.Findings); err != nil {
			return nil, fmt.Errorf("score %s: unmarshal findings: %w", sc.ID, err)
		}
		var perr error
		if sc.CheckedAt, perr = parseStoredTime(checkedStr); perr != nil {
			return nil, fmt.Errorf("score %s: parse checked_at: %w", sc.ID, perr)
		}
		scores = append(scores, &sc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate scores: %w", err)
	}
	return scores, nil
}

func (s *SQLiteBrandStore) Close() error {
	return s.db.Close()
}

// parseStoredTime parses an RFC3339 timestamp read back from one of our own
// rows. An empty string (a NULL or never-set column) is not an error — it
// yields the zero time; a non-empty but unparseable value is stored corruption
// and is returned so the caller can surface it rather than silently substitute
// the zero time.
func parseStoredTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}
