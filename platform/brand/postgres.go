package brand

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/bowrain/storage"
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

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO brand_profiles (id, workspace_id, name, description, tone, style, vocabulary, examples, locales, channels, version, created_at, updated_at, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		profile.ID, profile.WorkspaceID, profile.Name, profile.Description,
		string(tone), string(style), string(vocab), string(examples),
		string(locales), string(channels),
		profile.Version, now, now, profile.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert brand profile: %w", err)
	}
	return nil
}

func (s *PostgresBrandStore) GetProfile(ctx context.Context, profileID string) (*corebrand.VoiceProfile, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, name, description, tone, style, vocabulary, examples, locales, channels, version, created_at, updated_at, created_by
		 FROM brand_profiles WHERE id = $1`, profileID)
	return scanProfile(row)
}

func (s *PostgresBrandStore) UpdateProfile(ctx context.Context, profile *corebrand.VoiceProfile) error {
	now := time.Now().UTC()
	profile.UpdatedAt = now

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

	res, err := s.db.ExecContext(ctx,
		`UPDATE brand_profiles
		 SET name=$1, description=$2, tone=$3, style=$4, vocabulary=$5, examples=$6,
		     locales=$7, channels=$8, version=version+1, updated_at=$9
		 WHERE id=$10`,
		profile.Name, profile.Description,
		string(tone), string(style), string(vocab), string(examples),
		string(locales), string(channels),
		now, profile.ID)
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
		`SELECT id, workspace_id, name, description, tone, style, vocabulary, examples, locales, channels, version, created_at, updated_at, created_by
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
		`INSERT INTO brand_voice_scores (id, project_id, stream, block_id, profile_id, locale, score, dimensions, findings, checked_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		score.ID, score.ProjectID, stream, score.BlockID, score.ProfileID,
		score.Locale, score.Score, string(dims), string(findings), score.CheckedAt)
	if err != nil {
		return fmt.Errorf("insert brand voice score: %w", err)
	}
	return nil
}

func (s *PostgresBrandStore) GetScores(ctx context.Context, projectID, locale string) ([]*corebrand.StoredScore, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, stream, block_id, profile_id, locale, score, dimensions, findings, checked_at
		 FROM brand_voice_scores WHERE project_id = $1 AND locale = $2
		 ORDER BY checked_at DESC`, projectID, locale)
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
	return result, rows.Err()
}

// ---------------------------------------------------------------------------
// Scan helpers
// ---------------------------------------------------------------------------

type scanner interface {
	Scan(dest ...any) error
}

func scanProfile(row scanner) (*corebrand.VoiceProfile, error) {
	var p corebrand.VoiceProfile
	var toneJSON, styleJSON, vocabJSON, examplesJSON, localesJSON, channelsJSON string

	err := row.Scan(
		&p.ID, &p.WorkspaceID, &p.Name, &p.Description,
		&toneJSON, &styleJSON, &vocabJSON, &examplesJSON,
		&localesJSON, &channelsJSON,
		&p.Version, &p.CreatedAt, &p.UpdatedAt, &p.CreatedBy)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("brand profile not found")
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
		p.Locales = map[string]corebrand.LocaleOverride{}
	}
	if err := json.Unmarshal([]byte(channelsJSON), &p.Channels); err != nil {
		p.Channels = map[string]corebrand.ChannelOverride{}
	}

	return &p, nil
}

func scanScore(row scanner) (*corebrand.StoredScore, error) {
	var sc corebrand.StoredScore
	var dimsJSON, findingsJSON string

	err := row.Scan(
		&sc.ID, &sc.ProjectID, &sc.Stream, &sc.BlockID, &sc.ProfileID,
		&sc.Locale, &sc.Score, &dimsJSON, &findingsJSON, &sc.CheckedAt)
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
