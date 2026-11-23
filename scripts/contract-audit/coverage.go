package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/format/spec"
)

// What this audit can check, and what it did check.
//
// A reference "Class#method" is checked when its class ran, or when its own
// format's unit tests ran (then a missing class is drift). A simple class
// name must resolve to one class; a name two modules share is reported as
// ambiguous. Anything else is not checked.
//
// The run fails when it checked nothing, when an expected module wrote no
// reports, or when a filter or format checks fewer references than the
// committed baseline.

const (
	refChecked   = "checked"
	refUnchecked = "unchecked"
	refAmbiguous = "ambiguous"
)

// classIndex is every Okapi test class that ran, by fully qualified name and
// by simple name.
type classIndex struct {
	fqn   map[string]bool
	short map[string]map[string]bool
	tests map[string]bool // fqn#method
}

func indexClasses(results map[string]*filterResult) classIndex {
	ix := classIndex{fqn: map[string]bool{}, short: map[string]map[string]bool{}, tests: map[string]bool{}}
	for _, fr := range results {
		if fr == nil {
			continue
		}
		for _, suite := range fr.Suites {
			for _, tc := range suite.Tests {
				ix.fqn[tc.ClassName] = true
				s := shortClass(tc.ClassName)
				if ix.short[s] == nil {
					ix.short[s] = map[string]bool{}
				}
				ix.short[s][tc.ClassName] = true
				ix.tests[tc.ClassName+"#"+tc.Name] = true
			}
		}
	}
	return ix
}

// resolve says whether a reference can be checked and, when it can, whether
// the test it names exists.
func (ix classIndex) resolve(ref string, unitTestsRan bool) (state string, exists bool) {
	class, method, _ := strings.Cut(ref, "#")
	if strings.Contains(class, ".") {
		if !ix.fqn[class] && !unitTestsRan {
			return refUnchecked, false
		}
		return refChecked, ix.tests[class+"#"+method]
	}
	switch fqns := ix.short[class]; len(fqns) {
	case 0:
		if !unitTestsRan {
			return refUnchecked, false
		}
		return refChecked, false
	case 1:
		for fqn := range fqns {
			return refChecked, ix.tests[fqn+"#"+method]
		}
	}
	return refAmbiguous, false
}

// auditCoverage records, per filter for spec refs and per format for
// annotations, how many references were checked and how many were not.
type auditCoverage struct {
	SpecRefs              map[string]int `json:"specRefs"`
	Annotations           map[string]int `json:"annotations"`
	SpecRefsNotChecked    map[string]int `json:"specRefsNotChecked,omitempty"`
	AnnotationsNotChecked map[string]int `json:"annotationsNotChecked,omitempty"`
	Ambiguous             []string       `json:"ambiguous,omitempty"`
	ModulesExpected       []string       `json:"modulesExpected,omitempty"`
	ModulesWithReports    []string       `json:"modulesWithReports,omitempty"`
}

func newCoverage() auditCoverage {
	return auditCoverage{SpecRefs: map[string]int{}, Annotations: map[string]int{},
		SpecRefsNotChecked: map[string]int{}, AnnotationsNotChecked: map[string]int{}}
}

func (c auditCoverage) total() int {
	n := 0
	for _, v := range c.SpecRefs {
		n += v
	}
	for _, v := range c.Annotations {
		n += v
	}
	return n
}

// checkSpecRefs returns the drift in one spec's okapi_refs and records what
// was checked.
func checkSpecRefs(filter string, s *spec.Spec, ix classIndex, unitTestsRan bool, cov *auditCoverage) (drift []specDriftEntry, checked, unchecked int) {
	if s == nil {
		return nil, 0, 0
	}
	for _, f := range s.Features {
		for _, ref := range f.OkapiRefs {
			state, exists := ix.resolve(ref, unitTestsRan)
			switch state {
			case refUnchecked:
				unchecked++
				continue
			case refAmbiguous:
				cov.Ambiguous = append(cov.Ambiguous, fmt.Sprintf("%s spec feature %s: %s", filter, f.ID, ref))
				continue
			}
			checked++
			if !exists {
				drift = append(drift, specDriftEntry{FeatureID: f.ID, OkapiRef: ref, Reason: "missing-from-okapi"})
			}
		}
	}
	if checked > 0 {
		cov.SpecRefs[filter] = checked
	}
	if unchecked > 0 {
		cov.SpecRefsNotChecked[filter] = unchecked
	}
	return drift, checked, unchecked
}

// checkAnnotations returns the annotations naming an Okapi test that does not
// exist, and records what was checked, by format. An annotation outside
// core/formats counts under "(other)" and is always checked.
func checkAnnotations(anns []annotation, unitFilters map[string]bool, ix classIndex, cov *auditCoverage) []annotation {
	var drift []annotation
	for _, a := range anns {
		format := annotationFormat(a)
		key := format
		ran := unitFilters[format]
		if format == "" {
			key, ran = "(other)", true
		}
		state, exists := ix.resolve(a.JavaClass+"#"+a.JavaMethod, ran)
		switch state {
		case refUnchecked:
			cov.AnnotationsNotChecked[key]++
			continue
		case refAmbiguous:
			cov.Ambiguous = append(cov.Ambiguous, fmt.Sprintf("%s:%d %s#%s", a.File, a.Line, a.JavaClass, a.JavaMethod))
			continue
		}
		cov.Annotations[key]++
		if !exists {
			drift = append(drift, a)
		}
	}
	sort.SliceStable(drift, func(i, j int) bool {
		if drift[i].File != drift[j].File {
			return drift[i].File < drift[j].File
		}
		return drift[i].Line < drift[j].Line
	})
	sort.Strings(cov.Ambiguous)
	return drift
}

// moduleOf names the Okapi filter module a report sits in
// (.../okapi/filters/<module>/target/...), or "".
func moduleOf(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "filters" && i+2 < len(parts) && parts[i+2] == "target" {
			return parts[i+1]
		}
	}
	return ""
}

// modulesWithReports lists the filter modules that wrote at least one
// Surefire report under dir.
func modulesWithReports(dir string) ([]string, error) {
	seen := map[string]bool{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasPrefix(d.Name(), "TEST-") && strings.HasSuffix(d.Name(), ".xml") {
			if m := moduleOf(path); m != "" {
				seen[m] = true
			}
		}
		return nil
	})
	return sortedKeys(seen), err
}

// expectedModules resolves -expect-modules: a comma-separated list, or "all",
// meaning every module under filtersDir that has tests (src/test/java). The
// expectation comes from what was asked for, never from what reported.
func expectedModules(asked, filtersDir string) ([]string, error) {
	if asked != "all" {
		var out []string
		for m := range strings.SplitSeq(asked, ",") {
			if m = strings.TrimSpace(m); m != "" {
				out = append(out, m)
			}
		}
		sort.Strings(out)
		return out, nil
	}
	entries, err := os.ReadDir(filtersDir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if st, err := os.Stat(filepath.Join(filtersDir, e.Name(), "src", "test", "java")); err == nil && st.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// missingModules lists the expected modules with no reports.
func missingModules(expected, withReports []string) []string {
	have := map[string]bool{}
	for _, m := range withReports {
		have[m] = true
	}
	var out []string
	for _, m := range expected {
		if !have[m] {
			out = append(out, m)
		}
	}
	return out
}

// baseline is the committed record of how many references each filter and
// format checked in the last accepted run.
type baseline struct {
	SpecRefs    map[string]int `json:"specRefs"`
	Annotations map[string]int `json:"annotations"`
}

func readBaseline(path string) (baseline, error) {
	var b baseline
	data, err := os.ReadFile(path)
	if err != nil {
		return b, err
	}
	err = json.Unmarshal(data, &b)
	return b, err
}

func writeBaseline(path string, c auditCoverage) error {
	body, err := json.MarshalIndent(baseline{SpecRefs: c.SpecRefs, Annotations: c.Annotations}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o644)
}

// compareBaseline returns the drops (a filter or format checking fewer
// references than the baseline, or none) and the rises.
func compareBaseline(b baseline, c auditCoverage) (drops, rises []string) {
	cmp := func(kind string, was, now map[string]int) {
		keys := map[string]bool{}
		for k := range was {
			keys[k] = true
		}
		for k := range now {
			keys[k] = true
		}
		for _, k := range sortedKeys(keys) {
			switch {
			case now[k] < was[k]:
				drops = append(drops, fmt.Sprintf("%s %s: %d checked, baseline %d", kind, k, now[k], was[k]))
			case now[k] > was[k]:
				rises = append(rises, fmt.Sprintf("%s %s: %d checked, baseline %d", kind, k, now[k], was[k]))
			}
		}
	}
	cmp("spec refs", b.SpecRefs, c.SpecRefs)
	cmp("annotations", b.Annotations, c.Annotations)
	return drops, rises
}

// coverageFailures returns the reasons the run must fail on coverage alone.
func coverageFailures(c auditCoverage, missing, drops []string) []string {
	var out []string
	if c.total() == 0 {
		out = append(out, "nothing was checked: no reference resolved against the Okapi tests that ran")
	}
	if len(missing) > 0 {
		out = append(out, fmt.Sprintf("%d expected Okapi module(s) produced no test reports: %s", len(missing), strings.Join(missing, ", ")))
	}
	for _, d := range drops {
		out = append(out, "fewer references checked than the baseline: "+d)
	}
	return out
}

func reportCoverage(w io.Writer, c auditCoverage, rises []string) {
	if len(c.Ambiguous) > 0 {
		fmt.Fprintf(w, "contract-audit: AMBIGUOUS: %d reference(s) name a simple class that more than one module has; give the full class name:\n", len(c.Ambiguous))
		for _, a := range c.Ambiguous {
			fmt.Fprintf(w, "  %s\n", a)
		}
	}
	for _, r := range rises {
		fmt.Fprintf(w, "contract-audit: more checked than the baseline (raise it with -write-baseline): %s\n", r)
	}
}
