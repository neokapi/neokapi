package model

import "maps"

// CopyEditionSet returns a copy of b that holds its own set of editions:
// adding an edition to either block, or removing one, leaves the other's set
// as it was. The derived editions are shared, so a change made to one in place
// (SetTargetRuns, SetEditionStatus or SetEdition on an edition both hold)
// reaches both. The edition the block was read in, and every field outside the
// editions, are copied the way a struct copy copies them: the copy shares the
// source runs and holds the source status as a value of its own.
func (b *Block) CopyEditionSet() *Block {
	c := *b
	c.Targets = maps.Clone(b.Targets)
	return &c
}

// CopyEditions returns a copy of b whose editions are its own: each edition's
// runs, and the source as read (SourceAsRead) once an edit has kept it, are
// copied with copyRuns, so nothing done to an edition of either block
// afterwards reaches the other. Every field outside the editions is copied the
// way a struct copy copies it.
func (b *Block) CopyEditions(copyRuns func([]Run) []Run) *Block {
	c := *b
	c.Source = copyRuns(b.Source)
	if b.sourceKept {
		c.readSource = copyRuns(b.readSource)
	}
	if b.Targets != nil {
		c.Targets = make(map[VariantKey]*Target, len(b.Targets))
		for k, t := range b.Targets {
			if t == nil {
				c.Targets[k] = nil
				continue
			}
			tc := *t
			tc.Runs = copyRuns(t.Runs)
			c.Targets[k] = &tc
		}
	}
	return &c
}
