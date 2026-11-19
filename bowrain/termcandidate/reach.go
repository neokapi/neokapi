package termcandidate

import (
	"cmp"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// Reach is how many blocks use a term, in all and per collection.
type Reach struct {
	Blocks      int
	Collections []CollectionCount
}

// CollectionCount is the blocks of one collection that use a term.
type CollectionCount struct {
	Name   string
	Blocks int
}

// Counter counts, for each candidate term, the blocks whose source text uses
// it. It matches with the engine's term matcher (core/check), as the term
// check does: an English source term with its regular inflections, any other
// language by its exact spelling, both case-insensitively and as whole words.
type Counter struct {
	english bool
	terms   []string
	total   []int
	per     []map[string]int
}

// NewCounter counts the uses of candidate terms in text of the source
// language. Blank terms and case-insensitive repeats are dropped.
func NewCounter(source model.LocaleID, candidates []string) *Counter {
	c := &Counter{english: terms.BaseLanguage(source) == "en"}
	seen := map[string]bool{}
	for _, t := range candidates {
		t = strings.TrimSpace(t)
		key := strings.ToLower(t)
		if t == "" || seen[key] {
			continue
		}
		seen[key] = true
		c.terms = append(c.terms, t)
		c.total = append(c.total, 0)
		c.per = append(c.per, map[string]int{})
	}
	return c
}

// Terms are the terms the counter counts. A block that uses one of them
// contains it as a substring, so a store can read only the blocks holding
// one (platstore.BlockQuery.TextAny) and the count is the same.
func (c *Counter) Terms() []string { return slices.Clone(c.terms) }

// Add counts one block of a collection.
func (c *Counter) Add(collection, sourceText string) {
	if strings.TrimSpace(sourceText) == "" {
		return
	}
	p := check.PrepareText(check.TermText(sourceText))
	for i, t := range c.terms {
		if !uses(p, t, c.english) {
			continue
		}
		c.total[i]++
		c.per[i][collection]++
	}
}

// uses reports whether prepared text uses term: an English term with its
// regular inflections, any other by its exact spelling.
func uses(p *check.PreparedText, term string, english bool) bool {
	if english {
		return len(check.FindEnglishInflectionsIn(p, term, false)) > 0
	}
	return len(check.FindTermFormsIn(p, []string{term}, false)) > 0
}

// Reach is the count for term, which compares case-insensitively.
func (c *Counter) Reach(term string) Reach {
	key := strings.ToLower(strings.TrimSpace(term))
	for i, t := range c.terms {
		if strings.ToLower(t) == key {
			return newReach(c.per[i])
		}
	}
	return Reach{}
}

// newReach orders a per-collection count by blocks, most first, then name.
func newReach(per map[string]int) Reach {
	r := Reach{}
	for name, n := range per {
		r.Blocks += n
		r.Collections = append(r.Collections, CollectionCount{Name: name, Blocks: n})
	}
	slices.SortFunc(r.Collections, func(a, b CollectionCount) int {
		return cmp.Or(cmp.Compare(b.Blocks, a.Blocks), strings.Compare(a.Name, b.Name))
	})
	return r
}

// Root is the coordinates every declared collection shares: the project's
// own point. The items no declared collection holds sit at it too, so they
// take no part in it.
func Root(cols []Collection) map[string]string {
	var root map[string]string
	first := true
	for _, col := range cols {
		if col.Name == "" {
			continue
		}
		if first {
			root, first = maps.Clone(col.Coordinates), false
			continue
		}
		maps.DeleteFunc(root, func(axis, value string) bool { return col.Coordinates[axis] != value })
	}
	return root
}

// Scope is the coordinates a term is scoped to: those every collection
// using it shares, less the project's own point. A term used only in the
// collections of one product is scoped to that product, one used only in
// one channel of it to the product and the channel, and one used across
// products to none (it holds across the project). A term no block uses is
// scoped to none.
func Scope(r Reach, cols []Collection) map[string]string {
	at := map[string]map[string]string{}
	for _, col := range cols {
		at[col.Name] = col.Coordinates
	}
	var shared map[string]string
	first := true
	for _, cc := range r.Collections {
		if cc.Blocks == 0 {
			continue
		}
		coords := at[cc.Name]
		if first {
			shared, first = maps.Clone(coords), false
			continue
		}
		maps.DeleteFunc(shared, func(axis, value string) bool { return coords[axis] != value })
	}
	root := Root(cols)
	maps.DeleteFunc(shared, func(axis, value string) bool { return root[axis] == value })
	if len(shared) == 0 {
		return nil
	}
	return shared
}

// Concept properties a candidate carries.
const (
	// PropReachBlocks is the number of blocks that use the candidate.
	PropReachBlocks = "reach_blocks"
	// PropReachCollections is the blocks per collection, written
	// `name=count` and joined with commas, most first.
	PropReachCollections = "reach_collections"
	// PropProposedBy is the identity that filed the candidate, such as
	// "tool/context-scan". The context deck names it as the author.
	PropProposedBy = "proposed_by"
	// PropProject is the project a candidate was found in. The PostgreSQL
	// terms store keeps no project column, so the id travels as a property
	// beside terms.Concept.ProjectID.
	PropProject = "project_id"
)

// Apply writes the reach onto a concept's properties.
func (r Reach) Apply(cp *terms.Concept) {
	if cp.Properties == nil {
		cp.Properties = map[string]string{}
	}
	cp.Properties[PropReachBlocks] = strconv.Itoa(r.Blocks)
	parts := make([]string, 0, len(r.Collections))
	for _, cc := range r.Collections {
		parts = append(parts, cc.Name+"="+strconv.Itoa(cc.Blocks))
	}
	if len(parts) > 0 {
		cp.Properties[PropReachCollections] = strings.Join(parts, ",")
	} else {
		delete(cp.Properties, PropReachCollections)
	}
}

// ReachOf reads the reach a concept carries, and whether it carries one.
func ReachOf(cp terms.Concept) (Reach, bool) {
	raw, ok := cp.Properties[PropReachBlocks]
	if !ok {
		return Reach{}, false
	}
	blocks, err := strconv.Atoi(raw)
	if err != nil {
		return Reach{}, false
	}
	r := Reach{Blocks: blocks}
	for part := range strings.SplitSeq(cp.Properties[PropReachCollections], ",") {
		name, count, ok := strings.CutLast(part, "=")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(count)
		if err != nil {
			continue
		}
		r.Collections = append(r.Collections, CollectionCount{Name: name, Blocks: n})
	}
	return r, true
}
