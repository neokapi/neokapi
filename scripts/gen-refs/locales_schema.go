package main

import (
	"encoding/json"
	"fmt"
	"strings"

	fschema "github.com/neokapi/neokapi/core/format/schema"
	corei18n "github.com/neokapi/neokapi/core/i18n"
	"github.com/neokapi/neokapi/core/model"
	coreschema "github.com/neokapi/neokapi/core/schema"
)

// The parameter text a reference page prints from an entry's schema: property
// titles and descriptions, option labels, enum descriptions, group labels. The
// builtins document carries the same leaves under the same scopes
// (tools.<id>.properties.<name>.title), so the framework's own schema
// localizer applies a catalog to a schema here exactly as the binary applies
// the compiled catalog at runtime.

// catalogTranslator answers the framework's Translator interface from a parsed
// catalog. The localizer derives every scope from the schema's tool id under
// `tools.`; a format's schema is localized through the same code with its
// scopes redirected to `formats.`.
type catalogTranslator struct {
	cat    map[string]any
	locale string
	domain string
	cov    *localeCoverage
}

func (c *catalogTranslator) Locale() model.LocaleID { return model.LocaleID(c.locale) }

func (c *catalogTranslator) T(scope corei18n.Scope, source string) string {
	path := strings.Split(string(scope), ".")
	if len(path) > 0 && path[0] == "tools" {
		path[0] = c.domain
	}
	// A schema's own title is the entry's display name, which the builtins
	// document records under that key.
	if len(path) == 3 && path[2] == "title" {
		if s, ok := lookupScoped(c.cat, []string{path[0], path[1], "displayName"}); ok {
			c.cov.translated++
			return s
		}
	}
	return c.cov.takeScoped(source, c.cat, path)
}

// takeScoped is take for a dotted scope whose segments may themselves carry
// dots: a property is keyed by its schema path (`parser.preserveWhitespace`),
// so a segment that names no key is joined with the next until one does.
func (c *localeCoverage) takeScoped(english string, cat map[string]any, path []string) string {
	if english == "" {
		return english
	}
	if s, ok := lookupScoped(cat, path); ok {
		c.translated++
		return s
	}
	if c.missing == nil {
		c.missing = map[string]bool{}
	}
	c.missing[strings.Join(path, ".")] = true
	return english
}

func lookupScoped(cat map[string]any, path []string) (string, bool) {
	var cur any = cat
	for i := 0; i < len(path); {
		m, ok := cur.(map[string]any)
		if !ok {
			return "", false
		}
		// The longest run of segments that names a key wins, so a dotted
		// property name is matched whole before its first segment is tried.
		matched := false
		for j := len(path); j > i; j-- {
			if v, ok := m[strings.Join(path[i:j], ".")]; ok {
				cur, i, matched = v, j, true
				break
			}
		}
		if !matched {
			return "", false
		}
	}
	s, ok := cur.(string)
	return s, ok && s != ""
}

// localizeSchema rewrites the translatable leaves of an entry's schema JSON
// through the catalog. The schema round-trips through the Go type it was
// marshalled from, so an entry nothing translates comes back byte-identical.
func localizeSchema(raw json.RawMessage, kind, id string, cat map[string]any, locale string, cov *localeCoverage) (json.RawMessage, error) {
	if len(raw) == 0 || cat == nil {
		return raw, nil
	}
	tr := &catalogTranslator{cat: cat, locale: locale, domain: kind + "s", cov: cov}
	switch kind {
	case KindTool:
		var s coreschema.ComponentSchema
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("%s %s: parse schema: %w", kind, id, err)
		}
		if s.ToolMeta == nil {
			// The localizer scopes by the tool id it finds on the schema.
			s.ToolMeta = &coreschema.ToolMeta{ID: id}
			out := corei18n.LocalizeComponentSchema(&s, tr)
			out.ToolMeta = nil
			return json.Marshal(out)
		}
		return json.Marshal(corei18n.LocalizeComponentSchema(&s, tr))
	case KindFormat:
		var s fschema.FormatSchema
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("%s %s: parse schema: %w", kind, id, err)
		}
		// A format schema embeds the core property schema and redeclares
		// the nested properties in its own type; the localizer takes the
		// core shape, so the tree crosses over and back at every level.
		cs := &coreschema.ComponentSchema{
			ToolMeta:    &coreschema.ToolMeta{ID: id},
			Title:       s.Title,
			Description: s.Description,
			Groups:      s.Groups,
			Properties:  toCoreProperties(s.Properties),
		}
		out := corei18n.LocalizeComponentSchema(cs, tr)
		s.Title = out.Title
		s.Description = out.Description
		s.Groups = out.Groups
		s.Properties = fromCoreProperties(out.Properties, s.Properties)
		return json.Marshal(s)
	}
	return raw, nil
}

// toCoreProperties lifts a format property tree into the core shape, nested
// properties included.
func toCoreProperties(in map[string]fschema.PropertySchema) map[string]coreschema.PropertySchema {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]coreschema.PropertySchema, len(in))
	for name, p := range in {
		core := p.PropertySchema
		core.Properties = toCoreProperties(p.Properties)
		out[name] = core
	}
	return out
}

// fromCoreProperties writes the localized core tree back over the format
// tree it came from, keeping the format-level fields the core shape lacks.
func fromCoreProperties(in map[string]coreschema.PropertySchema, orig map[string]fschema.PropertySchema) map[string]fschema.PropertySchema {
	if len(orig) == 0 {
		return orig
	}
	out := make(map[string]fschema.PropertySchema, len(orig))
	for name, fp := range orig {
		core, ok := in[name]
		if !ok {
			out[name] = fp
			continue
		}
		nested := fromCoreProperties(core.Properties, fp.Properties)
		core.Properties = nil
		fp.PropertySchema = core
		fp.Properties = nested
		out[name] = fp
	}
	return out
}
