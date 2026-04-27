package project

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// RequiresMap is the type of KapiProject.Requires — a map of plugin name to
// semver-style version constraint. It defines a custom UnmarshalYAML that
// rejects the legacy bare-list form (`requires: [bowrain]`) with an
// actionable migration hint.
type RequiresMap map[string]string

// UnmarshalYAML implements yaml.Unmarshaler.
func (r *RequiresMap) UnmarshalYAML(node *yaml.Node) error {
	if node == nil || node.Kind == 0 {
		return nil
	}
	if node.Kind == yaml.SequenceNode {
		var list []string
		if err := node.Decode(&list); err == nil {
			items := make([]string, len(list))
			for i, name := range list {
				items[i] = name + `: "*"`
			}
			joined := strings.Join(items, ", ")
			return fmt.Errorf("requires: bare-list form is no longer supported (use the map form, e.g. requires: { %s })", joined)
		}
		return errors.New("requires: bare-list form is no longer supported; use the map form (plugin: version-constraint)")
	}
	if node.Kind == yaml.MappingNode {
		m := make(map[string]string, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			k := node.Content[i]
			v := node.Content[i+1]
			if k.Kind != yaml.ScalarNode {
				return fmt.Errorf("requires: keys must be plugin names (got %s at line %d)", kindName(k.Kind), k.Line)
			}
			if v.Kind != yaml.ScalarNode {
				return fmt.Errorf("requires.%s: value must be a version constraint string (got %s at line %d)", k.Value, kindName(v.Kind), v.Line)
			}
			m[k.Value] = v.Value
		}
		*r = m
		return nil
	}
	return fmt.Errorf("requires: must be a map of plugin → version constraint (got %s at line %d)", kindName(node.Kind), node.Line)
}

// MarshalYAML implements yaml.Marshaler so the Requires map round-trips
// cleanly through Save.
func (r RequiresMap) MarshalYAML() (any, error) {
	if len(r) == 0 {
		return nil, nil
	}
	return map[string]string(r), nil
}

func kindName(k yaml.Kind) string {
	switch k {
	case yaml.DocumentNode:
		return "document"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.MappingNode:
		return "mapping"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	default:
		return "unknown"
	}
}

// validVersionConstraint returns true when c is a syntactically valid
// version constraint string. Empty strings are rejected — every entry
// must declare a constraint (use "*" for "any version").
func validVersionConstraint(c string) bool {
	if c == "" {
		return false
	}
	if c == "*" {
		return true
	}
	rest := c
	switch {
	case strings.HasPrefix(c, "^"):
		rest = c[1:]
	case strings.HasPrefix(c, "~"):
		rest = c[1:]
	case strings.HasPrefix(c, ">="):
		rest = c[2:]
	case strings.HasPrefix(c, "<="):
		rest = c[2:]
	case strings.HasPrefix(c, ">"):
		rest = c[1:]
	case strings.HasPrefix(c, "<"):
		rest = c[1:]
	case strings.HasPrefix(c, "="):
		rest = c[1:]
	}
	rest = strings.TrimPrefix(rest, "v")
	if rest == "" {
		return false
	}
	for _, r := range rest {
		if r == '.' || r == '-' || r == '+' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			continue
		}
		return false
	}
	return true
}
