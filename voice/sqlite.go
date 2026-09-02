package voice

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/storage"
)

// SQLiteStore implements profile.Store using SQLite.
type SQLiteStore struct {
	db *storage.DB
}

// migrations holds the voice profile store's schema. Version 1 is the launch
// baseline; user machines carry long-lived local databases, so every schema
// change after it MUST be an incremental migration — existing databases only
// ever run new versions, never a re-run of the baseline. Table names are the
// on-disk shape of those databases and stay as written.
var migrations = []storage.Migration{
	{
		Version:     1,
		Description: "voice profile store schema (baseline)",
		SQL: `
		CREATE TABLE IF NOT EXISTS voice_profiles (
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

		CREATE TABLE IF NOT EXISTS voice_scores (
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

		CREATE TABLE IF NOT EXISTS voice_corrections (
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

		CREATE TABLE IF NOT EXISTS voice_profile_versions (
			profile_id TEXT NOT NULL,
			version INTEGER NOT NULL,
			snapshot TEXT NOT NULL,
			note TEXT NOT NULL DEFAULT '',
			created_by TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			PRIMARY KEY (profile_id, version)
		);

		CREATE TABLE IF NOT EXISTS voice_profile_tags (
			profile_id TEXT NOT NULL,
			name TEXT NOT NULL,
			version INTEGER NOT NULL,
			created_by TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			PRIMARY KEY (profile_id, name)
		);

		CREATE TABLE IF NOT EXISTS voice_rule_decisions (
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
		Description: "author personas on voice profiles",
		SQL: `
		ALTER TABLE voice_profiles ADD COLUMN personas TEXT NOT NULL DEFAULT '{}';
		`,
	},
	{
		Version:     3,
		Description: "the profile's own compliance bar",
		// min_score is the score a block must reach to count as compliant: 0
		// means the default (core/profile.DefaultMinScore), which is what every
		// profile in an existing database answered while the column was absent.
		// The ship gate and bulk approve-passing read it through
		// VoiceProfile.ComplianceBar.
		SQL: `
		ALTER TABLE voice_profiles ADD COLUMN min_score INTEGER NOT NULL DEFAULT 0;
		`,
	},
}

// profileColumns is the one column list every profile statement is spelled
// from, in the order a profile row is written and read back. Spelled once
// because a column present in the table and missing from a statement is
// invisible: the row still scans and the field just comes back zero. That is
// exactly how min_score reached the model, the validator and the wire while
// this store dropped it on every write.
const profileColumns = `id, workspace_id, name, description, tone, style, vocabulary, examples, ` +
	`locales, channels, personas, autonomy, min_score, version, created_at, updated_at, created_by`

// profileFixedColumns are the facts an edit never rewrites — identity and
// creation. Everything else in profileColumns is editable, so a column added to
// the list reaches the UPDATE by default rather than by being remembered in a
// second list.
var profileFixedColumns = []string{"id", "workspace_id", "created_at", "created_by"}

var (
	// profileValues is one placeholder per column in profileColumns, so an
	// INSERT cannot go out of step with the list it inserts into: a column added
	// without its value is an argument-count error rather than a silent zero.
	profileValues = strings.TrimPrefix(strings.Repeat(", ?", len(columnNames(profileColumns))), ", ")
	// profileAssignments renders the editable columns as an UPDATE SET list, for
	// the same reason and in the same order.
	profileAssignments = assignmentList(editableColumns())
)

// columnNames splits a comma-separated column list into its trimmed names.
func columnNames(list string) []string {
	names := strings.Split(list, ",")
	for i, n := range names {
		names[i] = strings.TrimSpace(n)
	}
	return names
}

// editableColumns is profileColumns without the fixed ones, in profileColumns
// order.
func editableColumns() []string {
	var out []string
	for _, name := range columnNames(profileColumns) {
		if !slices.Contains(profileFixedColumns, name) {
			out = append(out, name)
		}
	}
	return out
}

// assignmentList renders column names as an UPDATE SET clause.
func assignmentList(names []string) string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = n + " = ?"
	}
	return strings.Join(out, ", ")
}

// NewSQLiteStore creates a new SQLite-backed voice profile store.
func NewSQLiteStore(db *storage.DB) (*SQLiteStore, error) {
	// The ledger name is "brand": it keys the applied-version bookkeeping in
	// databases users already carry, and renaming it replays every migration.
	if err := storage.Migrate(db, "brand", migrations); err != nil {
		return nil, fmt.Errorf("voice store migration: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) CreateProfile(ctx context.Context, profile *coreprofile.VoiceProfile) error {
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
		`INSERT INTO voice_profiles (`+profileColumns+`)
		 VALUES (`+profileValues+`)`,
		profile.ID, profile.Scope, profile.Name, profile.Description,
		string(tone), string(style), string(vocab), string(examples),
		string(locales), string(channels), string(personas), string(autonomy),
		profile.MinScore, profile.Version,
		profile.CreatedAt.Format(time.RFC3339), profile.UpdatedAt.Format(time.RFC3339),
		profile.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert profile: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetProfile(ctx context.Context, id string) (*coreprofile.VoiceProfile, error) {
	var p coreprofile.VoiceProfile
	var desc *string
	var toneJSON, styleJSON, vocabJSON, examplesJSON, localesJSON, channelsJSON, personasJSON, autonomyJSON string
	var createdStr, updatedStr string

	err := s.db.QueryRowContext(ctx,
		`SELECT `+profileColumns+`
		 FROM voice_profiles WHERE id = ?`, id).
		Scan(&p.ID, &p.Scope, &p.Name, &desc,
			&toneJSON, &styleJSON, &vocabJSON, &examplesJSON,
			&localesJSON, &channelsJSON, &personasJSON, &autonomyJSON,
			&p.MinScore, &p.Version,
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
	if p.CreatedAt, err = parseStoredTime(createdStr); err != nil {
		return nil, fmt.Errorf("profile %s: parse created_at: %w", p.ID, err)
	}
	if p.UpdatedAt, err = parseStoredTime(updatedStr); err != nil {
		return nil, fmt.Errorf("profile %s: parse updated_at: %w", p.ID, err)
	}
	return &p, nil
}

func (s *SQLiteStore) UpdateProfile(ctx context.Context, profile *coreprofile.VoiceProfile) error {
	// Archive the current state as an immutable ProfileVersion before applying the edit.
	existing, err := s.GetProfile(ctx, profile.ID)
	if err != nil {
		return fmt.Errorf("get existing profile for versioning: %w", err)
	}

	snapshotJSON, _ := json.Marshal(existing)
	now := time.Now()
	_, _ = s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO voice_profile_versions (profile_id, version, snapshot, note, created_by, created_at)
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
		`UPDATE voice_profiles SET `+profileAssignments+`
		 WHERE id = ?`,
		profile.Name, profile.Description,
		string(tone), string(style), string(vocab), string(examples),
		string(locales), string(channels), string(personas), string(autonomy),
		profile.MinScore, profile.Version,
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

func (s *SQLiteStore) DeleteProfile(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM voice_profiles WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete profile: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("profile not found: %s", id)
	}
	return nil
}

func (s *SQLiteStore) ListProfiles(ctx context.Context, scope string) ([]*coreprofile.VoiceProfile, error) {
	// Collect IDs first, then close the cursor before querying individual profiles.
	// SQLite :memory: databases use a single connection, so a nested query
	// while rows are open would deadlock.
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM voice_profiles WHERE workspace_id = ? ORDER BY name`, scope)
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

	var profiles []*coreprofile.VoiceProfile
	for _, id := range ids {
		p, err := s.GetProfile(ctx, id)
		if err == nil {
			profiles = append(profiles, p)
		}
	}
	return profiles, nil
}

func (s *SQLiteStore) StoreScore(ctx context.Context, score *coreprofile.StoredScore) error {
	dims, _ := json.Marshal(score.Dimensions)
	findings, _ := json.Marshal(score.Findings)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO voice_scores (id, project_id, stream, block_id, profile_id, profile_version, locale, score, dimensions, findings, checked_at)
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
// voice rollup use — not "rows stored with an empty locale".
func (s *SQLiteStore) GetScores(ctx context.Context, projectID string, loc model.LocaleID) ([]*coreprofile.StoredScore, error) {
	query := `SELECT id, project_id, stream, block_id, profile_id, locale, score, dimensions, findings, checked_at
		 FROM voice_scores WHERE project_id = ? AND locale = ? ORDER BY checked_at DESC`
	args := []any{projectID, string(locale.Normalize(loc))}
	if loc == "" {
		query = `SELECT id, project_id, stream, block_id, profile_id, locale, score, dimensions, findings, checked_at
		 FROM voice_scores WHERE project_id = ? ORDER BY checked_at DESC`
		args = []any{projectID}
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query scores: %w", err)
	}
	defer rows.Close()

	var scores []*coreprofile.StoredScore
	for rows.Next() {
		var sc coreprofile.StoredScore
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

func (s *SQLiteStore) GetScoreTrends(ctx context.Context, projectID string, days int) ([]*coreprofile.ScoreTrend, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DATE(checked_at) as date, AVG(score) as avg_score, COUNT(*) as count
		 FROM voice_scores
		 WHERE project_id = ? AND checked_at >= DATE('now', '-' || ? || ' days')
		 GROUP BY DATE(checked_at) ORDER BY date`, projectID, days)
	if err != nil {
		return nil, fmt.Errorf("query score trends: %w", err)
	}
	defer rows.Close()

	var trends []*coreprofile.ScoreTrend
	for rows.Next() {
		var t coreprofile.ScoreTrend
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

func (s *SQLiteStore) StoreCorrection(ctx context.Context, correction *coreprofile.Correction) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO voice_corrections (id, profile_id, block_id, dimension, original_text, corrected_text, finding_id, corrected_by, corrected_at)
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

func (s *SQLiteStore) GetSuggestedRules(ctx context.Context, scope string, minCount int) ([]*coreprofile.SuggestedRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.original_text, c.corrected_text, COUNT(*) as cnt, c.dimension
		 FROM voice_corrections c
		 JOIN voice_profiles p ON c.profile_id = p.id
		 WHERE p.workspace_id = ?
		 GROUP BY c.original_text, c.corrected_text, c.dimension
		 HAVING cnt >= ?
		 ORDER BY cnt DESC`, scope, minCount)
	if err != nil {
		return nil, fmt.Errorf("query suggested rules: %w", err)
	}
	defer rows.Close()

	var rules []*coreprofile.SuggestedRule
	for rows.Next() {
		var r coreprofile.SuggestedRule
		var dim string
		if err := rows.Scan(&r.Term, &r.Replacement, &r.CorrectionCount, &dim); err != nil {
			continue
		}
		r.Dimension = coreprofile.Dimension(dim)
		rules = append(rules, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rules: %w", err)
	}
	return rules, nil
}

func (s *SQLiteStore) RecordRuleDecision(ctx context.Context, d *coreprofile.RuleDecision) error {
	auto := 0
	if d.Auto {
		auto = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO voice_rule_decisions
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

func (s *SQLiteStore) GetRuleDecision(ctx context.Context, profileID, term string) (*coreprofile.RuleDecision, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT profile_id, term, replacement, dimension, status, correction_count, promoted_version, auto, decided_by, decided_at
		 FROM voice_rule_decisions WHERE profile_id = ? AND term = ? COLLATE NOCASE`, profileID, term)
	d, err := scanRuleDecision(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get rule decision: %w", err)
	}
	return d, nil
}

func (s *SQLiteStore) ListRuleDecisions(ctx context.Context, profileID string) ([]*coreprofile.RuleDecision, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT profile_id, term, replacement, dimension, status, correction_count, promoted_version, auto, decided_by, decided_at
		 FROM voice_rule_decisions WHERE profile_id = ? ORDER BY decided_at DESC`, profileID)
	if err != nil {
		return nil, fmt.Errorf("list rule decisions: %w", err)
	}
	defer rows.Close()
	var out []*coreprofile.RuleDecision
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

func scanRuleDecision(sc scanner) (*coreprofile.RuleDecision, error) {
	var d coreprofile.RuleDecision
	var dim, status, decidedAt string
	var auto int
	if err := sc.Scan(&d.ProfileID, &d.Term, &d.Replacement, &dim, &status,
		&d.CorrectionCount, &d.PromotedVersion, &auto, &d.DecidedBy, &decidedAt); err != nil {
		return nil, err
	}
	d.Dimension = coreprofile.Dimension(dim)
	d.Status = coreprofile.RuleDecisionStatus(status)
	d.Auto = auto != 0
	if t, err := time.Parse(time.RFC3339, decidedAt); err == nil {
		d.DecidedAt = t
	}
	return &d, nil
}

func (s *SQLiteStore) ListProfileVersions(ctx context.Context, profileID string) ([]*coreprofile.ProfileVersion, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT profile_id, version, snapshot, note, created_by, created_at
		 FROM voice_profile_versions WHERE profile_id = ? ORDER BY version DESC`, profileID)
	if err != nil {
		return nil, fmt.Errorf("list profile versions: %w", err)
	}
	defer rows.Close()

	var versions []*coreprofile.ProfileVersion
	for rows.Next() {
		var v coreprofile.ProfileVersion
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

func (s *SQLiteStore) GetProfileVersion(ctx context.Context, profileID string, version int) (*coreprofile.ProfileVersion, error) {
	var v coreprofile.ProfileVersion
	var snapshotJSON, createdStr string
	err := s.db.QueryRowContext(ctx,
		`SELECT profile_id, version, snapshot, note, created_by, created_at
		 FROM voice_profile_versions WHERE profile_id = ? AND version = ?`, profileID, version).
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

func (s *SQLiteStore) GetProfileAtTag(ctx context.Context, profileID, tagName string) (*coreprofile.VoiceProfile, error) {
	var version int
	err := s.db.QueryRowContext(ctx,
		`SELECT version FROM voice_profile_tags WHERE profile_id = ? AND name = ?`, profileID, tagName).
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

func (s *SQLiteStore) CreateProfileTag(ctx context.Context, tag *coreprofile.ProfileTag) error {
	if tag.CreatedAt.IsZero() {
		tag.CreatedAt = time.Now()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO voice_profile_tags (profile_id, name, version, created_by, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		tag.ProfileID, tag.Name, tag.Version, tag.CreatedBy,
		tag.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("create profile tag: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListProfileTags(ctx context.Context, profileID string) ([]*coreprofile.ProfileTag, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT profile_id, name, version, created_by, created_at
		 FROM voice_profile_tags WHERE profile_id = ? ORDER BY name`, profileID)
	if err != nil {
		return nil, fmt.Errorf("list profile tags: %w", err)
	}
	defer rows.Close()

	var tags []*coreprofile.ProfileTag
	for rows.Next() {
		var t coreprofile.ProfileTag
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

func (s *SQLiteStore) DeleteProfileTag(ctx context.Context, profileID, tagName string) error {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM voice_profile_tags WHERE profile_id = ? AND name = ?`, profileID, tagName)
	if err != nil {
		return fmt.Errorf("delete profile tag: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("profile tag not found: %s/%s", profileID, tagName)
	}
	return nil
}

// GetBlockScore implements coreprofile.BlockScoreReader. The locale is
// normalized to match how StoreScore writes it.
func (s *SQLiteStore) GetBlockScore(ctx context.Context, projectID, stream, blockID string, loc model.LocaleID) (*coreprofile.StoredScore, error) {
	if stream == "" {
		stream = "main"
	}
	var sc coreprofile.StoredScore
	var dimsJSON, findingsJSON, checkedStr string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, stream, block_id, profile_id, profile_version, locale, score, dimensions, findings, checked_at
		 FROM voice_scores
		 WHERE project_id = ? AND stream = ? AND block_id = ? AND locale = ?
		 ORDER BY checked_at DESC LIMIT 1`,
		projectID, stream, blockID, string(locale.Normalize(loc))).
		Scan(&sc.ID, &sc.ProjectID, &sc.Stream, &sc.BlockID, &sc.ProfileID,
			&sc.ProfileVersion, &sc.Locale, &sc.Score, &dimsJSON, &findingsJSON, &checkedStr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get block score: %w", err)
	}
	if err := json.Unmarshal([]byte(dimsJSON), &sc.Dimensions); err != nil {
		return nil, fmt.Errorf("score %s: unmarshal dimensions: %w", sc.ID, err)
	}
	if err := json.Unmarshal([]byte(findingsJSON), &sc.Findings); err != nil {
		return nil, fmt.Errorf("score %s: unmarshal findings: %w", sc.ID, err)
	}
	if sc.CheckedAt, err = parseStoredTime(checkedStr); err != nil {
		return nil, fmt.Errorf("score %s: parse checked_at: %w", sc.ID, err)
	}
	return &sc, nil
}

func (s *SQLiteStore) GetScoresByStream(ctx context.Context, projectID, stream string) ([]*coreprofile.StoredScore, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, stream, block_id, profile_id, profile_version, locale, score, dimensions, findings, checked_at
		 FROM voice_scores WHERE project_id = ? AND stream = ? ORDER BY checked_at DESC`, projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("query scores by stream: %w", err)
	}
	defer rows.Close()

	var scores []*coreprofile.StoredScore
	for rows.Next() {
		var sc coreprofile.StoredScore
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

func (s *SQLiteStore) Close() error {
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
