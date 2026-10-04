package sqlitestore

import (
	"fmt"
	"path/filepath"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// BenchmarkHydrateOverlays measures one hydrate chunk at its full size: the
// translations of 5,000 blocks in 10 languages filed onto freshly scanned
// blocks, as GetBlocks hydrates them. Each iteration hydrates new blocks, so
// every translation is filed rather than updated. Run it with -benchmem: the
// bytes and allocations per operation are what a whole-project hydrate costs
// per chunk.
func BenchmarkHydrateOverlays(b *testing.B) {
	const blocks = 5000
	locales := []model.LocaleID{"fr", "de", "es", "it", "nb", "sv", "da", "fi", "nl", "pt"}

	s, err := NewSQLiteStore(filepath.Join(b.TempDir(), "store.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = s.Close() })
	ctx := b.Context()
	p := &platstore.Project{
		Name:                  "Hydrate",
		DefaultSourceLanguage: model.LocaleEnglish,
		TargetLanguages:       locales,
	}
	if err := s.CreateProject(ctx, p); err != nil {
		b.Fatal(err)
	}

	ids := make([]string, blocks)
	seed := make([]*model.Block, blocks)
	for i := range seed {
		ids[i] = fmt.Sprintf("b%05d", i)
		blk := model.NewBlock(ids[i], fmt.Sprintf("Source sentence %d, as a reader filed it.", i))
		blk.SourceLocale = model.LocaleEnglish
		for _, loc := range locales {
			blk.SetTargetText(loc, fmt.Sprintf("Translation %d into %s, as a producer wrote it.", i, loc))
			blk.SetEditionStatus(model.Variant(loc), model.Status(model.TargetStatusTranslated))
		}
		seed[i] = blk
	}
	if err := s.StoreBlocks(ctx, p.ID, "", seed); err != nil {
		b.Fatal(err)
	}

	scanned := func() []*venue.StoredBlock {
		out := make([]*venue.StoredBlock, blocks)
		for i, id := range ids {
			blk := model.NewBlock(id, "")
			blk.SourceLocale = model.LocaleEnglish
			out[i] = &venue.StoredBlock{Block: blk}
		}
		return out
	}

	// One hydrate before timing, to check every translation is filed.
	check := scanned()
	if err := bstore.HydrateOverlays(ctx, s.DB(), "sqlite", p.ID, "main", check); err != nil {
		b.Fatal(err)
	}
	for _, sb := range check {
		if got := len(sb.Block.Editions()); got != len(locales)+1 {
			b.Fatalf("block %s holds %d editions after hydration, want %d", sb.Block.ID, got, len(locales)+1)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		in := scanned()
		b.StartTimer()
		if err := bstore.HydrateOverlays(ctx, s.DB(), "sqlite", p.ID, "main", in); err != nil {
			b.Fatal(err)
		}
	}
}
