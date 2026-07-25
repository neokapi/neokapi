
		ALTER TABLE tm_entry_entities ADD COLUMN concept_id TEXT NOT NULL DEFAULT '';
		CREATE INDEX IF NOT EXISTS idx_entities_concept ON tm_entry_entities(concept_id);
		