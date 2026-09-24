package memory

import "strings"

// Content memory records the context point where a translation was approved.
// When equivalent matches have different approved wording, proximity to the
// caller's point helps select the applicable approval.
//
// The host resolves recipe bindings into a point. This package compares the
// point's containment components without depending on the recipe.

const (
	// PointSeparator separates one rung of a context point from the next. A
	// unit separator rather than a slash, so a rung may hold any name a recipe
	// can write without the rendering becoming ambiguous.
	PointSeparator = "\x1f"

	// PointRungs is the depth of the containment ladder a point names: the
	// product profile, the channel it ships on, and the collection it belongs
	// to — coarsest first, so a shared prefix is a shared containment.
	PointRungs = 3
)

// NewPoint renders a point from the ladder's rungs. A rung nothing declares is
// empty, which is a real position — the project's default point — rather than a
// missing one, and a point that declares nothing at all renders as the empty
// string.
func NewPoint(profile, channel, collection string) string {
	p := strings.Join([]string{profile, channel, collection}, PointSeparator)
	if strings.Trim(p, PointSeparator) == "" {
		return ""
	}
	return p
}

// PointRung returns one rung of a point, counting from the coarsest (0 is the
// profile, PointRungs-1 the collection). Out of range, or a point that names
// nothing, is the empty rung.
func PointRung(point string, i int) string {
	if i < 0 || i >= PointRungs {
		return ""
	}
	rungs := strings.Split(point, PointSeparator)
	if i >= len(rungs) {
		return ""
	}
	return rungs[i]
}

// PointDistance measures how far apart two points are on the containment
// ladder: 0 for the same collection, 1 for the same channel, 2 for the same
// product, and PointRungs for two points that share no containment at all.
//
// It is a prefix comparison from the coarsest rung down, because containment is
// what the ladder means: two files in one collection ship on one channel by
// construction, so a match at a fine rung that disagreed at a coarse one would
// not describe anything. An answer carrying no point sits at the project's
// default point and is therefore PointRungs away from every answer that names a
// product — the right reading for an entry a seed or an import taught the
// corpus, bound to no location at all.
func PointDistance(a, b string) int {
	if a == b {
		return 0
	}
	matched := 0
	for i := range PointRungs {
		if PointRung(a, i) != PointRung(b, i) {
			break
		}
		matched++
	}
	return PointRungs - matched
}

// NearerAnswer compares approvals by their distance from at. Equal distances
// are ordered lexicographically by target text for deterministic selection.
// That tie-break depends only on the candidates, so unrelated corpus changes
// cannot affect it. Callers must also report unresolved approval ties.
func NearerAnswer(pointA, textA, pointB, textB, at string) bool {
	da, db := PointDistance(pointA, at), PointDistance(pointB, at)
	if da != db {
		return da < db
	}
	return textA < textB
}
