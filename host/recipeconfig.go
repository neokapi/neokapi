package host

import (
	"fmt"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"gopkg.in/yaml.v3"
)

// Recipe config: the positional `kapi config <key> [value]` surface reads and
// writes fields of the project recipe. Core fields (name, source_language,
// preset) are handled natively; dotted keys whose head is a project-scope
// extension (server.url, server.stream, …) are edited generically through the
// raw extras node, so connected-project settings need no plugin command —
// registered extension decoders still validate the result before saving.

// RecipeConfigGet returns the string value of one recipe key.
func RecipeConfigGet(projectPath, key string) (string, error) {
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return "", fmt.Errorf("load project: %w", err)
	}
	switch normalizeRecipeKey(key) {
	case "name":
		return proj.Name, nil
	case "source_language":
		return string(proj.Defaults.SourceLanguage), nil
	case "preset":
		return proj.Preset, nil
	}
	head, rest, ok := splitExtensionKey(key)
	if !ok {
		return "", unknownRecipeKeyError(key)
	}
	node, found := proj.Extras[head]
	if !found {
		return "", fmt.Errorf("recipe key %q is not set (no %s: block in the recipe)", key, head)
	}
	var m map[string]any
	if err := node.Decode(&m); err != nil {
		return "", fmt.Errorf("read %s: block: %w", head, err)
	}
	v, found := lookupPath(m, rest)
	if !found {
		return "", fmt.Errorf("recipe key %q is not set", key)
	}
	return fmt.Sprintf("%v", v), nil
}

// RecipeConfigSet writes one recipe key and saves the recipe. Extension
// blocks are created on first set (e.g. setting bowrain.url on a recipe that
// binds no venue yet); the registered extension decoder, when present,
// validates the new value before anything is written.
func RecipeConfigSet(projectPath, key, value string) error {
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	switch normalizeRecipeKey(key) {
	case "name":
		proj.Name = value
		return project.Save(projectPath, proj)
	case "source_language":
		proj.Defaults.SourceLanguage = model.LocaleID(value)
		return project.Save(projectPath, proj)
	case "preset":
		proj.Preset = value
		return project.Save(projectPath, proj)
	}
	head, rest, ok := splitExtensionKey(key)
	if !ok {
		return unknownRecipeKeyError(key)
	}
	m := map[string]any{}
	if node, found := proj.Extras[head]; found {
		if err := node.Decode(&m); err != nil {
			return fmt.Errorf("read %s: block: %w", head, err)
		}
	}
	if err := setPath(m, rest, value); err != nil {
		return fmt.Errorf("set %q: %w", key, err)
	}
	var node yaml.Node
	if err := node.Encode(m); err != nil {
		return fmt.Errorf("encode %s: block: %w", head, err)
	}
	if proj.Extras == nil {
		proj.Extras = map[string]yaml.Node{}
	}
	proj.Extras[head] = node
	// Re-validate through the registered extension decoder (when a schema
	// package or manifest-driven plugin registered one), so a bad value is
	// rejected before the recipe is written.
	if err := proj.Validate(); err != nil {
		return fmt.Errorf("invalid value for %q: %w", key, err)
	}
	return project.Save(projectPath, proj)
}

// normalizeRecipeKey maps the accepted spellings of the core keys onto one
// canonical form.
func normalizeRecipeKey(key string) string {
	switch strings.ToLower(key) {
	case "name", "project.name":
		return "name"
	case "source_language", "defaults.source_language":
		return "source_language"
	case "preset":
		return "preset"
	}
	return key
}

// splitExtensionKey splits a dotted key into a project-scope extension head
// and the remaining path (server.url → "server", ["url"]). ok is false when
// the head is not a registered project-scope extension — unknown top-level
// recipe fields are never invented by config.
func splitExtensionKey(key string) (head string, rest []string, ok bool) {
	parts := strings.Split(key, ".")
	if len(parts) < 2 {
		return "", nil, false
	}
	if _, registered := project.ExtensionRegistered(project.ScopeProject, parts[0]); !registered {
		return "", nil, false
	}
	return parts[0], parts[1:], true
}

func lookupPath(m map[string]any, path []string) (any, bool) {
	cur := any(m)
	for _, p := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = mm[p]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func setPath(m map[string]any, path []string, value string) error {
	cur := m
	for i, p := range path {
		if i == len(path)-1 {
			cur[p] = value
			return nil
		}
		next, found := cur[p]
		if !found {
			child := map[string]any{}
			cur[p] = child
			cur = child
			continue
		}
		child, ok := next.(map[string]any)
		if !ok {
			return fmt.Errorf("%s is not a map", strings.Join(path[:i+1], "."))
		}
		cur = child
	}
	return nil
}

// unknownRecipeKeyError lists what the positional form accepts, including
// the registered project-scope extension blocks of this install.
func unknownRecipeKeyError(key string) error {
	exts := project.RegisteredExtensions(project.ScopeProject)
	sort.Strings(exts)
	hint := "name, source_language, preset"
	if len(exts) > 0 {
		hint += ", or <block>.<field> for a registered recipe block (" + strings.Join(exts, ", ") + ")"
	}
	return fmt.Errorf("unknown recipe key %q. Accepted keys: %s (app configuration lives under 'kapi config get/set')", key, hint)
}
