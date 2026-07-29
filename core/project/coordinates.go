package project

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// The context space.
//
// Content is written for a point in a space the project itself defines: under
// `coordinates:` a recipe declares its own axes — product, channel, market,
// tenant, whatever the taxonomy is — and each named content collection names
// the point its content sits at. `profiles:` binds governance (a voice, a
// vocabulary) to a region of that space, and the most specific match governs.
//
// The framework interprets exactly one axis, ChannelAxis, because a channel
// selects an override inside the voice profile a match resolved to. Every other
// axis is the user's own: matching compares values and reads nothing into them.
//
// Values are slugs, never concept references. A concept is designed to be
// renamed and deprecated as vocabulary is revised, and governance that moved
// when someone edited a term would be governance nobody could rely on. A value
// may *carry* a concept for display (CoordinateValue.Concept); matching never
// looks at it.

// ChannelAxis is the one axis the framework reads for itself: after a profile
// is selected, the collection's value on this axis selects the matching
// override inside that profile (profile.VoiceProfile.Channels). A channel the
// profile declares no override for is not an error — the base voice applies,
// which is the right answer for a voice that reads the same everywhere.
//
// The axis may also appear in a profile's `when:`. The two roles compose:
// matching decides which voice profile governs, the override then refines the
// register within it.
const ChannelAxis = "channel"

// slugRule describes the shape of an axis name and of a coordinate value, for
// the error message that rejects anything else.
const slugRule = "lowercase letters, digits and hyphens, starting with a letter or digit"

// slugPattern is that shape. Coordinates are machine identifiers: stable,
// never translated, and comparable byte for byte.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// conceptPattern is the shape of a concept reference — an identifier carrying
// no whitespace, conventionally `term:<id>`. The reference is not resolved
// here: a recipe has to load without a terms store, so validation only rejects
// what could never be an id.
var conceptPattern = regexp.MustCompile(`^\S+$`)

// defaultVoiceField names the recipe key a voice came from when no profile
// bound one.
const defaultVoiceField = "defaults.brand_voice"

// defaultPointSubject names the point a run sits at when no collection claims
// its content, for error messages.
const defaultPointSubject = "the project default point"

// Coordinates declares the axes of a project's context space: axis name → the
// values it may take.
//
// An axis declared with no values is open — the recipe names the axis but not
// its values, and any well-formed slug is accepted on it. An axis absent from
// this map does not exist: naming it in a `context:` or a `when:` is a load
// error, so a misspelt axis cannot silently open a new dimension that nothing
// governs.
type Coordinates map[string][]CoordinateValue

// CoordinateValue is one value an axis may take. The short form is the value
// itself; the long form adds the concept that names it:
//
//	coordinates:
//	  product:
//	    - id: bowrain
//	      concept: term:9a1c0f42b7
//	    - id: kapi
//
// Concept is display metadata — the terms entry a reader should be shown for
// this value. It takes no part in matching or identity: the slug is the
// identity precisely so that revising the vocabulary cannot move governance.
type CoordinateValue struct {
	// ID is the slug: the value as a collection or a profile writes it.
	ID string `yaml:"id" json:"id"`
	// Concept optionally references the concept that names this value, for
	// display. Carried and shape-checked; never resolved during matching.
	Concept string `yaml:"concept,omitempty" json:"concept,omitempty"`
}

// UnmarshalYAML accepts both forms: a scalar is the id, a mapping is the full
// value.
func (v *CoordinateValue) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		v.ID = node.Value
		return nil
	}
	type coordinateValueAlias CoordinateValue
	var alias coordinateValueAlias
	if err := node.Decode(&alias); err != nil {
		return err
	}
	*v = CoordinateValue(alias)
	return nil
}

// MarshalYAML writes back the form the value was authored in, so saving a
// recipe does not expand a plain list of slugs into a list of mappings.
func (v CoordinateValue) MarshalYAML() (any, error) {
	if v.Concept == "" {
		return v.ID, nil
	}
	type coordinateValueAlias CoordinateValue
	return coordinateValueAlias(v), nil
}

// ProfileBinding binds governance — a voice, a vocabulary, or both — to a
// region of the context space.
//
// A profile matches a point when every coordinate in its `when:` equals the
// point's coordinate of the same name; the empty `when:` matches everything and
// is the project's base. Among the profiles that match, the one matching on the
// most coordinates governs, so a broad profile is refined by a narrow one
// rather than replaced by a second copy of it. A tie is a load error, never a
// coin flip.
type ProfileBinding struct {
	// When is the coordinate match. Empty matches every point.
	When map[string]string `yaml:"when,omitempty" json:"when,omitempty"`
	// Voice is the voice profile governing the matched content, in the same
	// forms as defaults.brand_voice (a bare path, or profile_file / profile /
	// pack). nil keeps defaults.brand_voice.
	Voice *BrandVoiceBinding `yaml:"voice,omitempty" json:"voice,omitempty"`
	// Terms is the approved vocabulary for the matched content, resolved
	// relative to the project root. Empty keeps defaults.terms.
	Terms string `yaml:"terms,omitempty" json:"terms,omitempty"`
}

// ResolvedContext is the governance in force over one collection's content:
// the profile that matched its coordinates, resolved into what a run carries.
//
// Voice and Terms are resolved already, so a caller applies them without
// knowing whether they came from a profile or from the project defaults.
// Channel is passed on to profile resolution, which selects the matching
// override inside the voice profile.
type ResolvedContext struct {
	// Channel is the collection's value on ChannelAxis, empty when it names
	// none.
	Channel string
	// Voice is the matched profile's voice, else defaults.brand_voice. nil
	// when neither binds one.
	Voice *BrandVoiceBinding
	// Terms is the matched profile's terms, else defaults.terms. As written in
	// the recipe — relative to the project root unless absolute.
	Terms string
	// VoiceField names the recipe key Voice came from (`profiles[1].voice` or
	// `defaults.brand_voice`), so a profile that cannot be loaded names the
	// line to fix.
	VoiceField string
}

// ResolveContext returns the governance in force over the named content
// collection: the most specific profile matching its coordinates, resolved over
// defaults.brand_voice / defaults.terms.
//
// An empty or unknown collection name resolves the project's default point, so
// a caller holding a path no collection claims — an ad-hoc file, a pattern that
// matched nothing — keeps the project's voice rather than none. The error is
// the ambiguous recipe: two profiles matching equally well. Validation runs the
// same selection for every collection at load, so a project that loaded cleanly
// has no failure left here.
func (p *KapiProject) ResolveContext(collection string) (*ResolvedContext, error) {
	if collection != "" {
		for i := range p.Content {
			if c := &p.Content[i]; c.Name == collection {
				return p.resolveAt(c.Context, collectionSubject(i, c.Name))
			}
		}
	}
	return p.resolveAt(nil, defaultPointSubject)
}

// resolveAt resolves the governance at one point. subject names that point for
// the ambiguity error.
func (p *KapiProject) resolveAt(coords map[string]string, subject string) (*ResolvedContext, error) {
	rc := &ResolvedContext{
		Channel:    coords[ChannelAxis],
		Voice:      p.Defaults.BrandVoice,
		Terms:      p.Defaults.Terms,
		VoiceField: defaultVoiceField,
	}
	i, prof, err := p.selectProfile(coords, subject)
	if err != nil {
		return nil, err
	}
	if prof == nil {
		return rc, nil
	}
	if prof.Voice != nil {
		rc.Voice, rc.VoiceField = prof.Voice, fmt.Sprintf("profiles[%d].voice", i)
	}
	if prof.Terms != "" {
		rc.Terms = prof.Terms
	}
	return rc, nil
}

// selectProfile returns the index and the profile governing coords: of the
// profiles that match, the one matching on the most coordinates. Returns
// (-1, nil, nil) when none matches — the project defaults then govern.
//
// Two profiles matching on the same number of coordinates is an error rather
// than an arbitrary winner: which voice a piece of content is written in is not
// something to decide by map order.
func (p *KapiProject) selectProfile(coords map[string]string, subject string) (int, *ProfileBinding, error) {
	best, bestScore, tied := -1, -1, -1
	for i := range p.Profiles {
		when := p.Profiles[i].When
		if !matchesPoint(when, coords) {
			continue
		}
		switch score := len(when); {
		case score > bestScore:
			best, bestScore, tied = i, score, -1
		case score == bestScore:
			tied = i
		}
	}
	if tied >= 0 {
		return -1, nil, fmt.Errorf(
			"%s is matched by profiles[%d] (when: %s) and profiles[%d] (when: %s) with equal specificity; narrow one so the choice is not arbitrary",
			subject, best, renderPoint(p.Profiles[best].When), tied, renderPoint(p.Profiles[tied].When))
	}
	if best < 0 {
		return -1, nil, nil
	}
	return best, &p.Profiles[best], nil
}

// matchesPoint reports whether every coordinate in when equals the point's
// coordinate of the same name. An axis the point does not set never matches,
// because a coordinate value is a non-empty slug.
func matchesPoint(when, coords map[string]string) bool {
	for axis, value := range when {
		if coords[axis] != value {
			return false
		}
	}
	return true
}

// CollectionForPath returns the name of the content collection whose item
// pattern matches relPath — a project-relative, slash-separated path — so a run
// can resolve the governance for the file it is about to process. Matching uses
// the same glob semantics and first-match-wins order as target resolution
// (MatchGlob over EffectiveItems, core/project/glob.go). Returns "" when
// nothing matches or the first match came from an unnamed bare entry: both mean
// "the project's default point governs this file".
func (p *KapiProject) CollectionForPath(relPath string) string {
	for i := range p.Content {
		coll := &p.Content[i]
		for _, item := range coll.EffectiveItems() {
			if item.Path == "" {
				continue
			}
			if MatchGlob(item.Path, relPath) {
				return coll.Name
			}
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// validation
// ---------------------------------------------------------------------------

// validateContextSpace checks the context model at load: the declared axes, the
// profiles bound to them, every collection's coordinates, and finally the
// selection each of those points resolves through — so an ambiguous recipe
// fails on load rather than at the moment a run has to pick a voice.
func (p *KapiProject) validateContextSpace() error {
	if err := p.validateCoordinates(); err != nil {
		return err
	}
	if err := p.validateProfiles(); err != nil {
		return err
	}
	for i := range p.Content {
		if err := p.validateCollectionCoordinates(i, &p.Content[i]); err != nil {
			return err
		}
	}
	return p.validateSelection()
}

// validateCoordinates checks the declared axes: slug-shaped names, slug-shaped
// values, no value declared twice, and a well-formed concept reference where
// one is carried.
func (p *KapiProject) validateCoordinates() error {
	for _, axis := range sortedKeys(p.Coordinates) {
		if !slugPattern.MatchString(axis) {
			return fmt.Errorf("coordinates: axis name %q is not a slug (%s)", axis, slugRule)
		}
		seen := make(map[string]bool, len(p.Coordinates[axis]))
		for i, v := range p.Coordinates[axis] {
			switch {
			case v.ID == "":
				return fmt.Errorf("coordinates.%s[%d]: a value needs an id", axis, i)
			case !slugPattern.MatchString(v.ID):
				return fmt.Errorf("coordinates.%s: value %q is not a slug (%s)", axis, v.ID, slugRule)
			case seen[v.ID]:
				return fmt.Errorf("coordinates.%s: value %q is declared twice", axis, v.ID)
			case v.Concept != "" && !conceptPattern.MatchString(v.Concept):
				return fmt.Errorf("coordinates.%s: value %q has an invalid concept reference %q (want a concept id, e.g. term:9a1c0f42b7)",
					axis, v.ID, v.Concept)
			}
			seen[v.ID] = true
		}
	}
	return nil
}

// validateProfiles checks every profile: its match names declared coordinates,
// it binds something, its voice selects exactly one source, and no two profiles
// claim the same point.
func (p *KapiProject) validateProfiles() error {
	bound := make(map[string]int, len(p.Profiles))
	for i := range p.Profiles {
		prof := &p.Profiles[i]
		if err := p.validatePoint(fmt.Sprintf("profiles[%d]: when", i), prof.When); err != nil {
			return err
		}
		if prof.Voice == nil && prof.Terms == "" {
			return fmt.Errorf("profiles[%d]: a profile must bind a voice, terms, or both", i)
		}
		if err := prof.Voice.validate(fmt.Sprintf("profiles[%d].voice", i)); err != nil {
			return err
		}
		point := renderPoint(prof.When)
		if first, dup := bound[point]; dup {
			return fmt.Errorf("profiles[%d]: when %s is already bound by profiles[%d]; one point cannot have two profiles",
				i, point, first)
		}
		bound[point] = i
	}
	return nil
}

// validateCollectionCoordinates checks one collection's declared point.
// Coordinates are resolved by collection name, so a bare entry has nothing to
// resolve — refuse it rather than accept a binding that is never read.
func (p *KapiProject) validateCollectionCoordinates(i int, c *ContentCollection) error {
	if c.Context == nil {
		return nil
	}
	if c.Name == "" {
		return fmt.Errorf("content[%d]: context requires a named collection (add `name:`)", i)
	}
	return p.validatePoint(collectionSubject(i, c.Name), c.Context)
}

// validatePoint checks one set of coordinates — a collection's `context:` or a
// profile's `when:` — against the declared axes. where prefixes the message
// with the recipe location that carries them.
//
// An undeclared axis and an undeclared value are both load errors, and both
// name what the recipe does declare: the failure is nearly always a typo, and a
// silent fall-back would govern that content in a plausible-looking wrong
// voice. An open axis (declared with no values) accepts any well-formed slug.
func (p *KapiProject) validatePoint(where string, coords map[string]string) error {
	for _, axis := range sortedKeys(coords) {
		value := coords[axis]
		declared, ok := p.Coordinates[axis]
		if !ok {
			return fmt.Errorf("%s sets coordinate %q, which coordinates: does not declare (declared: %s)",
				where, axis, p.declaredAxes())
		}
		if !slugPattern.MatchString(value) {
			return fmt.Errorf("%s sets %s to %q, which is not a slug (%s)", where, axis, value, slugRule)
		}
		if len(declared) == 0 {
			continue
		}
		if !declaresValue(declared, value) {
			return fmt.Errorf("%s sets %s to %q, which coordinates.%s does not declare (declared: %s)",
				where, axis, value, axis, declaredValues(declared))
		}
	}
	return nil
}

// validateSelection runs profile selection for every point the recipe can
// resolve — each collection's, plus the project's own default point — so an
// ambiguity that a run would hit is reported at load.
func (p *KapiProject) validateSelection() error {
	if _, _, err := p.selectProfile(nil, defaultPointSubject); err != nil {
		return err
	}
	for i := range p.Content {
		c := &p.Content[i]
		if len(c.Context) == 0 {
			continue
		}
		if _, _, err := p.selectProfile(c.Context, collectionSubject(i, c.Name)); err != nil {
			return err
		}
	}
	return nil
}

// declaresValue reports whether an axis declares the given value.
func declaresValue(declared []CoordinateValue, value string) bool {
	for _, v := range declared {
		if v.ID == value {
			return true
		}
	}
	return false
}

// declaredAxes renders the declared axis names for an error message, sorted so
// the message is the same on every load.
func (p *KapiProject) declaredAxes() string {
	if len(p.Coordinates) == 0 {
		return "none"
	}
	return strings.Join(sortedKeys(p.Coordinates), ", ")
}

// declaredValues renders an axis's values for an error message, in the order
// the recipe declares them.
func declaredValues(declared []CoordinateValue) string {
	ids := make([]string, 0, len(declared))
	for _, v := range declared {
		ids = append(ids, v.ID)
	}
	return strings.Join(ids, ", ")
}

// renderPoint renders a set of coordinates in a stable order, for error
// messages and for the duplicate-profile check.
func renderPoint(coords map[string]string) string {
	if len(coords) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(coords))
	for _, axis := range sortedKeys(coords) {
		parts = append(parts, axis+": "+coords[axis])
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// collectionSubject names a collection by recipe location and name, so an error
// about its coordinates points at the entry to edit.
func collectionSubject(i int, name string) string {
	return fmt.Sprintf("content[%d]: collection %q", i, name)
}
