package main

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/neokapi/neokapi/core/check"
)

// nativeDocFile is the authored YAML sidecar for a native format, tool or
// check, living under nativedocs/{formats,tools,checks}/<id>.yaml. It mirrors
// the bridge doc.json so native entries reach the same documentation richness.
type nativeDocFile struct {
	DisplayName     string                    `yaml:"displayName"`
	Description     string                    `yaml:"description"`
	Overview        string                    `yaml:"overview"`
	Parameters      map[string]nativeDocParam `yaml:"parameters"`
	Limitations     []string                  `yaml:"limitations"`
	ProcessingNotes []string                  `yaml:"processingNotes"`
	Examples        []nativeDocExample        `yaml:"examples"`
	WikiURL         string                    `yaml:"wikiUrl"`
	// Rules documents each rule id a check reports. Checks only: the ids are
	// held to the ones the code reports (collectChecks).
	Rules []nativeDocRule `yaml:"rules"`
}

// nativeDocRule is one rule id of a check: what the finding reports, at which
// severity, and what fixes it.
type nativeDocRule struct {
	ID       string `yaml:"id"`
	Severity string `yaml:"severity"`
	Reports  string `yaml:"reports"`
	Fix      string `yaml:"fix"`
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

// collectChecks builds the check entries: one per source-side checker `kapi
// check` runs, with the id, the rule family and the rule ids the code reports
// (core/check.SourceChecks), and the prose its dossier under dir/checks/
// authors. The dossier is the only source of a check's name and description,
// so every check needs one, and the binding holds in both directions
// (verifyCheckDocs). The rule ids a dossier documents are held to the ones the
// code reports as well: a dossier that documents a rule the checker never
// reports, or leaves one out, fails the build.
func collectChecks(dir string, checks []check.SourceCheck) ([]Entry, error) {
	ids := make([]string, 0, len(checks))
	for _, c := range checks {
		ids = append(ids, c.ID)
	}
	if err := verifyCheckDocs(dir, ids); err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(checks))
	for _, c := range checks {
		e := Entry{ID: c.ID, Source: SourceBuiltIn, Kind: KindCheck, RuleFamily: c.Family}
		for _, rule := range c.RuleIDs() {
			e.Rules = append(e.Rules, CheckRule{ID: rule})
		}
		entries = append(entries, e)
	}
	docs, err := loadNativeDocs(dir, KindCheck, entries)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		ndf := docs[entries[i].ID]
		file := filepath.Join(dir, KindCheck+"s", entries[i].ID+".yaml")
		if ndf.DisplayName == "" || ndf.Description == "" {
			return nil, fmt.Errorf("%s: a check dossier names the check (displayName) and says what it does (description)", file)
		}
		if err := bindCheckRules(&entries[i], ndf, file); err != nil {
			return nil, err
		}
		applyNativeDoc(&entries[i], ndf)
	}
	return entries, nil
}

// bindCheckRules holds the rules a dossier documents to the rule ids the
// checker reports: each one documented, with what it reports and what fixes
// it, and none the checker never reports. applyNativeDoc carries the prose
// onto the entry's rules, which keep the code's order, so a page lists the
// rules as the checker reports them.
func bindCheckRules(e *Entry, ndf *nativeDocFile, file string) error {
	documented := make(map[string]nativeDocRule, len(ndf.Rules))
	for _, r := range ndf.Rules {
		if _, dup := documented[r.ID]; dup {
			return fmt.Errorf("%s documents the rule %q twice", file, r.ID)
		}
		documented[r.ID] = r
	}
	for i := range e.Rules {
		r, ok := documented[e.Rules[i].ID]
		if !ok {
			return fmt.Errorf("%s documents no rule %q, which the %s check reports: add it under rules", file, e.Rules[i].ID, e.ID)
		}
		if r.Reports == "" || r.Fix == "" {
			return fmt.Errorf("%s: the rule %q says what it reports (reports) and what fixes it (fix)", file, r.ID)
		}
		delete(documented, r.ID)
	}
	unknown := slices.Sorted(maps.Keys(documented))
	if len(unknown) > 0 {
		return fmt.Errorf("%s documents the rule %q, which the %s check never reports", file, unknown[0], e.ID)
	}
	return nil
}

// verifyCheckDocs holds the dossiers under dir/checks/ to the source-side
// checkers `kapi check` runs: every checker has one, and every one names a
// checker. ids is core/check.SourceCheckIDs.
//
// A checker off the registry has no entry of its own to bind to, so the set is
// named in code rather than read from a registry, and the binding is a
// statement in both directions: retire a checker and its dossier fails the
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
	// A check's rules keep the code's list and order; the dossier supplies
	// the prose of each rule it names. A rule it names that the entry does not
	// carry is left out here, and bindCheckRules refuses it in English.
	for _, r := range ndf.Rules {
		i := slices.IndexFunc(e.Rules, func(cr CheckRule) bool { return cr.ID == r.ID })
		if i < 0 {
			continue
		}
		if r.Severity != "" {
			e.Rules[i].Severity = r.Severity
		}
		if r.Reports != "" {
			e.Rules[i].Reports = r.Reports
		}
		if r.Fix != "" {
			e.Rules[i].Fix = r.Fix
		}
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
