package project

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// CommentDefaults is `defaults.comments`: what applies to the comments of every
// content item that declares them.
type CommentDefaults struct {
	// Directives are the markers the project's own tools read at the start of a
	// comment line, such as `okapi-skip:`. A comment line that opens with one is
	// set aside wherever a check reads comments, and is never read as prose.
	Directives []string `yaml:"directives,omitempty" json:"directives,omitempty"`

	// Channel is the point, a qualified `profile/channel`, at which the comments
	// of every item that declares them sit, unless the item names its own. Empty
	// leaves each item's comments at the item's point.
	Channel string `yaml:"channel,omitempty" json:"channel,omitempty"`
}

// UnmarshalYAML rejects a key the mapping does not have.
func (d *CommentDefaults) UnmarshalYAML(node *yaml.Node) error {
	if err := knownKeys(node, "defaults.comments", "directives", "channel"); err != nil {
		return err
	}
	type alias CommentDefaults
	var a alias
	if err := node.Decode(&a); err != nil {
		return err
	}
	*d = CommentDefaults(a)
	return nil
}

// ContentComments is a content item's `comments:`, written `comments: true` or
// as a mapping that also carries what applies to the item's comments alone.
type ContentComments struct {
	// Declared reports that the comments in the item's files are content: a
	// check reads them at the item's point, under the voice and terms that
	// govern it.
	//
	// For a file no format reader covers, such as Go source, the comments are
	// the file's only content, as they are for every file of an item that sets
	// Only (ResolvedFile.CommentsOnly).
	Declared bool `yaml:"-" json:"declared,omitempty"`

	// Only narrows the item's content to its comments. The comment provider for
	// the file's format or language locates them, and nothing reads the values:
	// no extraction, convergence run, flow run, merge, coverage count or ship
	// gate. Such an item names no target.
	Only bool `yaml:"only,omitempty" json:"only,omitempty"`

	// Directives are markers the item's files carry beside the ones under
	// `defaults.comments`.
	Directives []string `yaml:"directives,omitempty" json:"directives,omitempty"`

	// Channel is the point, a qualified `profile/channel`, at which the item's
	// comments sit. It outranks `defaults.comments.channel`, and both outrank the
	// item's own `channel:` for the comments alone: the content the file's reader
	// extracts stays at the item's point.
	Channel string `yaml:"channel,omitempty" json:"channel,omitempty"`
}

// UnmarshalYAML accepts a boolean, or a mapping that declares the comments and
// says more about them.
func (c *ContentComments) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		var declared bool
		if node.ShortTag() != "!!bool" || node.Decode(&declared) != nil {
			return fmt.Errorf("line %d: comments: want true, false or a mapping, not %q", node.Line, node.Value)
		}
		*c = ContentComments{Declared: declared}
		return nil
	case yaml.MappingNode:
		if err := knownKeys(node, "comments", "directives", "channel", "only"); err != nil {
			return err
		}
		type alias ContentComments
		var a alias
		if err := node.Decode(&a); err != nil {
			return err
		}
		*c = ContentComments(a)
		c.Declared = true
		return nil
	default:
		return fmt.Errorf("line %d: comments: want true, false or a mapping", node.Line)
	}
}

// MarshalYAML writes `comments: true` for an item that declares nothing more,
// and the mapping otherwise.
func (c ContentComments) MarshalYAML() (any, error) {
	if len(c.Directives) == 0 && c.Channel == "" && !c.Only {
		return c.Declared, nil
	}
	type alias ContentComments
	return alias(c), nil
}

// IsZero reports an item that does not declare its comments.
func (c ContentComments) IsZero() bool {
	return !c.Declared && len(c.Directives) == 0 && c.Channel == "" && !c.Only
}

// validateCommentsOnly rejects an item that sets `comments.only` beside a key
// that shapes, redacts or delivers values, since the item has none. The
// format's name is allowed: it picks the comment provider.
func (item *ContentItem) validateCommentsOnly(at string) error {
	if !item.Comments.Only {
		return nil
	}
	switch {
	case item.Target != "":
		return fmt.Errorf("%s: comments.only is set, so the item cannot have a target (found %q)", at, item.Target)
	case len(item.TargetLanguages) > 0:
		return fmt.Errorf("%s: comments.only is set, so the item cannot have target_languages (found %v)", at, item.TargetLanguages)
	case item.Redaction != nil:
		return fmt.Errorf("%s: comments.only is set, so the item has no values to redact", at)
	case item.Format != nil && len(item.Format.Config) > 0:
		return fmt.Errorf("%s: comments.only is set, so format.config has no values to configure", at)
	case item.Format != nil && item.Format.Preset != "":
		return fmt.Errorf("%s: comments.only is set, so format.preset has no values to configure", at)
	}
	return nil
}

// CommentDirectives returns the directives in force in the comments of this
// item's files: the ones under `defaults.comments`, then the item's own.
func (item *ContentItem) CommentDirectives(defaults Defaults) []string {
	return slices.Concat(defaults.Comments.Directives, item.Comments.Directives)
}

// validateDirectives checks one list of declared directives, named by field.
// inherited is the defaults' list, which an item's own list does not repeat.
func validateDirectives(field string, directives, inherited []string) error {
	for i, d := range directives {
		at := fmt.Sprintf("%s[%d]", field, i)
		first, _ := utf8.DecodeRuneInString(d)
		switch {
		case d == "":
			return fmt.Errorf("%s: a directive is empty", at)
		case unicode.IsSpace(first):
			return fmt.Errorf("%s: directive %q starts with whitespace, and a comment line is matched after its leading whitespace", at, d)
		case strings.ContainsAny(d, "\r\n"):
			return fmt.Errorf("%s: directive %q holds a line break, and a directive marks one line", at, d)
		case slices.Contains(directives[:i], d):
			return fmt.Errorf("%s: directive %q is declared twice", at, d)
		case slices.Contains(inherited, d):
			return fmt.Errorf("%s: directive %q is already declared by defaults.comments.directives", at, d)
		}
	}
	return nil
}

// knownKeys rejects a mapping key outside want, naming the mapping it sits in.
func knownKeys(node *yaml.Node, field string, want ...string) error {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		if key := node.Content[i]; !slices.Contains(want, key.Value) {
			return fmt.Errorf("line %d: %s: unknown key %q (want %s)", key.Line, field, key.Value, strings.Join(want, ", "))
		}
	}
	return nil
}
