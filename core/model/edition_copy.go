package model

import (
	"maps"
	"slices"
)

// CopyEditionSet returns a copy of b that holds its own set of editions:
// adding an edition to either block, or removing one, leaves the other's set
// as it was, and so does marking one native. The derived editions, the
// translation filed under no language among them, are shared, so a change made
// to one in place (SetTargetRuns, SetEditionStatus or SetEdition on an edition
// both hold) reaches both. The edition the block was read in is copied as an
// entry of its own: the copy shares its runs and holds its status and its
// derivation as values of its own. Every field outside the editions is copied
// the way a struct copy copies it.
func (b *Block) CopyEditionSet() *Block {
	c := *b
	c.Editions = maps.Clone(b.Editions)
	if s := b.source(); s != nil {
		sc := *s
		c.Editions[EditionKey{}] = &sc
	}
	c.Native = slices.Clone(b.Native)
	return &c
}

// CopyEditions returns a copy of b whose editions are its own: each edition's
// runs, and the source as read (SourceAsRead) once an edit has kept it, are
// copied with copyRuns, so nothing done to an edition of either block
// afterwards reaches the other. Every field outside the editions is copied the
// way a struct copy copies it.
func (b *Block) CopyEditions(copyRuns func([]Run) []Run) *Block {
	c := *b
	if b.sourceKept {
		c.readSource = copyRuns(b.readSource)
	}
	if b.Editions != nil {
		c.Editions = make(map[EditionKey]*Edition, len(b.Editions))
		for k, e := range b.Editions {
			c.Editions[k] = copyEdition(e, copyRuns)
		}
	}
	c.unlabelled = copyEdition(b.unlabelled, copyRuns)
	c.Native = slices.Clone(b.Native)
	return &c
}

// copyEdition returns a copy of e with its runs copied by copyRuns and a
// derivation of its own, or nil for nil.
func copyEdition(e *Edition, copyRuns func([]Run) []Run) *Edition {
	if e == nil {
		return nil
	}
	ec := *e
	ec.Runs = copyRuns(e.Runs)
	if e.Derived != nil {
		d := *e.Derived
		ec.Derived = &d
	}
	return &ec
}
