package store

import "github.com/neokapi/neokapi/bowrain/storage"

// storeMigrations defines the complete PostgreSQL content store schema.
// Bowrain is not yet in production; there is no migration history to
// preserve, so we keep a single baseline migration that represents the
// current design.
var storeMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "content store schema (baseline)",
		SQL: `
			-- Projects
			CREATE TABLE projects (
				id                      TEXT PRIMARY KEY,
				name                    TEXT NOT NULL,
				default_source_language TEXT NOT NULL DEFAULT '',
				target_languages        TEXT NOT NULL DEFAULT '',
				target_language_mode    TEXT NOT NULL DEFAULT 'defined',
				default_stream          TEXT NOT NULL DEFAULT '',
				dashboard_visibility    TEXT NOT NULL DEFAULT 'private',
				properties              TEXT NOT NULL DEFAULT '{}',
				workspace_id            TEXT NOT NULL DEFAULT '',
				archived                BOOLEAN NOT NULL DEFAULT FALSE,
				archived_at             TIMESTAMPTZ,
				created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_projects_workspace ON projects(workspace_id);

			-- Streams
			CREATE TABLE streams (
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
				created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				created_by  TEXT NOT NULL DEFAULT '',
				PRIMARY KEY (project_id, name)
			);

			CREATE TABLE stream_members (
				project_id TEXT NOT NULL,
				stream     TEXT NOT NULL,
				user_id    TEXT NOT NULL,
				added_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, stream, user_id),
				FOREIGN KEY (project_id, stream) REFERENCES streams(project_id, name) ON DELETE CASCADE
			);

			CREATE TABLE stream_tags (
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
			CREATE UNIQUE INDEX idx_stream_tags_unique ON stream_tags(project_id, stream, name);
			CREATE INDEX idx_stream_tags_stream ON stream_tags(project_id, stream);
			CREATE INDEX idx_stream_tags_project_kind ON stream_tags(project_id, kind);

			-- Collections
			CREATE TABLE collections (
				id               TEXT NOT NULL,
				project_id       TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				name             TEXT NOT NULL,
				kind             TEXT NOT NULL DEFAULT 'uploaded',
				item_label       TEXT NOT NULL DEFAULT 'item',
				is_default       BOOLEAN NOT NULL DEFAULT FALSE,
				stream           TEXT NOT NULL DEFAULT '',
				connector_config TEXT NOT NULL DEFAULT '{}',
				created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, id)
			);
			CREATE INDEX idx_collections_project ON collections(project_id);
			CREATE UNIQUE INDEX idx_collections_default ON collections(project_id)
				WHERE is_default = TRUE;

			-- Items
			CREATE TABLE items (
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
			CREATE INDEX idx_items_project ON items(project_id);
			CREATE INDEX idx_items_project_stream ON items(project_id, stream);
			CREATE INDEX idx_items_collection ON items(project_id, collection_id);
			CREATE UNIQUE INDEX idx_items_id ON items(project_id, stream, id);

			-- Blocks hold source content + project metadata only.
			-- Targets and annotations live in their own kind-specific
			-- tables (#403/#405) so each access pattern gets the right
			-- indexes and a single source of truth.
			CREATE TABLE blocks (
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
				properties   TEXT NOT NULL DEFAULT '{}',
				stored_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, id)
			);
			CREATE INDEX idx_blocks_content_hash ON blocks(content_hash);
			CREATE INDEX idx_blocks_project ON blocks(project_id);
			CREATE INDEX idx_blocks_item ON blocks(project_id, item_name);
			CREATE UNIQUE INDEX idx_blocks_source_id ON blocks(project_id, item_name, source_id)
				WHERE source_id != '';

			-- Change log
			CREATE TABLE change_log (
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
			CREATE INDEX idx_changelog_project_seq ON change_log(project_id, seq);
			CREATE INDEX idx_changelog_project_locale ON change_log(project_id, locale, seq);
			CREATE INDEX idx_changelog_stream ON change_log(project_id, stream, seq);
			CREATE INDEX idx_changelog_correlation ON change_log(project_id, correlation_id);

			-- Block history: append-only prior content per (block, locale). The
			-- attribution columns (actor_role/edit_reason/correlation_id) make it
			-- audit-grade and let a whole push/merge be reverted as a unit.
			CREATE TABLE block_history (
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
			CREATE INDEX idx_block_history_lookup ON block_history(project_id, block_id, locale);
			CREATE INDEX idx_block_history_stream ON block_history(project_id, stream, block_id, locale);
			CREATE INDEX idx_block_history_correlation ON block_history(project_id, correlation_id);

			-- Block notes
			CREATE TABLE block_notes (
				id         TEXT PRIMARY KEY,
				project_id TEXT NOT NULL,
				block_id   TEXT NOT NULL,
				author     TEXT NOT NULL DEFAULT '',
				text       TEXT NOT NULL,
				stream     TEXT NOT NULL DEFAULT 'main',
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_block_notes_lookup ON block_notes(project_id, block_id);
			CREATE INDEX idx_block_notes_stream ON block_notes(project_id, stream, block_id);

			-- Versions
			CREATE TABLE versions (
				id          TEXT PRIMARY KEY,
				project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				label       TEXT NOT NULL,
				description TEXT NOT NULL DEFAULT '',
				block_count INTEGER NOT NULL DEFAULT 0,
				created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_versions_project ON versions(project_id);

			CREATE TABLE version_blocks (
				version_id   TEXT NOT NULL REFERENCES versions(id) ON DELETE CASCADE,
				block_id     TEXT NOT NULL,
				content_hash TEXT NOT NULL,
				PRIMARY KEY (version_id, block_id)
			);

			-- Assets (Bowrain AD-007)
			CREATE TABLE assets (
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
			CREATE INDEX idx_assets_project_item ON assets(project_id, item_name);
			CREATE UNIQUE INDEX idx_assets_blob ON assets(project_id, blob_key)
				WHERE stream = 'main';

			CREATE TABLE asset_variants (
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

			CREATE TABLE block_asset_refs (
				project_id TEXT NOT NULL,
				block_id   TEXT NOT NULL,
				asset_id   TEXT NOT NULL,
				ref_type   TEXT NOT NULL DEFAULT 'embedded',
				stream     TEXT NOT NULL DEFAULT 'main',
				PRIMARY KEY (project_id, block_id, asset_id)
			);
			CREATE INDEX idx_block_asset_refs_asset ON block_asset_refs(project_id, asset_id);

			-- Automations
			CREATE TABLE automation_rules (
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
			CREATE INDEX idx_automation_rules_project ON automation_rules(project_id);

			CREATE TABLE automation_history (
				id         TEXT PRIMARY KEY,
				rule_id    TEXT NOT NULL,
				project_id TEXT NOT NULL,
				event_id   TEXT NOT NULL DEFAULT '',
				status     TEXT NOT NULL,
				error      TEXT NOT NULL DEFAULT '',
				started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				ended_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_automation_history_project ON automation_history(project_id);
			CREATE INDEX idx_automation_history_rule ON automation_history(rule_id);

			-- Automation runs (Bowrain AD-013)
			CREATE TABLE automation_runs (
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
			CREATE INDEX idx_automation_runs_project ON automation_runs(project_id, started_at DESC);

			CREATE TABLE automation_steps (
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
			CREATE INDEX idx_automation_steps_run ON automation_steps(run_id);

			CREATE TABLE automation_logs (
				id        TEXT PRIMARY KEY,
				step_id   TEXT NOT NULL,
				run_id    TEXT NOT NULL,
				level     TEXT NOT NULL DEFAULT 'info',
				message   TEXT NOT NULL,
				data      JSONB NOT NULL DEFAULT '{}',
				timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_automation_logs_step ON automation_logs(step_id, timestamp);
			CREATE INDEX idx_automation_logs_run ON automation_logs(run_id, timestamp);

			-- Review queue
			CREATE TABLE review_items (
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
			CREATE INDEX idx_review_items_project_status ON review_items(project_id, status);
			CREATE INDEX idx_review_items_project_type ON review_items(project_id, type);
			CREATE INDEX idx_review_items_assigned ON review_items(project_id, assigned_to);
			CREATE INDEX idx_review_items_confidence ON review_items(project_id, confidence);

			CREATE TABLE rejected_terms (
				project_id  TEXT NOT NULL,
				term_text   TEXT NOT NULL,
				locale      TEXT NOT NULL,
				rejected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, term_text, locale)
			);

			CREATE TABLE dnt_entries (
				project_id  TEXT NOT NULL,
				text        TEXT NOT NULL,
				entity_type TEXT NOT NULL DEFAULT '',
				locale      TEXT NOT NULL,
				source      TEXT NOT NULL DEFAULT '',
				created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, text, locale)
			);

			-- Notifications
			CREATE TABLE notifications (
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
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX idx_notifications_user ON notifications(user_id, read, created_at DESC);

			CREATE TABLE notification_preferences (
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
			CREATE INDEX idx_notif_pref_user ON notification_preferences(user_id, workspace_id);

			-- Activities
			CREATE TABLE activities (
				id           TEXT PRIMARY KEY,
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
			CREATE INDEX idx_activities_workspace ON activities(workspace_id, created_at DESC);
			CREATE INDEX idx_activities_project ON activities(workspace_id, project_id, created_at DESC);
			CREATE INDEX idx_activities_actor ON activities(workspace_id, actor_id, created_at DESC);

			-- Tasks
			CREATE TABLE tasks (
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
			CREATE INDEX idx_tasks_workspace ON tasks(workspace_id, status, created_at DESC);
			CREATE INDEX idx_tasks_project ON tasks(workspace_id, project_id, status);
			CREATE INDEX idx_tasks_assignee ON tasks(workspace_id, assignee_id, status);

			-- Digest
			CREATE TABLE digest_settings (
				user_id      TEXT NOT NULL,
				workspace_id TEXT NOT NULL,
				frequency    TEXT NOT NULL DEFAULT 'daily',
				quiet_start  TEXT NOT NULL DEFAULT '',
				quiet_end    TEXT NOT NULL DEFAULT '',
				timezone     TEXT NOT NULL DEFAULT 'UTC',
				PRIMARY KEY (user_id, workspace_id)
			);

			CREATE TABLE digest_state (
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
			CREATE TABLE audit_log (
				id            BIGSERIAL PRIMARY KEY,
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
			CREATE INDEX idx_audit_log_project ON audit_log(project_id, created_at DESC);
			CREATE INDEX idx_audit_log_workspace ON audit_log(workspace_id, created_at DESC);
			CREATE INDEX idx_audit_log_type ON audit_log(workspace_id, event_type, created_at DESC);
			CREATE INDEX idx_audit_log_actor ON audit_log(actor, created_at DESC);
			CREATE INDEX idx_audit_log_chain ON audit_log(chain_key, id);

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

			CREATE TRIGGER audit_log_append_only
				BEFORE UPDATE OR DELETE ON audit_log
				FOR EACH ROW EXECUTE FUNCTION audit_log_no_mutate();

			-- Project flow definitions (Bowrain AD-013). Server-side, editable
			-- flow graphs (reader → tool(s) → writer) that automation run_flow
			-- actions reference by id. graph holds the full FlowDefinition JSON
			-- (nodes, edges, stages, positions). Built-in flows are not stored
			-- here; they are merged in at the API layer.
			CREATE TABLE flow_definitions (
				id          TEXT NOT NULL,
				project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				name        TEXT NOT NULL,
				description TEXT NOT NULL DEFAULT '',
				graph       JSONB NOT NULL DEFAULT '{}',
				created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, id)
			);
			CREATE INDEX idx_flow_definitions_project ON flow_definitions(project_id, name);

			-- Leader leases (distributed coordination)
			CREATE TABLE leader_leases (
				name       TEXT PRIMARY KEY,
				holder_id  TEXT NOT NULL,
				expires_at TIMESTAMPTZ NOT NULL
			);

			-- Pending pushes (push completion tracking)
			CREATE TABLE pending_pushes (
				push_id    TEXT PRIMARY KEY,
				project_id TEXT NOT NULL,
				items      TEXT NOT NULL DEFAULT '',
				ws_slug    TEXT NOT NULL DEFAULT '',
				actor      TEXT NOT NULL DEFAULT '',
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);

			-- Activity read state (cross-device sync)
			CREATE TABLE activity_state (
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
			CREATE TABLE translations (
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
			CREATE TABLE translations_p0 PARTITION OF translations FOR VALUES WITH (MODULUS 8, REMAINDER 0);
			CREATE TABLE translations_p1 PARTITION OF translations FOR VALUES WITH (MODULUS 8, REMAINDER 1);
			CREATE TABLE translations_p2 PARTITION OF translations FOR VALUES WITH (MODULUS 8, REMAINDER 2);
			CREATE TABLE translations_p3 PARTITION OF translations FOR VALUES WITH (MODULUS 8, REMAINDER 3);
			CREATE TABLE translations_p4 PARTITION OF translations FOR VALUES WITH (MODULUS 8, REMAINDER 4);
			CREATE TABLE translations_p5 PARTITION OF translations FOR VALUES WITH (MODULUS 8, REMAINDER 5);
			CREATE TABLE translations_p6 PARTITION OF translations FOR VALUES WITH (MODULUS 8, REMAINDER 6);
			CREATE TABLE translations_p7 PARTITION OF translations FOR VALUES WITH (MODULUS 8, REMAINDER 7);
			CREATE INDEX idx_translations_project_locale
				ON translations(project_id, stream, locale, updated_at DESC);
			CREATE INDEX idx_translations_project_block
				ON translations(project_id, stream, block_id);

			-- Semantic annotations (TM hits, term hits, QA findings,
			-- translator notes). Grouped-by queries are the common
			-- access pattern: "all QA findings for this project".
			CREATE TABLE annotations (
				project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				stream     TEXT NOT NULL DEFAULT 'main',
				block_id   TEXT NOT NULL,
				kind       TEXT NOT NULL,
				payload    JSONB NOT NULL DEFAULT '{}'::jsonb,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, stream, block_id, kind)
			) PARTITION BY HASH (project_id);
			CREATE TABLE annotations_p0 PARTITION OF annotations FOR VALUES WITH (MODULUS 8, REMAINDER 0);
			CREATE TABLE annotations_p1 PARTITION OF annotations FOR VALUES WITH (MODULUS 8, REMAINDER 1);
			CREATE TABLE annotations_p2 PARTITION OF annotations FOR VALUES WITH (MODULUS 8, REMAINDER 2);
			CREATE TABLE annotations_p3 PARTITION OF annotations FOR VALUES WITH (MODULUS 8, REMAINDER 3);
			CREATE TABLE annotations_p4 PARTITION OF annotations FOR VALUES WITH (MODULUS 8, REMAINDER 4);
			CREATE TABLE annotations_p5 PARTITION OF annotations FOR VALUES WITH (MODULUS 8, REMAINDER 5);
			CREATE TABLE annotations_p6 PARTITION OF annotations FOR VALUES WITH (MODULUS 8, REMAINDER 6);
			CREATE TABLE annotations_p7 PARTITION OF annotations FOR VALUES WITH (MODULUS 8, REMAINDER 7);
			CREATE INDEX idx_annotations_project_kind
				ON annotations(project_id, stream, kind, updated_at DESC);
			CREATE INDEX idx_annotations_project_block
				ON annotations(project_id, stream, block_id);

			-- Plugin catchall for overlay kinds that don't fit the
			-- purpose-built tables above. Same schema shape as the
			-- former block_overlays; renamed to signal "extension".
			CREATE TABLE overlays_ext (
				project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				stream     TEXT NOT NULL DEFAULT 'main',
				block_id   TEXT NOT NULL,
				kind       TEXT NOT NULL,
				payload    JSONB NOT NULL DEFAULT '{}'::jsonb,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (project_id, stream, block_id, kind)
			) PARTITION BY HASH (project_id);
			CREATE TABLE overlays_ext_p0 PARTITION OF overlays_ext FOR VALUES WITH (MODULUS 8, REMAINDER 0);
			CREATE TABLE overlays_ext_p1 PARTITION OF overlays_ext FOR VALUES WITH (MODULUS 8, REMAINDER 1);
			CREATE TABLE overlays_ext_p2 PARTITION OF overlays_ext FOR VALUES WITH (MODULUS 8, REMAINDER 2);
			CREATE TABLE overlays_ext_p3 PARTITION OF overlays_ext FOR VALUES WITH (MODULUS 8, REMAINDER 3);
			CREATE TABLE overlays_ext_p4 PARTITION OF overlays_ext FOR VALUES WITH (MODULUS 8, REMAINDER 4);
			CREATE TABLE overlays_ext_p5 PARTITION OF overlays_ext FOR VALUES WITH (MODULUS 8, REMAINDER 5);
			CREATE TABLE overlays_ext_p6 PARTITION OF overlays_ext FOR VALUES WITH (MODULUS 8, REMAINDER 6);
			CREATE TABLE overlays_ext_p7 PARTITION OF overlays_ext FOR VALUES WITH (MODULUS 8, REMAINDER 7);
			CREATE INDEX idx_overlays_ext_project_kind
				ON overlays_ext(project_id, stream, kind);
			CREATE INDEX idx_overlays_ext_project_block
				ON overlays_ext(project_id, stream, block_id);
		`,
	},
}
