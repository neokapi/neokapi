package source

import (
	"context"
	"log/slog"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/host"
)

// A checkout records a basis under the key its reader filed the source by: the
// change service and a flow keep the language a format declares (an ARB file's
// @@locale, an XLIFF file's source language), a project read files the source
// under the project's language, and a format that declares none files it under
// no language. The checkout accepts a basis taken under any of them. A venue
// takes the revision of a source under the project's language alone and grades
// a basis by equality with it, so a push sends each basis as the venue takes
// it (venue.Basis), read off the source the push reads.

// pushedSources finds the block a record names as the push reads it.
type pushedSources struct {
	c      *BowrainSourceConnector
	source model.LocaleID
	// priors is the venue's tree the push resolved its blocks against, so an
	// item read here for a record is keyed as the push keys it. Nil when the
	// push could not fetch the tree.
	priors *host.Priors
	// items maps an item to its blocks, under the key the checkout's records
	// name each one by and under the key the venue files it by. An item the
	// push read and found nothing in maps to an empty index, so it is read
	// once.
	items map[string]blockIndex
}

// blockIndex is one item's blocks by key: local under the key the checkout's
// records name a block by (change.BlockKey before resolution), venue under the
// key the push resolved it to (Block.Key).
type blockIndex struct {
	local map[string]*model.Block
	venue map[string]*model.Block
}

// pushedSources indexes the blocks the push scanned, under the key the venue
// files each unit by and the key the block had before the push resolved it.
func (c *BowrainSourceConnector) pushedSources(blockMap map[string][]*model.Block, local map[*model.Block]string, priors *host.Priors) *pushedSources {
	s := &pushedSources{c: c, source: c.sourceLanguage(), priors: priors, items: map[string]blockIndex{}}
	for item, blocks := range blockMap {
		s.index(item, blocks, local)
	}
	return s
}

func (s *pushedSources) index(item string, blocks []*model.Block, local map[*model.Block]string) {
	idx := blockIndex{local: make(map[string]*model.Block, len(blocks)), venue: make(map[string]*model.Block, len(blocks))}
	for _, b := range blocks {
		k := local[b]
		if k == "" {
			k = change.BlockKey(b)
		}
		idx.local[k] = b
		idx.venue[convergence.BlockKey(b)] = b
	}
	s.items[item] = idx
}

// keyDecisions names each decision's unit as the venue files it and carries
// its basis as the venue takes it.
//
// The checkout's records name a block by the key its reader gave it, and the
// push resolves every block to a durable key against what the venue holds
// (host.ResolveIdentity), minting one for content the venue has never seen. A
// decision sent under the reader's key would land on no unit the venue holds,
// so it is sent under the key the push filed its block under. A record whose
// block the push cannot find travels as it is.
//
// A basis that names another source than the one the push reads travels as
// it is: the venue grades it stale, as the checkout does.
func (s *pushedSources) keyDecisions(ctx context.Context, decisions []venue.UnitDecision) {
	for i := range decisions {
		d := &decisions[i]
		b := s.block(ctx, d.ItemName, d.Unit)
		if b == nil {
			continue
		}
		d.Unit = convergence.BlockKey(b)
		if d.Basis != "" {
			d.Basis = venue.Basis(b, s.source, d.Basis)
		}
	}
}

// block is the block a record keyed unit in item names: the block the
// checkout's records name by unit, else the one the venue files under it. An
// item this push did not scan (a push of named paths carries the decisions of
// every document) is read and resolved as a push of it would read it, once.
func (s *pushedSources) block(ctx context.Context, item, unit string) *model.Block {
	idx, ok := s.items[item]
	if !ok {
		s.items[item] = blockIndex{}
		if !writableItemName(item) {
			return nil
		}
		scan, err := s.c.scanLocal(ctx, []string{item})
		if err != nil {
			slog.DebugContext(ctx, "read the source a decision names", "item", item, "error", err)
			return nil
		}
		if blocks, read := scan.blocks[item]; read {
			local := localBlockKeys(scan.blocks)
			if s.priors != nil {
				host.ResolveIdentity(scan.blocks, *s.priors)
			}
			s.index(item, blocks, local)
		}
		idx = s.items[item]
	}
	if b := idx.local[unit]; b != nil {
		return b
	}
	return idx.venue[unit]
}
