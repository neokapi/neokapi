package memory

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	fw "github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/require"
)

// TestLoadEntriesByIDs_VariantsQueryFailureSurfaces is the finding-4 guard: a
// transient failure reading an entry's variants must surface as an error, not
// as an entry with an empty variant map — which the caller reads as "no
// translation in memory" and re-translates, discarding an approved target.
func TestLoadEntriesByIDs_VariantsQueryFailureSurfaces(t *testing.T) {
	db := pgtest.NewTestDB(t)
	tm, err := NewPostgresStoreFromDB(db, "ws-fault")
	require.NoError(t, err)
	ctx := t.Context()

	entry := fw.Entry{
		ID:          "e1",
		HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Hello"}}},
			"fr": {{Text: &model.TextRun{Text: "Bonjour"}}},
		},
	}
	require.NoError(t, tm.Add(ctx, entry))

	// The entry and its variant must load cleanly before the fault is injected.
	got, ok, err := tm.GetEntry(ctx, "e1")
	require.NoError(t, err)
	require.True(t, ok)
	require.NotEmpty(t, got.Variants, "variant must load before the fault is injected")

	// Break the variants read only — tm_entries is untouched — so the failure is
	// isolated to the variants query the old code swallowed.
	_, err = db.ExecContext(ctx, `ALTER TABLE tm_variants RENAME COLUMN coded TO coded_broken`)
	require.NoError(t, err)

	_, _, err = tm.GetEntry(ctx, "e1")
	require.Error(t, err, "a failed variants read must surface, not degrade to empty variants")
}
