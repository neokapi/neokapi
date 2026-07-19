package server

import (
	"context"
	"testing"

	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedDeliveryBlocks stores one item (locales/en/app.json) with the given blocks
// under proj1/main for the forge delivery-gate tests.
func seedDeliveryBlocks(t *testing.T, s *Server, blocks []*model.Block) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, s.ContentStore.StoreItem(ctx, "proj1", "main", &platstore.Item{
		ID: "item1", ProjectID: "proj1", Name: "locales/en/app.json", Format: "json",
	}))
	require.NoError(t, s.ContentStore.StoreBlocksForItem(ctx, "proj1", "main", "locales/en/app.json", blocks))
}

// TestMaterializeDelivery_GovernedShipsApprovedOnly (RV-A): a governed project
// delivers only reviewed/signed-off targets; a non-governed one (workflow off)
// keeps the kapi-drafts-ship semantics and delivers any committed target.
func TestMaterializeDelivery_GovernedShipsApprovedOnly(t *testing.T) {
	s, _ := newForgeTestServer(t, "conn1", "proj1", "s3cret")
	ctx := context.Background()

	// Block A: fr reviewed (approved), de translated (unreviewed draft).
	bA := model.NewBlock("a", "Hello")
	bA.SetTargetText("fr", "Bonjour")
	bA.Target("fr").Status = model.TargetStatusReviewed
	bA.SetTargetText("de", "Hallo")
	// Block B: fr translated (unreviewed draft).
	bB := model.NewBlock("b", "Bye")
	bB.SetTargetText("fr", "Au revoir")
	seedDeliveryBlocks(t, s, []*model.Block{bA, bB})

	proj, err := s.ContentStore.GetProject(ctx, "proj1")
	require.NoError(t, err)

	// Governed (workflow_enabled unset → governed): only the approved fr block
	// ships; de has no approved block, so it is skipped as empty.
	items, err := s.materializeDelivery(ctx, proj)
	require.NoError(t, err)
	require.Len(t, items, 1, "governed: only fr, and only its one approved block")
	assert.Equal(t, model.LocaleID("fr"), items[0].Locale)
	require.Len(t, items[0].Blocks, 1)
	assert.Equal(t, "Bonjour", items[0].Blocks[0].SourceText())

	// Non-governed: any committed target ships. materializeDelivery reads
	// governance off the passed project, so mutating it in memory is enough.
	proj.Properties = map[string]string{"workflow_enabled": "false"}
	items, err = s.materializeDelivery(ctx, proj)
	require.NoError(t, err)
	byLocale := map[model.LocaleID]*platconn.ContentItem{}
	for _, it := range items {
		byLocale[it.Locale] = it
	}
	require.Contains(t, byLocale, model.LocaleID("fr"))
	require.Contains(t, byLocale, model.LocaleID("de"))
	assert.Len(t, byLocale[model.LocaleID("fr")].Blocks, 2, "non-governed fr: draft + approved both ship")
	assert.Len(t, byLocale[model.LocaleID("de")].Blocks, 1, "non-governed de: the draft ships")
}

// TestMaterializeDelivery_GovernedFullyDraftDeliversNothing (RV-A): a governed
// project whose targets are all unreviewed delivers nothing.
func TestMaterializeDelivery_GovernedFullyDraftDeliversNothing(t *testing.T) {
	s, _ := newForgeTestServer(t, "conn1", "proj1", "s3cret")
	ctx := context.Background()

	b := model.NewBlock("a", "Hello")
	b.SetTargetText("fr", "Bonjour") // translated, unreviewed
	b.SetTargetText("de", "Hallo")   // translated, unreviewed
	seedDeliveryBlocks(t, s, []*model.Block{b})

	proj, err := s.ContentStore.GetProject(ctx, "proj1")
	require.NoError(t, err)
	items, err := s.materializeDelivery(ctx, proj)
	require.NoError(t, err)
	assert.Empty(t, items, "governed project with zero approved targets delivers nothing")
}
