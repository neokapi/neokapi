package brand

import "github.com/neokapi/neokapi/bowrain/storage"

// brandMigrations holds the brand-voice schema. Version 1 is the launch
// baseline; the platform is live now, so every schema change after it MUST be
// an incremental migration — existing databases only ever run new versions,
// never a re-run of the baseline.
var brandMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "brand voice schema (baseline)",
		SQL: `
			CREATE TABLE brand_profiles (
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
				version      INTEGER NOT NULL DEFAULT 1,
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				created_by   TEXT NOT NULL DEFAULT '',
				UNIQUE (workspace_id, name)
			);
			CREATE INDEX idx_brand_profiles_workspace ON brand_profiles(workspace_id);

			CREATE TABLE brand_profile_versions (
				profile_id TEXT NOT NULL,
				version    INTEGER NOT NULL,
				snapshot   JSONB NOT NULL,
				note       TEXT NOT NULL DEFAULT '',
				created_by TEXT NOT NULL DEFAULT '',
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (profile_id, version)
			);

			CREATE TABLE brand_profile_tags (
				profile_id TEXT NOT NULL,
				name       TEXT NOT NULL,
				version    INTEGER NOT NULL,
				created_by TEXT NOT NULL DEFAULT '',
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (profile_id, name)
			);

			CREATE TABLE brand_voice_scores (
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
			CREATE INDEX idx_bvs_project_stream ON brand_voice_scores(project_id, stream);
			CREATE INDEX idx_bvs_profile_score ON brand_voice_scores(profile_id, score);
			CREATE INDEX idx_bvs_project_locale ON brand_voice_scores(project_id, locale);

			CREATE TABLE brand_voice_corrections (
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
			CREATE INDEX idx_bvc_profile_dim ON brand_voice_corrections(profile_id, dimension);

			CREATE TABLE brand_rule_decisions (
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
	{
		Version:     2,
		Description: "author personas on brand profiles",
		SQL: `
			ALTER TABLE brand_profiles
				ADD COLUMN IF NOT EXISTS personas JSONB NOT NULL DEFAULT '{}';
		`,
	},
}
