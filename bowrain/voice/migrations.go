package voice

import "github.com/neokapi/neokapi/bowrain/storage"

// Migrations is the brand-voice schema as a single consolidated baseline.
//
// LEDGER — every version this subsystem has ever issued, now folded in:
//
//	1  brand voice schema (baseline)
//	2  author personas on brand profiles
//	3  brand voice baseline (folds 1-2)
//
// The columns later versions added by ALTER (personas, min_score) are declared
// in the CREATE below as well: the CREATE serves an empty database, the ALTER
// serves one that already has the table, and both are idempotent.
//
// Baseline is version 4 — above every number issued, so an existing database
// applies it once and any drift between its schema and its bookkeeping is
// repaired. Because the baseline is idempotent and re-applied by design, the
// old warning that changes "MUST be incremental, never a re-run of the
// baseline" no longer holds: a re-run is now the mechanism, not the hazard.
//
// Retired numbers are never reused; the next migration is version 5. Version 4
// added voice_profiles.min_score — the profile's own on-brand bar, which the
// API accepted and the store then dropped, pinning every workspace profile to
// the default bar no matter what was authored.
var Migrations = []storage.Migration{
	{
		Version:     4,
		Description: "brand voice baseline (folds 1-3) + the profile's own on-brand bar",
		SQL: `
			CREATE TABLE IF NOT EXISTS voice_profiles (
				id           TEXT PRIMARY KEY,
				workspace_id TEXT NOT NULL,
				name         TEXT NOT NULL,
				description  TEXT NOT NULL DEFAULT '',
				tone         JSONB NOT NULL DEFAULT '{}',
				style        JSONB NOT NULL DEFAULT '{}',
				vocabulary   JSONB NOT NULL DEFAULT '{}',
				examples     JSONB NOT NULL DEFAULT '[]',
				locales      JSONB NOT NULL DEFAULT '{}',
				channels     JSONB NOT NULL DEFAULT '{}',
				autonomy     JSONB NOT NULL DEFAULT '{}',
				personas     JSONB NOT NULL DEFAULT '{}',
				-- The profile's own on-brand bar (0-100); 0 means the default
				-- (core/profile.DefaultMinScore). The ship gate and bulk
				-- approve-passing read it through VoiceProfile.ComplianceBar.
				min_score    INTEGER NOT NULL DEFAULT 0,
				version      INTEGER NOT NULL DEFAULT 1,
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				created_by   TEXT NOT NULL DEFAULT '',
				UNIQUE (workspace_id, name)
			);
			CREATE INDEX IF NOT EXISTS idx_voice_profiles_workspace ON voice_profiles(workspace_id);
			-- The CREATE above serves an empty database; this serves one that
			-- already has the table, where CREATE ... IF NOT EXISTS is a no-op
			-- and would leave the new column missing. Both are idempotent.
			ALTER TABLE voice_profiles ADD COLUMN IF NOT EXISTS min_score INTEGER NOT NULL DEFAULT 0;

			CREATE TABLE IF NOT EXISTS voice_profile_versions (
				profile_id TEXT NOT NULL,
				version    INTEGER NOT NULL,
				snapshot   JSONB NOT NULL,
				note       TEXT NOT NULL DEFAULT '',
				created_by TEXT NOT NULL DEFAULT '',
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (profile_id, version)
			);

			CREATE TABLE IF NOT EXISTS voice_profile_tags (
				profile_id TEXT NOT NULL,
				name       TEXT NOT NULL,
				version    INTEGER NOT NULL,
				created_by TEXT NOT NULL DEFAULT '',
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (profile_id, name)
			);

			CREATE TABLE IF NOT EXISTS voice_scores (
				id              TEXT PRIMARY KEY,
				project_id      TEXT NOT NULL,
				stream          TEXT NOT NULL DEFAULT 'main',
				block_id        TEXT NOT NULL,
				profile_id      TEXT NOT NULL,
				profile_version INTEGER NOT NULL DEFAULT 0,
				locale          TEXT NOT NULL,
				score           INTEGER NOT NULL,
				dimensions      JSONB NOT NULL,
				findings        JSONB NOT NULL,
				checked_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_bvs_project_stream ON voice_scores(project_id, stream);
			CREATE INDEX IF NOT EXISTS idx_bvs_profile_score ON voice_scores(profile_id, score);
			CREATE INDEX IF NOT EXISTS idx_bvs_project_locale ON voice_scores(project_id, locale);

			CREATE TABLE IF NOT EXISTS voice_corrections (
				id             TEXT PRIMARY KEY,
				profile_id     TEXT NOT NULL,
				block_id       TEXT NOT NULL,
				dimension      TEXT NOT NULL,
				original_text  TEXT NOT NULL,
				corrected_text TEXT NOT NULL,
				finding_id     TEXT NOT NULL DEFAULT '',
				corrected_by   TEXT NOT NULL,
				corrected_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_bvc_profile_dim ON voice_corrections(profile_id, dimension);

			CREATE TABLE IF NOT EXISTS voice_rule_decisions (
				profile_id       TEXT NOT NULL,
				term             TEXT NOT NULL,
				replacement      TEXT NOT NULL DEFAULT '',
				dimension        TEXT NOT NULL DEFAULT '',
				status           TEXT NOT NULL,
				correction_count INTEGER NOT NULL DEFAULT 0,
				promoted_version INTEGER NOT NULL DEFAULT 0,
				auto             BOOLEAN NOT NULL DEFAULT FALSE,
				concept_id       TEXT NOT NULL DEFAULT '',
				decided_by       TEXT NOT NULL DEFAULT '',
				decided_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (profile_id, term)
			);
		`,
	},
}
