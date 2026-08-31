package store

import (
	"slices"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedShipGateProject stores two translatable blocks in one collection and one
// in another, each with a French target, plus a non-translatable block that no
// verdict should ever be asked for.
func seedShipGateProject(t *testing.T, s *PostgresStore) *platstore.Project {
	t.Helper()
	ctx := t.Context()
	p := &platstore.Project{
		Name:                  "ship gate",
		DefaultSourceLanguage: model.LocaleEnglish,
		TargetLanguages:       []model.LocaleID{model.LocaleFrench},
		Properties:            map[string]string{},
	}
	require.NoError(t, s.CreateProject(ctx, p))
	require.NoError(t, s.StoreItem(ctx, p.ID, "main", &platstore.Item{
		Name: "a.json", Format: "json", ItemType: "file", CollectionID: "col-a",
	}))
	require.NoError(t, s.StoreItem(ctx, p.ID, "main", &platstore.Item{
		Name: "b.json", Format: "json", ItemType: "file", CollectionID: "col-b",
	}))

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "a.json", shipGateItemA("traduit")))
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "b.json",
		[]*model.Block{shipGateBlock("b4", "four", "traduit four", true)}))
	return p
}

// shipGateItemA is a.json's three blocks, with b1's target under the caller's
// control. Rewriting one block of an item means storing the item's blocks
// again, so the fixture is a function rather than a literal.
func shipGateItemA(b1Target string) []*model.Block {
	return []*model.Block{
		shipGateBlock("b1", "one", b1Target, true),
		shipGateBlock("b2", "two", "traduit two", true),
		shipGateBlock("b3", "three", "", false),
	}
}

// shipGateBlock builds one block whose source_id the store keeps, so a test can
// name the pairs a rollup reports. The store mints its own project-unique block
// ids, so the caller's id survives as source_id and nothing else.
func shipGateBlock(sourceID, text, target string, translatable bool) *model.Block {
	b := &model.Block{ID: sourceID, Translatable: translatable}
	b.SetSourceText(text)
	if translatable {
		b.SetTargetText(model.LocaleFrench, target)
	}
	return b
}

// shipGateNames maps the store's minted block ids to the source ids the
// fixtures name them by, and back, so an assertion reads b1/b2/b4 rather than a
// mint.
func shipGateNames(t *testing.T, s *PostgresStore, projectID string) (names, ids map[string]string) {
	t.Helper()
	blocks, err := s.GetBlocks(t.Context(), platstore.BlockQuery{ProjectID: projectID, Stream: "main"})
	require.NoError(t, err)
	names = make(map[string]string, len(blocks))
	ids = make(map[string]string, len(blocks))
	for _, b := range blocks {
		names[b.Block.ID] = b.SourceID
		ids[b.SourceID] = b.Block.ID
	}
	return names, ids
}

func shipGateQuery(projectID, gate string, scores ...platstore.ShipGateScore) platstore.ShipGateQuery {
	return platstore.ShipGateQuery{
		ProjectID: projectID, Stream: "main", Gate: gate,
		Locales: []string{string(model.LocaleFrench)}, Scores: scores,
	}
}

// storeVerdicts records the fails value against every stale pair the rollup
// named, which is what the dashboard's derivation does once it has judged them.
func storeVerdicts(t *testing.T, s *PostgresStore, projectID, gate string, stale []platstore.ShipGateStale, fails map[string]bool) {
	t.Helper()
	verdicts := make([]platstore.ShipGateVerdict, 0, len(stale))
	for _, st := range stale {
		verdicts = append(verdicts, platstore.ShipGateVerdict{
			ShipGateRef: st.ShipGateRef, Basis: st.Basis, Fails: fails[st.BlockID],
		})
	}
	require.NoError(t, s.PutShipGateVerdicts(t.Context(), projectID, "main", gate, verdicts))
}

// staleNames lists the stale pairs by the source id their fixture named them
// by, sorted so the assertion does not depend on the mint's ordering.
func staleNames(names map[string]string, stale []platstore.ShipGateStale) []string {
	out := make([]string, 0, len(stale))
	for _, st := range stale {
		out = append(out, names[st.BlockID])
	}
	slices.Sort(out)
	return out
}

// TestShipGateRollup covers the contract the dashboard leans on: every
// translated pair is stale until it has a verdict, a stored verdict is counted
// in its collection's scope, and it stops being counted the moment the content
// or the governance it was computed under moves.
func TestShipGateRollup(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := seedShipGateProject(t, s)
	names, ids := shipGateNames(t, s, p.ID)
	const gate = "gate-1"
	fr := string(model.LocaleFrench)

	t.Run("every translated pair is stale before anything is stored", func(t *testing.T) {
		got, err := s.ShipGateRollup(ctx, shipGateQuery(p.ID, gate))
		require.NoError(t, err)
		assert.Empty(t, got.Scopes, "nothing is counted before a verdict exists")
		assert.Equal(t, []string{"b1", "b2", "b4"}, staleNames(names, got.Stale),
			"the untranslatable block is not a pair the gate judges")
	})

	rollup, err := s.ShipGateRollup(ctx, shipGateQuery(p.ID, gate))
	require.NoError(t, err)
	storeVerdicts(t, s, p.ID, gate, rollup.Stale, map[string]bool{ids["b2"]: true})

	t.Run("stored verdicts are counted in their collection", func(t *testing.T) {
		got, err := s.ShipGateRollup(ctx, shipGateQuery(p.ID, gate))
		require.NoError(t, err)
		assert.Empty(t, got.Stale, "nothing has changed, so nothing needs judging")

		a := got.CountsFor("col-a", fr)
		assert.Equal(t, 1, a.Clean)
		assert.Equal(t, 1, a.Failing)
		b := got.CountsFor("col-b", fr)
		assert.Equal(t, 1, b.Clean)
		assert.Zero(t, b.Failing)
	})

	t.Run("a voice score below the bar withholds a clean block", func(t *testing.T) {
		got, err := s.ShipGateRollup(ctx, shipGateQuery(p.ID, gate,
			platstore.ShipGateScore{
				BlockID: ids["b1"], Locale: fr,
				BelowBar: true,
			},
			platstore.ShipGateScore{
				BlockID: ids["b4"], Locale: fr,
			}))
		require.NoError(t, err)
		a := got.CountsFor("col-a", fr)
		assert.Equal(t, 1, a.Scored)
		assert.Equal(t, 1, a.CleanBelowBar)
		b := got.CountsFor("col-b", fr)
		assert.Equal(t, 1, b.Scored)
		assert.Zero(t, b.CleanBelowBar, "a score above the bar withholds nothing")
	})

	t.Run("a different gate retires every verdict", func(t *testing.T) {
		got, err := s.ShipGateRollup(ctx, shipGateQuery(p.ID, "gate-2"))
		require.NoError(t, err)
		assert.Empty(t, got.Scopes)
		assert.Equal(t, []string{"b1", "b2", "b4"}, staleNames(names, got.Stale))
	})

	t.Run("a rewritten item retires its verdicts and no other item's", func(t *testing.T) {
		require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "a.json", shipGateItemA("réécrit")))

		got, err := s.ShipGateRollup(ctx, shipGateQuery(p.ID, gate))
		require.NoError(t, err)
		// Storing an item rewrites every target row it holds, so the whole
		// item is judged again — which is the delta an ordinary push leaves:
		// the files it wrote, not the project they sit in.
		assert.Equal(t, []string{"b1", "b2"}, staleNames(names, got.Stale))
		assert.Zero(t, got.CountsFor("col-a", fr).Clean, "a.json holds no verdict that still applies")
		assert.Equal(t, 1, got.CountsFor("col-b", fr).Clean, "b.json was not written and keeps its own")
	})
}

// TestPutShipGateVerdictsRefusesAMovedBasis asserts the write-side half of the
// bargain: a verdict computed against content that has since changed is not
// recorded at all. Recording it would leave a counter claiming to have judged
// text nothing ever read.
func TestPutShipGateVerdictsRefusesAMovedBasis(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := seedShipGateProject(t, s)
	names, _ := shipGateNames(t, s, p.ID)
	const gate = "gate-1"

	rollup, err := s.ShipGateRollup(ctx, shipGateQuery(p.ID, gate))
	require.NoError(t, err)
	require.NotEmpty(t, rollup.Stale)

	// The target moves after the pass read it and before it writes back.
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "a.json", shipGateItemA("déplacé")))

	storeVerdicts(t, s, p.ID, gate, rollup.Stale, nil)

	got, err := s.ShipGateRollup(ctx, shipGateQuery(p.ID, gate))
	require.NoError(t, err)
	assert.Equal(t, []string{"b1", "b2"}, staleNames(names, got.Stale),
		"the pairs whose basis moved keep no verdict and are judged again")
	assert.Equal(t, 1, got.CountsFor("col-b", string(model.LocaleFrench)).Clean,
		"the pair whose basis held is recorded as normal")
}
