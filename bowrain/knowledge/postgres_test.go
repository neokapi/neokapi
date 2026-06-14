package knowledge

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresKnowledgeStore_ImplementsInterface(t *testing.T) {
	// Compile-time check that PostgresKnowledgeStore satisfies Store.
	var _ Store = (*PostgresKnowledgeStore)(nil)
}

func TestKgMigrations_Baseline(t *testing.T) {
	// Single clean baseline (pre-launch — no migration history to preserve).
	require.Len(t, kgMigrations, 1)
	assert.Equal(t, 1, kgMigrations[0].Version)
	assert.NotEmpty(t, kgMigrations[0].SQL)

	sql := kgMigrations[0].SQL
	// All seven governance tables are part of the baseline.
	for _, table := range []string{
		"kg_markets", "kg_observations", "kg_comments", "kg_concept_revisions",
		"kg_changesets", "kg_changeset_ops", "kg_changeset_reviews", "kg_pilots",
	} {
		assert.Contains(t, sql, "CREATE TABLE "+table, "missing table %s", table)
	}
	// Workspace-scoped composite primary keys, per the data-model note.
	for _, pk := range []string{
		"PRIMARY KEY (workspace_id, id)",
		"PRIMARY KEY (workspace_id, concept_id, rev)",
		"PRIMARY KEY (workspace_id, changeset_id, seq)",
		"PRIMARY KEY (workspace_id, changeset_id, reviewer)",
		"PRIMARY KEY (workspace_id, changeset_id, project_id, stream)",
	} {
		assert.Contains(t, sql, pk)
	}
	// Postgres column types: JSONB for snapshot/payload/locales, TIMESTAMPTZ stamps.
	assert.Contains(t, sql, "JSONB")
	assert.Contains(t, sql, "TIMESTAMPTZ")
	assert.Contains(t, sql, "BOOLEAN")
}

// TestMarketLocales_Roundtrip exercises the exact JSON path CreateMarket and
// scanMarket use to persist []model.LocaleID in the JSONB locales column.
func TestMarketLocales_Roundtrip(t *testing.T) {
	in := []model.LocaleID{"de-DE", "de-AT", "de-CH"}

	raw, err := json.Marshal(in)
	require.NoError(t, err)
	assert.Equal(t, `["de-DE","de-AT","de-CH"]`, string(raw))

	var out []model.LocaleID
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Equal(t, in, out)
}

// TestConceptRevisionSnapshot_Roundtrip checks that a termbase.Concept stored in
// ConceptRevision.Snapshot (mapped to a JSONB column) survives a marshal/scan
// round-trip and decodes back to the same concept.
func TestConceptRevisionSnapshot_Roundtrip(t *testing.T) {
	concept := termbase.Concept{
		ID:     "c1",
		Domain: "software",
		Terms: []termbase.Term{
			{Text: "sign in", Locale: "en-US", Status: model.TermPreferred},
		},
	}
	snap, err := json.Marshal(concept)
	require.NoError(t, err)

	rev := ConceptRevision{
		WorkspaceID: "ws-1",
		ConceptID:   "c1",
		Rev:         1,
		Snapshot:    json.RawMessage(snap),
		Actor:       "alice",
	}

	// The store writes string(Snapshot) into JSONB and scans the bytes back into
	// a RawMessage; emulate that with an envelope round-trip.
	envelope, err := json.Marshal(rev)
	require.NoError(t, err)
	var back ConceptRevision
	require.NoError(t, json.Unmarshal(envelope, &back))

	var decoded termbase.Concept
	require.NoError(t, json.Unmarshal(back.Snapshot, &decoded))
	assert.Equal(t, concept.ID, decoded.ID)
	assert.Equal(t, "software", decoded.Domain)
	require.Len(t, decoded.Terms, 1)
	assert.Equal(t, model.TermPreferred, decoded.Terms[0].Status)
}

// TestChangeSetOpPayload_Roundtrip checks that an op payload stored in the JSONB
// payload column round-trips and stays valid + ordinary/governed-classifiable.
func TestChangeSetOpPayload_Roundtrip(t *testing.T) {
	raw, err := json.Marshal(ConceptCreatePayload{Concept: termbase.Concept{ID: "c1"}})
	require.NoError(t, err)

	op := ChangeSetOp{
		WorkspaceID: "ws-1",
		ChangesetID: "cs-1",
		Op:          OpConceptCreate,
		Payload:     json.RawMessage(raw),
	}

	require.NoError(t, ValidateOp(op))
	governed, err := IsGovernedOp(op)
	require.NoError(t, err)
	assert.False(t, governed, "concept.create is an ordinary op")

	var decoded ConceptCreatePayload
	require.NoError(t, json.Unmarshal(op.Payload, &decoded))
	assert.Equal(t, "c1", decoded.Concept.ID)
}
