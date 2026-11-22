//go:build integration

package migrations_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/migrations"
	pgterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// Terms written before the normalized key existed hold an empty key. The
// upgrade fills it in for every workspace, across more than one batch, and
// the normalized and fuzzy tiers find the rows by it.
func TestUpgradeBackfillsNormalizedText(t *testing.T) {
	for _, tc := range []struct {
		name   string
		before int
	}{
		{"before the column", 10},
		{"column left empty", 12},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := databaseBeforeTermsVersion(t, tc.before)
			ctx := t.Context()
			rows := map[string]string{"latte": "  Café\t\tAu  Lait ", "blank": "   "}
			for i := range 1003 {
				rows[fmt.Sprintf("filler-%04d", i)] = fmt.Sprintf("Filler Term %04d", i)
			}
			for id, text := range rows {
				for _, ws := range []string{"oak", "ash"} {
					_, err := db.ExecContext(ctx, `INSERT INTO tb_concepts (workspace_id, id) VALUES ($1, $2)`, ws, id)
					require.NoError(t, err)
					_, err = db.ExecContext(ctx, `INSERT INTO tb_terms (workspace_id, concept_id, text, text_lower, locale) VALUES ($1, $2, $3, lower($3), 'en')`, ws, id, text)
					require.NoError(t, err)
				}
			}

			require.NoError(t, migrations.Apply(db, nil))
			var empty int
			require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM tb_terms WHERE text_normalized = ''`).Scan(&empty))
			assert.Equal(t, 2, empty, "only the whitespace terms normalize to the empty string")
			var key string
			require.NoError(t, db.QueryRowContext(ctx, `SELECT text_normalized FROM tb_terms WHERE workspace_id = 'ash' AND concept_id = 'filler-1002'`).Scan(&key))
			assert.Equal(t, "filler term 1002", key)

			for _, ws := range []string{"oak", "ash"} {
				store, err := pgterms.NewPostgresStoreFromDB(db, ws)
				require.NoError(t, err)
				matches, err := store.Lookup(ctx, "CAFÉ AU LAIT", terms.LookupOptions{SourceLocale: "en", MatchModes: []model.MatchStrategy{model.MatchStrategyNormalized}})
				require.NoError(t, err)
				assert.Equal(t, []string{"latte"}, terms.MatchedConceptIDs(matches), ws)
				matches, err = store.Lookup(ctx, "cafe au lai", terms.LookupOptions{SourceLocale: "en", MatchModes: []model.MatchStrategy{model.MatchStrategyFuzzy}})
				require.NoError(t, err)
				assert.Equal(t, []string{"latte"}, terms.MatchedConceptIDs(matches), ws)
			}

			_, err := db.ExecContext(ctx, `DELETE FROM tb_schema_migrations WHERE version >= 12`)
			require.NoError(t, err)
			require.NoError(t, migrations.Apply(db, nil))
			require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM tb_terms WHERE text_normalized = ''`).Scan(&empty))
			assert.Equal(t, 2, empty)
		})
	}
}
