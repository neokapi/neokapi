package project

import (
	"fmt"
	"sort"

	"github.com/neokapi/neokapi/core/axis"
)

// AxisSpace is the project's typed axes (core/axis), nil for a nil project. An
// axis it does not declare is an open category, so a nil space compares every
// coordinate by equality.
func (p *KapiProject) AxisSpace() axis.Space {
	if p == nil {
		return nil
	}
	return p.Axes
}

// DeclaredAxisNames lists every declared axis the recipe names: the axes typed
// under `axes:`, and every axis written under `defaults.coordinates` or a
// collection's `coordinates:`, sorted. The structural axes (product, channel)
// are derived rather than declared and are not listed.
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

// validateAxes checks the `axes:` block and every coordinate the recipe writes
// against it: an axis name is a slug and never one of the structural axes, its
// spec is well formed (axis.Space.Check), and each value under
// `defaults.coordinates` and a collection's `coordinates:` is legal for its
// axis. An axis the block leaves out stays an open category, so a recipe with
// no `axes:` loads exactly as it did.
func (p *KapiProject) validateAxes() error {
	for _, name := range sortedKeys(p.Axes) {
		if name == ProductAxis || name == ChannelAxis {
			return fmt.Errorf("axes.%s: %q is derived from a collection's channel, not declared: remove it from axes and declare the product as a profile", name, name)
		}
	}
	if err := p.Axes.Check("axes"); err != nil {
		return err
	}
	for _, name := range sortedKeys(p.Defaults.Coordinates) {
		if err := p.validateCoordinate(name, p.Defaults.Coordinates[name]); err != nil {
			return fmt.Errorf("defaults.coordinates.%s: %w", name, err)
		}
	}
	for i, c := range p.Collections {
		for _, name := range sortedKeys(c.Coordinates) {
			if err := p.validateCoordinate(name, c.Coordinates[name]); err != nil {
				where := fmt.Sprintf("collections[%d]", i)
				if c.Name != "" {
					where = collectionSubject(i, c.Name)
				}
				return fmt.Errorf("%s: coordinates.%s: %w", where, name, err)
			}
		}
	}
	return nil
}

// validateCoordinate checks one written coordinate against the axis it names.
// An empty value is an incomplete edit that MergeCoordinates never applies, so
// it is left alone here too.
func (p *KapiProject) validateCoordinate(name, value string) error {
	if value == "" {
		return nil
	}
	return p.Axes.Validate(name, value)
}
