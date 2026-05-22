package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// nativeDocFile is the authored YAML sidecar for a native format or tool,
// living under nativedocs/{formats,tools}/<id>.yaml. It mirrors the bridge
// doc.json so native entries reach the same documentation richness.
type nativeDocFile struct {
	DisplayName     string                    `yaml:"displayName"`
	Description     string                    `yaml:"description"`
	Overview        string                    `yaml:"overview"`
	Parameters      map[string]nativeDocParam `yaml:"parameters"`
	Limitations     []string                  `yaml:"limitations"`
	ProcessingNotes []string                  `yaml:"processingNotes"`
	Examples        []nativeDocExample        `yaml:"examples"`
	WikiURL         string                    `yaml:"wikiUrl"`
}

type nativeDocParam struct {
	Description  string             `yaml:"description"`
	Help         string             `yaml:"help"`
	Values       string             `yaml:"values"`
	Notes        []string           `yaml:"notes"`
	Examples     []string           `yaml:"examples"`
	DependsOn    []nativeDocDepends `yaml:"dependsOn"`
	IntroducedIn string             `yaml:"introducedIn"`
	SeeAlso      string             `yaml:"seeAlso"`
}

type nativeDocDepends struct {
	Property  string `yaml:"property"`
	Condition string `yaml:"condition"`
}

type nativeDocExample struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Config      string `yaml:"config"`
}

// loadNativeDocs reads every sidecar under dir/<kind>s/ keyed by entry id.
// A missing directory yields an empty map (no native docs authored yet).
func loadNativeDocs(dir, kind string) (map[string]*nativeDocFile, error) {
	subdir := filepath.Join(dir, kind+"s")
	files, err := filepath.Glob(filepath.Join(subdir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	out := make(map[string]*nativeDocFile, len(files))
	for _, f := range files {
		data, rerr := os.ReadFile(f)
		if rerr != nil {
			return nil, rerr
		}
		var ndf nativeDocFile
		if uerr := yaml.Unmarshal(data, &ndf); uerr != nil {
			return nil, fmt.Errorf("parse %s: %w", f, uerr)
		}
		id := trimSuffix(filepath.Base(f), ".yaml")
		out[id] = &ndf
	}
	return out, nil
}

// applyNativeDoc overlays an authored sidecar onto a native entry.
func applyNativeDoc(e *Entry, ndf *nativeDocFile) {
	if ndf == nil {
		return
	}
	if ndf.DisplayName != "" {
		e.DisplayName = ndf.DisplayName
	}
	if ndf.Description != "" {
		e.Description = ndf.Description
	}

	doc := &Doc{
		Overview:        ndf.Overview,
		Limitations:     ndf.Limitations,
		ProcessingNotes: ndf.ProcessingNotes,
		WikiURL:         ndf.WikiURL,
	}
	for _, ex := range ndf.Examples {
		doc.Examples = append(doc.Examples, DocExample{
			Title:       ex.Title,
			Description: ex.Description,
			Config:      ex.Config,
		})
	}
	if len(ndf.Parameters) > 0 {
		doc.Parameters = make(map[string]DocParam, len(ndf.Parameters))
		for name, p := range ndf.Parameters {
			dp := DocParam{
				Description:  p.Description,
				Help:         p.Help,
				Values:       p.Values,
				Notes:        p.Notes,
				Examples:     p.Examples,
				IntroducedIn: p.IntroducedIn,
				SeeAlso:      p.SeeAlso,
			}
			for _, d := range p.DependsOn {
				dp.DependsOn = append(dp.DependsOn, DocDepends{Property: d.Property, Condition: d.Condition})
			}
			doc.Parameters[name] = dp
		}
	}

	if !doc.empty() {
		e.Doc = doc
	}
}

// empty reports whether the doc carries no content.
func (d *Doc) empty() bool {
	return d == nil || (d.Overview == "" && len(d.Parameters) == 0 &&
		len(d.Limitations) == 0 && len(d.ProcessingNotes) == 0 &&
		len(d.Examples) == 0 && d.WikiURL == "")
}

func trimSuffix(s, suffix string) string {
	if len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix {
		return s[:len(s)-len(suffix)]
	}
	return s
}
