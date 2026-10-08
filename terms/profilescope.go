package terms

import (
	"sort"
	"strings"
)

// PropProfile is the concept property naming the profile a concept is scoped
// to: the point a rule's evidence was seen at, or the profile whose voice or
// terms file it was imported from. A concept without it holds across the whole
// project.
const PropProfile = "profile"

// PropCoordinates is the concept property naming the coordinates a concept is
// scoped to, written `axis=value` and joined with commas in axis order, such
// as `channel=docs,product=quickcast`. A rule kept from evidence seen at a
// point holds where those coordinates match; a concept without the property
// answers whatever the coordinates of the point asked about.
const PropCoordinates = "coordinates"

// Profile is the profile a concept is scoped to, empty for one that holds
// across the whole project.
func (c Concept) Profile() string { return c.Properties[PropProfile] }

// Coordinates are the coordinates a concept is scoped to, nil for one that
// holds at every point of the profile it is scoped to.
func (c Concept) Coordinates() map[string]string {
	return parseCoordinates(c.Properties[PropCoordinates])
}

// ScopeToProfile scopes a concept to a profile. An empty profile leaves it
// holding across the whole project.
func (c *Concept) ScopeToProfile(profile string) {
	if profile == "" {
		return
	}
	if c.Properties == nil {
		c.Properties = map[string]string{}
	}
	c.Properties[PropProfile] = profile
}

// ScopeToPoint scopes a concept to a profile and to the coordinates of a
// point, replacing whatever scope it carried. An empty profile and no
// coordinates leave it holding across the whole project.
func (c *Concept) ScopeToPoint(profile string, coordinates map[string]string) {
	c.Unscope()
	c.ScopeToProfile(profile)
	if text := formatCoordinates(coordinates); text != "" {
		if c.Properties == nil {
			c.Properties = map[string]string{}
		}
		c.Properties[PropCoordinates] = text
	}
}

// Unscope removes the concept's profile and coordinates, so it holds across
// the project.
func (c *Concept) Unscope() {
	delete(c.Properties, PropProfile)
	delete(c.Properties, PropCoordinates)
}

// AtProfile keeps the concepts that hold where a profile governs: the ones
// scoped to no profile, and the ones scoped to that profile. An empty profile
// is the project's default point, where only unscoped concepts hold.
func AtProfile(concepts []Concept, profile string) []Concept {
	out := make([]Concept, 0, len(concepts))
	for _, c := range concepts {
		if p := c.Profile(); p == "" || p == profile {
			out = append(out, c)
		}
	}
	return out
}

// AtPoint keeps the concepts that hold at a point: the ones AtProfile keeps
// for the profile governing there, less the ones scoped to coordinates the
// point does not sit at. Every axis a concept names must have the same value
// at the point; an axis it leaves out is one it holds across.
func AtPoint(concepts []Concept, profile string, coordinates map[string]string) []Concept {
	out := make([]Concept, 0, len(concepts))
	for _, c := range AtProfile(concepts, profile) {
		if coversPoint(c.Coordinates(), coordinates) {
			out = append(out, c)
		}
	}
	return out
}

// coversPoint reports whether a scope's coordinates all hold at a point.
func coversPoint(scope, point map[string]string) bool {
	for axis, want := range scope {
		if point[axis] != want {
			return false
		}
	}
	return true
}

// formatCoordinates writes coordinates as the property holds them.
func formatCoordinates(coordinates map[string]string) string {
	parts := make([]string, 0, len(coordinates))
	for axis, value := range coordinates {
		if axis == "" || value == "" {
			continue
		}
		parts = append(parts, axis+"="+value)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// parseCoordinates reads coordinates as the property holds them.
func parseCoordinates(text string) map[string]string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	out := map[string]string{}
	for part := range strings.SplitSeq(text, ",") {
		axis, value, ok := strings.Cut(part, "=")
		if ok && axis != "" && value != "" {
			out[strings.TrimSpace(axis)] = strings.TrimSpace(value)
		}
	}
	return out
}
