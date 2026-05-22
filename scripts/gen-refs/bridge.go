package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// bridgeManifest is the okapi-bridge plugin manifest (dist/plugin/manifest.json).
type bridgeManifest struct {
	Name         string      `json:"name"`
	Version      string      `json:"version"`
	Capabilities []bridgeCap `json:"capabilities"`
}

// bridgeCap is one format or tool capability declared in the manifest.
type bridgeCap struct {
	Type         string   `json:"type"` // "format" | "tool"
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	DisplayName  string   `json:"display_name"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities"` // "read","write"
	MimeTypes    []string `json:"mime_types"`
	Extensions   []string `json:"extensions"`
	Category     string   `json:"category"`
	Inputs       []string `json:"inputs"`
	Outputs      []string `json:"outputs"`
	Tags         []string `json:"tags"`
	Requires     []string `json:"requires"`
	Schema       string   `json:"schema"`
	Doc          string   `json:"doc"`
	PresetsDir   string   `json:"presets_dir"`
}

// bridgeDoc is the per-capability doc.json shipped by okapi-bridge.
type bridgeDoc struct {
	Overview            string                       `json:"overview"`
	Parameters          map[string]bridgeDocParam    `json:"parameters"`
	Limitations         []string                     `json:"limitations"`
	ProcessingNotes     []string                     `json:"processingNotes"`
	Examples            []DocExample                  `json:"examples"`
	PropertySuggestions map[string]bridgePropSuggest `json:"propertySuggestions"`
	WikiURL             string                       `json:"wikiUrl"`
}

type bridgeDocParam struct {
	Help      string       `json:"help"`
	Values    string       `json:"values"`
	Examples  []string     `json:"examples"`
	Notes     []string     `json:"notes"`
	DependsOn []DocDepends `json:"dependsOn"`
}

type bridgePropSuggest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// collectBridge reads the bridge plugin dir and returns format and tool
// entries. A missing dir is not fatal: the caller decides whether to warn.
func collectBridge(pluginDir string) (formats, tools []Entry, err error) {
	manifestPath := filepath.Join(pluginDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, nil, err
	}
	var man bridgeManifest
	if err := json.Unmarshal(data, &man); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", manifestPath, err)
	}

	for _, cap := range man.Capabilities {
		e := Entry{
			ID:          cap.ID,
			Source:      SourceOkapi,
			DisplayName: firstNonEmpty(cap.DisplayName, cap.Name, cap.ID),
			Description: cap.Description,
		}

		switch cap.Type {
		case "format":
			e.Kind = KindFormat
			e.Extensions = cap.Extensions
			e.MimeTypes = cap.MimeTypes
			e.HasReader = sliceContains(cap.Capabilities, "read")
			e.HasWriter = sliceContains(cap.Capabilities, "write")
			e.Presets = readBridgePresets(pluginDir, cap.PresetsDir)
		case "tool":
			e.Kind = KindTool
			e.Category = cap.Category
			e.Inputs = cap.Inputs
			e.Outputs = cap.Outputs
			e.Tags = cap.Tags
			e.Requires = cap.Requires
		default:
			continue
		}

		if cap.Schema != "" {
			if raw, rerr := os.ReadFile(filepath.Join(pluginDir, cap.Schema)); rerr == nil {
				e.Schema = json.RawMessage(raw)
				// Manifest format capabilities omit description/title; fall back
				// to the schema's own title/description.
				var head struct {
					Title       string `json:"title"`
					Description string `json:"description"`
				}
				if json.Unmarshal(raw, &head) == nil {
					if e.DisplayName == "" || e.DisplayName == e.ID {
						e.DisplayName = firstNonEmpty(head.Title, e.DisplayName)
					}
					if e.Description == "" {
						e.Description = head.Description
					}
				}
			}
		}
		if cap.Doc != "" {
			e.Doc = readBridgeDoc(filepath.Join(pluginDir, cap.Doc))
		}

		if cap.Type == "format" {
			formats = append(formats, e)
		} else {
			tools = append(tools, e)
		}
	}
	return formats, tools, nil
}

// readBridgeDoc loads and converts a bridge doc.json into the unified Doc.
func readBridgeDoc(path string) *Doc {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var bd bridgeDoc
	if err := json.Unmarshal(data, &bd); err != nil {
		return nil
	}

	doc := &Doc{
		Overview:        bd.Overview,
		Limitations:     bd.Limitations,
		ProcessingNotes: bd.ProcessingNotes,
		Examples:        bd.Examples,
		WikiURL:         bd.WikiURL,
	}

	if len(bd.Parameters) > 0 || len(bd.PropertySuggestions) > 0 {
		doc.Parameters = make(map[string]DocParam)
		for name, p := range bd.Parameters {
			doc.Parameters[name] = DocParam{
				Help:      p.Help,
				Values:    p.Values,
				Examples:  p.Examples,
				Notes:     p.Notes,
				DependsOn: p.DependsOn,
			}
		}
		// Fold propertySuggestions (title/description) into the param docs so
		// the renderer has a short description alongside the longer help text.
		for name, sug := range bd.PropertySuggestions {
			dp := doc.Parameters[name]
			if dp.Description == "" {
				dp.Description = sug.Description
			}
			doc.Parameters[name] = dp
		}
	}
	return doc
}

// readBridgePresets loads every <name>.json under presetsDir into a preset map.
func readBridgePresets(pluginDir, presetsDir string) map[string]map[string]any {
	if presetsDir == "" {
		return nil
	}
	dir := filepath.Join(pluginDir, presetsDir)
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(files) == 0 {
		return nil
	}
	sort.Strings(files)
	out := make(map[string]map[string]any)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var params map[string]any
		if err := json.Unmarshal(data, &params); err != nil {
			continue
		}
		id := strings.TrimSuffix(filepath.Base(f), ".json")
		out[id] = params
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func sliceContains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
