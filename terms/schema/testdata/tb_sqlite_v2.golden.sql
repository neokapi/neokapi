
		ALTER TABLE tb_concepts ADD COLUMN source TEXT NOT NULL DEFAULT 'terminology';
		ALTER TABLE tb_terms ADD COLUMN competitor_term INTEGER NOT NULL DEFAULT 0;
		