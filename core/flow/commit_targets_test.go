package flow

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
)

// commit-targets writes the target TargetRuns reads for the run's locale,
// whatever key it is filed under: no language (a run whose flow needs no
// target language commits under the empty locale, and the KBF reader files a
// bundle's unlabelled edition there), a malformed locale x/text reads in two steps,
// and the source language.
func TestCommitTargets_CommitsTheTargetTargetRunsReads(t *testing.T) {
	twoStep := model.LocaleID("AA-u-00-00-u-00-00")
	once := model.NormalizeLocale(twoStep)
	locales := []model.LocaleID{"", "en-US", "fr", twoStep, once, model.NormalizeLocale(once)}
	ctx := context.Background()

	for _, filed := range locales {
		for _, loc := range locales {
			b := model.NewBlock("b1", "Hello")
			b.SourceLocale = "en-US"
			b.SetTargetRuns(filed, []model.Run{model.TextR("filed")})
			b.StampTargetProvenance(filed, model.TargetStatusTranslated, model.Origin{Kind: model.OriginAI})

			sess, err := blockstore.NewMemoryStore().Begin(ctx)
			require.NoError(t, err)
			kind := blockstore.TargetOverlayKind(loc)
			require.NoError(t, newCommitTargetsTool(loc).commitOne(ctx, sess, kind, &model.Part{Type: model.PartBlock, Resource: b}))

			var got []blockstore.TargetOverlay
			for ov, lerr := range sess.ListOverlays(kind) {
				require.NoError(t, lerr)
				var o blockstore.TargetOverlay
				require.NoError(t, json.Unmarshal(ov.Payload, &o))
				got = append(got, o)
			}
			if len(b.TargetRuns(loc)) == 0 {
				assert.Empty(t, got, "filed under %q, committed under %q", filed, loc)
				continue
			}
			require.Len(t, got, 1, "filed under %q, committed under %q", filed, loc)
			assert.Equal(t, "filed", got[0].TargetText(), "filed under %q, committed under %q", filed, loc)
			assert.Equal(t, string(model.TargetStatusTranslated), got[0].Status, "filed under %q, committed under %q", filed, loc)
		}
	}
}
