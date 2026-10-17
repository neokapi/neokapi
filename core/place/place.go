// Package place is the gazetteer the context space reads geography from.
//
// A place is a named area with an id: the world, a UN M49 region, an ISO
// 3166-1 country, an ISO 3166-2 subdivision, or a place a project declares
// under `places:` in its recipe, such as a city, a district or a sales
// territory. Each place sits within one or more places or is made of places,
// and those declarations are the whole of containment: a rule scoped to
// Norway answers for content at Oslo because Oslo sits within the county
// no-03, which sits within Norway. No boundary is ever computed from a shape.
//
// The built-in places form a tree under world. Countries carry their M49
// region and an approximate centroid. Subdivisions, their English names and
// their containment (including second levels such as fr-ara holding fr-01)
// come from Unicode CLDR, at the release internal/gen pins, plus the ISO
// 3166-2 changes listed in internal/gen/amendments.tsv that the release does
// not record yet. UNICODE-LICENSE.txt carries the licence of that data.
//
// The package depends on nothing else in neokapi, so the recipe, the axis
// space, the operation log, the host and the atlas share one reading of a
// place.
package place

//go:generate go run ./internal/gen -out subdivisions.tsv

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"

	"gopkg.in/yaml.v3"
)

// World is the id of the place that contains every other place.
const World = "world"

// Type names what kind of place a place is.
type Type string

const (
	// TypeWorld is the one place "world".
	TypeWorld Type = "world"
	// TypeRegion is a UN M49 region, subregion or intermediate region.
	TypeRegion Type = "region"
	// TypeCountry is an ISO 3166-1 country or territory.
	TypeCountry Type = "country"
	// TypeSubdivision is an ISO 3166-2 subdivision. A recipe may declare one
	// too, for a municipality ISO 3166-2 does not list.
	TypeSubdivision Type = "subdivision"
	// TypeTerritory is a market or sales territory, usually made of places.
	// It is the type of a declared place that names `of:` and no type.
	TypeTerritory Type = "territory"
	// TypeMetro is a metropolitan area, usually made of places.
	TypeMetro Type = "metro"
	// TypeLocality is a city, town or village.
	TypeLocality Type = "locality"
	// TypeDistrict is a borough or a city district.
	TypeDistrict Type = "district"
	// TypeNeighbourhood is a neighbourhood within a locality or district.
	TypeNeighbourhood Type = "neighbourhood"
	// TypePostalArea is the area of a postal code.
	TypePostalArea Type = "postal-area"
	// TypeArea is any other area. It is the type of a declared place that
	// names `within:` and no type.
	TypeArea Type = "area"
)

// DeclarableTypes are the types a recipe may give a place it declares, in
// the order a reference lists them.
var DeclarableTypes = []Type{
	TypeSubdivision, TypeTerritory, TypeMetro, TypeLocality, TypeDistrict,
	TypeNeighbourhood, TypePostalArea, TypeArea,
}

// Rank orders types from the widest kind to the narrowest: world, region,
// country, subdivision, then the declared types in DeclarableTypes order. A
// type the package does not define ranks last.
func (t Type) Rank() int {
	switch t {
	case TypeWorld:
		return 0
	case TypeRegion:
		return 1
	case TypeCountry:
		return 2
	}
	if i := slices.Index(DeclarableTypes, t); i >= 0 {
		return 3 + i
	}
	return 3 + len(DeclarableTypes)
}

// Place is one place in a gazetteer.
type Place struct {
	// ID is the place's slug, matched byte for byte: "oslo", "no-03", "no",
	// "northern-europe", "world". A declared place's id is its key under
	// `places:`.
	ID string `yaml:"-" json:"id"`
	// Name is the English display name, from CLDR for a built-in place.
	Name string `yaml:"name,omitempty" json:"name"`
	// Names are display names per BCP 47 locale, for a declared place.
	Names map[string]string `yaml:"names,omitempty" json:"names,omitempty"`
	// Type is the kind of place.
	Type Type `yaml:"type,omitempty" json:"type"`
	// Within are the places this one sits inside. A built-in place sits
	// within exactly one place, except world, which sits within none.
	Within []string `yaml:"within,omitempty" json:"within,omitempty"`
	// Of are the places this one is made of. A place names Within or Of,
	// never both.
	Of []string `yaml:"of,omitempty" json:"of,omitempty"`
	// Refs identify the place in other gazetteers, as scheme:value:
	// "wikidata:Q585", "geonames:3143244", "nuts:NO081",
	// "osm:relation/406091", "wof:101752073".
	Refs []string `yaml:"refs,omitempty" json:"refs,omitempty"`
	// Centroid is an approximate centre for drawing the place, nil when the
	// place states none.
	Centroid *LatLon `yaml:"centroid,omitempty" json:"centroid,omitempty"`
	// Shape is the project-relative, slash-separated path of a GeoJSON file
	// holding a declared place's outline. Built-in places have none.
	Shape string `yaml:"shape,omitempty" json:"shape,omitempty"`
	// Declared is true for a place a recipe declares.
	Declared bool `yaml:"-" json:"declared,omitempty"`
}

// MadeOf reports whether the place is made of other places.
func (p Place) MadeOf() bool { return len(p.Of) > 0 }

func (p Place) clone() Place {
	p.Within = slices.Clone(p.Within)
	p.Of = slices.Clone(p.Of)
	p.Refs = slices.Clone(p.Refs)
	p.Names = maps.Clone(p.Names)
	if p.Centroid != nil {
		c := *p.Centroid
		p.Centroid = &c
	}
	return p
}

// LatLon is a point in degrees. YAML and JSON write it as [lat, lon].
type LatLon struct{ Lat, Lon float64 }

// MarshalJSON writes the point as [lat, lon].
func (ll LatLon) MarshalJSON() ([]byte, error) { return json.Marshal([2]float64{ll.Lat, ll.Lon}) }

// UnmarshalJSON reads a point written as [lat, lon].
func (ll *LatLon) UnmarshalJSON(data []byte) error {
	var pair []float64
	if err := json.Unmarshal(data, &pair); err != nil || len(pair) != 2 {
		return errors.New("a centroid is [lat, lon], two numbers")
	}
	ll.Lat, ll.Lon = pair[0], pair[1]
	return nil
}

// MarshalYAML writes the point as a flow sequence [lat, lon].
func (ll LatLon) MarshalYAML() (any, error) {
	n := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
	for _, v := range []float64{ll.Lat, ll.Lon} {
		var item yaml.Node
		if err := item.Encode(v); err != nil {
			return nil, err
		}
		n.Content = append(n.Content, &item)
	}
	return n, nil
}

// UnmarshalYAML reads a point written as [lat, lon].
func (ll *LatLon) UnmarshalYAML(n *yaml.Node) error {
	var pair []float64
	if n.Kind != yaml.SequenceNode || n.Decode(&pair) != nil || len(pair) != 2 {
		return fmt.Errorf("line %d: a centroid is [lat, lon], two numbers", n.Line)
	}
	ll.Lat, ll.Lon = pair[0], pair[1]
	return nil
}

// Valid reports whether the point lies on the globe.
func (ll LatLon) Valid() bool {
	return !math.IsNaN(ll.Lat) && !math.IsNaN(ll.Lon) &&
		ll.Lat >= -90 && ll.Lat <= 90 && ll.Lon >= -180 && ll.Lon <= 180
}

// placeKeys are the keys a declared place takes in a recipe.
var placeKeys = []string{"name", "names", "type", "within", "of", "refs", "centroid", "shape"}

// UnmarshalYAML reads a declared place. `within:` and `of:` take one id or a
// list of ids, and a key the place does not take is an error.
func (p *Place) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: a place is a mapping of %v", n.Line, placeKeys)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if key := n.Content[i].Value; !slices.Contains(placeKeys, key) {
			return fmt.Errorf("line %d: a place takes no key %q (it takes %v)", n.Content[i].Line, key, placeKeys)
		}
	}
	var raw struct {
		Name     string            `yaml:"name"`
		Names    map[string]string `yaml:"names"`
		Type     Type              `yaml:"type"`
		Within   yaml.Node         `yaml:"within"`
		Of       yaml.Node         `yaml:"of"`
		Refs     []string          `yaml:"refs"`
		Centroid *LatLon           `yaml:"centroid"`
		Shape    string            `yaml:"shape"`
	}
	if err := n.Decode(&raw); err != nil {
		return err
	}
	within, err := idList(&raw.Within, "within")
	if err != nil {
		return err
	}
	of, err := idList(&raw.Of, "of")
	if err != nil {
		return err
	}
	*p = Place{
		Name: raw.Name, Names: raw.Names, Type: raw.Type, Within: within, Of: of,
		Refs: raw.Refs, Centroid: raw.Centroid, Shape: raw.Shape,
	}
	return nil
}

// idList reads one id or a list of ids.
func idList(n *yaml.Node, key string) ([]string, error) {
	switch n.Kind {
	case 0:
		return nil, nil
	case yaml.ScalarNode:
		if n.Value == "" {
			return nil, nil
		}
		return []string{n.Value}, nil
	case yaml.SequenceNode:
		var out []string
		if err := n.Decode(&out); err != nil {
			return nil, fmt.Errorf("line %d: %s takes a place id or a list of place ids", n.Line, key)
		}
		return out, nil
	}
	return nil, fmt.Errorf("line %d: %s takes a place id or a list of place ids", n.Line, key)
}

// MarshalYAML writes a declared place the way a recipe holds it: a single
// `within:` parent as a scalar, every other list as a list.
func (p Place) MarshalYAML() (any, error) {
	type out struct {
		Name     string            `yaml:"name,omitempty"`
		Names    map[string]string `yaml:"names,omitempty"`
		Type     Type              `yaml:"type,omitempty"`
		Within   any               `yaml:"within,omitempty"`
		Of       []string          `yaml:"of,omitempty"`
		Refs     []string          `yaml:"refs,omitempty"`
		Centroid *LatLon           `yaml:"centroid,omitempty"`
		Shape    string            `yaml:"shape,omitempty"`
	}
	o := out{Name: p.Name, Names: p.Names, Type: p.Type, Of: p.Of, Refs: p.Refs, Centroid: p.Centroid, Shape: p.Shape}
	switch len(p.Within) {
	case 0:
	case 1:
		o.Within = p.Within[0]
	default:
		o.Within = p.Within
	}
	return o, nil
}
