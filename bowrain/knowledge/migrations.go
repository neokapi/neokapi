package knowledge

import "github.com/neokapi/neokapi/bowrain/storage"

// Migrations is the knowledge-graph governance schema as a single consolidated
// baseline: one clean definition of all seven tables from the data-model note.
// Every workspace-scoped table is keyed by (workspace_id, …); timestamps are
// TIMESTAMPTZ; snapshot, payload, and locales are JSONB.
//
// LEDGER — every version this subsystem has ever issued, now folded in:
//
//	1  knowledge graph governance schema (baseline)
//	2  knowledge graph baseline (folded 1)
//	3  solo self-approval marker
//	4  change-set origin and supersession
//
// Baseline is version 5 — above every number issued, so an existing database
// applies it once and any drift between its schema and its bookkeeping is
// repaired. Retired numbers are never reused; the next migration is version 6.
//
// The subsystem carries exactly one baseline (migrations/schema_test.go
// enforces it), so a schema change is made by editing the baseline in place and
// bumping its version. Version 5 added kg_changesets.impact_summary — the
// blast radius computed at submit, so reading it is a row rather than a walk
// over every stored block.
var Migrations = []storage.Migration{
	{
		Version:     5,
		Description: "knowledge graph baseline (folds 1-4) + stored change-set blast-radius summary",
		SQL: `
			CREATE TABLE IF NOT EXISTS kg_markets (
				workspace_id TEXT NOT NULL,
				id           TEXT NOT NULL,
				name         TEXT NOT NULL,
				description  TEXT NOT NULL DEFAULT '',
				locales      JSONB NOT NULL DEFAULT '[]',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (workspace_id, id)
			);

			CREATE TABLE IF NOT EXISTS kg_observations (
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
			CREATE INDEX IF NOT EXISTS idx_kg_observations_concept ON kg_observations(workspace_id, concept_id);

			CREATE TABLE IF NOT EXISTS kg_comments (
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
			CREATE INDEX IF NOT EXISTS idx_kg_comments_concept ON kg_comments(workspace_id, concept_id);
			CREATE INDEX IF NOT EXISTS idx_kg_comments_changeset ON kg_comments(workspace_id, changeset_id);

			CREATE TABLE IF NOT EXISTS kg_concept_revisions (
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

			CREATE TABLE IF NOT EXISTS kg_changesets (
				workspace_id TEXT NOT NULL,
				id           TEXT NOT NULL,
				name         TEXT NOT NULL DEFAULT '',
				description  TEXT NOT NULL DEFAULT '',
				status       TEXT NOT NULL DEFAULT 'draft',
				origin        TEXT NOT NULL DEFAULT '',
				superseded_by TEXT NOT NULL DEFAULT '',
				created_by   TEXT NOT NULL DEFAULT '',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				submitted_at TIMESTAMPTZ,
				merged_at    TIMESTAMPTZ,
				merged_by    TEXT NOT NULL DEFAULT '',
				impact_summary JSONB,
				PRIMARY KEY (workspace_id, id)
			);
			CREATE INDEX IF NOT EXISTS idx_kg_changesets_status ON kg_changesets(workspace_id, status);
			ALTER TABLE kg_changesets ADD COLUMN IF NOT EXISTS origin        TEXT NOT NULL DEFAULT '';
			ALTER TABLE kg_changesets ADD COLUMN IF NOT EXISTS superseded_by TEXT NOT NULL DEFAULT '';
			ALTER TABLE kg_changesets ADD COLUMN IF NOT EXISTS impact_summary JSONB;
			CREATE INDEX IF NOT EXISTS idx_kg_changesets_origin
				ON kg_changesets(workspace_id, origin) WHERE origin != '' AND status IN ('draft', 'in_review');

			CREATE TABLE IF NOT EXISTS kg_changeset_ops (
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

			CREATE TABLE IF NOT EXISTS kg_changeset_reviews (
				workspace_id TEXT NOT NULL,
				changeset_id TEXT NOT NULL,
				reviewer     TEXT NOT NULL,
				verdict      TEXT NOT NULL,
				comment      TEXT NOT NULL DEFAULT '',
				-- The governance rule under which this verdict was admitted:
				-- 'peer' (someone other than the author reviewed) or
				-- 'solo_owner' (the author was the workspace's owner and its
				-- only eligible reviewer). A vocabulary rather than one boolean
				-- per exception, so a future variant is a new value here.
				review_basis TEXT NOT NULL DEFAULT 'peer',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (workspace_id, changeset_id, reviewer)
			);
			-- The CREATE above serves an empty database; this serves one that
			-- already has the table, where CREATE ... IF NOT EXISTS is a no-op
			-- and would leave the new column missing. Both are idempotent.
			ALTER TABLE kg_changeset_reviews
				ADD COLUMN IF NOT EXISTS review_basis TEXT NOT NULL DEFAULT 'peer';

			CREATE TABLE IF NOT EXISTS kg_pilots (
				workspace_id TEXT NOT NULL,
				changeset_id TEXT NOT NULL,
				project_id   TEXT NOT NULL,
				stream       TEXT NOT NULL,
				created_by   TEXT NOT NULL DEFAULT '',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (workspace_id, changeset_id, project_id, stream)
			);
			CREATE INDEX IF NOT EXISTS idx_kg_pilots_stream ON kg_pilots(workspace_id, project_id, stream);
		`,
	},
}
