package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// pluginManifest is the in-repo Mode-C plugin manifest
// (plugins/<name>/manifest.json). Only the format capabilities are read for the
// reference; tools/commands are out of scope here.
type pluginManifest struct {
	Plugin       string `json:"plugin"`
	Capabilities struct {
		Formats []pluginFormat `json:"formats"`
	} `json:"capabilities"`
}

type pluginFormat struct {
	Name         string   `json:"name"`
	DisplayName  string   `json:"display_name"`
	Description  string   `json:"description"`
	Extensions   []string `json:"extensions"`
	MimeTypes    []string `json:"mime_types"`
	Capabilities []string `json:"capabilities"` // "read","write"
	Schema       string   `json:"schema"`       // relative path to a JSON Schema
}

// collectPlugins scans in-repo Mode-C plugin dirs (pluginsDir/<name>/manifest.json)
// and returns their declared format entries, so the reference includes
// plugin-provided formats (e.g. PDF via kapi-pdfium) reproducibly from in-repo
// source — unlike okapi-bridge, which lives in a separate repo. A missing
// pluginsDir is not fatal: the caller decides whether to warn.
func collectPlugins(pluginsDir string) (formats []Entry, err error) {
	dirents, err := os.ReadDir(pluginsDir)
	if err != nil {
		return nil, err
	}
	sort.Slice(dirents, func(i, j int) bool { return dirents[i].Name() < dirents[j].Name() })
	for _, de := range dirents {
		if !de.IsDir() {
			continue
		}
		dir := filepath.Join(pluginsDir, de.Name())
		data, rerr := os.ReadFile(filepath.Join(dir, "manifest.json"))
		if rerr != nil {
			continue // not every dir is a plugin (no manifest)
		}
		var man pluginManifest
		if json.Unmarshal(data, &man) != nil {
			continue
		}
		for _, f := range man.Capabilities.Formats {
			e := Entry{
				ID:          f.Name,
				Source:      SourcePlugin,
				Kind:        KindFormat,
				DisplayName: firstNonEmpty(f.DisplayName, f.Name),
				Description: f.Description,
				Extensions:  f.Extensions,
				MimeTypes:   f.MimeTypes,
				HasReader:   sliceContains(f.Capabilities, "read"),
				HasWriter:   sliceContains(f.Capabilities, "write"),
			}
			if f.Schema != "" {
				if raw, srr := os.ReadFile(filepath.Join(dir, f.Schema)); srr == nil {
					e.Schema = json.RawMessage(raw)
				}
			}
			formats = append(formats, e)
		}
	}
	return formats, nil
}
