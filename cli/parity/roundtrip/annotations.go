//go:build parity

package roundtrip

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"gopkg.in/yaml.v3"
)

// Annotation is per-fixture metadata loaded from
// core/formats/<format>/parity-annotations.yaml. It enriches the
// /parity/fixtures dashboard with severity, issue link, and spec
// citation, and replaces the legacy fileSkip map by carrying optional
// per-fixture engine skip directives.
//
// The annotation system is read-only metadata — it does NOT change tier
// comparison results. Severity classifies why a divergence is
// acceptable (or not); a "bug" severity means the divergence must be
// fixed, while "cosmetic" or "native-more-correct" mean the divergence
// is expected and documented.
type Annotation struct {
	// Severity is one of: bug, cosmetic, native-more-correct,
	// fixture-bug, unknown. Empty when no annotation file declared one.
	Severity string `yaml:"severity"`

	// Issue is an optional neokapi GitHub issue number. The dashboard
	// derives the link via the repo URL template.
	Issue int `yaml:"issue,omitempty"`

	// Summary is a one-line description of the divergence, shown in the
	// dashboard. Must explain WHY this fixture diverges and what would
	// change to clear it.
	Summary string `yaml:"summary,omitempty"`

	// SpecRef cites the relevant format spec section (e.g.
	// "ECMA-376-1 §17.3.2.35"). Optional but strongly preferred for
	// cosmetic / native-more-correct severity so the dashboard can show
	// the spec ground truth.
	SpecRef string `yaml:"spec_ref,omitempty"`

	// NotesAnchor is an optional Markdown anchor inside the format's
	// PARITY_NOTES.md (e.g. "deltextamp-investigation-notes-2026-05-16").
	// The dashboard can deep-link to it.
	NotesAnchor string `yaml:"notes_anchor,omitempty"`

	// Skip, when present, declares engines that should skip this
	// fixture. Replaces the legacy fileSkip map. Empty Engines means no
	// skip is requested.
	Skip *SkipDirective `yaml:"skip,omitempty"`
}

// SkipDirective is the per-fixture skip request: which engines to skip
// and why. Engines: "okapi" skips the whole fixture (no useful
// reference); any other name (e.g. "native") skips just that engine.
type SkipDirective struct {
	Engines []string `yaml:"engines"`
	Reason  string   `yaml:"reason"`
}

// annotationFile is the on-disk shape of one
// core/formats/<format>/parity-annotations.yaml file.
type annotationFile struct {
	Format   string                `yaml:"format"`
	Fixtures map[string]Annotation `yaml:"fixtures"`
}

var (
	annotationsMu     sync.Mutex
	annotationsByFmt  map[string]map[string]Annotation
	annotationsLoaded bool
	annotationsErr    error
)

// LookupAnnotation returns the annotation for (format, fixture) and
// whether one exists. Triggers a one-time load on first call. Returns
// the zero Annotation + ok=false when none is declared.
func LookupAnnotation(format, fixture string) (Annotation, bool) {
	if err := loadAnnotations(); err != nil {
		return Annotation{}, false
	}
	if perFmt, ok := annotationsByFmt[format]; ok {
		ann, ok := perFmt[fixture]
		return ann, ok
	}
	return Annotation{}, false
}

// LookupSkip returns the skip directive for (format, fixture) and
// whether one was declared. Convenience wrapper around
// LookupAnnotation that filters to entries carrying a Skip.
func LookupSkip(format, fixture string) (SkipDirective, bool) {
	ann, ok := LookupAnnotation(format, fixture)
	if !ok || ann.Skip == nil {
		return SkipDirective{}, false
	}
	return *ann.Skip, true
}

// AllAnnotations returns a copy of the full (format → fixture →
// annotation) map. Used by the CI gate test and dashboard JSON writer.
func AllAnnotations() (map[string]map[string]Annotation, error) {
	if err := loadAnnotations(); err != nil {
		return nil, err
	}
	out := make(map[string]map[string]Annotation, len(annotationsByFmt))
	for fmt, perFmt := range annotationsByFmt {
		copyFmt := make(map[string]Annotation, len(perFmt))
		maps.Copy(copyFmt, perFmt)
		out[fmt] = copyFmt
	}
	return out, nil
}

// ResetAnnotations clears the loader cache. Tests use this to reload
// after mutating annotation files.
func ResetAnnotations() {
	annotationsMu.Lock()
	defer annotationsMu.Unlock()
	annotationsByFmt = nil
	annotationsLoaded = false
	annotationsErr = nil
}

func loadAnnotations() error {
	annotationsMu.Lock()
	defer annotationsMu.Unlock()
	if annotationsLoaded {
		return annotationsErr
	}
	annotationsLoaded = true
	root, err := findRepoRoot()
	if err != nil {
		annotationsErr = err
		return err
	}
	formatsDir := filepath.Join(root, "core", "formats")
	entries, err := os.ReadDir(formatsDir)
	if err != nil {
		annotationsErr = fmt.Errorf("read formats dir %s: %w", formatsDir, err)
		return annotationsErr
	}
	out := map[string]map[string]Annotation{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(formatsDir, e.Name(), "parity-annotations.yaml")
		body, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			annotationsErr = fmt.Errorf("read %s: %w", path, err)
			return annotationsErr
		}
		var af annotationFile
		if err := yaml.Unmarshal(body, &af); err != nil {
			annotationsErr = fmt.Errorf("parse %s: %w", path, err)
			return annotationsErr
		}
		// Trust the dirname as the format key — but warn if the YAML
		// disagrees so a copy-paste typo surfaces immediately.
		key := e.Name()
		if af.Format != "" && af.Format != key {
			annotationsErr = fmt.Errorf("%s: format field %q doesn't match dir name %q", path, af.Format, key)
			return annotationsErr
		}
		out[key] = af.Fixtures
	}
	annotationsByFmt = out
	return nil
}

// findRepoRoot returns the absolute path to the neokapi repo root by
// walking up from this source file's directory looking for go.work.
// Using runtime.Caller anchors the lookup to the source tree regardless
// of the current working directory at test time.
func findRepoRoot() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("runtime.Caller failed")
	}
	dir := filepath.Dir(thisFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no go.work in any ancestor of " + thisFile)
		}
		dir = parent
	}
}
