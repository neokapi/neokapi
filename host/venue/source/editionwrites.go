package source

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/history"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// The edition writes a push carries beside the decisions.
//
// The decision ledger holds decisions only. What a run on this checkout made,
// and from which source, is in the project's block history, which a venue does
// not read. So a push reads the history for the documents it scans and sends,
// for each translation the checkout holds of them, the recorded write that
// left it as it is (venue.EditionWrite): the source it was made from, and who
// wrote it. A venue grades the translation stale against that source, as it
// grades the drafts its own runs make, and holds a reviewer to separation of
// duties over a translation they wrote by hand.
//
// The history is shared by every branch of the checkout, so the latest write
// of a translation can be another branch's. The push reads the revision each
// translation holds now and sends the write that left it.

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

// projectEditionWrites reads, for each document the push scanned, the
// recorded write that left every translation the checkout holds of its blocks
// as it is, named as the venue names the unit. A block the scan no longer
// finds, a translation the checkout no longer holds, and the document's own
// edition are left out. A document whose translations cannot be read sends
// none.
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
		key := docs.Key(item)
		rows, err := hist.Latest(ctx, key)
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
		var asked []model.EditionKey
		seen := map[string]bool{}
		for _, r := range rows {
			k, err := model.ParseEditionKey(r.Edition)
			if err != nil || k.Locale == "" || seen[r.Edition] {
				continue
			}
			seen[r.Edition] = true
			asked = append(asked, k)
		}
		held, err := c.app.EditionRevisions(ctx, c.project.RecipePath(), item, asked)
		if err != nil {
			slog.DebugContext(ctx, "read the translations a push describes", "item", item, "error", err)
			continue
		}
		for _, r := range rows {
			b := byKey[r.Block]
			if b == nil {
				continue
			}
			k, err := model.ParseEditionKey(r.Edition)
			if err != nil || k.Locale == "" || b.IsSourceEdition(k) {
				continue
			}
			rev, ok := held[history.EditionRef{Block: r.Block, Edition: r.Edition}]
			if !ok || rev == model.AbsentRevision {
				continue
			}
			if r.After != rev {
				wrote, found, err := hist.Wrote(ctx, key, r.Block, r.Edition, rev)
				if err != nil {
					return nil, fmt.Errorf("read the block history of %s: %w", item, err)
				}
				if !found {
					continue
				}
				r = wrote
			}
			w := venue.EditionWrite{
				ItemName: item, Unit: convergence.BlockKey(b), Variant: variantText(k),
				Revision: r.After, Writer: r.Actor, Origin: r.Origin,
				GoverningFingerprint: r.Producer.ContextFingerprint,
			}
			if w.Unit != r.Block {
				w.Block = r.Block
			}
			if r.Origin != history.OriginPull {
				// The revision of the source the write was made from. The
				// history holds it under the key the run's reader filed the
				// source by, and it travels as the venue takes the revision
				// of the source it holds (venue.Basis).
				w.Basis = venue.Basis(b, c.sourceLanguage(), r.Basis)
			}
			out = append(out, w)
		}
	}
	return out, nil
}

// unsentWrites returns the writes whose identity differs from what this
// client last saw a venue apply for the same translation.
func (c *BowrainSourceConnector) unsentWrites(writes []venue.EditionWrite) []venue.EditionWrite {
	var out []venue.EditionWrite
	for _, w := range writes {
		if c.cache.WritesSent[w.ItemName][w.Edition()] != w.Identity() {
			out = append(out, w)
		}
	}
	return out
}

// noteWritesSent records what a push sent. Once the venue applied it, each
// scanned item's record is the writes the push found for it, which forgets a
// translation that has none any more. A push the venue did not confirm
// forgets the writes it carried, so the next push sends them again.
func (c *BowrainSourceConnector) noteWritesSent(blockMap map[string][]*model.Block, current, sent []venue.EditionWrite, applied bool) {
	if !applied {
		for _, w := range sent {
			delete(c.cache.WritesSent[w.ItemName], w.Edition())
		}
		return
	}
	if c.cache.WritesSent == nil {
		c.cache.WritesSent = map[string]map[string]string{}
	}
	for item := range blockMap {
		delete(c.cache.WritesSent, item)
	}
	for _, w := range current {
		m := c.cache.WritesSent[w.ItemName]
		if m == nil {
			m = map[string]string{}
			c.cache.WritesSent[w.ItemName] = m
		}
		m[w.Edition()] = w.Identity()
	}
}
