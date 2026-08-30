package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// Setting one allowlisted recipe field from a JSON value.
//
// This lives here rather than beside `kapi apply` because two callers now write
// the same fields: the local fix loop applying a change-set, and a pull taking a
// recipe change an approval made on the venue. One allowlist and one writer
// means the two cannot drift into disagreeing about what a recipe may say.

// setRecipeField sets one allowlisted dotted recipe field from a JSON value,
// reporting whether the value actually changed (for idempotency). The allowlist
// is deliberate: it bounds apply to the safe, structured recipe surface a fix
// loop legitimately edits and keeps every set type-checked.
func SetField(proj *KapiProject, path string, raw json.RawMessage) (bool, error) {
	// defaults.coordinates.<axis> is the one settable path with a variable
	// tail: the axis set is open by design, so it cannot be enumerated here
	// the way the fixed fields below are.
	if axis, ok := strings.CutPrefix(path, "defaults.coordinates."); ok {
		return setDefaultCoordinate(proj, path, axis, raw)
	}
	if path == "defaults.coordinates" {
		return false, errors.New(`recipe: set one axis at a time, as "defaults.coordinates.<axis>"`)
	}

	// collections.<name>.channel is the structural half. product and channel
	// are derived from a collection's `channel:`, so this is where a decision
	// about them is written — a collection is addressed by NAME rather than by
	// position, because a name is what the decision was about and an index
	// moves when an unrelated entry is added above it.
	if rest, ok := strings.CutPrefix(path, "collections."); ok {
		return setCollectionField(proj, path, rest, raw)
	}

	switch path {
	case "name":
		var v string
		if err := decodeRecipeValue(path, raw, &v); err != nil {
			return false, err
		}
		if proj.Name == v {
			return false, nil
		}
		proj.Name = v
		return true, nil

	case "defaults.source_language":
		var v string
		if err := decodeRecipeValue(path, raw, &v); err != nil {
			return false, err
		}
		if proj.Defaults.SourceLanguage == model.LocaleID(v) {
			return false, nil
		}
		proj.Defaults.SourceLanguage = model.LocaleID(v)
		return true, nil

	case "defaults.target_languages":
		var v []string
		if err := decodeRecipeValue(path, raw, &v); err != nil {
			return false, err
		}
		next := make([]model.LocaleID, len(v))
		for i, s := range v {
			next[i] = model.LocaleID(s)
		}
		if localesEqual(proj.Defaults.TargetLanguages, next) {
			return false, nil
		}
		proj.Defaults.TargetLanguages = next
		return true, nil

	case "defaults.encoding":
		var v string
		if err := decodeRecipeValue(path, raw, &v); err != nil {
			return false, err
		}
		if proj.Defaults.Encoding == v {
			return false, nil
		}
		proj.Defaults.Encoding = v
		return true, nil

	case "defaults.terms_source":
		var v string
		if err := decodeRecipeValue(path, raw, &v); err != nil {
			return false, err
		}
		if proj.Defaults.TermsSource == v {
			return false, nil
		}
		proj.Defaults.TermsSource = v
		return true, nil

	case "defaults.memory_source":
		var v string
		if err := decodeRecipeValue(path, raw, &v); err != nil {
			return false, err
		}
		if proj.Defaults.MemorySource == v {
			return false, nil
		}
		proj.Defaults.MemorySource = v
		return true, nil

	default:
		return false, fmt.Errorf("recipe: unknown or unsettable path %q", path)
	}
}

// DeclarableAxis reports whether an axis may be declared under
// defaults.coordinates, returning the refusal when it may not.
//
// product and channel are DERIVED from a collection's `channel:`, which is why
// Defaults.Coordinates documents itself as declared axes only: writing one here
// would state a point the recipe also computes, and the two would be free to
// disagree. Every surface that offers a coordinate editor asks here, so a
// refusal reads the same wherever it is met.
func DeclarableAxis(axis string) error {
	if axis == "" {
		return errors.New(`recipe: empty axis in "defaults.coordinates."`)
	}
	if axis == ProductAxis || axis == ChannelAxis {
		return fmt.Errorf("recipe: %q is derived from a collection's channel, not declared: remove it from the point or set the collection's channel instead", axis)
	}
	return nil
}

// setDefaultCoordinate sets one axis of the project's default point. An empty
// value removes the axis, so a change-set can withdraw a coordinate as well as
// declare one and the operation stays total.
func setDefaultCoordinate(proj *KapiProject, path, axis string, raw json.RawMessage) (bool, error) {
	if err := DeclarableAxis(axis); err != nil {
		return false, err
	}

	var v string
	if err := decodeRecipeValue(path, raw, &v); err != nil {
		return false, err
	}

	if v == "" {
		if _, present := proj.Defaults.Coordinates[axis]; !present {
			return false, nil
		}
		delete(proj.Defaults.Coordinates, axis)
		if len(proj.Defaults.Coordinates) == 0 {
			proj.Defaults.Coordinates = nil
		}
		return true, nil
	}

	if proj.Defaults.Coordinates[axis] == v {
		return false, nil
	}
	if proj.Defaults.Coordinates == nil {
		proj.Defaults.Coordinates = map[string]string{}
	}
	proj.Defaults.Coordinates[axis] = v
	return true, nil
}

// setCollectionField sets one field of one named collection. Only `channel` is
// settable: it is the recipe key the structural axes are derived from, and the
// rest of a collection — its content globs, its target template — describes
// what files it covers rather than where its content sits.
func setCollectionField(proj *KapiProject, path, rest string, raw json.RawMessage) (bool, error) {
	// Split at the LAST dot: the field name has none, and a collection name
	// might.
	dot := strings.LastIndex(rest, ".")
	if dot <= 0 || dot == len(rest)-1 {
		return false, fmt.Errorf(`recipe: %q is not a settable collection field (want "collections.<name>.channel")`, path)
	}
	name, field := rest[:dot], rest[dot+1:]
	if field != "channel" {
		return false, fmt.Errorf("recipe: %q is not settable — a collection's %s is not a coordinate", path, field)
	}

	col := proj.collectionNamed(name)
	if col == nil {
		return false, fmt.Errorf("recipe: no collection named %q (declared: %s)", name, proj.declaredCollections())
	}

	var v string
	if err := decodeRecipeValue(path, raw, &v); err != nil {
		return false, err
	}
	v = strings.TrimSpace(v)

	// An empty value withdraws the binding, so the collection falls back to the
	// project's default point. The operation stays total, as it is for a
	// declared coordinate.
	if v == "" {
		if col.Channel == "" {
			return false, nil
		}
		col.Channel = ""
		return true, nil
	}

	profile, channel, qualified := strings.Cut(v, "/")
	if !qualified || profile == "" || channel == "" {
		return false, fmt.Errorf("recipe: %s must be `product/channel`, got %q — a channel is a surface OF a product, and the binding reads as one", path, v)
	}
	if strings.Contains(channel, "/") {
		return false, fmt.Errorf("recipe: %s has too many parts (want `product/channel`), got %q", path, v)
	}
	if !slugPattern.MatchString(profile) || !slugPattern.MatchString(channel) {
		return false, fmt.Errorf("recipe: %s must be %s on both sides of the slash, got %q", path, slugRule, v)
	}

	if col.Channel == v && proj.declaresChannelRef(profile, channel) {
		return false, nil
	}

	// The point the collection is moving to has to EXIST, and saying it does is
	// the same decision. A `channel:` naming a profile that `profiles:` does not
	// declare will not load, so writing one and leaving the declaration to a
	// second change means an ordering contract between two rows — and a working
	// tree that does not load if they arrive out of order.
	proj.declareChannel(profile, channel)
	col.Channel = v
	return true, nil
}

// collectionNamed finds a collection by name, or nil. Unnamed entries have no
// point to resolve, so they are never addressable here.
func (p *KapiProject) collectionNamed(name string) *Collection {
	if name == "" {
		return nil
	}
	for i := range p.Collections {
		if p.Collections[i].Name == name {
			return &p.Collections[i]
		}
	}
	return nil
}

// declaredCollections renders the named collections, for an error a reader can
// act on.
func (p *KapiProject) declaredCollections() string {
	var names []string
	for _, c := range p.Collections {
		if c.Name != "" {
			names = append(names, c.Name)
		}
	}
	if len(names) == 0 {
		return "none"
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// declaresChannelRef reports whether profiles: already declares this point.
func (p *KapiProject) declaresChannelRef(profile, channel string) bool {
	prof, ok := p.Profiles[profile]
	return ok && declaresChannel(prof.Channels, channel)
}

// declareChannel adds the profile and channel if the recipe does not already
// carry them, leaving everything else about an existing profile alone.
func (p *KapiProject) declareChannel(profile, channel string) {
	if p.Profiles == nil {
		p.Profiles = map[string]Profile{}
	}
	prof := p.Profiles[profile]
	if !declaresChannel(prof.Channels, channel) {
		prof.Channels = append(prof.Channels, Channel{ID: channel})
	}
	p.Profiles[profile] = prof
}

// decodeRecipeValue JSON-decodes a recipe value into dst, wrapping the error
// with the path so a type mismatch is actionable.
func decodeRecipeValue(path string, raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		return fmt.Errorf("recipe: %s has no value", path)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("recipe: %s: %w", path, err)
	}
	return nil
}

func localesEqual(a, b []model.LocaleID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
