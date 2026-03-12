// gen-format-docs generates website/static/data/formats.json from built-in
// format metadata and configuration schemas. Run via:
//
//	go run ./scripts/gen-format-docs
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/schema"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/registry"
)

// FormatDoc is the JSON structure for a single format.
type FormatDoc struct {
	ID          string             `json:"id"`
	DisplayName string             `json:"displayName"`
	Description string             `json:"description"`
	Extensions  []string           `json:"extensions"`
	MIMETypes   []string           `json:"mimeTypes"`
	HasReader   bool               `json:"hasReader"`
	HasWriter   bool               `json:"hasWriter"`
	Groups      []GroupDoc         `json:"groups,omitempty"`
	Properties  map[string]PropDoc `json:"properties,omitempty"`
	Presets     []PresetDoc        `json:"presets,omitempty"`
}

// GroupDoc is a parameter group for UI display.
type GroupDoc struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Description string   `json:"description,omitempty"`
	Collapsed   bool     `json:"collapsed,omitempty"`
	Fields      []string `json:"fields"`
}

// PropDoc is a single configurable property.
type PropDoc struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Default     any    `json:"default,omitempty"`
	Widget      string `json:"widget,omitempty"`
}

// PresetDoc is a named preset configuration.
type PresetDoc struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// OutputDoc is the top-level JSON structure.
type OutputDoc struct {
	Formats []FormatDoc `json:"formats"`
}

func main() {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	infos := reg.FormatInfos()
	var docs []FormatDoc

	for _, info := range infos {
		doc := FormatDoc{
			ID:          info.Name,
			DisplayName: info.DisplayName,
			Extensions:  info.Extensions,
			MIMETypes:   info.MimeTypes,
			HasReader:   info.HasReader,
			HasWriter:   info.HasWriter,
		}

		// Get schema from reader if available.
		if info.HasReader {
			reader, err := reg.NewReader(info.Name)
			if err == nil && reader != nil {
				if sp, ok := reader.Config().(format.SchemaProvider); ok {
					s := sp.Schema()
					doc.Description = s.Description
					doc.Properties = convertProperties(s.Properties)
					doc.Groups = convertGroups(s.Groups, s.Properties)
					doc.Presets = convertPresets(s.FilterMeta.Configurations)

					// Use schema extensions/mimeTypes if the reader didn't provide them.
					if len(doc.Extensions) == 0 {
						doc.Extensions = s.FilterMeta.Extensions
					}
					if len(doc.MIMETypes) == 0 {
						doc.MIMETypes = s.FilterMeta.MimeTypes
					}
				}

				// Fill display name from reader if registry didn't have it.
				if doc.DisplayName == "" {
					doc.DisplayName = reader.DisplayName()
				}
				// Fill extensions from signature if not yet set.
				if len(doc.Extensions) == 0 {
					doc.Extensions = reader.Signature().Extensions
				}
				if len(doc.MIMETypes) == 0 {
					doc.MIMETypes = reader.Signature().MIMETypes
				}
			}
		}

		docs = append(docs, doc)
	}

	sort.Slice(docs, func(i, j int) bool {
		return docs[i].ID < docs[j].ID
	})

	output := OutputDoc{Formats: docs}
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling JSON: %v\n", err)
		os.Exit(1)
	}

	outPath := "website/static/data/formats.json"
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", outPath, err)
		os.Exit(1)
	}
	fmt.Printf("Generated %s (%d formats)\n", outPath, len(docs))
}

func convertGroups(groups []schema.ParameterGroup, props map[string]schema.PropertySchema) []GroupDoc {
	if len(groups) == 0 {
		return nil
	}
	var result []GroupDoc
	for _, g := range groups {
		// Expand fields that reference object properties with FlattenPath children.
		var fields []string
		for _, field := range g.Fields {
			if p, ok := props[field]; ok && p.Type == "object" && len(p.Properties) > 0 && hasAnyFlattened(p.Properties) {
				for _, sub := range p.Properties {
					if sub.FlattenPath != "" {
						fields = append(fields, sub.FlattenPath)
					}
				}
			} else {
				fields = append(fields, field)
			}
		}
		result = append(result, GroupDoc{
			ID:          g.ID,
			Label:       g.Label,
			Description: g.Description,
			Collapsed:   g.Collapsed,
			Fields:      fields,
		})
	}
	return result
}

func convertProperties(props map[string]schema.PropertySchema) map[string]PropDoc {
	if len(props) == 0 {
		return nil
	}
	result := make(map[string]PropDoc, len(props))
	for name, p := range props {
		// Flatten object properties that have sub-properties with FlattenPath.
		if p.Type == "object" && len(p.Properties) > 0 {
			for _, sub := range p.Properties {
				flatName := sub.FlattenPath
				if flatName == "" {
					continue
				}
				result[flatName] = PropDoc{
					Type:        sub.Type,
					Description: sub.Description,
					Default:     sub.Default,
					Widget:      sub.Widget,
				}
			}
			// If the object had no flattened children, emit it as-is.
			if !hasAnyFlattened(p.Properties) {
				result[name] = PropDoc{
					Type:        p.Type,
					Description: p.Description,
					Default:     p.Default,
					Widget:      p.Widget,
				}
			}
		} else {
			result[name] = PropDoc{
				Type:        p.Type,
				Description: p.Description,
				Default:     p.Default,
				Widget:      p.Widget,
			}
		}
	}
	return result
}

func hasAnyFlattened(props map[string]schema.PropertySchema) bool {
	for _, p := range props {
		if p.FlattenPath != "" {
			return true
		}
	}
	return false
}

func convertPresets(configs []schema.FilterConfiguration) []PresetDoc {
	if len(configs) == 0 {
		return nil
	}
	var result []PresetDoc
	for _, c := range configs {
		result = append(result, PresetDoc{
			ID:          c.ConfigID,
			Name:        c.Name,
			Description: c.Description,
			Parameters:  c.Parameters,
		})
	}
	return result
}
