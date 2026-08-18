package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

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

// loadNativeDocs reads every sidecar under dir/<kind>s/ keyed by entry id, and
// refuses one whose name matches no entry in known. A missing directory yields
// an empty map (no native docs authored yet).
//
// The file name is the binding: applyNativeDoc overlays a sidecar onto the entry
// carrying that id, so a name the registry does not carry documents nothing.
// Dropped in silence, it leaves the entry it was written for shipping the
// registry's bare metadata while its prose sits unread in the tree, and the gap
// report — which measures the entry, not the file — says the entry has no
// overview without saying why. Two tool dossiers outlived a tool rename that
// way, so a sidecar that documents nothing is an error rather than a no-op.
func loadNativeDocs(dir, kind string, known []Entry) (map[string]*nativeDocFile, error) {
	subdir := filepath.Join(dir, kind+"s")
	files, err := filepath.Glob(filepath.Join(subdir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	ids := make(map[string]bool, len(known))
	for _, e := range known {
		ids[e.ID] = true
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
		if !ids[id] {
			return nil, fmt.Errorf("%s documents nothing: no built-in %s has the id %q the file name binds it to", f, kind, id)
		}
		out[id] = &ndf
	}
	return out, nil
}

// verifyCheckDocs holds the dossiers under dir/checks/ to the source-side
// checkers `kapi check` runs: every checker has one, and every one names a
// checker. ids is core/check.SourceCheckIDs.
//
// These are the sidecars with nothing to overlay onto. A checker off the
// registry has no dataset entry, so the file-name binding [loadNativeDocs]
// enforces has nothing to bind to — and the documentation is still owed, because
// a finding a user reads carries the checker's id and the behaviour behind it is
// live. Naming the set in code rather than exempting the files makes the binding
// a statement in both directions: retire a checker and its dossier fails the
// build, add one and the build asks for its dossier.
//
// Each file is parsed, so a dossier that stopped being YAML fails here rather
// than at the venue that reads it.
func verifyCheckDocs(dir string, ids []string) error {
	subdir := filepath.Join(dir, KindCheck+"s")
	files, err := filepath.Glob(filepath.Join(subdir, "*.yaml"))
	if err != nil {
		return err
	}
	documented := make(map[string]bool, len(files))
	for _, f := range files {
		data, rerr := os.ReadFile(f)
		if rerr != nil {
			return rerr
		}
		var ndf nativeDocFile
		if uerr := yaml.Unmarshal(data, &ndf); uerr != nil {
			return fmt.Errorf("parse %s: %w", f, uerr)
		}
		id := trimSuffix(filepath.Base(f), ".yaml")
		if !slices.Contains(ids, id) {
			return fmt.Errorf("%s documents nothing: no source-side check has the id %q the file name binds it to", f, id)
		}
		documented[id] = true
	}
	for _, id := range ids {
		if !documented[id] {
			return fmt.Errorf("the %s check has no dossier: write %s",
				id, filepath.Join(subdir, id+".yaml"))
		}
	}
	return nil
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
				dp.DependsOn = append(dp.DependsOn, DocDepends(d))
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
