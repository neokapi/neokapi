package place

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"

	"golang.org/x/text/language"
)

// slugPattern is the shape of a declared place's id.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

const slugRule = "lowercase letters, digits and hyphens, starting with a letter or digit"

// refPatterns are the schemes a declared place's refs may use, with the shape
// of each scheme's value.
var refPatterns = map[string]*regexp.Regexp{
	"wikidata": regexp.MustCompile(`^Q[0-9]+$`),
	"geonames": regexp.MustCompile(`^[0-9]+$`),
	"nuts":     regexp.MustCompile(`^[A-Z]{2}[0-9A-Z]{0,3}$`),
	"osm":      regexp.MustCompile(`^(node|way|relation)/[0-9]+$`),
	"wof":      regexp.MustCompile(`^[0-9]+$`),
}

var refExamples = "wikidata:Q585, geonames:3143244, nuts:NO081, osm:relation/406091 or wof:101752073"

// New is the gazetteer of the built-in places plus declared, a recipe's
// `places:` block by id. A nil or empty map is Builtin().
//
// Every declared place is validated, and an error names the recipe key at
// fault (`places.<id>...`). The id is a slug no built-in place holds. The
// place sits within places or is made of places, exactly one of the two, and
// every place it names is known. No place reaches itself. The type is one a
// recipe may declare, names are keyed by locale, and refs, the centroid and
// the shape path are well formed.
//
// An empty type is a territory for a place made of places and an area
// otherwise. An empty name is the id.
func New(declared map[string]Place) (*Gazetteer, error) {
	base := Builtin()
	if len(declared) == 0 {
		return base, nil
	}
	g := &Gazetteer{base: base.base, own: map[string]*Place{}, parent: map[string]string{}, children: map[string][]string{}}
	ids := make([]string, 0, len(declared))
	for id := range declared {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		where := "places." + id
		if !slugPattern.MatchString(id) {
			return nil, fmt.Errorf("places: place id %q is not a slug (%s)", id, slugRule)
		}
		if _, ok := base.base.places[id]; ok {
			return nil, fmt.Errorf("%s: %s is a built-in place (%s); a declared place takes an id of its own", where, id, base.Describe(id))
		}
		p := declared[id].clone()
		if p.ID != "" && p.ID != id {
			return nil, fmt.Errorf("%s: the place's id %q differs from its key", where, p.ID)
		}
		p.ID = id
		p.Declared = true
		if err := normalize(where, &p); err != nil {
			return nil, err
		}
		g.own[id] = &p
	}
	g.ownIDs = ids
	for _, id := range ids {
		p := g.own[id]
		field := "within"
		if p.MadeOf() {
			field = "of"
		}
		for _, ref := range slices.Concat(p.Within, p.Of) {
			if !g.Has(ref) {
				return nil, fmt.Errorf("places.%s.%s: %q is not a place. %s", id, field, ref, Hint(g, ref))
			}
		}
	}
	if cycle := g.cycle(); cycle != "" {
		return nil, fmt.Errorf("places.%s: the place sits within itself through %s", cycle, strings.Join(g.cyclePath(cycle), " › "))
	}
	for _, id := range ids {
		parent := g.declaredParent(g.own[id])
		g.parent[id] = parent
		g.children[parent] = append(g.children[parent], id)
	}
	for parent := range g.children {
		sortByName(g.children[parent], func(id string) *Place { p, _ := g.get(id); return p })
	}
	return g, nil
}

// normalize checks one declared place's own fields and fills its defaults.
func normalize(where string, p *Place) error {
	switch {
	case len(p.Within) > 0 && len(p.Of) > 0:
		return fmt.Errorf("%s: a place sits within places or is made of places, never both", where)
	case len(p.Within) == 0 && len(p.Of) == 0:
		return fmt.Errorf("%s: a place names the places it sits within (`within:`) or is made of (`of:`)", where)
	}
	for field, list := range map[string][]string{"within": p.Within, "of": p.Of} {
		seen := map[string]bool{}
		for _, ref := range list {
			switch {
			case ref == "":
				return fmt.Errorf("%s.%s: an empty place id", where, field)
			case ref == p.ID:
				return fmt.Errorf("%s.%s: a place cannot sit within or be made of itself", where, field)
			case seen[ref]:
				return fmt.Errorf("%s.%s: %q is named twice", where, field, ref)
			}
			seen[ref] = true
		}
	}
	switch {
	case p.Type == "" && p.MadeOf():
		p.Type = TypeTerritory
	case p.Type == "":
		p.Type = TypeArea
	case !slices.Contains(DeclarableTypes, p.Type):
		return fmt.Errorf("%s.type: %q is not a type a declared place takes. Use one of %s", where, p.Type, typeList())
	}
	if strings.TrimSpace(p.Name) == "" {
		p.Name = p.ID
	}
	if len(p.Names) > 0 {
		names := make(map[string]string, len(p.Names))
		for _, key := range sortedKeys(p.Names) {
			tag, err := language.Parse(key)
			if err != nil {
				return fmt.Errorf("%s.names: %q is not a locale (write a BCP 47 tag such as nb or en-GB)", where, key)
			}
			canonical := tag.String()
			if _, dup := names[canonical]; dup {
				return fmt.Errorf("%s.names: %q and another key name the same locale %s", where, key, canonical)
			}
			if strings.TrimSpace(p.Names[key]) == "" {
				return fmt.Errorf("%s.names.%s: an empty name", where, key)
			}
			names[canonical] = p.Names[key]
		}
		p.Names = names
	}
	seen := map[string]bool{}
	for _, ref := range p.Refs {
		scheme, value, ok := strings.Cut(ref, ":")
		pattern, known := refPatterns[scheme]
		switch {
		case !ok || !known:
			return fmt.Errorf("%s.refs: %q is not a reference. Write scheme:value, such as %s", where, ref, refExamples)
		case !pattern.MatchString(value):
			return fmt.Errorf("%s.refs: %q does not follow the %s scheme (write it as %s)", where, ref, scheme, refExample(scheme))
		case seen[ref]:
			return fmt.Errorf("%s.refs: %q is listed twice", where, ref)
		}
		seen[ref] = true
	}
	if p.Centroid != nil && !p.Centroid.Valid() {
		return fmt.Errorf("%s.centroid: [%v, %v] is not on the globe: latitude runs from -90 to 90 and longitude from -180 to 180", where, p.Centroid.Lat, p.Centroid.Lon)
	}
	if p.Shape != "" {
		if err := checkShapePath(p.Shape); err != nil {
			return fmt.Errorf("%s.shape: %w", where, err)
		}
	}
	return nil
}

// checkShapePath checks a shape's path: project-relative, slash-separated,
// inside the project, and naming a .geojson or .json file.
func checkShapePath(p string) error {
	switch {
	case strings.Contains(p, `\`):
		return fmt.Errorf("%q: write the path with forward slashes", p)
	case strings.HasPrefix(p, "/") || (len(p) > 1 && p[1] == ':'):
		return fmt.Errorf("%q: write the path relative to the project", p)
	case slices.Contains(strings.Split(p, "/"), ".."):
		return fmt.Errorf("%q: the path leaves the project", p)
	case path.Clean(p) != p:
		return fmt.Errorf("%q: write the path as %s", p, path.Clean(p))
	}
	if ext := strings.ToLower(path.Ext(p)); ext != ".geojson" && ext != ".json" {
		return fmt.Errorf("%q: a shape is a GeoJSON file ending .geojson or .json", p)
	}
	return nil
}

func refExample(scheme string) string {
	return map[string]string{
		"wikidata": "wikidata:Q585", "geonames": "geonames:3143244", "nuts": "nuts:NO081",
		"osm": "osm:relation/406091", "wof": "wof:101752073",
	}[scheme]
}

func typeList() string {
	out := make([]string, len(DeclarableTypes))
	for i, t := range DeclarableTypes {
		out[i] = string(t)
	}
	return strings.Join(out, ", ")
}

// cycle names a declared place that reaches itself through `within:` and
// `of:`, empty when none does.
func (g *Gazetteer) cycle() string {
	const (
		unvisited = iota
		visiting
		done
	)
	state := map[string]int{}
	var visit func(id string) bool
	visit = func(id string) bool {
		p, ok := g.own[id]
		if !ok {
			return false
		}
		switch state[id] {
		case visiting:
			return true
		case done:
			return false
		}
		state[id] = visiting
		if slices.ContainsFunc(slices.Concat(p.Within, p.Of), visit) {
			return true
		}
		state[id] = done
		return false
	}
	for _, id := range g.ownIDs {
		if state[id] == unvisited && visit(id) {
			for _, c := range g.ownIDs {
				if state[c] == visiting && g.reaches(c, c, map[string]bool{}) {
					return c
				}
			}
			return id
		}
	}
	return ""
}

// reaches reports whether target is reachable from id through declared
// `within:` and `of:` edges.
func (g *Gazetteer) reaches(id, target string, seen map[string]bool) bool {
	p, ok := g.own[id]
	if !ok || seen[id] {
		return false
	}
	seen[id] = true
	for _, next := range slices.Concat(p.Within, p.Of) {
		if next == target || g.reaches(next, target, seen) {
			return true
		}
	}
	return false
}

// cyclePath is the loop from id back to id, for an error message.
func (g *Gazetteer) cyclePath(id string) []string {
	var walk func(cur string, path []string, seen map[string]bool) []string
	walk = func(cur string, path []string, seen map[string]bool) []string {
		p, ok := g.own[cur]
		if !ok || seen[cur] {
			return nil
		}
		seen[cur] = true
		for _, next := range slices.Concat(p.Within, p.Of) {
			if next == id {
				return append(path, next)
			}
			if found := walk(next, append(path, next), seen); found != nil {
				return found
			}
		}
		return nil
	}
	return walk(id, []string{id}, map[string]bool{})
}

// declaredParent is a declared place's Path parent: its first `within:`
// place, or, for a place made of places, the nearest place on its first
// member's path, starting from the member itself, that contains every member.
func (g *Gazetteer) declaredParent(p *Place) string {
	if !p.MadeOf() {
		return p.Within[0]
	}
	for _, candidate := range slices.Backward(g.pathWhileBuilding(p.Of[0])) {
		if g.containsAll(candidate, p.Of) {
			return candidate
		}
	}
	return World
}

// containsAll reports whether outer contains every one of ids.
func (g *Gazetteer) containsAll(outer string, ids []string) bool {
	for _, id := range ids {
		if !g.Contains(outer, id) {
			return false
		}
	}
	return true
}

// pathWhileBuilding is Path during New, computing a declared place's parent
// on demand when it is not recorded yet.
func (g *Gazetteer) pathWhileBuilding(id string) []string {
	var out []string
	seen := map[string]bool{}
	for v := id; v != "" && !seen[v]; {
		seen[v] = true
		out = append(out, v)
		if dp, ok := g.own[v]; ok {
			parent, recorded := g.parent[v]
			if !recorded {
				parent = g.declaredParent(dp)
				g.parent[v] = parent
			}
			v = parent
			continue
		}
		v = g.parentOf(v)
	}
	slices.Reverse(out)
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
