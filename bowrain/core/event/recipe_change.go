package event

import (
	"encoding/json"
	"maps"
	"slices"
)

// The ways a recipe change can touch one collection.
const (
	CollectionAdded       = "added"
	CollectionRemoved     = "removed"
	CollectionCoordinates = "coordinates"
)

// CollectionChange is one collection a recipe change touched. Before and
// After hold its coordinates (brand, product, channel, market) on either
// side of the change; an added collection has no Before.
type CollectionChange struct {
	Name   string            `json:"name"`
	Change string            `json:"change"`
	Before map[string]string `json:"before,omitempty"`
	After  map[string]string `json:"after,omitempty"`
}

// SettingChange is one recipe-owned setting a push changed.
type SettingChange struct {
	Name   string `json:"name"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// TargetsChange is a project's target languages before and after a change.
type TargetsChange struct {
	Before []string `json:"before"`
	After  []string `json:"after"`
}

// Added lists the languages After holds and Before does not.
func (t TargetsChange) Added() []string { return missingFrom(t.After, t.Before) }

// Removed lists the languages Before holds and After does not.
func (t TargetsChange) Removed() []string { return missingFrom(t.Before, t.After) }

func missingFrom(from, in []string) []string {
	var out []string
	for _, l := range from {
		if !slices.Contains(in, l) && !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out
}

// RecipeChange is the typed payload of a recipe.changed event: what one
// write changed in what a project declares, with the values on both sides.
// Data and ParseRecipeChange convert it to and from an event's Data map.
type RecipeChange struct {
	// WorkspaceSlug files the activity row under the workspace the feed
	// lists by.
	WorkspaceSlug string `json:"-"`
	// ActorName is the name the actor is shown by, when the emitter knows
	// it.
	ActorName string `json:"-"`
	// Source names the write path: "push" or "project".
	Source string `json:"-"`
	// Stream is the stream a push declared the collections for.
	Stream string `json:"-"`

	Collections []CollectionChange `json:"collections,omitempty"`
	Settings    []SettingChange    `json:"settings,omitempty"`
	Targets     *TargetsChange     `json:"targets,omitempty"`
}

// Empty reports whether the change changes nothing.
func (r RecipeChange) Empty() bool {
	targets := r.Targets != nil && (len(r.Targets.Added()) > 0 || len(r.Targets.Removed()) > 0)
	return len(r.Collections) == 0 && len(r.Settings) == 0 && !targets
}

// Counts returns the number of changes of each kind, keyed as the project
// feed counts them. A kind with no change is left out.
func (r RecipeChange) Counts() map[string]int {
	counts := map[string]int{}
	add := func(k string, n int) {
		if n > 0 {
			counts[k] += n
		}
	}
	for _, c := range r.Collections {
		switch c.Change {
		case CollectionAdded:
			add("collections_added", 1)
		case CollectionRemoved:
			add("collections_removed", 1)
		case CollectionCoordinates:
			add("coordinates_changed", 1)
		}
	}
	if r.Targets != nil {
		add("targets_added", len(r.Targets.Added()))
		add("targets_removed", len(r.Targets.Removed()))
	}
	add("settings_changed", len(r.Settings))
	return counts
}

// Locales lists the languages the change added or removed.
func (r RecipeChange) Locales() []string {
	if r.Targets == nil {
		return nil
	}
	return append(r.Targets.Added(), r.Targets.Removed()...)
}

// Coordinates returns the coordinates of the collection the change touched,
// when it touched exactly one; the feed files the entry at that point.
func (r RecipeChange) Coordinates() map[string]string {
	if len(r.Collections) != 1 {
		return nil
	}
	c := r.Collections[0]
	if len(c.After) > 0 {
		return maps.Clone(c.After)
	}
	return maps.Clone(c.Before)
}

// Data renders the change as an event's Data map. The collections, settings
// and target languages ride as one JSON value under "recipe_diff".
func (r RecipeChange) Data() map[string]string {
	data := map[string]string{}
	put := func(k, v string) {
		if v != "" {
			data[k] = v
		}
	}
	put("workspace_slug", r.WorkspaceSlug)
	put("actor_name", r.ActorName)
	put("source", r.Source)
	put("stream", r.Stream)
	if raw, err := json.Marshal(r); err == nil {
		data["recipe_diff"] = string(raw)
	}
	return data
}

// ParseRecipeChange reads a recipe change back from an event's Data map. A
// diff that does not parse is read as no change.
func ParseRecipeChange(data map[string]string) RecipeChange {
	var r RecipeChange
	if raw := data["recipe_diff"]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			r = RecipeChange{}
		}
	}
	r.WorkspaceSlug = data["workspace_slug"]
	r.ActorName = data["actor_name"]
	r.Source = data["source"]
	r.Stream = data["stream"]
	return r
}
