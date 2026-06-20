// Package gen emits the builtin tool/format metadata document that the
// i18n pipeline reads via the standard JSON filter. The generated
// document is object-keyed (not array-keyed) so block names produced by
// the JSON filter with useKeyAsName + useFullKeyPath are stable scope
// identifiers — "tools/translate/displayName" stays put even when a
// new tool is added. That stability is what makes msgctxt lookup from
// (toolID, field) work in the runtime Translator.
//
// Invoked from core/i18n/doc.go via //go:generate go run ./gen/cmd. The
// generated tree lives at core/i18n/builtins/ and is committed so CI can
// enforce freshness with `git diff --exit-code`.
package gen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	neokapiconfig "github.com/neokapi/neokapi/core/config"
	fschema "github.com/neokapi/neokapi/core/format/schema"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/registry"
	libtools "github.com/neokapi/neokapi/core/tools"
)

// Entry is a single translatable surface in the generated document.
// Maps 1:1 to object keys on the tools / formats / plugins object —
// the entry's parent object key (e.g. "translate") is the owner ID.
type Entry struct {
	DisplayName string                       `json:"displayName,omitempty"`
	Description string                       `json:"description,omitempty"`
	Category    string                       `json:"category,omitempty"`
	Extensions  []string                     `json:"extensions,omitempty"`
	MimeTypes   []string                     `json:"mimeTypes,omitempty"`
	Properties  map[string]PropertyEntry     `json:"properties,omitempty"`
	Groups      map[string]GroupEntry        `json:"groups,omitempty"`
	Options     map[string]OptionEntry       `json:"options,omitempty"` // enum options on the entry root
	Enums       map[string]map[string]string `json:"enumDescriptions,omitempty"`
}

// PropertyEntry mirrors schema.PropertySchema's translatable subset.
type PropertyEntry struct {
	Title            string                   `json:"title,omitempty"`
	Description      string                   `json:"description,omitempty"`
	Options          map[string]OptionEntry   `json:"options,omitempty"`
	EnumDescriptions map[string]string        `json:"enumDescriptions,omitempty"`
	Properties       map[string]PropertyEntry `json:"properties,omitempty"`
	Items            *PropertyEntry           `json:"items,omitempty"`
}

// GroupEntry mirrors schema.ParameterGroup's translatable subset.
type GroupEntry struct {
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
}

// OptionEntry mirrors schema.OptionItem's translatable subset.
type OptionEntry struct {
	Label string `json:"label,omitempty"`
}

// Document is the top-level i18n source document. Keyed by domain (tools,
// formats, plugins), then by owner ID. The JSON filter with
// useKeyAsName + useFullKeyPath + useLeadingSlashOnKeyPath = true
// produces block names like "/tools/translate/displayName", which
// match the Scope values used by the runtime Translator.
type Document struct {
	Tools   map[string]Entry `json:"tools,omitempty"`
	Formats map[string]Entry `json:"formats,omitempty"`
	Plugins map[string]Entry `json:"plugins,omitempty"`
}

// Generate writes metadata.json (translatable text only) and schemas/
// (per-tool/per-format JSON Schemas, kept for reference / future
// downstream tooling) to outDir. Deterministic output: map iteration is
// sorted by key via json.MarshalIndent's stable ordering.
func Generate(outDir string) error {
	toolReg := registry.NewToolRegistry()
	libtools.RegisterAll(toolReg)
	aitools.RegisterAll(toolReg)

	formatReg := registry.NewFormatRegistry()
	schemaReg := fschema.NewSchemaRegistry()
	formats.RegisterAll(formatReg, formats.RegisterOptions{
		SchemaReg: schemaReg,
		ConfigReg: neokapiconfig.NewRegistry(),
	})

	doc := buildDocument(toolReg, formatReg, schemaReg)

	if err := resetDir(outDir); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(outDir, "metadata.json"), doc); err != nil {
		return err
	}
	return nil
}

func buildDocument(
	toolReg *registry.ToolRegistry,
	formatReg *registry.FormatRegistry,
	schemaReg *fschema.SchemaRegistry,
) *Document {
	doc := &Document{
		Tools:   map[string]Entry{},
		Formats: map[string]Entry{},
	}

	for _, entry := range toolReg.CLITools() {
		name := string(entry.Info.Name)
		e := Entry{
			DisplayName: entry.Info.DisplayName,
			Description: entry.Info.Description,
			Category:    entry.Info.Category,
		}
		if entry.Schema != nil {
			if len(entry.Schema.Properties) > 0 {
				e.Properties = buildProperties(entry.Schema.Properties)
			}
			if len(entry.Schema.Groups) > 0 {
				e.Groups = map[string]GroupEntry{}
				for _, g := range entry.Schema.Groups {
					if g.Label != "" || g.Description != "" {
						e.Groups[g.ID] = GroupEntry{Label: g.Label, Description: g.Description}
					}
				}
				if len(e.Groups) == 0 {
					e.Groups = nil
				}
			}
		}
		doc.Tools[name] = e
	}

	for _, info := range formatReg.FormatInfos() {
		name := string(info.Name)
		e := Entry{
			DisplayName: info.DisplayName,
			Extensions:  info.Extensions,
			MimeTypes:   info.MimeTypes,
		}
		if s, ok := schemaReg.GetSchema(name); ok {
			// Format schemas use format/schema.PropertySchema (embeds
			// core/schema.PropertySchema). Extract the translatable
			// subset via the core shape.
			core := make(map[string]any, len(s.Properties))
			for k, p := range s.Properties {
				core[k] = p.PropertySchema
			}
			e.Properties = buildPropertiesFromAny(core)
		}
		doc.Formats[name] = e
	}

	return doc
}

// buildProperties walks a map of core/schema.PropertySchema values and
// returns the translatable subset in PropertyEntry form.
func buildProperties(src any) map[string]PropertyEntry {
	// Delegate via the any-typed helper to share recursion with format
	// schemas (which use format/schema.PropertySchema that embeds the
	// core type).
	switch m := src.(type) {
	case map[string]any:
		return buildPropertiesFromAny(m)
	}
	// Cast by reflection: round-trip through JSON. Small, correct, and
	// avoids importing core/schema just for the type assertion here.
	b, err := json.Marshal(src)
	if err != nil {
		return nil
	}
	var generic map[string]map[string]any
	if err := json.Unmarshal(b, &generic); err != nil {
		return nil
	}
	return buildPropertiesFromGeneric(generic)
}

// buildPropertiesFromAny is used when the input is already generic.
func buildPropertiesFromAny(src map[string]any) map[string]PropertyEntry {
	b, err := json.Marshal(src)
	if err != nil {
		return nil
	}
	var generic map[string]map[string]any
	if err := json.Unmarshal(b, &generic); err != nil {
		return nil
	}
	return buildPropertiesFromGeneric(generic)
}

func buildPropertiesFromGeneric(src map[string]map[string]any) map[string]PropertyEntry {
	out := map[string]PropertyEntry{}
	for key, prop := range src {
		p := PropertyEntry{}
		if s, ok := prop["title"].(string); ok {
			p.Title = s
		}
		if s, ok := prop["description"].(string); ok {
			p.Description = s
		}
		if opts, ok := prop["options"].([]any); ok {
			p.Options = map[string]OptionEntry{}
			for _, o := range opts {
				om, _ := o.(map[string]any)
				val := fmt.Sprint(om["value"])
				if label, _ := om["label"].(string); label != "" {
					p.Options[val] = OptionEntry{Label: label}
				}
			}
			if len(p.Options) == 0 {
				p.Options = nil
			}
		}
		if ed, ok := prop["ui:enum-descriptions"].(map[string]any); ok {
			p.EnumDescriptions = map[string]string{}
			for k, v := range ed {
				if s, ok := v.(string); ok {
					p.EnumDescriptions[k] = s
				}
			}
			if len(p.EnumDescriptions) == 0 {
				p.EnumDescriptions = nil
			}
		}
		if children, ok := prop["properties"].(map[string]any); ok {
			nested := map[string]map[string]any{}
			for k, v := range children {
				if m, ok := v.(map[string]any); ok {
					nested[k] = m
				}
			}
			if len(nested) > 0 {
				p.Properties = buildPropertiesFromGeneric(nested)
			}
		}
		if items, ok := prop["items"].(map[string]any); ok {
			single := buildPropertiesFromGeneric(map[string]map[string]any{"items": items})
			if v, ok := single["items"]; ok {
				p.Items = &v
			}
		}
		// Only include properties with at least one translatable leaf.
		if p.Title != "" || p.Description != "" || len(p.Options) > 0 ||
			len(p.EnumDescriptions) > 0 || len(p.Properties) > 0 || p.Items != nil {
			out[key] = p
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// writeJSON emits pretty-printed JSON with stable 2-space indentation and
// a trailing newline. Stability matters: the freshness check (`git diff
// --exit-code core/i18n/builtins/`) depends on byte-identical output.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func resetDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return os.MkdirAll(dir, 0o755)
		}
		return err
	}
	for _, e := range entries {
		if e.Name() == ".gitkeep" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}
