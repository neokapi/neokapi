package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/config"
	"github.com/neokapi/neokapi/core/format"
	fschema "github.com/neokapi/neokapi/core/format/schema"
	"github.com/neokapi/neokapi/core/formats"
	mttools "github.com/neokapi/neokapi/core/mt/tools"
	"github.com/neokapi/neokapi/core/registry"
	coreschema "github.com/neokapi/neokapi/core/schema"
	libtools "github.com/neokapi/neokapi/core/tools"
)

// nativeMeta is the slice of core/i18n/builtins/metadata.json we consume for
// English display names and descriptions (the registries leave tool
// descriptions empty — they live in the i18n catalog).
type nativeMeta struct {
	Tools   map[string]metaEntry `json:"tools"`
	Formats map[string]metaEntry `json:"formats"`
}

type metaEntry struct {
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

func loadNativeMeta(path string) (*nativeMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m nativeMeta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &m, nil
}

// nativeRegistries builds the format and tool registries exactly as
// cli.App.InitRegistries does, so the dataset matches the real CLI.
func nativeRegistries() (*registry.FormatRegistry, *registry.ToolRegistry) {
	freg := registry.NewFormatRegistry()
	formats.RegisterAll(freg, formats.RegisterOptions{
		SchemaReg: fschema.NewSchemaRegistry(),
		ConfigReg: config.DefaultRegistry,
	})
	treg := registry.NewToolRegistry()
	libtools.RegisterAll(treg)
	tools.RegisterAll(treg)
	mttools.RegisterAll(treg)
	return freg, treg
}

// collectNativeFormats produces one Entry per built-in format.
func collectNativeFormats(freg *registry.FormatRegistry, meta *nativeMeta) []Entry {
	var out []Entry
	for _, info := range freg.FormatInfos() {
		id := string(info.Name)
		e := Entry{
			ID:          id,
			Source:      SourceBuiltIn,
			Kind:        KindFormat,
			DisplayName: info.DisplayName,
			Extensions:  info.Extensions,
			MimeTypes:   info.MimeTypes,
			HasReader:   info.HasReader,
			HasWriter:   info.HasWriter,
		}

		if info.HasReader {
			if reader, err := freg.NewReader(info.Name); err == nil && reader != nil {
				if e.DisplayName == "" {
					e.DisplayName = reader.DisplayName()
				}
				if sp, ok := reader.Config().(format.SchemaProvider); ok {
					if s := sp.Schema(); s != nil {
						e.Description = s.Description
						e.Presets = s.Presets
						if raw, err := json.Marshal(s); err == nil {
							e.Schema = raw
						}
						if len(e.Extensions) == 0 {
							e.Extensions = s.FormatMeta.Extensions
						}
						if len(e.MimeTypes) == 0 {
							e.MimeTypes = s.FormatMeta.MimeTypes
						}
					}
				}
			}
		}

		applyMetaFallback(&e, meta.Formats[id])
		out = append(out, e)
	}
	return out
}

// portNames renders an IO contract as "type@side" tokens (optional
// consumed ports get a trailing "?") for the generated reference.
func portNames(fs []coreschema.IOPort) []string {
	if len(fs) == 0 {
		return nil
	}
	out := make([]string, len(fs))
	for i, f := range fs {
		s := string(f.Type) + "@" + f.Side.String()
		if f.Optional {
			s += "?"
		}
		out[i] = s
	}
	return out
}

// sideEffectNames stringifies a tool's declared side effects for the dataset.
func sideEffectNames(ses []coreschema.SideEffect) []string {
	if len(ses) == 0 {
		return nil
	}
	out := make([]string, len(ses))
	for i, se := range ses {
		out[i] = string(se)
	}
	return out
}

// collectNativeTools produces one Entry per built-in tool.
func collectNativeTools(treg *registry.ToolRegistry, meta *nativeMeta) []Entry {
	var out []Entry
	for _, info := range treg.ListWithSchemas() {
		id := string(info.Name)
		e := Entry{
			ID:                id,
			Source:            SourceBuiltIn,
			Kind:              KindTool,
			DisplayName:       info.DisplayName,
			Description:       info.Description,
			Category:          coreschema.NormalizeCategory(info.Category),
			Consumes:          portNames(info.Consumes),
			Produces:          portNames(info.Produces),
			SideEffects:       sideEffectNames(info.SideEffects),
			IsSourceTransform: info.IsSourceTransform,
			Tags:              info.Tags,
			Requires:          info.Requires,
			Cardinality:       string(info.Cardinality),
			Aliases:           info.Aliases,
		}

		if s := treg.Schema(info.Name); s != nil {
			if e.Description == "" {
				e.Description = s.Description
			}
			if e.DisplayName == "" && s.ToolMeta != nil {
				e.DisplayName = s.ToolMeta.DisplayName
			}
			if raw, err := json.Marshal(s); err == nil {
				e.Schema = raw
			}
		}

		applyMetaFallback(&e, meta.Tools[id])
		out = append(out, e)
	}
	return out
}

// applyMetaFallback fills empty display name / description from the i18n
// metadata catalog (the registries leave these blank for many entries).
func applyMetaFallback(e *Entry, m metaEntry) {
	if e.DisplayName == "" {
		e.DisplayName = m.DisplayName
	}
	if e.Description == "" {
		e.Description = m.Description
	}
	if e.DisplayName == "" {
		e.DisplayName = e.ID
	}
}

// schemaPropertyNames returns the top-level property names declared by a
// marshalled ComponentSchema/FormatSchema, used for gap detection.
func schemaPropertyNames(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var s struct {
		Properties map[string]coreschema.PropertySchema `json:"properties"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil
	}
	names := make([]string, 0, len(s.Properties))
	for k := range s.Properties {
		names = append(names, k)
	}
	return names
}
