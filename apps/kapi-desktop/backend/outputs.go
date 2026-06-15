package backend

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/project"
)

// OutputFileInfo describes a single generated output file for a source file in
// a content collection. Outputs are derived from each collection item's
// `target` template (with {lang} and `*` resolved), then stat-ed on disk so
// the UI can show which target files exist and let the user inspect them.
type OutputFileInfo struct {
	Lang     string `json:"lang"`
	Path     string `json:"path"`     // absolute path on disk
	Relative string `json:"relative"` // path relative to the project root
	Format   string `json:"format,omitempty"`
	Exists   bool   `json:"exists"`
	Size     int64  `json:"size"`
	ModTime  string `json:"mod_time,omitempty"` // RFC3339, empty when the file is absent
}

// ListOutputs returns, for each source file matched by a content collection
// item that declares a `target` template, the set of output files that flow
// (one per target language). The result is keyed by the source file's path
// relative to the project root, so the frontend can render outputs as child
// rows beneath the source that produces them (issue #5).
//
// Output paths are computed from the collection's target template — the same
// resolution RunFlow uses — and stat-ed, so a never-run project reports the
// expected outputs as not-yet-existing rather than hiding them under "Other".
func (a *App) ListOutputs(tabID string) (map[string][]OutputFileInfo, error) {
	op := a.getOpenProject(tabID)
	if op == nil || op.Path == "" {
		return map[string][]OutputFileInfo{}, nil
	}
	basePath := filepath.Dir(op.Path)
	ctx := project.NewProjectContext(op.Project, op.Path)
	defaults := op.Project.Defaults

	out := make(map[string][]OutputFileInfo)
	for ci := range op.Project.Content {
		coll := &op.Project.Content[ci]
		for _, item := range coll.EffectiveItems() {
			if item.Target == "" || item.Path == "" {
				continue
			}
			langs := item.ResolvedTargetLanguages(coll, defaults)
			if len(langs) == 0 {
				continue
			}
			matches, err := filepath.Glob(filepath.Join(basePath, filepath.FromSlash(item.Path)))
			if err != nil {
				continue
			}
			ofiFor := func(lang, outPath string) OutputFileInfo {
				ofi := OutputFileInfo{
					Lang:     lang,
					Path:     outPath,
					Relative: filepath.ToSlash(relOrSelf(basePath, outPath)),
					Format:   ctx.DetectFormat(a.formatReg, outPath),
				}
				if st, err := os.Stat(outPath); err == nil && !st.IsDir() {
					ofi.Exists = true
					ofi.Size = st.Size()
					ofi.ModTime = st.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00")
				}
				return ofi
			}
			for _, src := range matches {
				info, statErr := os.Stat(src)
				if statErr != nil || info.IsDir() {
					continue
				}
				rel, err := filepath.Rel(basePath, src)
				if err != nil {
					continue
				}
				rel = filepath.ToSlash(rel)

				seen := make(map[string]bool)
				// Declared target languages — shown even when not yet generated,
				// so the source advertises the outputs a run will produce.
				for _, lang := range langs {
					ls := string(lang)
					out[rel] = append(out[rel], ofiFor(ls, outputPathFor(basePath, rel, item.Target, ls)))
					seen[ls] = true
				}
				// Discover already-generated outputs for any *other* language too
				// (e.g. a pseudo-translate run wrote output/qps/* though qps isn't a
				// declared target), so they appear under their source instead of
				// being dumped into "Other files".
				for _, lang := range discoverOutputLangs(basePath, rel, item.Target) {
					if seen[lang] {
						continue
					}
					out[rel] = append(out[rel], ofiFor(lang, outputPathFor(basePath, rel, item.Target, lang)))
					seen[lang] = true
				}
			}
		}
	}
	return out, nil
}

// langPlaceholder is an unlikely sentinel that stands in for {lang} while we
// build a glob and a capture regex from a target template.
const langPlaceholder = "\x00LANG\x00"

// discoverOutputLangs finds the languages for which an output file already
// exists on disk for sourceRel under the given target template, by globbing the
// template with {lang} treated as a wildcard and capturing the matched segment.
// Returns a sorted, de-duplicated list of language codes (e.g. ["qps"]). Targets
// without a {lang} placeholder yield nothing — those are handled by the caller's
// declared-language pass.
func discoverOutputLangs(basePath, sourceRel, target string) []string {
	if !strings.Contains(target, "{lang}") {
		return nil
	}
	// Fill the filename wildcard with the concrete source name; keep {lang}.
	tmpl := strings.ReplaceAll(target, "{lang}", langPlaceholder)
	tmpl = strings.ReplaceAll(tmpl, "*", filepath.Base(sourceRel))

	globPat := filepath.Join(basePath, filepath.FromSlash(strings.ReplaceAll(tmpl, langPlaceholder, "*")))
	hits, err := filepath.Glob(globPat)
	if err != nil || len(hits) == 0 {
		return nil
	}
	re, err := regexp.Compile("^" + strings.ReplaceAll(regexp.QuoteMeta(filepath.ToSlash(tmpl)), langPlaceholder, "([^/]+)") + "$")
	if err != nil {
		return nil
	}
	seen := make(map[string]bool)
	var langs []string
	for _, h := range hits {
		st, err := os.Stat(h)
		if err != nil || st.IsDir() {
			continue
		}
		rel, err := filepath.Rel(basePath, h)
		if err != nil {
			continue
		}
		m := re.FindStringSubmatch(filepath.ToSlash(rel))
		if m == nil || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		langs = append(langs, m[1])
	}
	sort.Strings(langs)
	return langs
}

// relOrSelf returns path relative to base, or the original path if it can't be
// made relative (e.g. a target template that escapes the project root).
func relOrSelf(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return rel
}

// outputPathFor resolves a content item's target template into a concrete
// output path for one source file and target language. {lang} is substituted
// with the language and a `*` wildcard is replaced with the source file's base
// name — mirroring resolveOutputPath used by the flow runner.
func outputPathFor(basePath, sourceRel, target, lang string) string {
	resolved := strings.ReplaceAll(target, "{lang}", lang)
	if strings.Contains(resolved, "*") {
		resolved = strings.ReplaceAll(resolved, "*", filepath.Base(sourceRel))
	}
	return filepath.Join(basePath, filepath.FromSlash(resolved))
}

// InspectOutput returns the absolute path of an output file for a tab, after
// confirming it lives inside the project root. The frontend uses this to open
// an output file for inspection without trusting a client-supplied path.
func (a *App) InspectOutput(tabID, relative string) (string, error) {
	op := a.getOpenProject(tabID)
	if op == nil || op.Path == "" {
		return "", fmt.Errorf("tab %q not found", tabID)
	}
	if strings.Contains(relative, "..") {
		return "", errors.New("path must not contain '..'")
	}
	basePath := filepath.Dir(op.Path)
	abs := filepath.Join(basePath, filepath.FromSlash(relative))
	if !strings.HasPrefix(abs, basePath) {
		return "", errors.New("path escapes project root")
	}
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("output not found: %w", err)
	}
	return abs, nil
}
