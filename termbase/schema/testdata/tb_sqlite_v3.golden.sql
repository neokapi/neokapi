
		CREATE TABLE IF NOT EXISTS tb_relations (
			id          TEXT PRIMARY KEY,
			source_id   TEXT NOT NULL REFERENCES tb_concepts(id) ON DELETE CASCADE,
			target_id   TEXT NOT NULL REFERENCES tb_concepts(id) ON DELETE CASCADE,
			relation    TEXT NOT NULL,
			note        TEXT NOT NULL DEFAULT '',
			stream      TEXT NOT NULL DEFAULT '',
			valid_from  TEXT,            -- RFC3339, NULL = unbounded
			valid_to    TEXT,
			tags        TEXT NOT NULL DEFAULT '{}',  -- JSON object
			created_at  TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_tb_relations_source ON tb_relations(source_id);
		CREATE INDEX IF NOT EXISTS idx_tb_relations_target ON tb_relations(target_id);
		CREATE INDEX IF NOT EXISTS idx_tb_relations_stream ON tb_relations(stream);

		ALTER TABLE tb_terms ADD COLUMN valid_from TEXT;
		ALTER TABLE tb_terms ADD COLUMN valid_to TEXT;
		ALTER TABLE tb_terms ADD COLUMN tags TEXT NOT NULL DEFAULT '{}';
		