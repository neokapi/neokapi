package host

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/ignore"
	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
)

// Proposing a new project's collections from the files already in its tree.
//
// `kapi init` reads the repository it is run in and writes a collection for
// each kind of content it can read there, so a new project works before anyone
// edits YAML. The proposal is deterministic and asks nothing: the same tree
// gives the same collections, from a terminal, from CI or from an agent.
//
// Two sources decide what counts as content:
//
//   - The format registry. A file whose built-in format carries documents
//     (marked-up text, office documents, timed text) is content wherever it
//     sits. Documents are grouped by top-level directory and extension, so a
//     docs tree becomes `docs/**/*.md` rather than one entry per page, and a
//     file at the root keeps an entry of its own.
//   - The framework presets, for i18n catalogs. A catalog is a key/value file
//     in a format also used for configuration, so a catalog is proposed only
//     where a preset's layout matches it, or where its path names the source
//     language (`locales/en/*.json`, `messages/en.json`, `app_en.arb`). A file
//     named for any other language is a translation and is left out.
//
// Nothing else is proposed. Interchange files (XLIFF, PO exchanges), data
// files and plain text are read by kapi but are rarely a project's source
// content, and a recipe that names them is one edit away.

// ProposedCollection is one collection kapi init proposes.
type ProposedCollection struct {
	// Path is the project-relative glob the collection reads.
	Path string `json:"path"`
	// Format is the format id the collection names.
	Format string `json:"format"`
	// Target is where translations go, a template over {lang}. Empty when the
	// project declares no target language, and for documents, whose
	// destination is the project's own decision.
	Target string `json:"target,omitempty"`
	// Reason is the one-line comment written above the entry: what it matched
	// and why kapi reads it.
	Reason string `json:"reason"`
	// Files are the project-relative paths the glob matched, sorted.
	Files []string `json:"files"`

	// signal is what marked a catalog as source content, for its comment.
	signal catalogSignal
}

// catalogSignal is what marked a catalog-family file as source content.
type catalogSignal int

const (
	// signalFormat: the format is a catalog format by definition.
	signalFormat catalogSignal = iota
	// signalDir: a directory on its path is named for the source language.
	signalDir
	// signalStem: its own name ends in the source language.
	signalStem
)

// ProposeOptions configures ProposeCollections.
type ProposeOptions struct {
	// Formats is the registry that recognises files. Only built-in formats
	// are proposed, because a recipe that names a plugin's format stops
	// loading on a machine without that plugin.
	Formats *registry.FormatRegistry
	// SourceLocale is the project's source language; a catalog is proposed
	// where its path names it.
	SourceLocale string
	// Targets reports whether the project declares target languages. A
	// catalog then gets a target template beside its source.
	Targets bool
	// Framework names a preset whose mappings are proposed whether or not
	// their files exist yet (the `--framework` flag).
	Framework string
}

// initScanSkipDirs are directories never read for content, whatever the ignore
// files say: dependency trees, vendored code, build and test output. Hidden
// directories (`.git`, `.kapi`, `.claude`, `.github`, …) are skipped as a class.
var initScanSkipDirs = map[string]bool{
	"node_modules":     true,
	"bower_components": true,
	"jspm_packages":    true,
	"vendor":           true,
	"third_party":      true,
	"Pods":             true,
	"Carthage":         true,
	"dist":             true,
	"build":            true,
	"out":              true,
	"target":           true,
	"bin":              true,
	"obj":              true,
	"coverage":         true,
	"DerivedData":      true,
	"__pycache__":      true,
	"venv":             true,
	"site-packages":    true,
	"storybook-static": true,
	"_site":            true,
	"__generated__":    true,
	"testdata":         true,
	"fixtures":         true,
	"__fixtures__":     true,
	"__snapshots__":    true,
	"tmp":              true,
}

// documentFamilies are the content shapes a file carries when it is a
// document wherever it sits.
var documentFamilies = map[registry.FormatFamily]bool{
	registry.FamilyRichMarkup:        true,
	registry.FamilyOfficeDoc:         true,
	registry.FamilySubtitleTimedText: true,
}

// genericCarriers are the catalog formats also used for configuration and data.
// A file in one of them is a catalog only when a preset or its path says so;
// every other catalog format (PO, ARB, RESX, string tables) is one by
// definition.
var genericCarriers = map[string]bool{"json": true, "yaml": true, "properties": true}

// localeLike matches a path segment or file stem shaped like a language tag.
var localeLike = regexp.MustCompile(`^[a-z]{2,3}([-_][a-z0-9]{2,8})*$`)

// ProposeCollections scans root and returns the collections kapi init writes
// for it, in the order they are written: preset catalogs, then root files,
// then each directory's documents, then catalogs found by their path.
func ProposeCollections(root string, opts ProposeOptions) ([]ProposedCollection, error) {
	if opts.Formats == nil {
		return nil, errors.New("propose collections: no format registry")
	}
	files, err := scanContentTree(root)
	if err != nil {
		return nil, err
	}

	claimed := map[string]bool{}
	out, err := presetProposals(files, opts, claimed)
	if err != nil {
		return nil, err
	}

	type docGroup struct {
		dir, ext, format string
		files            []string
		nested           bool
	}
	groups := map[string]*docGroup{}
	var rootDocs []ProposedCollection
	var catalogs []ProposedCollection
	catalogIndex := map[string]int{}

	for _, rel := range files {
		if claimed[rel] {
			continue
		}
		id, family, ok := recognise(opts.Formats, root, rel)
		if !ok {
			continue
		}
		ext := format.Ext(rel)
		switch {
		case documentFamilies[family]:
			top, rest, nested := strings.Cut(rel, "/")
			if !nested {
				rootDocs = append(rootDocs, ProposedCollection{
					Path:   rel,
					Format: id,
					Reason: fmt.Sprintf("%s: %s, at the root", rel, displayName(opts.Formats, id)),
					Files:  []string{rel},
				})
				continue
			}
			key := top + "\x00" + ext + "\x00" + id
			g := groups[key]
			if g == nil {
				g = &docGroup{dir: top, ext: ext, format: id}
				groups[key] = g
			}
			g.files = append(g.files, rel)
			if strings.Contains(rest, "/") {
				g.nested = true
			}
		case family == registry.FamilyCatalogKeyValue:
			p, ok := catalogProposal(rel, id, ext, opts)
			if !ok {
				continue
			}
			if i, seen := catalogIndex[p.Path]; seen {
				catalogs[i].Files = append(catalogs[i].Files, rel)
				continue
			}
			catalogIndex[p.Path] = len(catalogs)
			catalogs = append(catalogs, p)
		}
	}

	out = append(out, rootDocs...)
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		g := groups[k]
		glob := g.dir + "/*" + g.ext
		if g.nested {
			glob = g.dir + "/**/*" + g.ext
		}
		out = append(out, ProposedCollection{
			Path:   glob,
			Format: g.format,
			Reason: fmt.Sprintf("%s/: %s", g.dir, plural(len(g.files), displayName(opts.Formats, g.format)+" file", displayName(opts.Formats, g.format)+" files")),
			Files:  g.files,
		})
	}
	for i := range catalogs {
		catalogs[i].Reason = catalogReason(catalogs[i], displayName(opts.Formats, catalogs[i].Format), opts.SourceLocale)
	}
	out = append(out, catalogs...)
	for i := range out {
		sort.Strings(out[i].Files)
	}
	return out, nil
}

// presetProposals proposes each framework preset mapping whose source files
// exist, plus every mapping of the preset --framework named, and marks the
// files they cover as claimed.
func presetProposals(files []string, opts ProposeOptions, claimed map[string]bool) ([]ProposedCollection, error) {
	reg := preset.NewPresetRegistry()
	preset.RegisterBuiltins(reg)
	if opts.Framework != "" && reg.GetFrameworkPreset(opts.Framework) == nil {
		var names []string
		for _, p := range reg.ListFrameworkPresets() {
			names = append(names, p.Name)
		}
		return nil, fmt.Errorf("unknown framework %q; available: %s", opts.Framework, strings.Join(names, ", "))
	}

	var out []ProposedCollection
	seen := map[string]bool{}
	for _, fp := range reg.ListFrameworkPresets() {
		forced := fp.Name == opts.Framework
		for _, m := range fp.Mappings {
			var matched []string
			for _, rel := range files {
				if project.MatchGlob(m.Local, rel) {
					matched = append(matched, rel)
				}
			}
			if (len(matched) == 0 && !forced) || seen[m.Local] {
				continue
			}
			seen[m.Local] = true
			for _, rel := range matched {
				claimed[rel] = true
			}
			p := ProposedCollection{
				Path:   m.Local,
				Format: m.Format,
				Files:  matched,
			}
			if opts.Targets || forced {
				p.Target = strings.ReplaceAll(m.TargetPath, "{locale}", "{lang}")
			}
			switch {
			case len(matched) == 0:
				p.Reason = fmt.Sprintf("%s: the %s catalog layout (--framework); no file there yet", m.Local, fp.Name)
			default:
				p.Reason = fmt.Sprintf("%s: %s in the %s catalog layout", m.Local, plural(len(matched), "file", "files"), fp.Name)
			}
			out = append(out, p)
		}
	}
	return out, nil
}

// catalogProposal places a catalog-family file: a glob over its directory
// when a directory names the source language, the file itself with a
// {lang} target when its own name does, and nothing when neither does and
// the format is one configuration also uses.
func catalogProposal(rel, id, ext string, opts ProposeOptions) (ProposedCollection, bool) {
	dir, base := path.Split(rel)
	dir = strings.TrimSuffix(dir, "/")
	stem := strings.TrimSuffix(base, ext)
	src := primaryLanguage(opts.SourceLocale)

	segments := strings.Split(dir, "/")
	for i := len(segments) - 1; i >= 0 && dir != ""; i-- {
		seg := segments[i]
		if !localeLike.MatchString(strings.ToLower(seg)) {
			continue
		}
		if primaryLanguage(seg) != src {
			return ProposedCollection{}, false // a translation, not the source
		}
		prefix := strings.Join(segments[:i+1], "/")
		p := ProposedCollection{Path: prefix + "/*" + ext, Format: id, Files: []string{rel}, signal: signalDir}
		if i < len(segments)-1 {
			p.Path = prefix + "/**/*" + ext
		}
		if opts.Targets {
			p.Target = strings.Join(append(append([]string{}, segments[:i]...), "{lang}"), "/") + "/{path}" + ext
		}
		return p, true
	}

	if lang, pre, ok := stemLanguage(stem); ok {
		if lang != src {
			return ProposedCollection{}, false
		}
		p := ProposedCollection{Path: rel, Format: id, Files: []string{rel}, signal: signalStem}
		if opts.Targets {
			p.Target = path.Join(dir, pre+"{lang}"+ext)
		}
		return p, true
	}

	if genericCarriers[id] {
		return ProposedCollection{}, false
	}
	return ProposedCollection{Path: rel, Format: id, Files: []string{rel}}, true
}

// stemLanguage reads a language from a file stem that is one (`en`) or ends
// in one after a separator (`app_en`, `messages.en`). pre is the stem up to
// the language, separator included.
func stemLanguage(stem string) (lang, pre string, ok bool) {
	lower := strings.ToLower(stem)
	if localeLike.MatchString(lower) && len(lower) <= 5 {
		return primaryLanguage(lower), "", true
	}
	for _, sep := range []string{"_", ".", "-"} {
		i := strings.LastIndex(stem, sep)
		if i <= 0 {
			continue
		}
		tail := strings.ToLower(stem[i+1:])
		if len(tail) >= 2 && len(tail) <= 3 && localeLike.MatchString(tail) {
			return tail, stem[:i+1], true
		}
	}
	return "", "", false
}

// primaryLanguage returns a language tag's primary subtag, lower-cased.
func primaryLanguage(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if i := strings.IndexAny(tag, "-_"); i >= 0 {
		tag = tag[:i]
	}
	return tag
}

// catalogReason renders a catalog proposal's comment: where it is, what it
// is read as, and what marked it as the source.
func catalogReason(p ProposedCollection, display, sourceLocale string) string {
	count := plural(len(p.Files), display+" catalog", display+" catalogs")
	switch p.signal {
	case signalDir:
		dir := p.Path[:strings.Index(p.Path, "/*")]
		return fmt.Sprintf("%s/: %s in the directory named for the source language (%s)", dir, count, sourceLocale)
	case signalStem:
		return fmt.Sprintf("%s: %s named for the source language (%s)", p.Path, count, sourceLocale)
	default:
		return fmt.Sprintf("%s: %s", p.Path, count)
	}
}

// recognise returns the built-in format a file is read as and the content
// shape that format carries.
func recognise(reg *registry.FormatRegistry, root, rel string) (string, registry.FormatFamily, bool) {
	id, err := reg.Detect(filepath.Join(root, filepath.FromSlash(rel)), registry.DetectOptions{
		AllowedSources: []string{"built-in"},
	})
	if err != nil || id == "" {
		return "", "", false
	}
	info := reg.FormatInfo(id)
	if info == nil || !info.HasReader {
		return "", "", false
	}
	return string(id), info.Family, true
}

// displayName is a format's name as a person reads it.
func displayName(reg *registry.FormatRegistry, id string) string {
	if info := reg.FormatInfo(registry.FormatID(id)); info != nil && info.DisplayName != "" {
		return info.DisplayName
	}
	return id
}

// scanContentTree lists the files under root a proposal may read, as sorted
// project-relative slash paths. It honours every `.gitignore` and the
// project's `.kapiignore` on the way down, and skips hidden entries and the
// directories in initScanSkipDirs.
func scanContentTree(root string) ([]string, error) {
	type scope struct {
		dir     string // slash path relative to root, "" for root
		matcher *ignore.Matcher
	}
	rootMatcher := ignore.ForProjectDir(root)
	if err := rootMatcher.LoadFile(filepath.Join(root, ".gitignore")); err != nil {
		return nil, fmt.Errorf("read .gitignore: %w", err)
	}
	scopes := []scope{{dir: "", matcher: rootMatcher}}

	ignored := func(rel string, isDir bool) bool {
		for _, s := range scopes {
			sub := rel
			if s.dir != "" {
				if !strings.HasPrefix(rel, s.dir+"/") {
					continue
				}
				sub = strings.TrimPrefix(rel, s.dir+"/")
			}
			if s.matcher.Match(sub, isDir) {
				return true
			}
		}
		return false
	}

	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == root {
				return err
			}
			return nil // an unreadable entry is not content
		}
		if p == root {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		name := d.Name()
		if d.IsDir() {
			if strings.HasPrefix(name, ".") || initScanSkipDirs[name] || ignored(rel, true) {
				return filepath.SkipDir
			}
			// A nested .gitignore governs the tree below it.
			if _, serr := os.Stat(filepath.Join(p, ".gitignore")); serr == nil {
				m := ignore.New()
				if lerr := m.LoadFile(filepath.Join(p, ".gitignore")); lerr == nil {
					// Scopes are kept outermost first; WalkDir visits a
					// directory before anything under it, so a scope whose
					// directory the walk has left no longer matches a path.
					scopes = append(scopes, scope{dir: rel, matcher: m})
				}
			}
			return nil
		}
		if !d.Type().IsRegular() || strings.HasPrefix(name, ".") || name == project.RecipeFileName {
			return nil
		}
		if ignored(rel, false) {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", root, err)
	}
	sort.Strings(files)
	return files, nil
}
