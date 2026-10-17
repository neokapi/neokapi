package project

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/neokapi/neokapi/core/axis"
	"github.com/neokapi/neokapi/core/place"
)

// Gazetteer is the project's places: the built-in gazetteer plus the places
// the recipe declares under `places:`. A recipe that loaded holds the one
// built at validation; a project assembled in code builds it on each call, and
// one whose places do not validate answers with the built-in places alone. A
// nil project answers with the built-in places.
func (p *KapiProject) Gazetteer() *place.Gazetteer {
	if p == nil {
		return place.Builtin()
	}
	if p.gazetteer != nil {
		return p.gazetteer
	}
	g, err := place.New(p.Places)
	if err != nil {
		return place.Builtin()
	}
	return g
}

// AxisSpace is the project's typed axes (core/axis) over its gazetteer. An
// axis it does not declare is an open category, so a nil project's space
// compares every coordinate by equality.
func (p *KapiProject) AxisSpace() axis.Space {
	if p == nil {
		return axis.Space{}
	}
	return axis.NewSpace(p.Axes, p.Gazetteer())
}

// DeclaredAxisNames lists every declared axis the recipe names: the axes typed
// under `axes:`, and every axis written under `defaults.coordinates`, a
// collection's `coordinates:` or a content item's, sorted. The structural
// axes (product, channel) are derived rather than declared and are not
// listed.
func (p *KapiProject) DeclaredAxisNames() []string {
	if p == nil {
		return nil
	}
	seen := map[string]bool{}
	for name := range p.Axes {
		seen[name] = true
	}
	for name := range p.Defaults.Coordinates {
		seen[name] = true
	}
	for _, c := range p.Collections {
		for name := range c.Coordinates {
			seen[name] = true
		}
		for _, item := range c.Content {
			for name := range item.Coordinates {
				seen[name] = true
			}
		}
	}
	delete(seen, ProductAxis)
	delete(seen, ChannelAxis)
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// CoordinatesAt is the point's coordinates: the project defaults, overlaid
// with what the governance resolved there derives (rc, from
// ResolveGovernanceFor at the same point), overlaid with the coordinates of
// the collection the point sits in, overlaid with those of the content item
// that claims its path. Most specific wins, per axis (MergeCoordinates), so an
// item refines its collection's point as its `channel:` refines the
// collection's. The item and collection are the ones governance resolves
// through (pointAt). A nil result is the default point.
func (p *KapiProject) CoordinatesAt(pt GovernancePoint, rc *ResolvedGovernance) map[string]string {
	if p == nil {
		return MergeCoordinates(nil, rc.Ref().Coordinates(), nil)
	}
	item, i := p.pointAt(pt)
	var collection, own map[string]string
	if i >= 0 {
		collection = p.Collections[i].Coordinates
	}
	if item != nil {
		own = item.Coordinates
	}
	return MergeCoordinates(MergeCoordinates(p.Defaults.Coordinates, rc.Ref().Coordinates(), collection), own, nil)
}

// validateAxes checks the `places:` and `axes:` blocks and every coordinate
// the recipe writes against them. The declared places validate as a gazetteer
// (place.New). An axis name is a slug and never one of the structural axes,
// and its spec is well formed (axis.Space.Check). Each value under
// `defaults.coordinates`, a collection's `coordinates:` and a content item's
// is legal for its axis. An axis the block leaves out stays an open category.
func (p *KapiProject) validateAxes() error {
	g, err := place.New(p.Places)
	if err != nil {
		return err
	}
	p.gazetteer = g
	for _, name := range sortedKeys(p.Axes) {
		if name == ProductAxis || name == ChannelAxis {
			return fmt.Errorf("axes.%s: %q is derived from a collection's channel, not declared: remove it from axes and declare the product as a profile", name, name)
		}
	}
	space := p.AxisSpace()
	if err := space.Check("axes"); err != nil {
		return err
	}
	for _, name := range sortedKeys(p.Defaults.Coordinates) {
		if err := validateCoordinate(space, name, p.Defaults.Coordinates[name]); err != nil {
			return fmt.Errorf("defaults.coordinates.%s: %w", name, err)
		}
	}
	for i, c := range p.Collections {
		where := fmt.Sprintf("collections[%d]", i)
		if c.Name != "" {
			where = collectionSubject(i, c.Name)
		}
		for _, name := range sortedKeys(c.Coordinates) {
			if err := validateCoordinate(space, name, c.Coordinates[name]); err != nil {
				return fmt.Errorf("%s: coordinates.%s: %w", where, name, err)
			}
		}
		for j, item := range c.Content {
			for _, name := range sortedKeys(item.Coordinates) {
				if name == ProductAxis || name == ChannelAxis {
					return fmt.Errorf("%s: content[%d]: coordinates.%s: %q is derived from the item's channel, not declared", where, j, name, name)
				}
				if err := validateCoordinate(space, name, item.Coordinates[name]); err != nil {
					return fmt.Errorf("%s: content[%d]: coordinates.%s: %w", where, j, name, err)
				}
			}
		}
	}
	return nil
}

// validateCoordinate checks one written coordinate against the axis it names.
// An empty value is an incomplete edit that CoordinatesAt never applies, so it
// is left alone here too.
func validateCoordinate(space axis.Space, name, value string) error {
	if value == "" {
		return nil
	}
	return space.Validate(name, value)
}

// checkPlaceShapes reports a declared place whose `shape:` names a file that
// is missing under dir, the directory holding the recipe.
func (p *KapiProject) checkPlaceShapes(dir string) error {
	for _, id := range sortedKeys(p.Places) {
		shape := p.Places[id].Shape
		if shape == "" {
			continue
		}
		info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(shape)))
		switch {
		case err != nil:
			return fmt.Errorf("places.%s.shape: %s: no such file beside the recipe", id, shape)
		case info.IsDir():
			return fmt.Errorf("places.%s.shape: %s is a directory, not a GeoJSON file", id, shape)
		}
	}
	return nil
}
