package forge

import (
	"path"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// This file is the repository content detector behind the GitHub App setup
// wizard: given a repository's file listing (the git tree — no clone), it
// reports monorepo markers, i18n/content catalog signals, and a proposed
// `patterns` value for the forge/git connector, plus the files that proposal
// would ingest.
//
// The i18n signal table below mirrors the catalog-location knowledge of the
// kapi skill's i18n framework registry —
// cli/skills/data/kapi/references/i18n/frameworks.yaml — which is the source
// of truth (catalog layouts, detect files). When the registry gains or changes
// a framework's catalog layout, update the table here to match.

// MaxDetectTreeEntries caps how many tree entries a detection reads — the size
// cap on the recursive trees call. Repositories beyond it are reported as
// truncated rather than fetched deeper.
const MaxDetectTreeEntries = 20000

// DefaultContentPatterns returns the git/forge connector's default content
// globs: the formats a repository most commonly keeps translatable content in.
// Markdown/MDX are included so a plain README/docs repository ingests its
// documents instead of nothing.
func DefaultContentPatterns() []string {
	return []string{
		"**/*.html", "**/*.json", "**/*.yaml", "**/*.yml",
		"**/*.properties", "**/*.md", "**/*.mdx",
	}
}

// MatchesContentPatterns reports whether a slash-separated repository path
// matches any of the connector's content globs. Globs use doublestar
// semantics — `**` spans any number of directories, including none, so
// `**/*.md` matches both `README.md` and `docs/guide/intro.md`. A pattern
// without a slash also matches by base name (`*.json` means "any JSON file
// anywhere"), mirroring the git connector's historical behavior for simple
// patterns.
func MatchesContentPatterns(patterns []string, p string) bool {
	for _, pat := range patterns {
		pat = strings.TrimSpace(pat)
		if pat == "" {
			continue
		}
		if ok, err := doublestar.Match(pat, p); err == nil && ok {
			return true
		}
		if !strings.Contains(pat, "/") {
			if ok, err := doublestar.Match(pat, path.Base(p)); err == nil && ok {
				return true
			}
		}
	}
	return false
}

// ContentSignalHit is one detected i18n/content signal: which framework (or
// generic catalog kind) was recognised and where its catalogs live.
type ContentSignalHit struct {
	// ID is the signal's identifier — a frameworks.yaml registry id where one
	// exists (react-i18next, flutter, docusaurus, …) or a generic kind
	// (locale-dir, markdown).
	ID string `json:"id"`
	// Label is the human fragment for the wizard's conclusion line, e.g.
	// "react-i18next catalogs".
	Label string `json:"label"`
	// Dir is the catalog root actually seen in the tree ("" when the signal
	// is repo-wide, e.g. extension-only hits at the root).
	Dir string `json:"dir,omitempty"`
	// Files is how many tree files back this signal.
	Files int `json:"files"`

	patterns []string // proposal contribution (not serialized; folded into ProposedPatterns)
}

// RepoDetection is what the setup wizard shows for a repository before
// binding it: monorepo shape, content signals, the proposed connector
// patterns, and the files that proposal (or the caller's override) matches.
type RepoDetection struct {
	MonorepoMarkers  []string           `json:"monorepo_markers"`
	Workspaces       []string           `json:"workspaces,omitempty"`
	Signals          []ContentSignalHit `json:"signals"`
	ProposedPatterns string             `json:"proposed_patterns"`
	MatchCount       int                `json:"match_count"`
	MatchPreview     []string           `json:"match_preview"`
	Truncated        bool               `json:"truncated"`
}

// DetectOptions scope a detection run.
type DetectOptions struct {
	// Scope confines signal detection and the proposal to one subdirectory
	// (a monorepo workspace). Paths keep their full repository form; the
	// proposal's globs carry the prefix.
	Scope string
	// PatternsOverride, when set (comma-separated), is matched for
	// MatchCount/MatchPreview instead of the proposal — live feedback while
	// the user edits the patterns input.
	PatternsOverride string
}

// dirSignal recognises a framework by its catalog directory layout: a
// directory-path suffix looked up at any prefix depth (monorepos), holding
// files with the layout's extensions. Mirrors frameworks.yaml `catalog.layout`.
type dirSignal struct {
	id       string
	label    string
	suffix   string   // catalog dir suffix, e.g. "public/locales"
	exts     []string // catalog file extensions inside it
	patterns func(root string) []string
}

// extSignal recognises a catalog format by file extension alone, wherever the
// files sit. Mirrors frameworks.yaml entries whose detection is extension-led.
type extSignal struct {
	id    string
	label string
	exts  []string
}

// Directory-layout signals, most specific first. Root ordering matters only
// for the conclusion line; every hit contributes patterns.
var dirSignals = []dirSignal{
	// react-i18next / next-i18next / astro-i18next: public/locales/{lang}/*.json
	{id: "react-i18next", label: "i18next catalogs", suffix: "public/locales", exts: []string{".json"},
		patterns: func(root string) []string { return []string{root + "/**/*.json"} }},
	// nuxtjs-i18n: i18n/locales/{lang}.json
	{id: "nuxtjs-i18n", label: "@nuxtjs/i18n catalogs", suffix: "i18n/locales", exts: []string{".json"},
		patterns: func(root string) []string { return []string{root + "/**/*.json"} }},
	// transloco: assets/i18n/{lang}.json
	{id: "transloco", label: "Transloco catalogs", suffix: "assets/i18n", exts: []string{".json"},
		patterns: func(root string) []string { return []string{root + "/**/*.json"} }},
	// ngx-translate: public/i18n/{lang}.json
	{id: "ngx-translate", label: "ngx-translate catalogs", suffix: "public/i18n", exts: []string{".json"},
		patterns: func(root string) []string { return []string{root + "/**/*.json"} }},
	// rails: config/locales/{lang}.yml
	{id: "rails", label: "Rails locale files", suffix: "config/locales", exts: []string{".yml", ".yaml"},
		patterns: func(root string) []string { return []string{root + "/**/*.yml", root + "/**/*.yaml"} }},
	// flutter: lib/l10n/app_{lang}.arb
	{id: "flutter", label: "Flutter ARB catalogs", suffix: "lib/l10n", exts: []string{".arb"},
		patterns: func(root string) []string { return []string{root + "/**/*.arb"} }},
	// next-intl: messages/{lang}.json (paraglide shares the layout; its
	// project.inlang marker does not change the catalog location)
	{id: "next-intl", label: "message catalogs", suffix: "messages", exts: []string{".json"},
		patterns: func(root string) []string { return []string{root + "/**/*.json"} }},
	// gettext / django / lingui: */LC_MESSAGES/*.po and po/{lang}.po are
	// caught by the .po extension signal below.
}

// Extension-led signals. The proposal glob is anchored at the hits' common
// directory so a monorepo proposal stays scoped to where the catalogs are.
var extSignals = []extSignal{
	{id: "ios-xcstrings", label: "Xcode string catalogs", exts: []string{".xcstrings"}},
	{id: "apple-strings", label: "Apple .strings files", exts: []string{".strings"}},
	{id: "flutter", label: "Flutter ARB catalogs", exts: []string{".arb"}},
	{id: "gettext", label: "gettext catalogs", exts: []string{".po", ".pot"}},
	{id: "jvm", label: "Java properties bundles", exts: []string{".properties"}},
	{id: "dotnet", label: ".NET resource files", exts: []string{".resx"}},
	{id: "angular", label: "XLIFF catalogs", exts: []string{".xlf", ".xliff"}},
	{id: "neokapi-i18n", label: "KLF catalogs", exts: []string{".klf"}},
}

// Generic locale-directory names: a directory with one of these exact segment
// names holding JSON/YAML files is a catalog dir even when no framework
// layout matched (vue-i18n's src/locales, react-native's locales/, …).
var genericLocaleDirs = map[string]bool{
	"locales": true, "i18n": true, "translations": true, "lang": true,
}

// Monorepo marker files, checked at the repository root.
var monorepoMarkers = []string{"pnpm-workspace.yaml", "turbo.json", "go.work", "lerna.json"}

// Directories detection ignores outright — committed dependency/build trees
// would otherwise fabricate catalog signals.
var ignoredTopDirs = map[string]bool{
	"node_modules": true, "vendor": true, "dist": true, "build": true,
	".git": true, ".next": true, "target": true,
}

func ignored(p string) bool {
	for seg := range strings.SplitSeq(p, "/") {
		if ignoredTopDirs[seg] {
			return true
		}
	}
	return false
}

// DetectRepoContent inspects a repository file listing (slash paths, blobs
// only) and returns the wizard's detection: monorepo markers/workspaces,
// content signals, a proposed `patterns` value, and the match preview.
func DetectRepoContent(paths []string, opts DetectOptions) RepoDetection {
	det := RepoDetection{
		Signals:      []ContentSignalHit{},
		MatchPreview: []string{},
	}

	// Monorepo shape is a whole-repository property — read it before scoping.
	present := make(map[string]bool, len(paths))
	for _, p := range paths {
		present[p] = true
	}
	for _, m := range monorepoMarkers {
		if present[m] {
			det.MonorepoMarkers = append(det.MonorepoMarkers, m)
		}
	}
	det.Workspaces = detectWorkspaces(paths)

	scoped := paths
	if opts.Scope != "" {
		prefix := strings.TrimSuffix(opts.Scope, "/") + "/"
		scoped = nil
		for _, p := range paths {
			if strings.HasPrefix(p, prefix) {
				scoped = append(scoped, p)
			}
		}
	}

	det.Signals = detectSignals(scoped)

	var proposal []string
	seen := map[string]bool{}
	for _, hit := range det.Signals {
		for _, pat := range hit.patterns {
			if !seen[pat] {
				seen[pat] = true
				proposal = append(proposal, pat)
			}
		}
	}
	if len(proposal) == 0 {
		proposal = DefaultContentPatterns()
		if opts.Scope != "" {
			scopePrefix := strings.TrimSuffix(opts.Scope, "/") + "/"
			for i, pat := range proposal {
				proposal[i] = scopePrefix + pat
			}
		}
	}
	det.ProposedPatterns = strings.Join(proposal, ",")

	matchPatterns := proposal
	if opts.PatternsOverride != "" {
		matchPatterns = strings.Split(opts.PatternsOverride, ",")
	}
	for _, p := range paths {
		if MatchesContentPatterns(matchPatterns, p) {
			det.MatchCount++
			if len(det.MatchPreview) < 20 {
				det.MatchPreview = append(det.MatchPreview, p)
			}
		}
	}
	return det
}

// detectWorkspaces lists monorepo workspace directories: apps/* and
// packages/* members, plus top-level directories carrying their own module
// manifest. These feed the wizard's subdirectory scope selector.
func detectWorkspaces(paths []string) []string {
	manifests := map[string]bool{
		"package.json": true, "go.mod": true, "pubspec.yaml": true, "Cargo.toml": true,
	}
	set := map[string]bool{}
	for _, p := range paths {
		if ignored(p) {
			continue
		}
		segs := strings.Split(p, "/")
		if len(segs) >= 3 && (segs[0] == "apps" || segs[0] == "packages") {
			set[segs[0]+"/"+segs[1]] = true
		}
		if len(segs) == 2 && manifests[segs[1]] && !strings.HasPrefix(segs[0], ".") {
			set[segs[0]] = true
		}
	}
	out := make([]string, 0, len(set))
	for ws := range set {
		out = append(out, ws)
	}
	sort.Strings(out)
	if len(out) > 30 {
		out = out[:30]
	}
	return out
}

// detectSignals runs the signal table over the (possibly scoped) listing.
// Order: directory-layout signals, the Android res/values layout, generic
// locale dirs, extension signals, docs-site signals, and — only when nothing
// else matched — a plain-markdown fallback.
func detectSignals(paths []string) []ContentSignalHit {
	var hits []ContentSignalHit
	// Roots already claimed by a more specific signal; generic passes skip
	// files under them so one catalog tree yields one signal.
	claimed := map[string]bool{}
	underClaimed := func(p string) bool {
		for root := range claimed {
			if strings.HasPrefix(p, root+"/") {
				return true
			}
		}
		return false
	}

	// --- Directory-layout signals -------------------------------------------
	for _, sig := range dirSignals {
		roots := map[string]int{}
		for _, p := range paths {
			if ignored(p) || underClaimed(p) {
				continue
			}
			if !hasExt(p, sig.exts) {
				continue
			}
			if root, ok := dirWithSuffix(path.Dir(p), sig.suffix); ok {
				roots[root]++
			}
		}
		for _, root := range sortedKeys(roots) {
			claimed[root] = true
			hits = append(hits, ContentSignalHit{
				ID: sig.id, Label: sig.label, Dir: root, Files: roots[root],
				patterns: sig.patterns(root),
			})
		}
	}

	// --- Android res/values-*/strings.xml -----------------------------------
	androidRoots := map[string]int{}
	for _, p := range paths {
		if ignored(p) || path.Base(p) != "strings.xml" {
			continue
		}
		dir := path.Dir(p)
		if i := strings.LastIndex("/"+dir+"/", "/res/values"); i >= 0 {
			full := "/" + dir + "/"
			root := strings.Trim(full[:i]+"/res", "/")
			androidRoots[root]++
		}
	}
	for _, root := range sortedKeys(androidRoots) {
		claimed[root] = true
		hits = append(hits, ContentSignalHit{
			ID: "android", Label: "Android string resources", Dir: root, Files: androidRoots[root],
			patterns: []string{root + "/values*/strings.xml"},
		})
	}

	// --- Generic locale dirs -------------------------------------------------
	catalogExts := []string{".json", ".yaml", ".yml", ".klf"}
	genericRoots := map[string]int{}
	for _, p := range paths {
		if ignored(p) || underClaimed(p) || !hasExt(p, catalogExts) {
			continue
		}
		dir := path.Dir(p)
		if root, ok := topSegmentIn(dir, genericLocaleDirs); ok {
			genericRoots[root]++
		}
	}
	for _, root := range sortedKeys(genericRoots) {
		claimed[root] = true
		hits = append(hits, ContentSignalHit{
			ID: "locale-dir", Label: "locale catalogs", Dir: root, Files: genericRoots[root],
			patterns: []string{root + "/**/*.json", root + "/**/*.yaml", root + "/**/*.yml"},
		})
	}

	// --- Extension signals ---------------------------------------------------
	for _, sig := range extSignals {
		var files []string
		for _, p := range paths {
			if ignored(p) || underClaimed(p) {
				continue
			}
			if hasExt(p, sig.exts) {
				files = append(files, p)
			}
		}
		if len(files) == 0 {
			continue
		}
		root := commonDir(files)
		var pats []string
		for _, ext := range sig.exts {
			if root == "" {
				pats = append(pats, "**/*"+ext)
			} else {
				pats = append(pats, root+"/**/*"+ext)
			}
		}
		if root != "" {
			claimed[root] = true
		}
		hits = append(hits, ContentSignalHit{
			ID: sig.id, Label: sig.label, Dir: root, Files: len(files), patterns: pats,
		})
	}

	// --- Docs-site signals ---------------------------------------------------
	hits = append(hits, detectDocsSignals(paths)...)

	// --- Plain-markdown fallback --------------------------------------------
	if len(hits) == 0 {
		var md []string
		for _, p := range paths {
			if ignored(p) {
				continue
			}
			if hasExt(p, []string{".md", ".mdx"}) {
				md = append(md, p)
			}
		}
		if len(md) > 0 {
			hits = append(hits, ContentSignalHit{
				ID: "markdown", Label: "Markdown documents", Dir: commonDir(md), Files: len(md),
				patterns: []string{"**/*.md", "**/*.mdx"},
			})
		}
	}
	return hits
}

// detectDocsSignals recognises documentation sites by their config markers.
func detectDocsSignals(paths []string) []ContentSignalHit {
	var hits []ContentSignalHit
	mdCount := func(prefix string) (int, []string) {
		n := 0
		for _, p := range paths {
			if ignored(p) {
				continue
			}
			if strings.HasPrefix(p, prefix+"/") && hasExt(p, []string{".md", ".mdx"}) {
				n++
			}
		}
		if n == 0 {
			return 0, nil
		}
		return n, []string{prefix + "/**/*.md", prefix + "/**/*.mdx"}
	}
	seenDoc := map[string]bool{}
	for _, p := range paths {
		if ignored(p) {
			continue
		}
		base := path.Base(p)
		dir := path.Dir(p)
		if dir == "." {
			dir = ""
		}
		switch {
		case strings.HasPrefix(base, "docusaurus.config."):
			if seenDoc["docusaurus:"+dir] {
				continue
			}
			seenDoc["docusaurus:"+dir] = true
			var pats []string
			files := 0
			for _, content := range []string{"docs", "blog"} {
				root := content
				if dir != "" {
					root = dir + "/" + content
				}
				if n, ps := mdCount(root); n > 0 {
					files += n
					pats = append(pats, ps...)
				}
			}
			if files > 0 {
				hits = append(hits, ContentSignalHit{
					ID: "docusaurus", Label: "Docusaurus site", Dir: dir, Files: files, patterns: pats,
				})
			}
		case base == "mkdocs.yml":
			if seenDoc["mkdocs:"+dir] {
				continue
			}
			seenDoc["mkdocs:"+dir] = true
			root := "docs"
			if dir != "" {
				root = dir + "/docs"
			}
			if n, ps := mdCount(root); n > 0 {
				hits = append(hits, ContentSignalHit{
					ID: "mkdocs", Label: "MkDocs site", Dir: dir, Files: n, patterns: ps,
				})
			}
		}
	}
	// A content/ tree of markdown (Hugo & friends) counts even without a
	// recognised site config.
	if n, ps := func() (int, []string) {
		n := 0
		for _, p := range paths {
			if !ignored(p) && strings.HasPrefix(p, "content/") && hasExt(p, []string{".md", ".mdx"}) {
				n++
			}
		}
		if n < 3 {
			return 0, nil
		}
		return n, []string{"content/**/*.md", "content/**/*.mdx"}
	}(); n > 0 && !seenDoc["docusaurus:"] && !seenDoc["mkdocs:"] {
		hits = append(hits, ContentSignalHit{
			ID: "content-dir", Label: "Markdown content", Dir: "content", Files: n, patterns: ps,
		})
	}
	return hits
}

// hasExt reports whether the path's extension is one of exts.
func hasExt(p string, exts []string) bool {
	e := strings.ToLower(path.Ext(p))
	for _, want := range exts {
		if e == want {
			return true
		}
	}
	return false
}

// dirWithSuffix finds the shortest ancestor of dir whose path ends with the
// segment-suffix (e.g. dir "apps/web/public/locales/en", suffix
// "public/locales" → "apps/web/public/locales").
func dirWithSuffix(dir, suffix string) (string, bool) {
	full := "/" + dir + "/"
	idx := strings.Index(full, "/"+suffix+"/")
	if idx < 0 {
		return "", false
	}
	return strings.Trim(full[:idx+1+len(suffix)], "/"), true
}

// topSegmentIn finds the shortest ancestor of dir whose last segment is one
// of names (e.g. "src/locales/en" with {"locales"} → "src/locales").
func topSegmentIn(dir string, names map[string]bool) (string, bool) {
	segs := strings.Split(dir, "/")
	for i, seg := range segs {
		if names[seg] {
			return strings.Join(segs[:i+1], "/"), true
		}
	}
	return "", false
}

// commonDir returns the longest common directory of the given paths ("" when
// they share none beyond the root).
func commonDir(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	common := strings.Split(path.Dir(paths[0]), "/")
	if path.Dir(paths[0]) == "." {
		return ""
	}
	for _, p := range paths[1:] {
		dir := path.Dir(p)
		if dir == "." {
			return ""
		}
		segs := strings.Split(dir, "/")
		n := 0
		for n < len(common) && n < len(segs) && common[n] == segs[n] {
			n++
		}
		common = common[:n]
		if len(common) == 0 {
			return ""
		}
	}
	return strings.Join(common, "/")
}

// sortedKeys returns the map's keys sorted, for deterministic output.
func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
