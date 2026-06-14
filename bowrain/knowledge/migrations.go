package knowledge

import "github.com/neokapi/neokapi/bowrain/storage"

// kgMigrations defines the complete knowledge-graph governance schema as a
// single baseline. The platform is pre-launch with no databases to preserve, so
// the schema is one clean definition (all seven tables from the data-model note)
// rather than an incremental migration history. Every workspace-scoped table is
// keyed by (workspace_id, …); timestamps are TIMESTAMPTZ; snapshot, payload, and
// locales are JSONB.
var kgMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "knowledge graph governance schema (baseline)",
		SQL: `
			CREATE TABLE kg_markets (
				workspace_id TEXT NOT NULL,
				id           TEXT NOT NULL,
				name         TEXT NOT NULL,
				description  TEXT NOT NULL DEFAULT '',
				locales      JSONB NOT NULL DEFAULT '[]',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (workspace_id, id)
			);

			CREATE TABLE kg_observations (
				workspace_id TEXT NOT NULL,
				id           TEXT NOT NULL,
				concept_id   TEXT NOT NULL,
				kind         TEXT NOT NULL,
				quote        TEXT NOT NULL DEFAULT '',
				source       TEXT NOT NULL DEFAULT '',
				url          TEXT NOT NULL DEFAULT '',
				locale       TEXT NOT NULL DEFAULT '',
				market       TEXT NOT NULL DEFAULT '',
				note         TEXT NOT NULL DEFAULT '',
				created_by   TEXT NOT NULL DEFAULT '',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (workspace_id, id)
			);
			CREATE INDEX idx_kg_observations_concept ON kg_observations(workspace_id, concept_id);

			CREATE TABLE kg_comments (
				workspace_id TEXT NOT NULL,
				id           TEXT NOT NULL,
				concept_id   TEXT NOT NULL,
				parent_id    TEXT NOT NULL DEFAULT '',
				changeset_id TEXT NOT NULL DEFAULT '',
				body         TEXT NOT NULL DEFAULT '',
				author       TEXT NOT NULL DEFAULT '',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				resolved     BOOLEAN NOT NULL DEFAULT FALSE,
				PRIMARY KEY (workspace_id, id)
			);
			CREATE INDEX idx_kg_comments_concept ON kg_comments(workspace_id, concept_id);
			CREATE INDEX idx_kg_comments_changeset ON kg_comments(workspace_id, changeset_id);

			CREATE TABLE kg_concept_revisions (
				workspace_id TEXT NOT NULL,
				concept_id   TEXT NOT NULL,
				rev          BIGINT NOT NULL,
				snapshot     JSONB NOT NULL DEFAULT 'null',
				summary      TEXT NOT NULL DEFAULT '',
				actor        TEXT NOT NULL DEFAULT '',
				changeset_id TEXT NOT NULL DEFAULT '',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (workspace_id, concept_id, rev)
			);

			CREATE TABLE kg_changesets (
				workspace_id TEXT NOT NULL,
				id           TEXT NOT NULL,
				name         TEXT NOT NULL DEFAULT '',
				description  TEXT NOT NULL DEFAULT '',
				status       TEXT NOT NULL DEFAULT 'draft',
				created_by   TEXT NOT NULL DEFAULT '',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				submitted_at TIMESTAMPTZ,
				merged_at    TIMESTAMPTZ,
				merged_by    TEXT NOT NULL DEFAULT '',
				PRIMARY KEY (workspace_id, id)
			);
			CREATE INDEX idx_kg_changesets_status ON kg_changesets(workspace_id, status);

			CREATE TABLE kg_changeset_ops (
				workspace_id TEXT NOT NULL,
				changeset_id TEXT NOT NULL,
				seq          BIGINT NOT NULL,
				op           TEXT NOT NULL,
				payload      JSONB NOT NULL DEFAULT 'null',
				base_rev     BIGINT NOT NULL DEFAULT 0,
				created_by   TEXT NOT NULL DEFAULT '',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (workspace_id, changeset_id, seq)
			);

			CREATE TABLE kg_changeset_reviews (
				workspace_id TEXT NOT NULL,
				changeset_id TEXT NOT NULL,
				reviewer     TEXT NOT NULL,
				verdict      TEXT NOT NULL,
				comment      TEXT NOT NULL DEFAULT '',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (workspace_id, changeset_id, reviewer)
			);

			CREATE TABLE kg_pilots (
				workspace_id TEXT NOT NULL,
				changeset_id TEXT NOT NULL,
				project_id   TEXT NOT NULL,
				stream       TEXT NOT NULL,
				created_by   TEXT NOT NULL DEFAULT '',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (workspace_id, changeset_id, project_id, stream)
			);
			CREATE INDEX idx_kg_pilots_stream ON kg_pilots(workspace_id, project_id, stream);
		`,
	},
}
