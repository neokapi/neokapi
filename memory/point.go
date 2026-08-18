package memory

import "strings"

// Where an answer was approved.
//
// A translation is approved somewhere — in a collection, on a channel, for a
// product — and that place is what qualifies it. The corpus therefore stores a
// point beside each answer it learned from a committed translation, and a
// caller that knows where it is asking from gets the answer approved nearest to
// it rather than the one the corpus happens to repeat most.
//
// The point is opaque here on purpose. Rendering one is a question about the
// recipe — which profile, which channel, which collection claims this file —
// and the corpus has no recipe; comparing two is arithmetic over the
// containment ladder, and that is all the matcher needs.

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

// NearerAnswer reports whether the answer approved at pointA beats the one
// approved at pointB, for a caller asking from the point `at`.
//
// Nearest wins. A genuine tie — two approvals at one point, or two points
// equally far from the asker — is broken by the answer's own text, smallest
// byte sequence first. That rule is arbitrary in meaning and deliberate in
// shape: it is a function of the two answers alone, so unlike a count of how
// often the corpus repeats each one it cannot move when unrelated content is
// added or removed, and a rebuild reproduces the wording it started from. A tie
// is reported as well as broken, because two approvals at one point is a
// question about the project's governance that the corpus cannot answer.
func NearerAnswer(pointA, textA, pointB, textB, at string) bool {
	da, db := PointDistance(pointA, at), PointDistance(pointB, at)
	if da != db {
		return da < db
	}
	return textA < textB
}
