package brand

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/storage"
)

// SQLiteBrandStore implements brand.BrandStore using SQLite.
type SQLiteBrandStore struct {
	db *storage.DB
}

var migrations = []storage.Migration{
	{
		Version:     1,
		Description: "brand voice store schema",
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
		`,
	},
	{
		Version:     2,
		Description: "profile versioning, tags, and score profile_version",
		SQL: `
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

		ALTER TABLE brand_voice_scores ADD COLUMN profile_version INTEGER NOT NULL DEFAULT 0;
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

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO brand_profiles (id, workspace_id, name, description, tone, style, vocabulary, examples, locales, channels, version, created_at, updated_at, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		profile.ID, profile.WorkspaceID, profile.Name, profile.Description,
		string(tone), string(style), string(vocab), string(examples),
		string(locales), string(channels), profile.Version,
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
	var toneJSON, styleJSON, vocabJSON, examplesJSON, localesJSON, channelsJSON string
	var createdStr, updatedStr string

	err := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, name, description, tone, style, vocabulary, examples, locales, channels, version, created_at, updated_at, created_by
		 FROM brand_profiles WHERE id = ?`, id).
		Scan(&p.ID, &p.WorkspaceID, &p.Name, &desc,
			&toneJSON, &styleJSON, &vocabJSON, &examplesJSON,
			&localesJSON, &channelsJSON, &p.Version,
			&createdStr, &updatedStr, &p.CreatedBy)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("profile not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get profile: %w", err)
	}
	if desc != nil {
		p.Description = *desc
	}
	_ = json.Unmarshal([]byte(toneJSON), &p.Tone)
	_ = json.Unmarshal([]byte(styleJSON), &p.Style)
	_ = json.Unmarshal([]byte(vocabJSON), &p.Vocabulary)
	_ = json.Unmarshal([]byte(examplesJSON), &p.Examples)
	_ = json.Unmarshal([]byte(localesJSON), &p.Locales)
	_ = json.Unmarshal([]byte(channelsJSON), &p.Channels)
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
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
		existing.VersionNote, existing.CreatedBy, now.Format(time.RFC3339))

	profile.UpdatedAt = now
	profile.Version = existing.Version + 1
	tone, _ := json.Marshal(profile.Tone)
	style, _ := json.Marshal(profile.Style)
	vocab, _ := json.Marshal(profile.Vocabulary)
	examples, _ := json.Marshal(profile.Examples)
	locales, _ := json.Marshal(profile.Locales)
	channels, _ := json.Marshal(profile.Channels)

	result, err := s.db.ExecContext(ctx,
		`UPDATE brand_profiles SET name = ?, description = ?, tone = ?, style = ?, vocabulary = ?, examples = ?, locales = ?, channels = ?, version = ?, updated_at = ?
		 WHERE id = ?`,
		profile.Name, profile.Description,
		string(tone), string(style), string(vocab), string(examples),
		string(locales), string(channels), profile.Version,
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
		score.ProfileID, score.ProfileVersion, score.Locale, score.Score,
		string(dims), string(findings),
		score.CheckedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert score: %w", err)
	}
	return nil
}

func (s *SQLiteBrandStore) GetScores(ctx context.Context, projectID, locale string) ([]*corebrand.StoredScore, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, stream, block_id, profile_id, locale, score, dimensions, findings, checked_at
		 FROM brand_voice_scores WHERE project_id = ? AND locale = ? ORDER BY checked_at DESC`, projectID, locale)
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
		_ = json.Unmarshal([]byte(dimsJSON), &sc.Dimensions)
		_ = json.Unmarshal([]byte(findingsJSON), &sc.Findings)
		sc.CheckedAt, _ = time.Parse(time.RFC3339, checkedStr)
		scores = append(scores, &sc)
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
	return rules, nil
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
		_ = json.Unmarshal([]byte(snapshotJSON), &v.Snapshot)
		v.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		versions = append(versions, &v)
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
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("profile version not found: %s v%d", profileID, version)
	}
	if err != nil {
		return nil, fmt.Errorf("get profile version: %w", err)
	}
	_ = json.Unmarshal([]byte(snapshotJSON), &v.Snapshot)
	v.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	return &v, nil
}

func (s *SQLiteBrandStore) GetProfileAtTag(ctx context.Context, profileID, tagName string) (*corebrand.VoiceProfile, error) {
	var version int
	err := s.db.QueryRowContext(ctx,
		`SELECT version FROM brand_profile_tags WHERE profile_id = ? AND name = ?`, profileID, tagName).
		Scan(&version)
	if err == sql.ErrNoRows {
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
		t.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		tags = append(tags, &t)
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
		_ = json.Unmarshal([]byte(dimsJSON), &sc.Dimensions)
		_ = json.Unmarshal([]byte(findingsJSON), &sc.Findings)
		sc.CheckedAt, _ = time.Parse(time.RFC3339, checkedStr)
		scores = append(scores, &sc)
	}
	return scores, nil
}

func (s *SQLiteBrandStore) Close() error {
	return s.db.Close()
}
