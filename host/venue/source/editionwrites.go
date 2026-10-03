package source

import (
	"context"
	"fmt"
	"sort"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// The edition writes a push carries beside the decisions.
//
// The decision ledger holds decisions only. What a run on this checkout made,
// and from which source, is in the project's block history, which a venue does
// not read. So a push reads the history for the documents it scans and sends,
// for each translation the project holds of them, the last write the history
// records (venue.EditionWrite): the source it was made from, and who wrote it.
// A venue grades the translation stale against that source, as it grades the
// drafts its own runs make, and holds a reviewer to separation of duties over
// a translation they wrote by hand.

// localBlockKeys keeps the key each scanned block had before the push
// resolved it against the venue's identities: the key the block history
// records its changes under.
func localBlockKeys(blockMap map[string][]*model.Block) map[*model.Block]string {
	out := map[*model.Block]string{}
	for _, blocks := range blockMap {
		for _, b := range blocks {
			out[b] = change.BlockKey(b)
		}
	}
	return out
}

// projectEditionWrites reads, for each document the push scanned, the last
// recorded write of every translation the block history holds for its blocks,
// named as the venue names the unit. A block the scan no longer finds, and the
// document's own edition, are left out.
func (c *BowrainSourceConnector) projectEditionWrites(ctx context.Context, blockMap map[string][]*model.Block, local map[*model.Block]string) ([]venue.EditionWrite, error) {
	if c.app == nil || len(blockMap) == 0 {
		return nil, nil
	}
	db, err := c.app.ProjectDB(ctx, c.project.Root)
	if err != nil {
		return nil, fmt.Errorf("open the project store: %w", err)
	}
	hist := db.History()
	empty, err := hist.Empty(ctx)
	if err != nil || empty {
		return nil, err
	}
	docs, err := c.app.DocumentIndex(ctx, c.project.Root)
	if err != nil {
		return nil, fmt.Errorf("read the project's documents: %w", err)
	}
	items := make([]string, 0, len(blockMap))
	for item := range blockMap {
		items = append(items, item)
	}
	sort.Strings(items)

	var out []venue.EditionWrite
	for _, item := range items {
		rows, err := hist.Latest(ctx, docs.Key(item))
		if err != nil {
			return nil, fmt.Errorf("read the block history of %s: %w", item, err)
		}
		if len(rows) == 0 {
			continue
		}
		byKey := make(map[string]*model.Block, len(blockMap[item]))
		for _, b := range blockMap[item] {
			byKey[local[b]] = b
		}
		for _, r := range rows {
			b := byKey[r.Block]
			if b == nil || r.After == model.AbsentRevision {
				continue
			}
			k, err := model.ParseEditionKey(r.Edition)
			if err != nil || k.Locale == "" || b.IsSourceEdition(k) {
				continue
			}
			w := venue.EditionWrite{
				ItemName: item, Unit: convergence.BlockKey(b), Variant: variantText(k),
				Revision: r.After, Writer: r.Actor, Origin: r.Origin,
				GoverningFingerprint: r.Producer.ContextFingerprint,
			}
			if w.Unit != r.Block {
				w.Block = r.Block
			}
			if r.Basis != "" {
				// The content hash the record keeps is the source's, which is
				// the value a venue compares a block's source with.
				w.Basis = r.ContentHash
			}
			out = append(out, w)
		}
	}
	return out, nil
}
