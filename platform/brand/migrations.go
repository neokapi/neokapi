package brand

import "github.com/neokapi/neokapi/bowrain/storage"

var brandMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "create brand voice tables",
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
				version      INTEGER NOT NULL DEFAULT 1,
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				created_by   TEXT NOT NULL DEFAULT '',
				UNIQUE (workspace_id, name)
			);
			CREATE INDEX idx_brand_profiles_workspace ON brand_profiles(workspace_id);

			CREATE TABLE brand_voice_scores (
				id         TEXT PRIMARY KEY,
				project_id TEXT NOT NULL,
				stream     TEXT NOT NULL DEFAULT 'main',
				block_id   TEXT NOT NULL,
				profile_id TEXT NOT NULL,
				locale     TEXT NOT NULL,
				score      INTEGER NOT NULL,
				dimensions JSONB NOT NULL,
				findings   JSONB NOT NULL,
				checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
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
		`,
	},
}
