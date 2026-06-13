package knowledge

import (
	"strings"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
)

// tree aggregates per-block evaluation hits into the project → collection →
// (stream, locale) breakdown shared by ChangeSetImpact and ConceptUsage. It is
// a generic accumulator: it carries both the change-set counters (newV,
// resolved) and the usage counter (occ); each report projection reads only the
// fields it needs. Aggregation preserves first-seen order at every level so the
// output is deterministic.
type tree struct {
	counts
	scanned    int
	projs      []*projNode
	projIdx    map[string]*projNode
	samples    []BlockSample
	maxSamples int
}

// counts holds the additive measures of a node. blocks is the number of
// affected (or matching) evaluation units folded into the node.
type counts struct {
	blocks   int
	newV     int
	resolved int
	words    int
	occ      int
}

func (c *counts) add(newV, resolved, words, occ int) {
	c.blocks++
	c.newV += newV
	c.resolved += resolved
	c.words += words
	c.occ += occ
}

type projNode struct {
	counts
	id, name string
	cols     []*colNode
	colIdx   map[string]*colNode
}

type colNode struct {
	counts
	id, name string
	leaves   []*leafNode
	leafIdx  map[string]*leafNode
}

type leafNode struct {
	counts
	stream string
	locale model.LocaleID
}

func newTree(maxSamples int) *tree {
	return &tree{projIdx: map[string]*projNode{}, maxSamples: maxSamples}
}

// scan records that one evaluation unit was examined (whether or not it was a
// hit), feeding the TotalBlocks counter.
func (t *tree) scan() { t.scanned++ }

// hit folds an affected (or matching) evaluation unit into every level of the
// tree and records a capped sample.
func (t *tree) hit(p *store.Project, colID, colName, stream string, locale model.LocaleID, newV, resolved, words, occ int, sample BlockSample) {
	t.counts.add(newV, resolved, words, occ)

	pn := t.proj(p)
	pn.counts.add(newV, resolved, words, occ)

	cn := pn.col(colID, colName)
	cn.counts.add(newV, resolved, words, occ)

	lf := cn.leaf(stream, locale)
	lf.counts.add(newV, resolved, words, occ)

	if len(t.samples) < t.maxSamples {
		t.samples = append(t.samples, sample)
	}
}

func (t *tree) proj(p *store.Project) *projNode {
	if n, ok := t.projIdx[p.ID]; ok {
		return n
	}
	n := &projNode{id: p.ID, name: p.Name, colIdx: map[string]*colNode{}}
	t.projIdx[p.ID] = n
	t.projs = append(t.projs, n)
	return n
}

func (n *projNode) col(id, name string) *colNode {
	key := collKey(id, name)
	if c, ok := n.colIdx[key]; ok {
		return c
	}
	c := &colNode{id: id, name: name, leafIdx: map[string]*leafNode{}}
	n.colIdx[key] = c
	n.cols = append(n.cols, c)
	return c
}

func (c *colNode) leaf(stream string, locale model.LocaleID) *leafNode {
	key := stream + "\x00" + string(locale)
	if l, ok := c.leafIdx[key]; ok {
		return l
	}
	l := &leafNode{stream: stream, locale: locale}
	c.leafIdx[key] = l
	c.leaves = append(c.leaves, l)
	return l
}

// collKey groups blocks by collection ID, falling back to the (item) name when
// the block could not be resolved to a collection, so distinct items remain
// distinct buckets.
func collKey(id, name string) string {
	if id != "" {
		return id
	}
	return "name:" + name
}

// toImpact projects the tree into a ChangeSetImpact, using the new/resolved
// counters. All slices are non-nil so the result marshals arrays, never null.
func (t *tree) toImpact() *ChangeSetImpact {
	out := &ChangeSetImpact{
		TotalBlocks:    t.scanned,
		AffectedBlocks: t.counts.blocks,
		NewViolations:  t.counts.newV,
		Resolved:       t.counts.resolved,
		Words:          t.counts.words,
		Projects:       make([]ProjectImpact, 0, len(t.projs)),
		Samples:        t.nonNilSamples(),
	}
	for _, pn := range t.projs {
		pi := ProjectImpact{
			ProjectID:      pn.id,
			ProjectName:    pn.name,
			AffectedBlocks: pn.counts.blocks,
			NewViolations:  pn.counts.newV,
			Resolved:       pn.counts.resolved,
			Words:          pn.counts.words,
			Collections:    make([]CollectionImpact, 0, len(pn.cols)),
		}
		for _, cn := range pn.cols {
			ci := CollectionImpact{
				CollectionID:   cn.id,
				CollectionName: cn.name,
				AffectedBlocks: cn.counts.blocks,
				NewViolations:  cn.counts.newV,
				Resolved:       cn.counts.resolved,
				Words:          cn.counts.words,
				Locales:        make([]LocaleImpact, 0, len(cn.leaves)),
			}
			for _, lf := range cn.leaves {
				ci.Locales = append(ci.Locales, LocaleImpact{
					Stream:         lf.stream,
					Locale:         lf.locale,
					AffectedBlocks: lf.counts.blocks,
					NewViolations:  lf.counts.newV,
					Resolved:       lf.counts.resolved,
					Words:          lf.counts.words,
				})
			}
			pi.Collections = append(pi.Collections, ci)
		}
		out.Projects = append(out.Projects, pi)
	}
	return out
}

// toUsage projects the tree into a ConceptUsage, using the occurrence counter.
func (t *tree) toUsage(conceptID string) *ConceptUsage {
	out := &ConceptUsage{
		ConceptID:   conceptID,
		TotalBlocks: t.scanned,
		Blocks:      t.counts.blocks,
		Occurrences: t.counts.occ,
		Words:       t.counts.words,
		Projects:    make([]ProjectUsage, 0, len(t.projs)),
		Samples:     t.nonNilSamples(),
	}
	for _, pn := range t.projs {
		pu := ProjectUsage{
			ProjectID:   pn.id,
			ProjectName: pn.name,
			Blocks:      pn.counts.blocks,
			Occurrences: pn.counts.occ,
			Words:       pn.counts.words,
			Collections: make([]CollectionUsage, 0, len(pn.cols)),
		}
		for _, cn := range pn.cols {
			cu := CollectionUsage{
				CollectionID:   cn.id,
				CollectionName: cn.name,
				Blocks:         cn.counts.blocks,
				Occurrences:    cn.counts.occ,
				Words:          cn.counts.words,
				Locales:        make([]LocaleUsage, 0, len(cn.leaves)),
			}
			for _, lf := range cn.leaves {
				cu.Locales = append(cu.Locales, LocaleUsage{
					Stream:      lf.stream,
					Locale:      lf.locale,
					Blocks:      lf.counts.blocks,
					Occurrences: lf.counts.occ,
					Words:       lf.counts.words,
				})
			}
			pu.Collections = append(pu.Collections, cu)
		}
		out.Projects = append(out.Projects, pu)
	}
	return out
}

func (t *tree) nonNilSamples() []BlockSample {
	if t.samples == nil {
		return []BlockSample{}
	}
	return t.samples
}

// isBlank reports whether s is empty or all whitespace.
func isBlank(s string) bool {
	return strings.TrimSpace(s) == ""
}

// truncateText caps s to sampleTextLimit runes, appending an ellipsis when it
// trims, so a sample stays light.
func truncateText(s string) string {
	r := []rune(s)
	if len(r) <= sampleTextLimit {
		return s
	}
	return string(r[:sampleTextLimit]) + "…"
}
