package place

import (
	"bufio"
	"bytes"
	_ "embed"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
)

//go:embed subdivisions.tsv
var subdivisionData []byte

// Gazetteer is the built-in places plus a project's declared places. A nil
// *Gazetteer answers as Builtin() in every method, and a gazetteer is safe for
// concurrent use.
type Gazetteer struct {
	base *index
	// own are the declared places, by id.
	own map[string]*Place
	// ownIDs are the declared ids in order.
	ownIDs []string
	// parent is each declared place's Path parent.
	parent map[string]string
	// children are the declared places by Path parent, sorted by name.
	children map[string][]string
}

// index is the built-in places.
type index struct {
	places map[string]*Place
	// children are the built-in places by parent, sorted by name.
	children map[string][]string
	// m49 maps a three-digit M49 code to its region's id.
	m49 map[string]string
	// order is every built-in id, sorted by type rank and then id, the order
	// Search walks.
	order []string
	// folded is each built-in place's name, folded for Search.
	folded map[string]string
}

var (
	builtinOnce sync.Once
	builtin     *Gazetteer
)

// Builtin is the gazetteer of built-in places: world, the UN M49 regions, the
// ISO 3166-1 countries and the ISO 3166-2 subdivisions. It is shared and
// immutable.
func Builtin() *Gazetteer {
	builtinOnce.Do(func() {
		idx, err := buildIndex()
		if err != nil {
			panic("place: built-in data: " + err.Error())
		}
		builtin = &Gazetteer{base: idx}
	})
	return builtin
}

func buildIndex() (*index, error) {
	idx := &index{places: map[string]*Place{}, children: map[string][]string{}, m49: map[string]string{}, folded: map[string]string{}}
	add := func(p *Place) error {
		if _, dup := idx.places[p.ID]; dup {
			return fmt.Errorf("%s is listed twice", p.ID)
		}
		idx.places[p.ID] = p
		return nil
	}
	if err := add(&Place{ID: World, Name: "World", Type: TypeWorld}); err != nil {
		return nil, err
	}
	for _, r := range regions {
		if err := add(&Place{ID: r.slug, Name: r.name, Type: TypeRegion, Within: []string{r.parent}}); err != nil {
			return nil, err
		}
		idx.m49[r.code] = r.slug
	}
	for _, c := range countries {
		if err := add(&Place{ID: c.code, Name: c.name, Type: TypeCountry, Within: []string{c.region}, Centroid: &LatLon{Lat: c.lat, Lon: c.lon}}); err != nil {
			return nil, err
		}
	}
	sc := bufio.NewScanner(bytes.NewReader(subdivisionData))
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 3 || f[0] == "" || f[1] == "" || f[2] == "" {
			return nil, fmt.Errorf("subdivisions.tsv line %d: want id, parent and name", n)
		}
		if err := add(&Place{ID: f[0], Name: f[2], Type: TypeSubdivision, Within: []string{f[1]}}); err != nil {
			return nil, err
		}
	}
	for id, p := range idx.places {
		for _, parent := range p.Within {
			if _, ok := idx.places[parent]; !ok {
				return nil, fmt.Errorf("%s sits within %s, which is not a place", id, parent)
			}
			idx.children[parent] = append(idx.children[parent], id)
		}
		idx.order = append(idx.order, id)
		idx.folded[id] = fold(p.Name)
	}
	for parent := range idx.children {
		sortByName(idx.children[parent], func(id string) *Place { return idx.places[id] })
	}
	sort.Slice(idx.order, func(i, j int) bool {
		a, b := idx.places[idx.order[i]], idx.places[idx.order[j]]
		if ra, rb := a.Type.Rank(), b.Type.Rank(); ra != rb {
			return ra < rb
		}
		return a.ID < b.ID
	})
	return idx, nil
}

// sortByName sorts ids by their place's name, then by id.
func sortByName(ids []string, get func(string) *Place) {
	sort.SliceStable(ids, func(i, j int) bool {
		a, b := get(ids[i]), get(ids[j])
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ID < b.ID
	})
}

func (g *Gazetteer) or() *Gazetteer {
	if g == nil || g.base == nil {
		return Builtin()
	}
	return g
}

// get returns the place with id, shared rather than copied.
func (g *Gazetteer) get(id string) (*Place, bool) {
	g = g.or()
	if p, ok := g.own[id]; ok {
		return p, true
	}
	p, ok := g.base.places[id]
	return p, ok
}

// Place returns the place with id.
func (g *Gazetteer) Place(id string) (Place, bool) {
	p, ok := g.get(id)
	if !ok {
		return Place{}, false
	}
	return p.clone(), true
}

// Has reports whether id names a place.
func (g *Gazetteer) Has(id string) bool {
	_, ok := g.get(id)
	return ok
}

// Name is the display name of the place with id, empty when id names none.
func (g *Gazetteer) Name(id string) string {
	if p, ok := g.get(id); ok {
		return p.Name
	}
	return ""
}

// Contains reports whether inner sits within outer. Every id contains itself,
// known or not, so a value another checkout declares still answers content at
// that same value; an unknown id contains nothing else and sits within nothing
// else. World contains every known place. A place made of places sits within
// outer when every member does, and contains inner when some member does or
// inner is declared within it.
func (g *Gazetteer) Contains(outer, inner string) bool {
	if outer == "" || inner == "" {
		return false
	}
	if outer == inner {
		return true
	}
	g = g.or()
	o, ok := g.get(outer)
	if !ok {
		return false
	}
	i, ok := g.get(inner)
	if !ok {
		return false
	}
	return g.contains(o, i)
}

func (g *Gazetteer) contains(o, i *Place) bool {
	if o.ID == i.ID {
		return true
	}
	if o.ID == World {
		return true
	}
	if i.MadeOf() {
		for _, m := range i.Of {
			mp, ok := g.get(m)
			if !ok || !g.contains(o, mp) {
				return false
			}
		}
		return true
	}
	if o.MadeOf() {
		for _, m := range o.Of {
			if mp, ok := g.get(m); ok && g.contains(mp, i) {
				return true
			}
		}
	}
	for _, parent := range i.Within {
		if pp, ok := g.get(parent); ok && g.contains(o, pp) {
			return true
		}
	}
	return false
}

// Overlaps reports whether two places share ground: one contains the other, a
// member of a place made of places overlaps the other, or a declared place
// sits within both. Built-in places form a tree, so two built-in places
// neither of which contains the other share no ground; only a declared place
// with several `within:` parents joins two branches.
func (g *Gazetteer) Overlaps(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	g = g.or()
	pa, ok := g.get(a)
	if !ok {
		return false
	}
	pb, ok := g.get(b)
	if !ok {
		return false
	}
	return g.overlaps(pa, pb)
}

func (g *Gazetteer) overlaps(a, b *Place) bool {
	if g.contains(a, b) || g.contains(b, a) {
		return true
	}
	if a.MadeOf() {
		for _, m := range a.Of {
			if mp, ok := g.get(m); ok && g.overlaps(mp, b) {
				return true
			}
		}
		return false
	}
	if b.MadeOf() {
		for _, m := range b.Of {
			if mp, ok := g.get(m); ok && g.overlaps(a, mp) {
				return true
			}
		}
		return false
	}
	for _, id := range g.ownIDs {
		x := g.own[id]
		if g.contains(a, x) && g.contains(b, x) {
			return true
		}
	}
	return false
}

// parentOf is a place's Path parent: its first `within:` place, or for a
// place made of places the nearest place that contains every member. World
// and an unknown id have none.
func (g *Gazetteer) parentOf(id string) string {
	g = g.or()
	if p, ok := g.parent[id]; ok {
		return p
	}
	if p, ok := g.base.places[id]; ok && len(p.Within) > 0 {
		return p.Within[0]
	}
	return ""
}

// Path is the chain of Path parents from world down to id:
// [world europe northern-europe no no-03 oslo]. A place within several places
// follows its first `within:` parent; a place made of places follows the
// nearest place that contains every member. An unknown id has no path.
func (g *Gazetteer) Path(id string) []string {
	g = g.or()
	if !g.Has(id) {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for v := id; v != "" && !seen[v]; v = g.parentOf(v) {
		seen[v] = true
		out = append(out, v)
	}
	slices.Reverse(out)
	return out
}

// Ancestors lists every place that contains id, nearest first, without
// duplicates: the places it sits within, the places those sit within, and
// every declared place made of places that contains it. An unknown id has
// none.
func (g *Gazetteer) Ancestors(id string) []string {
	g = g.or()
	p, ok := g.get(id)
	if !ok {
		return nil
	}
	var chain []string
	if p.MadeOf() {
		first := p.Of[0]
		for _, c := range append([]string{first}, g.Ancestors(first)...) {
			if c != id && g.Contains(c, id) {
				chain = append(chain, c)
			}
		}
		return chain
	}
	seen := map[string]bool{id: true}
	frontier := p.Within
	for len(frontier) > 0 {
		var next []string
		for _, v := range frontier {
			if seen[v] {
				continue
			}
			seen[v] = true
			chain = append(chain, v)
			if vp, ok := g.get(v); ok {
				next = append(next, vp.Within...)
			}
		}
		frontier = next
	}
	return g.withMadeOf(id, chain, seen)
}

// withMadeOf inserts into chain each declared place made of places that
// contains id, just before the nearest place in chain that contains it.
func (g *Gazetteer) withMadeOf(id string, chain []string, seen map[string]bool) []string {
	type insert struct {
		id  string
		pos int
	}
	var inserts []insert
	for _, oid := range g.ownIDs {
		op := g.own[oid]
		if !op.MadeOf() || seen[oid] || !g.Contains(oid, id) {
			continue
		}
		pos := len(chain)
		for i, c := range chain {
			if g.Contains(c, oid) {
				pos = i
				break
			}
		}
		inserts = append(inserts, insert{oid, pos})
	}
	sort.SliceStable(inserts, func(i, j int) bool {
		if inserts[i].pos != inserts[j].pos {
			return inserts[i].pos < inserts[j].pos
		}
		// The narrower of two places at one position comes first.
		return g.Contains(inserts[j].id, inserts[i].id) && !g.Contains(inserts[i].id, inserts[j].id)
	})
	out := make([]string, 0, len(chain)+len(inserts))
	k := 0
	for i := 0; i <= len(chain); i++ {
		for k < len(inserts) && inserts[k].pos == i {
			out = append(out, inserts[k].id)
			k++
		}
		if i < len(chain) {
			out = append(out, chain[i])
		}
	}
	return out
}

// Children are the places directly below id: the places whose Path parent is
// id, and the members of a place made of places, without duplicates, sorted
// by name and then id.
func (g *Gazetteer) Children(id string) []string {
	g = g.or()
	p, ok := g.get(id)
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(ids []string) {
		for _, c := range ids {
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	add(g.base.children[id])
	add(g.children[id])
	add(p.Of)
	sortByName(out, func(id string) *Place { q, _ := g.get(id); return q })
	return out
}

// Declared lists the declared places, by id.
func (g *Gazetteer) Declared() []Place {
	g = g.or()
	out := make([]Place, 0, len(g.ownIDs))
	for _, id := range g.ownIDs {
		out = append(out, g.own[id].clone())
	}
	return out
}

// Describe is a place's name, followed by its country's name for a place
// below a country ("Oslo, Norway"), the id alone for an unknown id.
func (g *Gazetteer) Describe(id string) string {
	g = g.or()
	p, ok := g.get(id)
	if !ok {
		return id
	}
	for _, a := range g.Path(id) {
		if a == id {
			break
		}
		if ap, _ := g.get(a); ap.Type == TypeCountry {
			return p.Name + ", " + ap.Name
		}
	}
	return p.Name
}
