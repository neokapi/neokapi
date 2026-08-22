package project

import (
	"encoding/json"
	"errors"
	"fmt"
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

// setDefaultCoordinate sets one axis of the project's default point. An empty
// value removes the axis, so a change-set can withdraw a coordinate as well as
// declare one and the operation stays total.
//
// The structural axes are refused. product and channel are DERIVED from a
// collection's `channel:`, which is why Defaults.Coordinates documents itself
// as declared axes only: writing one here would state a point the recipe also
// computes, and the two would be free to disagree.
func setDefaultCoordinate(proj *KapiProject, path, axis string, raw json.RawMessage) (bool, error) {
	if axis == "" {
		return false, errors.New(`recipe: empty axis in "defaults.coordinates."`)
	}
	if axis == ProductAxis || axis == ChannelAxis {
		return false, fmt.Errorf("recipe: %q is derived from a collection's channel, not declared: remove it from the point or set the collection's channel instead", axis)
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
