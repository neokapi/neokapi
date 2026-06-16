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
// a content collection. Output paths are resolved from each collection item's
// `target` template via the shared core resolver (project.ResolveTargetPath),
// then stat-ed on disk so the UI can show which target files exist and let the
// user inspect them.
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
// rows beneath the source that produces them.
//
// Output paths come from the shared resolver (project.ResolveTargetPath) — the
// same one the flow runner, merge, and the CLI use — so the desktop, CLI, and
// `kapi merge` agree exactly, and source globs honour doublestar (`**`, `{a,b}`)
// via project.ExpandGlob.
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
			matches, err := project.ExpandGlob(basePath, item.Path, defaults.Exclude...)
			if err != nil {
				continue
			}
			ofiFor := func(lang, sourceRel string) OutputFileInfo {
				outRel := project.ResolveTargetPath(item.Path, item.Base, item.Target, sourceRel, lang)
				outPath := filepath.Join(basePath, outRel)
				ofi := OutputFileInfo{
					Lang:     lang,
					Path:     outPath,
					Relative: filepath.ToSlash(outRel),
					Format:   ctx.DetectFormat(a.formatReg, outPath),
				}
				if st, err := os.Stat(outPath); err == nil && !st.IsDir() {
					ofi.Exists = true
					ofi.Size = st.Size()
					ofi.ModTime = st.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00")
				}
				return ofi
			}
			for _, m := range matches {
				rel := filepath.ToSlash(m)
				if info, err := os.Stat(filepath.Join(basePath, m)); err != nil || info.IsDir() {
					continue
				}

				seen := make(map[string]bool)
				// Declared target languages — shown even when not yet generated,
				// so the source advertises the outputs a run will produce.
				for _, lang := range langs {
					ls := string(lang)
					out[rel] = append(out[rel], ofiFor(ls, rel))
					seen[ls] = true
				}
				// Discover already-generated outputs for any *other* language too
				// (e.g. a pseudo-translate run wrote output/qps/* though qps isn't a
				// declared target), so they appear under their source instead of
				// being dumped into "Other files".
				for _, lang := range discoverOutputLangs(basePath, item, rel) {
					if seen[lang] {
						continue
					}
					out[rel] = append(out[rel], ofiFor(lang, rel))
					seen[lang] = true
				}
			}
		}
	}
	return out, nil
}

// langPlaceholder is an unlikely sentinel that stands in for {lang} while we
// build a glob and a capture regex from a fully-resolved target path.
const langPlaceholder = "\x00LANG\x00"

// discoverOutputLangs finds the languages for which an output file already
// exists on disk for sourceRel under item's target, for any language (declared
// or not). It resolves the target with the language set to a sentinel — so the
// sentinel is the only variable in an otherwise-concrete path regardless of the
// target form (directory-mirror, tokens, or `*`) — then globs and captures the
// matched segment. Targets without {lang} yield nothing.
func discoverOutputLangs(basePath string, item project.ContentItem, sourceRel string) []string {
	if !strings.Contains(item.Target, "{lang}") {
		return nil
	}
	resolved := filepath.ToSlash(project.ResolveTargetPath(item.Path, item.Base, item.Target, sourceRel, langPlaceholder))

	globPat := filepath.Join(basePath, filepath.FromSlash(strings.ReplaceAll(resolved, langPlaceholder, "*")))
	hits, err := filepath.Glob(globPat)
	if err != nil || len(hits) == 0 {
		return nil
	}
	re, err := regexp.Compile("^" + strings.ReplaceAll(regexp.QuoteMeta(resolved), langPlaceholder, "([^/]+)") + "$")
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
