package source

import (
	"context"
	"log/slog"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// A checkout records a basis under the key its reader filed the source by: the
// change service and a flow keep the language a format declares (an ARB file's
// @@locale, an XLIFF file's source language), a project read files the source
// under the project's language, and a format that declares none files it under
// no language. The checkout accepts a basis taken under any of them. A venue
// takes the revision of a source under the project's language alone and grades
// a basis by equality with it, so a push sends each basis as the venue takes
// it (venue.Basis), read off the source the push reads.

// pushedSources finds the source a record names as the push reads it.
type pushedSources struct {
	c      *BowrainSourceConnector
	source model.LocaleID
	// items maps an item to its blocks by unit key. An item the push read and
	// found nothing in maps to nil, so it is read once.
	items map[string]map[string]*model.Block
}

// pushedSources indexes the blocks the push scanned, under the key the venue
// files each unit by and the key the block had before the push resolved it.
func (c *BowrainSourceConnector) pushedSources(blockMap map[string][]*model.Block, local map[*model.Block]string) *pushedSources {
	s := &pushedSources{c: c, source: c.sourceLanguage(), items: map[string]map[string]*model.Block{}}
	for item, blocks := range blockMap {
		s.index(item, blocks, local)
	}
	return s
}

func (s *pushedSources) index(item string, blocks []*model.Block, local map[*model.Block]string) {
	m := make(map[string]*model.Block, len(blocks))
	for _, b := range blocks {
		if k, ok := local[b]; ok && k != "" {
			m[k] = b
		}
	}
	for _, b := range blocks {
		m[convergence.BlockKey(b)] = b
	}
	s.items[item] = m
}

// carryBases rewrites the basis of each decision as the venue takes it. A
// basis that names another source than the one the push reads, and one whose
// unit the push cannot find, travel as they are.
func (s *pushedSources) carryBases(ctx context.Context, decisions []venue.UnitDecision) {
	for i := range decisions {
		d := &decisions[i]
		if d.Basis == "" {
			continue
		}
		d.Basis = venue.Basis(s.block(ctx, d.ItemName, d.Unit), s.source, d.Basis)
	}
}

// block is the block keyed unit in item. An item this push did not scan (a
// push of named paths carries the decisions of every document) is read as a
// push of it would read it, once.
func (s *pushedSources) block(ctx context.Context, item, unit string) *model.Block {
	m, ok := s.items[item]
	if !ok {
		s.items[item] = nil
		if !writableItemName(item) {
			return nil
		}
		scan, err := s.c.scanLocal(ctx, []string{item})
		if err != nil {
			slog.DebugContext(ctx, "read the source a decision names", "item", item, "error", err)
			return nil
		}
		if blocks, read := scan.blocks[item]; read {
			s.index(item, blocks, nil)
		}
		m = s.items[item]
	}
	return m[unit]
}
