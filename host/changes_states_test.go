package host

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/history"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/storage"
)

// countingHeads counts the lookups a read's edition states make of a real
// block history.
type countingHeads struct {
	*history.Store
	byBlock, whole int
}

func (c *countingHeads) LastWrite(ctx context.Context, doc, block, edition string) (history.Row, bool, error) {
	c.byBlock++
	return c.Store.LastWrite(ctx, doc, block, edition)
}

func (c *countingHeads) Latest(ctx context.Context, doc string, editions ...string) ([]history.Row, error) {
	c.whole++
	return c.Store.Latest(ctx, doc, editions...)
}

var statesLanguages = []model.LocaleID{"fr", "de", "ja", "nb", "es", "it", "pt", "nl", "sv", "da"}

// statesBlocks returns n blocks, each holding a translation into every
// edition of keys.
func statesBlocks(n int, keys []model.EditionKey) []*model.Block {
	bs := make([]*model.Block, n)
	for i := range n {
		b := model.NewBlock(fmt.Sprintf("p%05d", i), fmt.Sprintf("Paragraph %d tells the reader how the shop opens.", i))
		b.SourceLocale = "en"
		for _, k := range keys {
			b.SetEdition(k, model.Edition{Runs: []model.Run{{Text: &model.TextRun{Text: fmt.Sprintf("%s %d", k.Locale, i)}}}})
		}
		bs[i] = b
	}
	return bs
}

// statesFixture records the history of one document, "d-1": blocks blocks
// translated into langs languages, writes times each. The last write leaves
// every translation at the revision the returned blocks hold (statesBlocks),
// with the basis "r:basis", except the first block's first language, which
// somebody has rewritten since without recording it.
func statesFixture(tb testing.TB, blocks, langs, writes int) (*history.Store, []*model.Block, []model.EditionKey) {
	tb.Helper()
	db, err := storage.OpenWith(filepath.Join(tb.TempDir(), "context.db"), storage.ProjectOptions())
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = db.Close() })
	hist, err := history.Open(db)
	require.NoError(tb, err)

	keys := make([]model.EditionKey, langs)
	for l := range langs {
		keys[l] = model.EditionKey{Locale: statesLanguages[l]}.Canonical()
	}
	bs := statesBlocks(blocks, keys)
	at := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	op := 0
	for w := range writes {
		var rows []history.Row
		for _, k := range keys {
			op++
			for _, b := range bs {
				after := fmt.Sprintf("r:old-%d", w)
				if w == writes-1 {
					after = model.EditionRevision(b, k)
				}
				rows = append(rows, history.Row{
					Op: fmt.Sprintf("op%06d", op), Address: fmt.Sprintf("a%06d", op), Doc: "d-1",
					Block: change.BlockKey(b), Edition: editionText(b.EditionKeyOf(k)),
					Before: "r:x", After: after, Basis: "r:basis", Actor: "tool", At: at,
				})
			}
		}
		require.NoError(tb, hist.Put(context.Background(), rows))
	}
	bs[0].SetEdition(keys[0], model.Edition{Runs: []model.Run{{Text: &model.TextRun{Text: "rewritten outside kapi"}}}})
	return hist, bs, keys
}

// TestDocumentStates_ReadsWhatTheReadShows pins how a read's edition states
// ask the block history: a short read looks each block up and never reads the
// rest of the document's history, and a longer one reads the editions a block
// holds in one query the first time it asks about one of them. The answers
// are the same either way.
func TestDocumentStates_ReadsWhatTheReadShows(t *testing.T) {
	hist, blocks, keys := statesFixture(t, 300, 3, 4)
	// lead holds the first language alone and has no recorded change.
	lead := model.NewBlock("lead", "A block translated into one language.")
	lead.SourceLocale = "en"
	lead.SetEdition(keys[0], model.Edition{Runs: []model.Run{{Text: &model.TextRun{Text: "lead"}}}})
	tests := []struct {
		name string
		// shows is what the read says it shows at most, and blocks how many
		// it asks about, lead first when lead is set.
		shows, blocks    int
		lead             bool
		editions         []model.EditionKey
		byBlock, whole   int
		wantBasisMissing int
	}{
		{name: "one block named", shows: 1, blocks: 1, editions: keys[1:2], byBlock: 1},
		{name: "a page of 20 blocks in one of three languages", shows: 20, blocks: 20, editions: keys[:1], byBlock: 20, wantBasisMissing: 1},
		{name: "a page as long as a short read", shows: shortRead, blocks: shortRead, editions: keys[:1], byBlock: shortRead, wantBasisMissing: 1},
		{name: "a page longer than a short read", shows: shortRead + 1, blocks: shortRead + 1, editions: keys[:1], whole: 1, wantBasisMissing: 1},
		{name: "the whole document in one language", blocks: 300, editions: keys[:1], whole: 1, wantBasisMissing: 1},
		{name: "the whole document in three languages", blocks: 300, editions: keys, whole: 1, wantBasisMissing: 1},
		{name: "languages the first block does not hold", blocks: 300, lead: true, editions: keys, whole: 2, wantBasisMissing: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			heads := &countingHeads{Store: hist}
			states := &documentStates{ctx: t.Context(), hist: heads, docKey: "d-1", byBlock: isShortRead(tt.shows)}
			missing := 0
			ask := func(b *model.Block, k model.EditionKey) {
				if _, held := b.Edition(k); !held {
					return
				}
				st, ok := states.EditionState(b, k)
				if !ok {
					missing++
					return
				}
				assert.Equal(t, "r:basis", st.Basis, "%s@%s", b.ID, k)
			}
			asked := blocks[:tt.blocks]
			if tt.lead {
				asked = append([]*model.Block{lead}, asked...)
			}
			for _, b := range asked {
				for _, k := range tt.editions {
					ask(b, k)
				}
			}
			assert.Equal(t, tt.byBlock, heads.byBlock, "editions looked up one block at a time")
			assert.Equal(t, tt.whole, heads.whole, "queries over the document's history")
			assert.Equal(t, tt.wantBasisMissing, missing,
				"only a translation rewritten since its last recorded change, or never recorded, has no basis")
		})
	}
}

// TestChangeRead_ShortReadsLookUpTheBlocksTheyShow pins the hint the change
// service gives the block history, end to end: a read naming one block, or a
// short page, answers the same basis as a read of the whole document.
func TestChangeRead_ShortReadsLookUpTheBlocksTheyShow(t *testing.T) {
	_, svc := translatedGuide(t, 150)
	ctx := t.Context()
	fr, err := model.ParseEditionKey("fr")
	require.NoError(t, err)
	whole := map[string]change.EditionRead{}
	_, err = svc.ReadEach(ctx, change.ReadRequest{Doc: "docs/guide.md", Editions: []model.EditionKey{fr}}, func(_ *model.Block, r change.BlockRead) error {
		whole[r.Ref.Block] = r.Editions["fr"]
		return nil
	})
	require.NoError(t, err)
	require.Len(t, whole, 151)

	page, err := svc.Read(ctx, change.ReadRequest{Doc: "docs/guide.md", Editions: []model.EditionKey{fr}, Limit: 10})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 10)
	for _, b := range page.Blocks {
		assert.NotEmpty(t, b.Editions["fr"].Basis, b.Ref.Block)
		assert.Equal(t, whole[b.Ref.Block], b.Editions["fr"], b.Ref.Block)
	}
	one, err := svc.Read(ctx, change.ReadRequest{Doc: "docs/guide.md", Editions: []model.EditionKey{fr}, Blocks: []string{page.Blocks[7].Ref.Block}})
	require.NoError(t, err)
	require.Len(t, one.Blocks, 1)
	assert.Equal(t, whole[page.Blocks[7].Ref.Block], one.Blocks[0].Editions["fr"])
}

// BenchmarkDocumentStates measures what a read's edition states cost a read
// of one block, a page, and the whole document, as the document's history
// grows deeper and wider. A read in one language is of blocks holding that
// language alone (a source document with its translation's file joined),
// whatever other languages the history holds; a read in every language is of
// blocks holding them all (a multilingual document).
func BenchmarkDocumentStates(b *testing.B) {
	for _, shape := range []struct{ blocks, langs, writes int }{{400, 1, 1}, {2000, 1, 5}, {2000, 10, 5}} {
		hist, every, keys := statesFixture(b, shape.blocks, shape.langs, shape.writes)
		one := statesBlocks(shape.blocks, keys[:1])
		reads := []struct {
			name     string
			blocks   []*model.Block
			shows    int
			editions []model.EditionKey
		}{
			{"one-block", one, 1, keys[:1]},
			{"page-of-100", one, 100, keys[:1]},
			{"whole-one-language", one, 0, keys[:1]},
			{"whole-every-language", every, 0, keys},
		}
		for _, r := range reads {
			if r.name == "whole-every-language" && shape.langs == 1 {
				continue
			}
			b.Run(fmt.Sprintf("blocks=%d/langs=%d/writes=%d/%s", shape.blocks, shape.langs, shape.writes, r.name), func(b *testing.B) {
				// The first block's first language was rewritten since.
				blocks := r.blocks[1:]
				if r.shows > 0 {
					blocks = blocks[:r.shows]
				}
				for b.Loop() {
					states := &documentStates{ctx: context.Background(), hist: hist, docKey: "d-1", byBlock: isShortRead(r.shows)}
					for _, blk := range blocks {
						for _, k := range r.editions {
							if _, ok := states.EditionState(blk, k); !ok {
								b.Fatalf("no basis for %s@%s", blk.ID, k)
							}
						}
					}
				}
			})
		}
	}
}
