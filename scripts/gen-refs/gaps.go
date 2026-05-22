package main

import (
	"encoding/json"
	"sort"
)

// detectGaps walks every entry and reports missing documentation metadata:
// absent descriptions, absent overview/doc content, and parameters that have
// neither a schema description nor authored help. The report is committed as
// reference-gaps.json so the backlog stays visible rather than one-off.
func detectGaps(entries []Entry) []Gap {
	var gaps []Gap
	for _, e := range entries {
		hasOverview := e.Doc != nil && e.Doc.Overview != ""
		// A missing short description only matters when there is no overview to
		// fall back on — the UI shows the overview's lead when description is absent.
		if e.Description == "" && !hasOverview {
			gaps = append(gaps, Gap{e.Kind, e.Source, e.ID, "description", "no description or overview"})
		}
		if !hasOverview {
			gaps = append(gaps, Gap{e.Kind, e.Source, e.ID, "doc.overview", "no overview / rich documentation"})
		} else if len(e.Doc.Examples) == 0 && entryHasParams(e) {
			// Worked examples are only expected from configurable entries; a
			// parameter-less format has nothing meaningful to show.
			gaps = append(gaps, Gap{e.Kind, e.Source, e.ID, "doc.examples", "configurable but no worked examples"})
		}
		gaps = append(gaps, propertyGaps(e)...)
	}
	sort.Slice(gaps, func(i, j int) bool {
		if gaps[i].Kind != gaps[j].Kind {
			return gaps[i].Kind < gaps[j].Kind
		}
		if gaps[i].ID != gaps[j].ID {
			return gaps[i].ID < gaps[j].ID
		}
		return gaps[i].Field < gaps[j].Field
	})
	return gaps
}

// entryHasParams reports whether the entry's schema declares any properties.
func entryHasParams(e Entry) bool {
	return len(schemaPropertyNames(e.Schema)) > 0
}

// propertyGaps reports leaf parameters lacking both a schema description and
// authored help text.
func propertyGaps(e Entry) []Gap {
	if len(e.Schema) == 0 {
		return nil
	}
	var s struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(e.Schema, &s); err != nil {
		return nil
	}
	var docParams map[string]DocParam
	if e.Doc != nil {
		docParams = e.Doc.Parameters
	}
	var gaps []Gap
	walkProps(e, "", s.Properties, docParams, &gaps, 0)
	return gaps
}

func walkProps(e Entry, prefix string, props map[string]json.RawMessage, docs map[string]DocParam, gaps *[]Gap, depth int) {
	if depth > 5 {
		return
	}
	for name, raw := range props {
		var p struct {
			Type        string                     `json:"type"`
			Title       string                     `json:"title"`
			Description string                     `json:"description"`
			Properties  map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			continue
		}
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		if p.Type == "object" && len(p.Properties) > 0 {
			walkProps(e, path, p.Properties, docs, gaps, depth+1)
			continue
		}
		if p.Description == "" && !hasDocHelp(docs, path, name) {
			*gaps = append(*gaps, Gap{e.Kind, e.Source, e.ID, "property:" + path, "parameter has no description or help"})
		}
	}
}

func hasDocHelp(docs map[string]DocParam, path, name string) bool {
	for _, key := range []string{path, name} {
		if dp, ok := docs[key]; ok && (dp.Help != "" || dp.Description != "") {
			return true
		}
	}
	return false
}

// summarize tallies gaps by "<source>/<kind>/<field-class>" for the report header.
func summarize(gaps []Gap) map[string]int {
	out := map[string]int{}
	for _, g := range gaps {
		field := g.Field
		if len(field) > 8 && field[:8] == "property" {
			field = "property"
		}
		out[g.Source+"/"+g.Kind+"/"+field]++
		out["total"]++
	}
	return out
}
