package store

import "github.com/neokapi/neokapi/bowrain/storage"

// Migrations is the complete PostgreSQL content store schema as a single
// consolidated baseline. It is the largest of the fifteen ledgers: sixty-nine
// tables, including the hash-partitioned blocks, translations, annotations and
// overlays_ext families.
//
// LEDGER — every version this subsystem has ever issued, now folded in:
//
//	 1  content store schema (baseline)
//	 2  workspace-scoped AI provider configs (Epic 004 hybrid AI)
//	 3  Live Preview: per-project settings + block content key (AD-023/AD-036)
//	 4  block occurrences (AD-036)
//	 5  workspace-scoped connector configs (durable connectors)
//	 6  stream properties (extensible metadata, incl. voice binding)
//	 7  convergence run loop-observability columns (stall_reason, stage, activity)
//	 8  source-first convergence: blocked-on-source count on a run
//	 9  connector last-sync timestamp (real status, not fabricated now)
//	10  connector last-error (observable background ingest failures)
//	11  proposed source changes (back-to-source review, RV-F)
//	12  block stand-off overlays column
//	13  GitHub App installation ownership
//	14  collection context: coordinates, ownership, and the entry hash
//	15  the first consolidated baseline (folded 1-14)
//	16  unit decisions ledger (decisions travel the sync protocol)
//	17  stream scope for convergence runs
//	18  source word count computed at write
//	19  blocks access ladder renamed (open|restricted|published)
//	20  the second consolidated baseline (folded 1-19)
//	21  the audit log keyed on the bus event it records
//	22  channel alias proposals
//	23  the third consolidated baseline (folded 1-22) + channel alias judgements
//
// The subsystem carries exactly one baseline (migrations/schema_test.go
// enforces it), so a schema change is made by editing the baseline in place and
// bumping its version. Version 24 records the BASIS on a decision — the hash of
// the source wording it blessed — so a reader can tell an approval whose source
// has been rewritten from one that still describes the project.
//
// Versions 3 and 4 were already retired before the first consolidation — they
// ran on live databases and were then folded into the v1 baseline. They are
// listed because a retired number stays spent forever: a live database records
// them as applied, so a new migration reusing 3 or 4 would be silently skipped.
// That is why a consolidation numbers its baseline above the whole range
// rather than restarting at 1.
//
// Versions 16-19 were appended after the first consolidation and are now folded
// in turn, so the rule the drift tests enforce — a consolidated subsystem
// carries exactly one baseline — holds again. Folding is editing the CREATE
// statements, never appending an ALTER beside them: 17's stream and 18's
// word_count are columns of their tables' CREATE, 19's rename is the access
// column the blocks CREATE already declares, and 16's ledger is a
// CREATE TABLE IF NOT EXISTS like every other table here. That is what lets one
// statement serve an empty database and a database that already ran 15-19
// alike.
//
// Baseline is version 24 — above every number issued, so an existing database
// applies it once and any drift between its schema and its bookkeeping is
// repaired. Retired numbers are never reused; the next migration is version 32.
//
// 25  where a collection's strings can be read in place
// 26  the ship gate's per-block verdict
// 27  a stream owns its content, and a file is identified by what it is
// 30  the ledger records the source the platform last drafted a unit against
// 31  the ledger records the governing context a decision was made under
var Migrations = []storage.Migration{
	{
		Version:     24,
		Description: "content store baseline (folds 1-23) + the decision basis",
		SQL: `
			-- Projects
			CREATE TABLE IF NOT EXISTS projects (
				id                      TEXT PRIMARY KEY,
				name                    TEXT NOT NULL,
				default_source_language TEXT NOT NULL DEFAULT '',
				target_languages        TEXT NOT NULL DEFAULT '',
				target_language_mode    TEXT NOT NULL DEFAULT 'defined',
				default_stream          TEXT NOT NULL DEFAULT '',
				dashboard_visibility    TEXT NOT NULL DEFAULT 'private',
				properties              TEXT NOT NULL DEFAULT '{}',
				workspace_id            TEXT NOT NULL DEFAULT '',
				converge_policy         TEXT NOT NULL DEFAULT 'on-push',
				archived                BOOLEAN NOT NULL DEFAULT FALSE,
				archived_at             TIMESTAMPTZ,
				created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_projects_workspace ON projects(workspace_id);

			-- Streams
			CREATE TABLE IF NOT EXISTS streams (
				project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				name        TEXT NOT NULL,
				parent      TEXT NOT NULL DEFAULT '',
				base_cursor BIGINT NOT NULL DEFAULT 0,
				archived    BOOLEAN NOT NULL DEFAULT FALSE,
				visibility  TEXT NOT NULL DEFAULT 'public',
				description TEXT NOT NULL DEFAULT '',
				locked      BOOLEAN NOT NULL DEFAULT FALSE,
				locked_by   TEXT NOT NULL DEFAULT '',
				locked_at   TIMESTAMPTZ,
				-- Extensible key/value metadata, like projects and items carry:
				-- most immediately the stream-level voice binding
				-- (voice_profile_id), a rung in the hierarchical profile
				-- resolver.
				properties  TEXT NOT NULL DEFAULT '{}',
				created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				created_by  TEXT NOT NULL DEFAULT '',
				PRIMARY KEY (project_id, name)
			);

			CREATE TABLE IF NOT EXISTS stream_members (
				project_id TEXT NOT NULL,
				stream     TEXT NOT NULL,
				user_id    TEXT NOT NULL,
				added_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, stream, user_id),
				FOREIGN KEY (project_id, stream) REFERENCES streams(project_id, name) ON DELETE CASCADE
			);

			CREATE TABLE IF NOT EXISTS stream_tags (
				id         TEXT PRIMARY KEY,
				project_id TEXT NOT NULL,
				stream     TEXT NOT NULL,
				name       TEXT NOT NULL,
				kind       TEXT NOT NULL DEFAULT 'custom',
				cursor     BIGINT NOT NULL DEFAULT 0,
				metadata   TEXT NOT NULL DEFAULT '{}',
				created_by TEXT NOT NULL DEFAULT '',
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				FOREIGN KEY (project_id, stream) REFERENCES streams(project_id, name) ON DELETE CASCADE
			);
			CREATE UNIQUE INDEX IF NOT EXISTS idx_stream_tags_unique ON stream_tags(project_id, stream, name);
			CREATE INDEX IF NOT EXISTS idx_stream_tags_stream ON stream_tags(project_id, stream);
			CREATE INDEX IF NOT EXISTS idx_stream_tags_project_kind ON stream_tags(project_id, kind);

			-- Collections
			CREATE TABLE IF NOT EXISTS collections (
				id               TEXT NOT NULL,
				project_id       TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				name             TEXT NOT NULL,
				kind             TEXT NOT NULL DEFAULT 'uploaded',
				item_label       TEXT NOT NULL DEFAULT 'item',
				is_default       BOOLEAN NOT NULL DEFAULT FALSE,
				stream           TEXT NOT NULL DEFAULT '',
				connector_config TEXT NOT NULL DEFAULT '{}',
				-- The point the collection's content occupies in the project's
				-- context space (axis → value), as the recipe declares under
				-- 'context:'. Slugs a recipe writes in plain sight, so unlike
				-- connector_config (credentials) this column is not sealed.
				context          TEXT NOT NULL DEFAULT '{}',
				-- 'recipe' or 'workspace': which side is authoritative. Rows
				-- created by the web hub, the editor or a connector default to
				-- 'workspace', because reading them as recipe-owned would hand authority
				-- over them to a kapi.yaml that never mentioned them.
				owner            TEXT NOT NULL DEFAULT 'workspace',
				-- Hash of the context entry the row was reconciled from. It is
				-- what makes a re-push idempotent: an unchanged hash leaves the
				-- row, and its updated_at, untouched. Empty until a push
				-- reconciles the row.
				context_hash     TEXT NOT NULL DEFAULT '',
				created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, id)
			);
			CREATE INDEX IF NOT EXISTS idx_collections_project ON collections(project_id);
			CREATE UNIQUE INDEX IF NOT EXISTS idx_collections_default ON collections(project_id)
				WHERE is_default = TRUE;

			-- Items
			CREATE TABLE IF NOT EXISTS items (
				project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				name          TEXT NOT NULL,
				id            TEXT NOT NULL DEFAULT '',
				stream        TEXT NOT NULL DEFAULT 'main',
				format        TEXT NOT NULL DEFAULT '',
				item_type     TEXT NOT NULL DEFAULT 'file',
				block_index   TEXT NOT NULL DEFAULT '{}',
				preview_html  TEXT NOT NULL DEFAULT '',
				properties    TEXT NOT NULL DEFAULT '{}',
				collection_id TEXT NOT NULL DEFAULT '',
				created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, stream, name)
			);
			CREATE INDEX IF NOT EXISTS idx_items_project ON items(project_id);
			CREATE INDEX IF NOT EXISTS idx_items_project_stream ON items(project_id, stream);
			CREATE INDEX IF NOT EXISTS idx_items_collection ON items(project_id, collection_id);
			CREATE UNIQUE INDEX IF NOT EXISTS idx_items_id ON items(project_id, stream, id);

			-- Blocks hold source content + project metadata only.
			-- Targets and annotations live in their own kind-specific
			-- tables (#403/#405) so each access pattern gets the right
			-- indexes and a single source of truth.
			CREATE TABLE IF NOT EXISTS blocks (
				id           TEXT NOT NULL,
				project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				item_name    TEXT NOT NULL DEFAULT '',
				source_id    TEXT NOT NULL DEFAULT '',
				name         TEXT NOT NULL DEFAULT '',
				type         TEXT NOT NULL DEFAULT '',
				mime_type    TEXT NOT NULL DEFAULT '',
				translatable BOOLEAN NOT NULL DEFAULT TRUE,
				content_hash TEXT NOT NULL DEFAULT '',
				context_hash TEXT NOT NULL DEFAULT '',
				source_json  TEXT NOT NULL DEFAULT '[]',
				-- The positional, run-anchored stand-off layers (segmentation,
				-- term, entity, term-candidate, qa, alignment) persist alongside
				-- source_json so they survive a store round-trip. Serialized as a
				-- JSON array (the model's own JSON, each span's typed Value in a
				-- {"type","data"} envelope, mirroring the annotations payload).
				overlays     JSONB NOT NULL DEFAULT '[]'::jsonb,
				-- Source words, counted where the words are written. NULL marks a
				-- row written before the column existed; readers decode its
				-- source_json once and every rewrite fills the column in. Deriving
				-- coverage used to deserialize every block's source runs on every
				-- call: minutes at corpus scale, for numbers the write path
				-- already knew.
				word_count   INTEGER,
				properties   TEXT NOT NULL DEFAULT '{}',
				owner_id     TEXT NOT NULL DEFAULT '',         -- ABAC: content owner
				-- ABAC, and its values name the access consequence rather than
				-- borrowing the review ladder's words: open (normal perms),
				-- restricted (review perms or ownership), published (re-opening is
				-- privileged). The column was called status and read 'draft' /
				-- 'in_review' (one letter from 'reviewed' on the other side of the
				-- same block) until version 19 renamed it.
				access       TEXT NOT NULL DEFAULT 'open',
				stored_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, id)
			);
			CREATE INDEX IF NOT EXISTS idx_blocks_content_hash ON blocks(content_hash);
			CREATE INDEX IF NOT EXISTS idx_blocks_project ON blocks(project_id);
			CREATE INDEX IF NOT EXISTS idx_blocks_item ON blocks(project_id, item_name);
			CREATE UNIQUE INDEX IF NOT EXISTS idx_blocks_source_id ON blocks(project_id, item_name, source_id)
				WHERE source_id != '';

			-- Change log
			CREATE TABLE IF NOT EXISTS change_log (
				seq         BIGSERIAL PRIMARY KEY,
				project_id  TEXT NOT NULL,
				block_id    TEXT NOT NULL,
				change_type TEXT NOT NULL,
				locale      TEXT,
				content_hash TEXT,
				stream      TEXT NOT NULL DEFAULT 'main',
				correlation_id TEXT NOT NULL DEFAULT '', -- groups changes from one push/merge/request
				logged_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_changelog_project_seq ON change_log(project_id, seq);
			CREATE INDEX IF NOT EXISTS idx_changelog_project_locale ON change_log(project_id, locale, seq);
			CREATE INDEX IF NOT EXISTS idx_changelog_stream ON change_log(project_id, stream, seq);
			CREATE INDEX IF NOT EXISTS idx_changelog_correlation ON change_log(project_id, correlation_id);

			-- Compaction moves old change_log rows here rather than deleting them,
			-- so the sync trail is retained (not destroyed) when the live feed is
			-- trimmed.
			CREATE TABLE IF NOT EXISTS change_log_archive (
				seq            BIGINT PRIMARY KEY,
				project_id     TEXT NOT NULL,
				block_id       TEXT NOT NULL,
				change_type    TEXT NOT NULL,
				locale         TEXT,
				content_hash   TEXT,
				stream         TEXT NOT NULL DEFAULT 'main',
				correlation_id TEXT NOT NULL DEFAULT '',
				logged_at      TIMESTAMPTZ NOT NULL,
				archived_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_changelog_archive_project ON change_log_archive(project_id, seq);

			-- Block history: append-only prior content per (block, locale). The
			-- attribution columns (actor_role/edit_reason/correlation_id) make it
			-- audit-grade and let a whole push/merge be reverted as a unit.
			CREATE TABLE IF NOT EXISTS block_history (
				id             BIGSERIAL PRIMARY KEY,
				project_id     TEXT NOT NULL,
				block_id       TEXT NOT NULL,
				locale         TEXT NOT NULL,
				change_type    TEXT NOT NULL,
				text           TEXT NOT NULL DEFAULT '',
				coded_text     TEXT NOT NULL DEFAULT '',
				origin         TEXT NOT NULL DEFAULT '',
				author         TEXT NOT NULL DEFAULT '',
				actor_role     TEXT NOT NULL DEFAULT '',
				edit_reason    TEXT NOT NULL DEFAULT '',
				correlation_id TEXT NOT NULL DEFAULT '',
				stream         TEXT NOT NULL DEFAULT 'main',
				created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_block_history_lookup ON block_history(project_id, block_id, locale);
			CREATE INDEX IF NOT EXISTS idx_block_history_stream ON block_history(project_id, stream, block_id, locale);
			CREATE INDEX IF NOT EXISTS idx_block_history_correlation ON block_history(project_id, correlation_id);

			-- Block notes
			CREATE TABLE IF NOT EXISTS block_notes (
				id         TEXT PRIMARY KEY,
				project_id TEXT NOT NULL,
				block_id   TEXT NOT NULL,
				author     TEXT NOT NULL DEFAULT '',
				text       TEXT NOT NULL,
				stream     TEXT NOT NULL DEFAULT 'main',
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_block_notes_lookup ON block_notes(project_id, block_id);
			CREATE INDEX IF NOT EXISTS idx_block_notes_stream ON block_notes(project_id, stream, block_id);

			-- Versions
			CREATE TABLE IF NOT EXISTS versions (
				id          TEXT PRIMARY KEY,
				project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				label       TEXT NOT NULL,
				description TEXT NOT NULL DEFAULT '',
				block_count INTEGER NOT NULL DEFAULT 0,
				created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_versions_project ON versions(project_id);

			CREATE TABLE IF NOT EXISTS version_blocks (
				version_id   TEXT NOT NULL REFERENCES versions(id) ON DELETE CASCADE,
				block_id     TEXT NOT NULL,
				content_hash TEXT NOT NULL,
				PRIMARY KEY (version_id, block_id)
			);

			-- Assets (Bowrain AD-007)
			CREATE TABLE IF NOT EXISTS assets (
				id                TEXT PRIMARY KEY,
				project_id        TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				item_name         TEXT NOT NULL DEFAULT '',
				source_id         TEXT NOT NULL DEFAULT '',
				blob_key          TEXT NOT NULL,
				mime_type         TEXT NOT NULL,
				filename          TEXT NOT NULL DEFAULT '',
				size_bytes        BIGINT NOT NULL DEFAULT 0,
				alt_text          TEXT NOT NULL DEFAULT '',
				properties        TEXT NOT NULL DEFAULT '{}',
				processing_status TEXT NOT NULL DEFAULT 'none',
				processing_hint   TEXT NOT NULL DEFAULT '',
				stream            TEXT NOT NULL DEFAULT 'main',
				created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_assets_project_item ON assets(project_id, item_name);
			CREATE UNIQUE INDEX IF NOT EXISTS idx_assets_blob ON assets(project_id, blob_key)
				WHERE stream = 'main';

			CREATE TABLE IF NOT EXISTS asset_variants (
				asset_id   TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
				locale     TEXT NOT NULL,
				blob_key   TEXT NOT NULL,
				status     TEXT NOT NULL DEFAULT 'pending',
				mime_type  TEXT NOT NULL DEFAULT '',
				size_bytes BIGINT NOT NULL DEFAULT 0,
				properties TEXT NOT NULL DEFAULT '{}',
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (asset_id, locale)
			);

			CREATE TABLE IF NOT EXISTS block_asset_refs (
				project_id TEXT NOT NULL,
				block_id   TEXT NOT NULL,
				asset_id   TEXT NOT NULL,
				ref_type   TEXT NOT NULL DEFAULT 'embedded',
				stream     TEXT NOT NULL DEFAULT 'main',
				PRIMARY KEY (project_id, block_id, asset_id)
			);
			CREATE INDEX IF NOT EXISTS idx_block_asset_refs_asset ON block_asset_refs(project_id, asset_id);

			-- Automations
			CREATE TABLE IF NOT EXISTS automation_rules (
				id         TEXT PRIMARY KEY,
				project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				name       TEXT NOT NULL,
				trigger    TEXT NOT NULL,
				conditions TEXT NOT NULL DEFAULT '[]',
				actions    TEXT NOT NULL DEFAULT '[]',
				enabled    BOOLEAN NOT NULL DEFAULT TRUE,
				builtin    BOOLEAN NOT NULL DEFAULT FALSE,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_automation_rules_project ON automation_rules(project_id);

			CREATE TABLE IF NOT EXISTS automation_history (
				id         TEXT PRIMARY KEY,
				rule_id    TEXT NOT NULL,
				project_id TEXT NOT NULL,
				event_id   TEXT NOT NULL DEFAULT '',
				status     TEXT NOT NULL,
				error      TEXT NOT NULL DEFAULT '',
				started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				ended_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_automation_history_project ON automation_history(project_id);
			CREATE INDEX IF NOT EXISTS idx_automation_history_rule ON automation_history(rule_id);

			-- Automation runs (Bowrain AD-013)
			CREATE TABLE IF NOT EXISTS automation_runs (
				id           TEXT PRIMARY KEY,
				project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				trigger_type TEXT NOT NULL,
				trigger_id   TEXT NOT NULL DEFAULT '',
				trigger_data JSONB NOT NULL DEFAULT '{}',
				status       TEXT NOT NULL DEFAULT 'pending',
				step_count   INT NOT NULL DEFAULT 0,
				done_count   INT NOT NULL DEFAULT 0,
				error        TEXT NOT NULL DEFAULT '',
				started_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				ended_at     TIMESTAMPTZ
			);
			CREATE INDEX IF NOT EXISTS idx_automation_runs_project ON automation_runs(project_id, started_at DESC);

			CREATE TABLE IF NOT EXISTS automation_steps (
				id          TEXT PRIMARY KEY,
				run_id      TEXT NOT NULL REFERENCES automation_runs(id) ON DELETE CASCADE,
				rule_name   TEXT NOT NULL DEFAULT '',
				action_type TEXT NOT NULL,
				status      TEXT NOT NULL DEFAULT 'pending',
				config      JSONB NOT NULL DEFAULT '{}',
				job_ids     JSONB NOT NULL DEFAULT '[]',
				task_ids    JSONB NOT NULL DEFAULT '[]',
				total_jobs  INT NOT NULL DEFAULT 0,
				done_jobs   INT NOT NULL DEFAULT 0,
				error       TEXT NOT NULL DEFAULT '',
				started_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				ended_at    TIMESTAMPTZ
			);
			CREATE INDEX IF NOT EXISTS idx_automation_steps_run ON automation_steps(run_id);

			CREATE TABLE IF NOT EXISTS automation_logs (
				id        TEXT PRIMARY KEY,
				step_id   TEXT NOT NULL,
				run_id    TEXT NOT NULL,
				level     TEXT NOT NULL DEFAULT 'info',
				message   TEXT NOT NULL,
				data      JSONB NOT NULL DEFAULT '{}',
				timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_automation_logs_step ON automation_logs(step_id, timestamp);
			CREATE INDEX IF NOT EXISTS idx_automation_logs_run ON automation_logs(run_id, timestamp);

			-- Convergence runs: one goal-seeking reconciliation of a project
			-- toward its ship gates (strategy 2026-07-kapi-up doc 03). A run
			-- drives the venue-neutral core/convergence.Loop server-side and
			-- streams its convergence.Event feed; standing holds the per-locale
			-- rollup, and every emitted event is persisted for SSE replay.
			-- Timestamps are stored as RFC3339 TEXT (not TIMESTAMPTZ) so a single
			-- ConvergenceRunStore(*sql.DB) scans identically on PostgreSQL and
			-- SQLite; UTC RFC3339Nano sorts chronologically under ORDER BY.
			CREATE TABLE IF NOT EXISTS convergence_runs (
				id             TEXT PRIMARY KEY,
				project_id     TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				trigger        TEXT NOT NULL DEFAULT 'manual',      -- cli | push | manual
				-- A run's derivation and produce are scoped to one stream (targets
				-- are per-stream overlays). '' means "main", so pre-stream rows and
				-- stream-naive starters keep their behavior.
				stream         TEXT NOT NULL DEFAULT '',
				state          TEXT NOT NULL DEFAULT 'running',     -- running | converged | parked | canceled | failed
				passes         INT NOT NULL DEFAULT 0,
				standing       TEXT NOT NULL DEFAULT '{}',          -- per-locale standing rollup (JSON)
				failing_checks INT NOT NULL DEFAULT 0,
				-- Loop observability + labeled stalls. A run carries the
				-- machine-readable stall_reason (needs_credits | needs_ai_key |
				-- rate_limited | no_progress | checks_failing), the current loop
				-- stage/locale, and a heartbeat (last_activity) refreshed from the
				-- job queue's updated_at, so a run that is "slow but alive" is
				-- distinguishable from one that has stalled.
				stall_reason   TEXT NOT NULL DEFAULT '',
				current_stage  TEXT NOT NULL DEFAULT '',
				current_locale TEXT NOT NULL DEFAULT '',
				last_activity  TEXT NOT NULL DEFAULT '',
				-- Source-first convergence: how many source blocks are held below
				-- the source gate (settle-then-translate): what the UI renders as
				-- "N segments need source review before translating", and the
				-- signal behind a source_not_ready hold.
				blocked_on_source INTEGER NOT NULL DEFAULT 0,
				error          TEXT NOT NULL DEFAULT '',
				created_at     TEXT NOT NULL DEFAULT '',
				finished_at    TEXT NOT NULL DEFAULT ''
			);
			CREATE INDEX IF NOT EXISTS idx_convergence_runs_project ON convergence_runs(project_id, created_at DESC);
			-- At most one running run per project. A DB constraint (not just an
			-- app-level check) makes the one-run-per-project guard atomic and
			-- correct across replicas: a concurrent push-event + CLI start race
			-- to INSERT and exactly one wins; the loser returns the existing run.
			CREATE UNIQUE INDEX IF NOT EXISTS idx_convergence_runs_one_active ON convergence_runs(project_id) WHERE state = 'running';

			CREATE TABLE IF NOT EXISTS convergence_run_events (
				run_id  TEXT NOT NULL REFERENCES convergence_runs(id) ON DELETE CASCADE,
				seq     INT NOT NULL,
				payload TEXT NOT NULL DEFAULT '{}',
				PRIMARY KEY (run_id, seq)
			);

			-- Review queue
			CREATE TABLE IF NOT EXISTS review_items (
				id          TEXT PRIMARY KEY,
				project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				type        TEXT NOT NULL,
				status      TEXT NOT NULL DEFAULT 'pending',
				push_id     TEXT NOT NULL DEFAULT '',
				data        TEXT NOT NULL,
				occurrences TEXT NOT NULL DEFAULT '[]',
				assigned_to TEXT NOT NULL DEFAULT '',
				decided_by  TEXT NOT NULL DEFAULT '',
				decided_at  TIMESTAMPTZ,
				comment     TEXT NOT NULL DEFAULT '',
				edits       TEXT NOT NULL DEFAULT '{}',
				confidence  DOUBLE PRECISION NOT NULL DEFAULT 0,
				locale      TEXT NOT NULL DEFAULT '',
				created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_review_items_project_status ON review_items(project_id, status);
			CREATE INDEX IF NOT EXISTS idx_review_items_project_type ON review_items(project_id, type);
			CREATE INDEX IF NOT EXISTS idx_review_items_assigned ON review_items(project_id, assigned_to);
			CREATE INDEX IF NOT EXISTS idx_review_items_confidence ON review_items(project_id, confidence);

			CREATE TABLE IF NOT EXISTS rejected_terms (
				project_id  TEXT NOT NULL,
				term_text   TEXT NOT NULL,
				locale      TEXT NOT NULL,
				rejected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, term_text, locale)
			);

			CREATE TABLE IF NOT EXISTS dnt_entries (
				project_id  TEXT NOT NULL,
				text        TEXT NOT NULL,
				entity_type TEXT NOT NULL DEFAULT '',
				locale      TEXT NOT NULL,
				source      TEXT NOT NULL DEFAULT '',
				created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, text, locale)
			);

			-- Notifications
			CREATE TABLE IF NOT EXISTS notifications (
				id         TEXT PRIMARY KEY,
				user_id    TEXT NOT NULL,
				type       TEXT NOT NULL DEFAULT 'general',
				title      TEXT NOT NULL,
				body       TEXT NOT NULL DEFAULT '',
				project_id TEXT NOT NULL DEFAULT '',
				link_url   TEXT NOT NULL DEFAULT '',
				read       BOOLEAN NOT NULL DEFAULT FALSE,
				category   TEXT NOT NULL DEFAULT '',
				group_key  TEXT NOT NULL DEFAULT '',
				actor_id   TEXT NOT NULL DEFAULT '',
				actor_name TEXT NOT NULL DEFAULT '',
				task_id    TEXT NOT NULL DEFAULT '',
				priority   TEXT NOT NULL DEFAULT 'normal',

				-- The bus event this notification came from. One user hears
				-- about one event once: a redelivery, which every deploy
				-- rollover produces, must not put a second row in the inbox
				-- and a second email in the mailbox.
				source_event_id TEXT NOT NULL DEFAULT '',

				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_notifications_user ON notifications(user_id, read, created_at DESC);
			-- Serves a database that already has notifications, where the
			-- CREATE above is a no-op.
			ALTER TABLE notifications ADD COLUMN IF NOT EXISTS source_event_id TEXT NOT NULL DEFAULT '';
			CREATE UNIQUE INDEX IF NOT EXISTS notifications_user_source_event
				ON notifications(user_id, source_event_id) WHERE source_event_id <> '';

			CREATE TABLE IF NOT EXISTS notification_preferences (
				user_id         TEXT NOT NULL,
				workspace_id    TEXT NOT NULL,
				category        TEXT NOT NULL,
				channel_web     BOOLEAN NOT NULL DEFAULT TRUE,
				channel_email   BOOLEAN NOT NULL DEFAULT FALSE,
				channel_push    BOOLEAN NOT NULL DEFAULT FALSE,
				channel_desktop BOOLEAN NOT NULL DEFAULT FALSE,
				updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE(user_id, workspace_id, category)
			);
			CREATE INDEX IF NOT EXISTS idx_notif_pref_user ON notification_preferences(user_id, workspace_id);

			-- Activities
			CREATE TABLE IF NOT EXISTS activities (
				id           TEXT PRIMARY KEY,

				-- The bus event this entry came from, so a redelivered event
				-- files one line rather than a second copy of it. '' for the
				-- entries written directly rather than from an event.
				event_id     TEXT NOT NULL DEFAULT '',

				workspace_id TEXT NOT NULL,
				project_id   TEXT NOT NULL DEFAULT '',
				stream       TEXT NOT NULL DEFAULT '',
				actor_id     TEXT NOT NULL,
				actor_name   TEXT NOT NULL DEFAULT '',
				type         TEXT NOT NULL,
				entity_type  TEXT NOT NULL DEFAULT '',
				entity_id    TEXT NOT NULL DEFAULT '',
				summary      TEXT NOT NULL DEFAULT '',
				data         JSONB NOT NULL DEFAULT '{}',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_activities_workspace ON activities(workspace_id, created_at DESC);
			CREATE INDEX IF NOT EXISTS idx_activities_project ON activities(workspace_id, project_id, created_at DESC);
			CREATE INDEX IF NOT EXISTS idx_activities_actor ON activities(workspace_id, actor_id, created_at DESC);
			-- Serves a database that already has activities, where the CREATE
			-- above is a no-op.
			ALTER TABLE activities ADD COLUMN IF NOT EXISTS event_id TEXT NOT NULL DEFAULT '';
			CREATE UNIQUE INDEX IF NOT EXISTS activities_event_id
				ON activities(event_id) WHERE event_id <> '';

			-- Tasks
			CREATE TABLE IF NOT EXISTS tasks (
				id           TEXT PRIMARY KEY,
				workspace_id TEXT NOT NULL,
				project_id   TEXT NOT NULL,
				stream       TEXT NOT NULL DEFAULT '',
				type         TEXT NOT NULL DEFAULT 'custom',
				status       TEXT NOT NULL DEFAULT 'open',
				priority     TEXT NOT NULL DEFAULT 'normal',
				title        TEXT NOT NULL,
				description  TEXT NOT NULL DEFAULT '',
				assignee_id  TEXT NOT NULL DEFAULT '',
				created_by   TEXT NOT NULL,
				completed_by TEXT NOT NULL DEFAULT '',
				data         JSONB NOT NULL DEFAULT '{}',
				due_at       TIMESTAMPTZ,
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				completed_at TIMESTAMPTZ
			);
			CREATE INDEX IF NOT EXISTS idx_tasks_workspace ON tasks(workspace_id, status, created_at DESC);
			CREATE INDEX IF NOT EXISTS idx_tasks_project ON tasks(workspace_id, project_id, status);
			CREATE INDEX IF NOT EXISTS idx_tasks_assignee ON tasks(workspace_id, assignee_id, status);

			-- Digest
			CREATE TABLE IF NOT EXISTS digest_settings (
				user_id      TEXT NOT NULL,
				workspace_id TEXT NOT NULL,
				frequency    TEXT NOT NULL DEFAULT 'daily',
				quiet_start  TEXT NOT NULL DEFAULT '',
				quiet_end    TEXT NOT NULL DEFAULT '',
				timezone     TEXT NOT NULL DEFAULT 'UTC',
				PRIMARY KEY (user_id, workspace_id)
			);

			CREATE TABLE IF NOT EXISTS digest_state (
				user_id      TEXT NOT NULL,
				workspace_id TEXT NOT NULL,
				frequency    TEXT NOT NULL DEFAULT 'daily',
				last_sent_at TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (user_id, workspace_id, frequency)
			);

			-- Audit log: append-only, hash-chained system-of-record for every
			-- auditable event (content mutations + security/governance actions).
			-- Each row links to the previous row in its chain (chain_key) via a
			-- SHA-256 hash chain so tampering is detectable. project_id is
			-- nullable so workspace-scoped (non-project) events are recorded.
			CREATE TABLE IF NOT EXISTS audit_log (
				id            BIGSERIAL PRIMARY KEY,

				-- The bus event this row records. A failed append leaves its
				-- event pending for redelivery, and the reclaim sweep re-runs
				-- handlers whose consumer died after finishing, so the append
				-- has to be keyed on something the event carries, or recovery
				-- would trade a lost record for a doubled one.
				event_id      TEXT NOT NULL DEFAULT '',

				chain_key     TEXT NOT NULL DEFAULT 'system', -- chain partition (workspace/project/system)
				project_id    TEXT,
				workspace_id  TEXT NOT NULL DEFAULT '',
				event_type    TEXT NOT NULL,
				actor         TEXT NOT NULL DEFAULT '',
				source        TEXT NOT NULL DEFAULT '',
				resource_type TEXT NOT NULL DEFAULT '',
				resource_id   TEXT NOT NULL DEFAULT '',
				effect        TEXT NOT NULL DEFAULT '',
				data          JSONB NOT NULL DEFAULT '{}',
				before_state  JSONB,
				after_state   JSONB,
				request_id    TEXT NOT NULL DEFAULT '',
				ip            TEXT NOT NULL DEFAULT '',
				user_agent    TEXT NOT NULL DEFAULT '',
				causation_id  TEXT NOT NULL DEFAULT '',
				prev_hash     TEXT NOT NULL DEFAULT '',
				hash          TEXT NOT NULL DEFAULT '',
				created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_audit_log_project ON audit_log(project_id, created_at DESC);
			CREATE INDEX IF NOT EXISTS idx_audit_log_workspace ON audit_log(workspace_id, created_at DESC);
			CREATE INDEX IF NOT EXISTS idx_audit_log_type ON audit_log(workspace_id, event_type, created_at DESC);
			CREATE INDEX IF NOT EXISTS idx_audit_log_actor ON audit_log(actor, created_at DESC);
			CREATE INDEX IF NOT EXISTS idx_audit_log_chain ON audit_log(chain_key, id);
			-- Serves a database that already has audit_log, where the CREATE
			-- above is a no-op.
			ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS event_id TEXT NOT NULL DEFAULT '';
			CREATE UNIQUE INDEX IF NOT EXISTS audit_log_event_id
				ON audit_log(event_id) WHERE event_id <> '';

			-- Append-only enforcement: block UPDATE always, and block DELETE
			-- unless a session explicitly opts in (used only by the retention
			-- pruner via SET LOCAL bowrain.audit_allow_delete = 'on'). This makes
			-- the trail tamper-evident at the database layer.
			CREATE OR REPLACE FUNCTION audit_log_no_mutate() RETURNS TRIGGER AS $audit$
			BEGIN
				IF (TG_OP = 'DELETE') THEN
					IF current_setting('bowrain.audit_allow_delete', true) = 'on' THEN
						RETURN OLD;
					END IF;
					RAISE EXCEPTION 'audit_log is append-only: DELETE is not permitted';
				END IF;
				RAISE EXCEPTION 'audit_log is append-only: UPDATE is not permitted';
			END;
			$audit$ LANGUAGE plpgsql;

			-- OR REPLACE, because a baseline must survive being applied to a
			-- database that already has the trigger. Postgres has no
			-- CREATE TRIGGER IF NOT EXISTS; replace is the idempotent form,
			-- and is available from Postgres 14 (production runs 16).
			CREATE OR REPLACE TRIGGER audit_log_append_only
				BEFORE UPDATE OR DELETE ON audit_log
				FOR EACH ROW EXECUTE FUNCTION audit_log_no_mutate();

			-- Project flow definitions (Bowrain AD-013). Server-side, editable
			-- flow graphs (reader → tool(s) → writer) that automation run_flow
			-- actions reference by id. graph holds the full FlowDefinition JSON
			-- (nodes, edges, stages, positions). Built-in flows are not stored
			-- here; they are merged in at the API layer.
			CREATE TABLE IF NOT EXISTS flow_definitions (
				id          TEXT NOT NULL,
				project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				name        TEXT NOT NULL,
				description TEXT NOT NULL DEFAULT '',
				graph       JSONB NOT NULL DEFAULT '{}',
				created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, id)
			);
			CREATE INDEX IF NOT EXISTS idx_flow_definitions_project ON flow_definitions(project_id, name);

			-- Leader leases (distributed coordination)
			CREATE TABLE IF NOT EXISTS leader_leases (
				name       TEXT PRIMARY KEY,
				holder_id  TEXT NOT NULL,
				expires_at TIMESTAMPTZ NOT NULL
			);

			-- Pending pushes (push completion tracking)
			CREATE TABLE IF NOT EXISTS pending_pushes (
				push_id    TEXT PRIMARY KEY,
				project_id TEXT NOT NULL,
				items      TEXT NOT NULL DEFAULT '',
				ws_slug    TEXT NOT NULL DEFAULT '',
				actor      TEXT NOT NULL DEFAULT '',
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);

			-- Activity read state (cross-device sync)
			CREATE TABLE IF NOT EXISTS activity_state (
				user_id      TEXT NOT NULL,
				workspace_id TEXT NOT NULL,
				last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (user_id, workspace_id)
			);

-- Overlay storage, split by kind for access-pattern-specific
			-- indexes (#403). The blockstore.Store interface routes by
			-- kind prefix: targets/* → translations, annotations/* →
			-- annotations, everything else → overlays_ext. Callers see
			-- one polymorphic Store API; the server-side adapter does
			-- the dispatch.

			-- All three overlay tables are hash-partitioned on project_id so
			-- per-project queries hit one partition and drop-project is
			-- O(1) per partition. 8 partitions covers single-digit-millions
			-- of rows per-kind-per-partition comfortably; bump via pg_repack
			-- if needed later.

			-- Per-locale translation targets. Hot read path: dashboards,
			-- editor fetches, sync export. Indexes serve both
			-- (project, locale, updated_at) feeds and per-block fetches.
			CREATE TABLE IF NOT EXISTS translations (
				project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				stream        TEXT NOT NULL DEFAULT 'main',
				block_id      TEXT NOT NULL,
				locale        TEXT NOT NULL,
				text          TEXT NOT NULL DEFAULT '',
				target_json   JSONB NOT NULL DEFAULT '{}'::jsonb,
				provider      TEXT NOT NULL DEFAULT '',
				metadata      JSONB NOT NULL DEFAULT '{}'::jsonb,
				updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, stream, block_id, locale)
			) PARTITION BY HASH (project_id);
			CREATE TABLE IF NOT EXISTS translations_p0 PARTITION OF translations FOR VALUES WITH (MODULUS 8, REMAINDER 0);
			CREATE TABLE IF NOT EXISTS translations_p1 PARTITION OF translations FOR VALUES WITH (MODULUS 8, REMAINDER 1);
			CREATE TABLE IF NOT EXISTS translations_p2 PARTITION OF translations FOR VALUES WITH (MODULUS 8, REMAINDER 2);
			CREATE TABLE IF NOT EXISTS translations_p3 PARTITION OF translations FOR VALUES WITH (MODULUS 8, REMAINDER 3);
			CREATE TABLE IF NOT EXISTS translations_p4 PARTITION OF translations FOR VALUES WITH (MODULUS 8, REMAINDER 4);
			CREATE TABLE IF NOT EXISTS translations_p5 PARTITION OF translations FOR VALUES WITH (MODULUS 8, REMAINDER 5);
			CREATE TABLE IF NOT EXISTS translations_p6 PARTITION OF translations FOR VALUES WITH (MODULUS 8, REMAINDER 6);
			CREATE TABLE IF NOT EXISTS translations_p7 PARTITION OF translations FOR VALUES WITH (MODULUS 8, REMAINDER 7);
			CREATE INDEX IF NOT EXISTS idx_translations_project_locale
				ON translations(project_id, stream, locale, updated_at DESC);
			CREATE INDEX IF NOT EXISTS idx_translations_project_block
				ON translations(project_id, stream, block_id);

			-- Semantic annotations (content-memory hits, term hits, check findings,
			-- translator notes). Grouped-by queries are the common
			-- access pattern: "all check findings for this project".
			CREATE TABLE IF NOT EXISTS annotations (
				project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				stream     TEXT NOT NULL DEFAULT 'main',
				block_id   TEXT NOT NULL,
				kind       TEXT NOT NULL,
				payload    JSONB NOT NULL DEFAULT '{}'::jsonb,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, stream, block_id, kind)
			) PARTITION BY HASH (project_id);
			CREATE TABLE IF NOT EXISTS annotations_p0 PARTITION OF annotations FOR VALUES WITH (MODULUS 8, REMAINDER 0);
			CREATE TABLE IF NOT EXISTS annotations_p1 PARTITION OF annotations FOR VALUES WITH (MODULUS 8, REMAINDER 1);
			CREATE TABLE IF NOT EXISTS annotations_p2 PARTITION OF annotations FOR VALUES WITH (MODULUS 8, REMAINDER 2);
			CREATE TABLE IF NOT EXISTS annotations_p3 PARTITION OF annotations FOR VALUES WITH (MODULUS 8, REMAINDER 3);
			CREATE TABLE IF NOT EXISTS annotations_p4 PARTITION OF annotations FOR VALUES WITH (MODULUS 8, REMAINDER 4);
			CREATE TABLE IF NOT EXISTS annotations_p5 PARTITION OF annotations FOR VALUES WITH (MODULUS 8, REMAINDER 5);
			CREATE TABLE IF NOT EXISTS annotations_p6 PARTITION OF annotations FOR VALUES WITH (MODULUS 8, REMAINDER 6);
			CREATE TABLE IF NOT EXISTS annotations_p7 PARTITION OF annotations FOR VALUES WITH (MODULUS 8, REMAINDER 7);
			CREATE INDEX IF NOT EXISTS idx_annotations_project_kind
				ON annotations(project_id, stream, kind, updated_at DESC);
			CREATE INDEX IF NOT EXISTS idx_annotations_project_block
				ON annotations(project_id, stream, block_id);

			-- Plugin catchall for overlay kinds that don't fit the
			-- purpose-built tables above. Same schema shape as the
			-- former block_overlays; renamed to signal "extension".
			CREATE TABLE IF NOT EXISTS overlays_ext (
				project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				stream     TEXT NOT NULL DEFAULT 'main',
				block_id   TEXT NOT NULL,
				kind       TEXT NOT NULL,
				payload    JSONB NOT NULL DEFAULT '{}'::jsonb,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, stream, block_id, kind)
			) PARTITION BY HASH (project_id);
			CREATE TABLE IF NOT EXISTS overlays_ext_p0 PARTITION OF overlays_ext FOR VALUES WITH (MODULUS 8, REMAINDER 0);
			CREATE TABLE IF NOT EXISTS overlays_ext_p1 PARTITION OF overlays_ext FOR VALUES WITH (MODULUS 8, REMAINDER 1);
			CREATE TABLE IF NOT EXISTS overlays_ext_p2 PARTITION OF overlays_ext FOR VALUES WITH (MODULUS 8, REMAINDER 2);
			CREATE TABLE IF NOT EXISTS overlays_ext_p3 PARTITION OF overlays_ext FOR VALUES WITH (MODULUS 8, REMAINDER 3);
			CREATE TABLE IF NOT EXISTS overlays_ext_p4 PARTITION OF overlays_ext FOR VALUES WITH (MODULUS 8, REMAINDER 4);
			CREATE TABLE IF NOT EXISTS overlays_ext_p5 PARTITION OF overlays_ext FOR VALUES WITH (MODULUS 8, REMAINDER 5);
			CREATE TABLE IF NOT EXISTS overlays_ext_p6 PARTITION OF overlays_ext FOR VALUES WITH (MODULUS 8, REMAINDER 6);
			CREATE TABLE IF NOT EXISTS overlays_ext_p7 PARTITION OF overlays_ext FOR VALUES WITH (MODULUS 8, REMAINDER 7);
			CREATE INDEX IF NOT EXISTS idx_overlays_ext_project_kind
				ON overlays_ext(project_id, stream, kind);
			CREATE INDEX IF NOT EXISTS idx_overlays_ext_project_block
				ON overlays_ext(project_id, stream, block_id);

			-- ---- folded from version 2: workspace-scoped AI provider configs (Epic 004 hybrid AI) ----
			-- Per-workspace AI provider configurations (Epic 004). This replaces
			-- the machine-global, keychain-backed core/credentials store for
			-- server-side provider keys, which does not work in a headless,
			-- multi-tenant production container (no Secret Service; one config
			-- shared across all workspaces).
			--
			-- The API key is sealed at rest with crypto.Cipher / BOWRAIN_SECRETS_KEY,
			-- exactly like connector secrets (collections.connector_config). A NULL
			-- api_key_sealed means "no key stored" (e.g. keyless providers like
			-- Ollama). workspace_id is the durable scope key; workspace_slug is kept
			-- as a secondary resolution key so the worker can resolve a saved config
			-- from a queued job that only persists the workspace slug.
			CREATE TABLE IF NOT EXISTS provider_configs (
				id             TEXT PRIMARY KEY,
				workspace_id   TEXT NOT NULL,
				workspace_slug TEXT NOT NULL DEFAULT '',
				name           TEXT NOT NULL,
				type           TEXT NOT NULL DEFAULT '',
				model          TEXT NOT NULL DEFAULT '',
				base_url       TEXT NOT NULL DEFAULT '',
				api_key_sealed BYTEA,
				created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE UNIQUE INDEX IF NOT EXISTS idx_provider_configs_ws_name ON provider_configs(workspace_id, name);
			CREATE INDEX IF NOT EXISTS idx_provider_configs_ws ON provider_configs(workspace_id);
			CREATE INDEX IF NOT EXISTS idx_provider_configs_ws_slug ON provider_configs(workspace_slug)
				WHERE workspace_slug != '';

			-- ---- folded from version 5: workspace-scoped connector configs (durable connectors) ----

			-- RETIRED VERSION NUMBERS, never reuse: 3 ("Live Preview: per-project
			-- settings + block content key", AD-023/AD-036) and 4 ("block occurrences",
			-- AD-036) ran on live databases before being folded into the v1 baseline.
			-- A live database records those numbers as applied, so a new migration
			-- reusing them is silently skipped. New migrations start at 5.
			-- Per-workspace connector configurations. The ConnectorService keeps
			-- only live instances in memory, so without this table a connector,
			-- and its credentials, is lost on restart. On boot the server reads
			-- these rows and re-instantiates each connector.
			--
			-- config is a JSON map (stored as TEXT, like collections.connector_config)
			-- whose SECRET values (wordpress=password, figma=token, hubspot=api_key)
			-- are individually sealed with crypto.Cipher / BOWRAIN_SECRETS_KEY,
			-- exactly like provider_configs.api_key_sealed. Non-secret fields (url,
			-- username, file_key, …) are stored in plaintext. Timestamps are TEXT
			-- RFC3339 (UTC) so one ConnectorConfigStore(*sql.DB) scans identically
			-- on PostgreSQL and SQLite.
			CREATE TABLE IF NOT EXISTS connector_configs (
				id           TEXT NOT NULL,
				workspace_id TEXT NOT NULL,
				type         TEXT NOT NULL,
				name         TEXT NOT NULL DEFAULT '',
				config       TEXT NOT NULL DEFAULT '{}',
				-- Last successful fetch/publish, so the status endpoint reports a
				-- real timestamp instead of the connector's own fabricated
				-- wall-clock time. Empty until the first sync ("never synced").
				last_sync_at TEXT NOT NULL DEFAULT '',
				-- Most recent sync failure, so an asynchronous ingest that fails (a
				-- webhook re-ingest, the first fetch after a GitHub App bind)
				-- surfaces on the connector's status instead of leaving a silently
				-- empty project. Cleared by the next successful sync.
				last_error   TEXT NOT NULL DEFAULT '',
				created_at   TEXT NOT NULL DEFAULT '',
				updated_at   TEXT NOT NULL DEFAULT '',
				PRIMARY KEY (workspace_id, id)
			);
			CREATE INDEX IF NOT EXISTS idx_connector_configs_ws ON connector_configs(workspace_id);

			-- ---- folded from version 6: stream properties (extensible metadata, incl. voice binding) ----
			-- Streams carry extensible key/value metadata like projects and items
			-- do: most immediately the stream-level voice binding
			-- (voice_profile_id), a rung in the hierarchical profile
			-- resolver. Stored as a JSON TEXT map, matching items.properties.

			-- ---- folded from version 7: convergence run loop-observability columns (stall_reason, stage, activity) ----
			-- Loop observability + labeled stalls (strategy 2026-07-dogfood doc 06,
			-- themes C/D). A run row now carries the machine-readable stall_reason
			-- (needs_credits | needs_ai_key | rate_limited | no_progress |
			-- checks_failing), the current loop stage/locale, and a heartbeat
			-- (last_activity) refreshed from the job queue's updated_at so a run
			-- that is "slow but alive" is distinguishable from one that has
			-- stalled. All default empty so existing rows read as an unlabeled
			-- run with no reason.

			-- ---- folded from version 8: source-first convergence: blocked-on-source count on a run ----
			-- Source-first convergence (strategy 2026-07-dogfood doc 07 / roadmap
			-- epic 019). A run row now carries how many source blocks are held
			-- below the source gate (settle-then-translate): the count the UI
			-- renders as "N segments need source review before translating" and
			-- the signal behind a source_not_ready hold. Defaults 0 so existing
			-- rows read as "nothing blocked on source".

			-- ---- folded from version 9: connector last-sync timestamp (real status, not fabricated now) ----
			-- Records the last successful fetch/publish per remote connector so the
			-- status endpoint reports a real timestamp instead of the connector's
			-- own fabricated wall-clock time. Empty until the first sync (read as
			-- "never synced").

			-- ---- folded from version 10: connector last-error (observable background ingest failures) ----
			-- Records the most recent sync failure per remote connector so an
			-- asynchronous ingest that fails (a webhook re-ingest, the first fetch
			-- after a GitHub App bind) surfaces on the connector's status instead
			-- of leaving a silently empty project. Cleared by the next successful
			-- sync (written together with last_sync_at).

			-- ---- folded from version 11: proposed source changes (back-to-source review, RV-F) ----
			-- A reviewer catching a source-TEXT problem while reviewing a target
			-- proposes a fix here. A source owner (PermEditSource) approves it,
			-- which applies the change to the block's source and re-drafts every
			-- locale; or rejects it. Additive, append-only lifecycle.
			CREATE TABLE IF NOT EXISTS proposed_source_changes (
				id              TEXT PRIMARY KEY,
				workspace_id    TEXT NOT NULL,
				project_id      TEXT NOT NULL,
				stream          TEXT NOT NULL DEFAULT 'main',
				item_name       TEXT NOT NULL DEFAULT '',
				block_id        TEXT NOT NULL,
				kind            TEXT NOT NULL DEFAULT 'text-fix',
				original_source TEXT NOT NULL DEFAULT '',
				proposed_source TEXT NOT NULL DEFAULT '',
				rationale       TEXT NOT NULL DEFAULT '',
				found_in_locale TEXT NOT NULL DEFAULT '',
				finder_user     TEXT NOT NULL DEFAULT '',
				status          TEXT NOT NULL DEFAULT 'open',
				decided_by      TEXT NOT NULL DEFAULT '',
				decision_reason TEXT NOT NULL DEFAULT '',
				created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				decided_at      TIMESTAMPTZ
			);
			CREATE INDEX IF NOT EXISTS idx_source_proposals_project_status
				ON proposed_source_changes(project_id, status);

			-- ---- folded from version 12: block stand-off overlays column (persist term/entity/segmentation/qa overlays across store round-trip) ----
			-- Block.Overlays, the positional, run-anchored stand-off layers
			-- (segmentation, term, entity, term-candidate, qa, alignment), persist
			-- alongside source_json so they survive a store round-trip. Without this
			-- column GetBlocks dropped every overlay, breaking entity/term handlers
			-- and the entity→concept promote path (which worked only while the block
			-- stayed in memory). Serialized as a JSON array (the model's own JSON,
			-- with each span's typed Value in a {"type","data"} envelope, mirroring
			-- the annotations payload). Empty overlays default to '[]', like
			-- source_json.

			-- ---- folded from version 13: GitHub App installation ownership (an installation belongs to one workspace) ----
			-- Which workspace a GitHub App installation belongs to. One registered
			-- app serves every workspace on a shared instance, and an installation
			-- id is a bare integer that carries no tenancy of its own, so the
			-- post-install setup endpoints have nothing to answer "may THIS
			-- workspace act on installation N?" with unless the answer is written
			-- down. This table is that record, and it is the sole authority: an
			-- installation with no row here is reachable from no workspace.
			--
			-- workspace_id is empty until the installation is CLAIMED. The two
			-- steps arrive independently and in either order:
			--   - the app-level 'installation' webhook records the installation as
			--     soon as GitHub reports it. It is authentic (signed with the app's
			--     webhook secret) but names no workspace, so it can only ever
			--     record, never claim;
			--   - the post-install redirect carries the signed setup state minted
			--     when the workspace started the install, which names the
			--     workspace and is what claims the row.
			-- First claim wins: once workspace_id is set, a claim from any other
			-- workspace is refused, and an uninstall ('installation' deleted) drops
			-- the row so a re-install starts unclaimed again.
			--
			-- Timestamps are TEXT RFC3339 (UTC) so one ForgeInstallationStore
			-- (*sql.DB) scans identically on PostgreSQL and SQLite, like
			-- connector_configs.
			CREATE TABLE IF NOT EXISTS forge_installations (
				installation_id BIGINT PRIMARY KEY,
				workspace_id    TEXT NOT NULL DEFAULT '',
				account         TEXT NOT NULL DEFAULT '',
				created_at      TEXT NOT NULL DEFAULT '',
				updated_at      TEXT NOT NULL DEFAULT ''
			);
			CREATE INDEX IF NOT EXISTS idx_forge_installations_ws
				ON forge_installations(workspace_id);

			-- ---- folded from version 14: collection context: coordinates, ownership, and the entry hash the context content type reconciles on ----
			-- The context content type (sync protocol): a push carries the
			-- collections the recipe declares, so a collection now has a
			-- server-side existence instead of being a name items merely
			-- mentioned.
			--
			-- context      is the point the collection's content occupies in the
			--                project's context space (axis → value), as the
			--                recipe declares under 'context:'. Slugs a recipe
			--                writes in plain sight, so unlike connector_config
			--                (credentials) this column is not sealed.
			-- owner        is 'recipe' or 'workspace': which side is
			--                authoritative. Existing rows were created by the web
			--                hub, the editor or a connector, so they backfill to
			--                'workspace', the conservative default, since
			--                reading them as recipe-owned would hand authority
			--                over them to a kapi.yaml that never mentioned them.
			-- context_hash is the hash of the context entry the row was
			--                reconciled from. It is what makes a re-push
			--                idempotent: an unchanged hash leaves the row, and
			--                its updated_at, untouched. Empty until a push
			--                reconciles the row.

			-- ---- folded from version 16: unit decisions ledger (decisions travel the sync protocol) ----
			-- The latest workflow decision per (item, unit, variant): the
			-- server side of core/state.UnitState. A decision is a FACT (who,
			-- when, which rung, and the hashes of the pairing it blesses: the
			-- translation, and the source it was blessed for); freshness against
			-- current content is derived by readers, never stored. History lives in block_history (change_type 'decision');
			-- this table is the fold of that log, kept because joins and
			-- projections need current state without replaying events.
			--
			-- unit is the durable identity (blocks.source_id), scoped by
			-- item_name because structural names recur across items. item_name
			-- may be '' when a decision arrives for content this store has
			-- never held, stored rather than dropped, so the ledger cannot
			-- lose what the corpus has not caught up to.
			CREATE TABLE IF NOT EXISTS unit_decisions (
				project_id  TEXT NOT NULL,
				stream      TEXT NOT NULL DEFAULT 'main',
				item_name   TEXT NOT NULL DEFAULT '',
				unit        TEXT NOT NULL,
				variant     TEXT NOT NULL,
				status      TEXT NOT NULL DEFAULT '',
				target_hash TEXT NOT NULL DEFAULT '',
				content_hash TEXT NOT NULL DEFAULT '',
				review_state TEXT NOT NULL DEFAULT '',
				decided_by  TEXT NOT NULL DEFAULT '',
				decided_at  TEXT NOT NULL DEFAULT '',
				note        TEXT NOT NULL DEFAULT '',
				parked      BOOLEAN NOT NULL DEFAULT FALSE,
				assignee    TEXT NOT NULL DEFAULT '',
				updated     TEXT NOT NULL DEFAULT '',
				updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, stream, item_name, unit, variant)
			);
			CREATE INDEX IF NOT EXISTS idx_unit_decisions_project ON unit_decisions(project_id, stream);
			-- The BASIS a decision blessed: the hash of the SOURCE it approved a
			-- translation FOR. Declared in the CREATE above for an empty database
			-- and added here for one that already ran 16-23. Empty means the
			-- record predates the column: unknown, which a reader must not read
			-- as a source that has moved.
			ALTER TABLE unit_decisions ADD COLUMN IF NOT EXISTS content_hash TEXT NOT NULL DEFAULT '';

			-- Channel-slug equivalence proposals.
			--
			-- A workspace owns EQUIVALENCE, never RESOLUTION. Two projects that
			-- spell one channel differently fragment the workspace's view of it,
			-- and the server can see that where neither project can, though it may
			-- not fix it: a project resolves its own coordinates from its own
			-- recipe, offline, and a server that rewrote a pushed slug would make
			-- the same recipe mean different things depending on whether it had
			-- been connected.
			--
			-- So the row is a PROPOSAL: these two slugs look like one channel,
			-- here is the evidence, here is who raised it. Nothing reads it to
			-- resolve anything.
			CREATE TABLE IF NOT EXISTS channel_alias_proposals (
				workspace_id     TEXT NOT NULL,
				profile          TEXT NOT NULL DEFAULT '',
				proposed_channel TEXT NOT NULL,
				existing_channel TEXT NOT NULL,
				evidence         TEXT NOT NULL DEFAULT '',
				project_id       TEXT NOT NULL DEFAULT '',
				collection       TEXT NOT NULL DEFAULT '',
				status           TEXT NOT NULL DEFAULT 'proposed',
				judged_by        TEXT NOT NULL DEFAULT '',
				judged_at        TIMESTAMPTZ,
				created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (workspace_id, profile, proposed_channel, existing_channel)
			);
			CREATE INDEX IF NOT EXISTS idx_channel_alias_proposals_ws
				ON channel_alias_proposals(workspace_id, status);
			-- ---- folded from version 22: the judgement's own instant ----
			-- updated_at moves on every re-sighting of the same fragmentation,
			-- so it cannot say when a reviewer settled the row. These two say
			-- who settled it and when, and nothing but a judgement writes them.
			ALTER TABLE channel_alias_proposals ADD COLUMN IF NOT EXISTS judged_by TEXT NOT NULL DEFAULT '';
			ALTER TABLE channel_alias_proposals ADD COLUMN IF NOT EXISTS judged_at TIMESTAMPTZ;

			-- ---- folded from version 17: stream scope for convergence runs ----
			-- convergence_runs.stream, declared in that table's CREATE above. A
			-- run's derivation and produce are scoped to one stream, because
			-- targets are per-stream overlays.

			-- ---- folded from version 18: source word count computed at write, not derived per read ----
			-- blocks.word_count, declared in that table's CREATE above. It is the
			-- one nullable column in blocks: NULL is "written before the column
			-- existed", which is what tells a reader to decode source_json and
			-- fill it in.

			-- ---- folded from version 19: the access ladder stops borrowing the review ladder's words ----
			-- blocks.access, declared in that table's CREATE above under its own
			-- name. Version 19 renamed the column from status and rewrote its
			-- values (draft → open, in_review → restricted); a database built
			-- from this baseline never has the old name to rename, which is why
			-- the fold is a column declaration rather than a guarded ALTER.
		`,
	},
	{
		Version:     25,
		Description: "where a collection's strings can be read in place",
		SQL: `
			-- The preview host a collection declares: the component explorer or
			-- running site that shows its strings as a reader sees them. Per
			-- collection because a repository publishes one per surface it
			-- ships, and carried on the collection's context entry, so these
			-- move with the coordinates and the voice rather than separately.
			--
			-- kind names how a view is FOUND within that host (a Storybook
			-- resolves an item to a story through its published index), and a
			-- kind this server does not recognise is stored and served
			-- unchanged: the client decides what it can read.
			ALTER TABLE collections ADD COLUMN IF NOT EXISTS preview_kind TEXT NOT NULL DEFAULT '';
			ALTER TABLE collections ADD COLUMN IF NOT EXISTS preview_url  TEXT NOT NULL DEFAULT '';
		`,
	},
	{
		Version:     26,
		Description: "the ship gate's per-block verdict, so the dashboard counts instead of reading",
		SQL: `
			-- One row per (block, locale) whose target has been judged against
			-- the ship gate: fails is true where the target carries an
			-- error-severity check finding or breaches terminology governance.
			-- The dashboard's failing and compliant counts are aggregates over
			-- this table, so a load costs two grouped queries rather than a
			-- read of every block the customer has.
			--
			-- Derived, and says so: gate fingerprints the governance in force,
			-- basis names the source hash and target revision judged. A row
			-- whose gate or basis has moved is not counted: it is recomputed.
			-- Nothing here is authored, so the table can be truncated at any
			-- time and the next dashboard load rebuilds it.
			--
			-- Not partitioned, unlike the overlay families: it holds one small
			-- fixed-width row per translated pair and is only ever read by the
			-- two project-scoped aggregates below.
			CREATE TABLE IF NOT EXISTS ship_verdicts (
				project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				stream     TEXT NOT NULL DEFAULT 'main',
				block_id   TEXT NOT NULL,
				locale     TEXT NOT NULL,
				gate       TEXT NOT NULL DEFAULT '',
				basis      TEXT NOT NULL DEFAULT '',
				fails      BOOLEAN NOT NULL DEFAULT FALSE,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, stream, block_id, locale)
			);
			CREATE INDEX IF NOT EXISTS idx_ship_verdicts_scope
				ON ship_verdicts(project_id, stream, locale);
		`,
	},
	{
		Version:     27,
		Description: "a stream owns its content, and a file is identified by what it is rather than where it sits",
		SQL: `
			-- Two changes, one migration, because they rewrite the same keys.
			--
			-- ONE: a stream owns its content. CreateStream copied item rows and
			-- left blocks shared, so two branches could not differ in source by
			-- construction: a branch was a translation branch, and MergeStream
			-- counted changes without moving any. blocks gains the stream every
			-- table describing a block already carries.
			--
			-- TWO: a file is identified by what it IS. items_pkey was
			-- (project_id, stream, name), making the PATH the primary key, so a
			-- rename was a delete and an insert, and every row pointing at the
			-- old name was orphaned. unit_decisions had the path in its key too,
			-- which is how renaming a file dropped its approvals.
			--
			-- The id is the identity and is STABLE ACROSS STREAMS: branching
			-- copies rows verbatim under a new stream, so the same unit has the
			-- same id on every branch that holds it. That is what lets a diff
			-- or a merge compare branches by key instead of guessing by content,
			-- and it is why a branch inherits its parent's approvals rather than
			-- starting ungoverned.

			-- ---- items: the id becomes the key, the path becomes an address ----
			-- Existing rows may carry '' (the column's old default) or an id
			-- minted per stream by CreateStream. Fill the blanks first.
			UPDATE items SET id = substr(md5(random()::text || project_id || stream || name), 1, 12)
				WHERE id = '';

			ALTER TABLE items DROP CONSTRAINT IF EXISTS items_pkey;
			ALTER TABLE items ADD PRIMARY KEY (project_id, stream, id);
			-- A path still addresses at most one item within a stream. It is a
			-- constraint on the address, no longer the identity.
			CREATE UNIQUE INDEX IF NOT EXISTS idx_items_stream_name
				ON items(project_id, stream, name);

			-- ---- blocks: per stream, and pointing at the item's id ----
			ALTER TABLE blocks ADD COLUMN IF NOT EXISTS stream  TEXT NOT NULL DEFAULT 'main';
			ALTER TABLE blocks ADD COLUMN IF NOT EXISTS item_id TEXT NOT NULL DEFAULT '';

			UPDATE blocks b SET item_id = i.id
				FROM items i
				WHERE i.project_id = b.project_id
				  AND i.stream     = b.stream
				  AND i.name       = b.item_name
				  AND b.item_id    = '';

			-- Every block written before this migration belongs to the stream it
			-- was authored on, which is main; the column default says so.
			--
			-- Branches that already exist are NOT carried over, and nothing here
			-- tries to. They held no blocks of their own (they read main's)
			-- and MergeStream never moved a row, so there is no branch work to
			-- preserve. Content this migration reshapes is re-pushed rather than
			-- repaired: see bowrain-infra/docs/runbooks/data-reset.md.

			ALTER TABLE blocks DROP CONSTRAINT IF EXISTS blocks_pkey;
			ALTER TABLE blocks ADD PRIMARY KEY (project_id, stream, id);

			-- The reader's id is unique within an item, and an item is within a
			-- stream: the old index spanned streams and would now collide
			-- between a branch and its parent.
			DROP INDEX IF EXISTS idx_blocks_source_id;
			CREATE UNIQUE INDEX IF NOT EXISTS idx_blocks_source_id
				ON blocks(project_id, stream, item_name, source_id) WHERE source_id != '';
			CREATE INDEX IF NOT EXISTS idx_blocks_item_id ON blocks(project_id, stream, item_id);
			DROP INDEX IF EXISTS idx_blocks_item;
			CREATE INDEX IF NOT EXISTS idx_blocks_item ON blocks(project_id, stream, item_name);

			-- ---- governance follows the file, not its address ----
			ALTER TABLE unit_decisions          ADD COLUMN IF NOT EXISTS item_id TEXT NOT NULL DEFAULT '';
			ALTER TABLE proposed_source_changes ADD COLUMN IF NOT EXISTS item_id TEXT NOT NULL DEFAULT '';
			ALTER TABLE assets                  ADD COLUMN IF NOT EXISTS item_id TEXT NOT NULL DEFAULT '';

			UPDATE unit_decisions d SET item_id = i.id
				FROM items i
				WHERE i.project_id = d.project_id AND i.stream = d.stream
				  AND i.name = d.item_name AND d.item_id = '';
			UPDATE proposed_source_changes c SET item_id = i.id
				FROM items i
				WHERE i.project_id = c.project_id AND i.stream = c.stream
				  AND i.name = c.item_name AND c.item_id = '';
			UPDATE assets a SET item_id = i.id
				FROM items i
				WHERE i.project_id = a.project_id AND i.stream = a.stream
				  AND i.name = a.item_name AND a.item_id = '';

			-- The ledger is the one table that cannot be re-derived from a push,
			-- so its key is the one that most needed the path out of it.
			ALTER TABLE unit_decisions DROP CONSTRAINT IF EXISTS unit_decisions_pkey;
			ALTER TABLE unit_decisions ADD PRIMARY KEY (project_id, stream, item_id, unit, variant);
			CREATE INDEX IF NOT EXISTS idx_unit_decisions_item
				ON unit_decisions(project_id, stream, item_id);
		`,
	},
	{
		Version:     28,
		Description: "drift activity and notifications carry the voice spelling",
		SQL: `
			-- The drift activity and notification types were renamed with the
			-- subsystem, identifier AND stored value. Both columns are plain
			-- TEXT with no CHECK, so rows written before the rename keep the
			-- former spelling and the two coexist indefinitely: nothing errors,
			-- and nothing today reads the value exactly: the feed matches on
			-- "drift" being present and preferences key on the category. The
			-- first consumer that does match exactly would silently miss every
			-- historical row.
			--
			-- One statement each, and idempotent by construction: the second
			-- pass finds nothing left to update.
			UPDATE activities    SET type = 'voice.drift' WHERE type = 'brand.drift';
			UPDATE notifications SET type = 'voice.drift' WHERE type = 'brand.drift';
		`,
	},
	{
		Version:     29,
		Description: "pending recipe changes: a proposal in flight to the working tree",
		SQL: `
			-- A coordinate is minted by the recipe and by nothing else, so an
			-- approved axis becomes real only once kapi.yaml says so and a push
			-- carries it. This table holds the proposal in between: one row per
			-- recipe field an approval wants set, waiting for the next pull to
			-- write it into the working tree where git reviews it.
			--
			-- It is deliberately NOT a registry of axes. A row exists to become
			-- a recipe line and is spent when applied; nothing reads it to
			-- decide what a project's context space is. That question is
			-- answered from the collections a push reconciled, which is the
			-- recipe's own account of itself.
			CREATE TABLE IF NOT EXISTS pending_recipe_changes (
				id           TEXT PRIMARY KEY,
				workspace_id TEXT NOT NULL,
				project_id   TEXT NOT NULL,
				-- The dotted recipe path and its JSON value, exactly as a
				-- kapi apply recipe entry carries them: the client hands
				-- these to the same setRecipeField the local fix loop uses,
				-- so there is one allowlist and one writer, not two.
				path         TEXT NOT NULL,
				value        TEXT NOT NULL,
				status       TEXT NOT NULL DEFAULT 'pending',
				created_by   TEXT NOT NULL DEFAULT '',
				created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				applied_at   TIMESTAMPTZ
			);
			-- The pull asks one question, what is pending for this project,
			-- so that is the index.
			CREATE INDEX IF NOT EXISTS idx_pending_recipe_changes_project
				ON pending_recipe_changes(project_id, status);
			-- One pending row per path: approving the same axis twice restates
			-- the same recipe line rather than queueing it again.
			CREATE UNIQUE INDEX IF NOT EXISTS idx_pending_recipe_changes_path
				ON pending_recipe_changes(project_id, path) WHERE status = 'pending';
		`,
	},
	{
		Version:     30,
		Description: "the ledger records the source the platform last drafted a unit against",
		SQL: `
			-- The source hash the platform's latest draft of the unit was made
			-- against, kept beside the decision on the same row and never
			-- written over it. A decision's content_hash is what a person
			-- blessed; this is what the platform last translated. The two
			-- differ for exactly as long as a rewritten source waits on a
			-- re-review: the decision stays stale, and this column is what
			-- tells the convergence loop that the re-draft is done and the
			-- unit now waits on a reviewer rather than on another pass.
			-- Empty means the platform has recorded no draft for the unit.
			ALTER TABLE unit_decisions ADD COLUMN IF NOT EXISTS draft_basis TEXT NOT NULL DEFAULT '';
		`,
	},
	{
		Version:     31,
		Description: "the ledger records the governing context a decision was made under",
		SQL: `
			-- The fingerprint of the voice guidance and term rules in force
			-- when the decision was made (state.UnitState.GoverningFingerprint,
			-- the value every translation producer stamps on a target's
			-- origin), kept beside the decision so a pull returns what the
			-- project's record says and the content memory can carry it for
			-- an answer read out of a format with no slot for provenance.
			-- Empty means the record carries none: an ungoverned decision, or
			-- a row written before the column existed. Additive, with no
			-- rewrite of existing rows.
			ALTER TABLE unit_decisions ADD COLUMN IF NOT EXISTS governing_fingerprint TEXT NOT NULL DEFAULT '';
		`,
	},
}
