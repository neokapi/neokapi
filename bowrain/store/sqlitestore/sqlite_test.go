package sqlitestore

import (
	"path/filepath"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "store.db")
	s, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func createTestProject(t *testing.T, s *SQLiteStore) *platstore.Project {
	t.Helper()
	p := &platstore.Project{
		Name:                  "Test Project",
		DefaultSourceLanguage: model.LocaleEnglish,
		TargetLanguages:       []model.LocaleID{model.LocaleFrench, model.LocaleGerman},
		Properties:            map[string]string{"client": "acme"},
	}
	require.NoError(t, s.CreateProject(t.Context(), p))
	return p
}

// TestStoreBlocks_SourceAddedThenModified exercises the StoreBlocks hash-diff
// path: a freshly stored block must classify as "source_added", and a
// content change on the same block must classify as "source_modified". This
// guards the existingHashes diff (including its rows.Err() check) against
// regressions that would mis-classify added vs modified entries.
func TestStoreBlocks_SourceAddedThenModified(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	// First write → source_added.
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{model.NewBlock("b1", "Hello")}))

	cs, err := s.GetChanges(ctx, p.ID, "", 0, nil, 100)
	require.NoError(t, err)
	require.Len(t, cs.Changes, 1)
	assert.Equal(t, "b1", cs.Changes[0].BlockID)
	assert.Equal(t, "source_added", cs.Changes[0].ChangeType)
	assert.NotEmpty(t, cs.Changes[0].ContentHash)

	// Second write with changed content → source_modified (diffed against the
	// existingHashes map built from the prior write).
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{model.NewBlock("b1", "Hello World")}))

	cs, err = s.GetChanges(ctx, p.ID, "", 0, nil, 100)
	require.NoError(t, err)
	require.Len(t, cs.Changes, 2)
	assert.Equal(t, "source_added", cs.Changes[0].ChangeType)
	assert.Equal(t, "source_modified", cs.Changes[1].ChangeType)
}

// TestStoreBlocks_UnchangedSourceNotLogged ensures that re-storing identical
// content produces no spurious "source_modified" entry — the content_hash read
// back through the existingHashes diff must match exactly.
func TestStoreBlocks_UnchangedSourceNotLogged(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	b := model.NewBlock("b1", "Hello")
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{b}))
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{b}))

	cs, err := s.GetChanges(ctx, p.ID, "", 0, nil, 100)
	require.NoError(t, err)
	assert.Len(t, cs.Changes, 1) // Only the initial source_added.
}

// TestStoreBlocks_MultipleBlocksMixedClassification stores a batch where one
// block is new and another is modified in the same call, verifying the
// existingHashes map correctly distinguishes added from modified across rows.
func TestStoreBlocks_MultipleBlocksMixedClassification(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	// Seed b1.
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{model.NewBlock("b1", "One")}))

	// Batch: b1 modified + b2 new.
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{
		model.NewBlock("b1", "One changed"),
		model.NewBlock("b2", "Two"),
	}))

	cs, err := s.GetChanges(ctx, p.ID, "", 0, nil, 100)
	require.NoError(t, err)
	require.Len(t, cs.Changes, 3)

	byBlock := map[string]string{}
	for _, c := range cs.Changes {
		// Last write wins per block for this assertion set; both b1 entries are
		// distinct change types, so collect them all.
		byBlock[c.BlockID+":"+c.ChangeType] = c.ChangeType
	}
	assert.Contains(t, byBlock, "b1:source_added")
	assert.Contains(t, byBlock, "b1:source_modified")
	assert.Contains(t, byBlock, "b2:source_added")
}

// TestStoreBlocksForItem_AdoptsLegacyItemlessRows: the historical
// connector-ingest path stored fetched blocks with an empty item_name under
// their raw format-reader IDs, invisible to the item-scoped
// stats/jobs/delivery paths.
// When the (fixed) ingest re-stores the same content per item, the store must
// ADOPT the orphaned row — same internal ID, item_name + source_id stamped,
// targets preserved — never mint a duplicate that would double the block count
// and park convergence on phantom failing checks.
func TestStoreBlocksForItem_AdoptsLegacyItemlessRows(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	// Legacy ingest: item-less block under its reader ID, already translated.
	legacy := model.NewBlock("greeting", "Hello")
	legacy.SetTargetText(model.LocaleFrench, "Bonjour")
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{legacy}))

	// The fixed ingest re-stores the same reader block scoped to its item.
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "", "locales/en/app.json",
		[]*model.Block{model.NewBlock("greeting", "Hello")}))

	blocks, err := s.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID})
	require.NoError(t, err)
	require.Len(t, blocks, 1, "the legacy row is adopted, not duplicated")
	sb := blocks[0]
	assert.Equal(t, "greeting", sb.Block.ID, "internal ID stays stable")
	assert.Equal(t, "locales/en/app.json", sb.ItemName)
	assert.Equal(t, "greeting", sb.SourceID)
	assert.Equal(t, "Bonjour", sb.Block.TargetText(model.LocaleFrench),
		"the adopted row keeps its targets")

	// The adopted row is now visible to the item-scoped stats path.
	require.NoError(t, s.StoreItem(ctx, p.ID, "", &platstore.Item{
		Name: "locales/en/app.json", Format: "json", ItemType: "file",
	}))
	stats, err := s.GetBlockStats(ctx, p.ID, "")
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.Equal(t, []string{string(model.LocaleFrench)}, stats[0].TargetLocales)
}

// TestGetBlockStats_ApprovedLocales exercises the SQLite status-extraction
// path (json_extract on target_json): only targets whose status carries a
// review decision (reviewed / signed-off) land in ApprovedLocales.
func TestGetBlockStats_ApprovedLocales(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	require.NoError(t, s.StoreItem(ctx, p.ID, "", &platstore.Item{
		Name: "messages.json", Format: "json", ItemType: "file",
	}))

	// b1: fr reviewed (approved), de merely translated.
	b1 := model.NewBlock("b1", "Hello")
	b1.SetTargetText(model.LocaleFrench, "Bonjour")
	b1.StampTargetProvenance(model.LocaleFrench, model.TargetStatusReviewed, model.Origin{Kind: model.OriginHuman})
	b1.SetTargetText(model.LocaleGerman, "Hallo")

	// b2: fr draft — below the review rungs, never approved.
	b2 := model.NewBlock("b2", "World")
	b2.SetTargetText(model.LocaleFrench, "Monde")
	b2.StampTargetProvenance(model.LocaleFrench, model.TargetStatusDraft, model.Origin{Kind: model.OriginAI})

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "", "messages.json", []*model.Block{b1, b2}))

	stats, err := s.GetBlockStats(ctx, p.ID, "")
	require.NoError(t, err)
	require.Len(t, stats, 2)

	for _, bs := range stats {
		switch len(bs.TargetLocales) {
		case 2: // b1
			assert.Equal(t, []string{string(model.LocaleFrench)}, bs.ApprovedLocales)
		case 1: // b2
			assert.Empty(t, bs.ApprovedLocales, "draft status is not a review decision")
		default:
			t.Fatalf("unexpected block stat row: %+v", bs)
		}
	}
}

// overlaysFixture builds a block's worth of stand-off overlays covering every
// kind that must survive a store round-trip: segmentation (incl. an ignorable
// span), an entity overlay carrying a typed *EntityAnnotation Value (the
// entity→concept promote path), a term overlay, a term-candidate overlay, a
// check-findings overlay (model.OverlayQA), and a target-side alignment
// overlay. Anchors use real run ranges.
func overlaysFixture() []model.Overlay {
	frVariant := model.VariantKey{Locale: model.LocaleFrench}
	return []model.Overlay{
		{
			Type:  model.OverlaySegmentation,
			Layer: model.LayerPrimary,
			Spans: []model.Span{
				{ID: "s1", Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 0, Offset: 4})},
				{
					ID:    "s2",
					Range: model.SpanAnchor(model.RunPos{Run: 0, Offset: 5}, model.RunPos{Run: 0, Offset: 11}),
					Props: map[string]string{model.SpanPropIgnorable: "true"},
				},
			},
		},
		{
			Type: model.OverlayEntity,
			Spans: []model.Span{{
				ID:    "entity:0",
				Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 0, Offset: 4}),
				Value: &model.EntityAnnotation{Text: "Acme", Type: model.EntityOrganization, Locale: model.LocaleEnglish, DNT: true, Source: "ner"},
			}},
		},
		{
			Type: model.OverlayTerm,
			Spans: []model.Span{{
				ID:    "term:0",
				Range: model.SpanAnchor(model.RunPos{Run: 0, Offset: 5}, model.RunPos{Run: 0, Offset: 11}),
				Value: &model.TermAnnotation{SourceTerm: "widget", ConceptID: "c-42", Status: model.TermPreferred, Score: 1, MatchType: model.MatchStrategyExact, TargetTerms: []model.TermRef{{Text: "gadget", Locale: model.LocaleFrench, Status: model.TermPreferred}}},
			}},
		},
		{
			Type: model.OverlayTermCandidate,
			Spans: []model.Span{{
				ID:    "term-candidate:0",
				Range: model.SpanAnchor(model.RunPos{Run: 0, Offset: 5}, model.RunPos{Run: 0, Offset: 11}),
				Value: &model.TermCandidateAnnotation{Text: "widget", Locale: model.LocaleEnglish},
			}},
		},
		{
			Type: model.OverlayQA,
			Spans: []model.Span{{
				ID:    "qa:0",
				Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 0, Offset: 11}),
				Props: map[string]string{"rule": "length", "severity": "warn"},
			}},
		},
		{
			Type:    model.OverlayAlignment,
			Variant: &frVariant,
			Spans: []model.Span{{
				ID:    "a0",
				Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 0, Offset: 4}),
				Props: map[string]string{"target": "0:0-0:6"},
			}},
		},
	}
}

// TestStoreBlocks_OverlaysRoundTrip proves a block's stand-off overlays survive
// a full StoreBlocks → GetBlock round-trip through SQLite with run-index anchors
// intact — and the entity span's typed Value rehydrates to the concrete
// *EntityAnnotation the entity→concept promote path depends on. A block with no
// overlays round-trips as no overlays.
func TestStoreBlocks_OverlaysRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	b := model.NewBlock("b1", "Acme widget")
	b.Overlays = overlaysFixture()
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{b}))

	got, err := s.GetBlock(ctx, p.ID, "", "b1")
	require.NoError(t, err)
	require.Equal(t, overlaysFixture(), got.Block.Overlays,
		"every overlay kind must survive the store round-trip with anchors intact")

	ent := got.Block.OverlayOf(model.OverlayEntity)
	require.NotNil(t, ent)
	require.Len(t, ent.Spans, 1)
	ea, ok := ent.Spans[0].Value.(*model.EntityAnnotation)
	require.True(t, ok, "entity span value must rehydrate to *EntityAnnotation (entity-promote)")
	assert.Equal(t, "Acme", ea.Text)
	assert.True(t, ea.DNT)

	// Re-storing keeps overlays intact (ON CONFLICT UPDATE path).
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{b}))
	got, err = s.GetBlock(ctx, p.ID, "", "b1")
	require.NoError(t, err)
	require.Equal(t, overlaysFixture(), got.Block.Overlays)

	// A block with no overlays round-trips as no overlays.
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{model.NewBlock("b2", "plain")}))
	got2, err := s.GetBlock(ctx, p.ID, "", "b2")
	require.NoError(t, err)
	assert.Nil(t, got2.Block.Overlays)
}

// The change log is what a client pulling by cursor reads to learn a variant
// moved. Committing the variant without its entry produces a change nobody
// downstream is ever told about, so the write is part of the transaction's
// promise rather than bookkeeping beside it.
func TestStoreAssetVariantRequiresItsChangeLogEntry(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	asset := &venue.Asset{BlobKey: "aabbccdd", MimeType: "image/png"}
	require.NoError(t, s.StoreAsset(ctx, p.ID, "main", asset))

	variant := &venue.AssetVariant{
		AssetID:  asset.ID,
		Locale:   "fr-FR",
		BlobKey:  "eeff0011",
		Status:   "draft",
		MimeType: "image/png",
	}
	require.NoError(t, s.StoreAssetVariant(ctx, p.ID, variant))

	var logged int
	require.NoError(t, s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM change_log WHERE block_id = ? AND locale = ?`,
		asset.ID, "fr-FR").Scan(&logged))
	assert.Positive(t, logged, "a stored variant must appear in the change log")

	// Take the change log away and store another variant. The variant write
	// must fail rather than commit a change the feed will never carry.
	_, err := s.db.ExecContext(ctx, `DROP TABLE change_log`)
	require.NoError(t, err)

	err = s.StoreAssetVariant(ctx, p.ID, &venue.AssetVariant{
		AssetID:  asset.ID,
		Locale:   "de-DE",
		BlobKey:  "22334455",
		Status:   "draft",
		MimeType: "image/png",
	})
	require.Error(t, err, "a variant whose change-log entry failed must not report success")
}
