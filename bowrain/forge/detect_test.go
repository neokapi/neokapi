package forge

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchesContentPatterns(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		path     string
		want     bool
	}{
		// `**` spans any number of directories, including none — the drill
		// finding: a root README.md must match `**/*.md`.
		{"doublestar matches root file", []string{"**/*.md"}, "README.md", true},
		{"doublestar matches deep file", []string{"**/*.md"}, "docs/guide/intro.md", true},
		{"doublestar matches one level", []string{"**/*.json"}, "locales/en.json", true},
		{"extension mismatch", []string{"**/*.json"}, "src/main.go", false},
		{"anchored pattern stays anchored", []string{"docs/**/*.md"}, "other/docs.md", false},
		{"anchored pattern matches subtree", []string{"docs/**/*.md"}, "docs/a/b.md", true},
		// Simple patterns (no slash) match by base name anywhere.
		{"basename fallback", []string{"*.json"}, "src/locales/en.json", true},
		{"multiple patterns, second wins", []string{"**/*.html", "**/*.yml"}, "ci/build.yml", true},
		{"whitespace around patterns tolerated", []string{" **/*.md "}, "README.md", true},
		{"empty pattern never matches", []string{""}, "README.md", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, MatchesContentPatterns(tt.patterns, tt.path))
		})
	}
}

func TestDefaultContentPatternsIncludeMarkdown(t *testing.T) {
	// Drill finding: a README/docs repository used to ingest zero content.
	defaults := DefaultContentPatterns()
	assert.Contains(t, defaults, "**/*.md")
	assert.Contains(t, defaults, "**/*.mdx")
	assert.True(t, MatchesContentPatterns(defaults, "README.md"))
	assert.True(t, MatchesContentPatterns(defaults, "docs/pages/intro.mdx"))
}

// A pnpm monorepo with react-i18next catalogs in one app: the wizard's
// headline case. Detection must name the monorepo, the workspaces, the
// catalog dir, and propose patterns that hit exactly the catalogs.
func TestDetectRepoContent_MonorepoI18next(t *testing.T) {
	paths := []string{
		"pnpm-workspace.yaml",
		"turbo.json",
		"package.json",
		"README.md",
		"apps/web/package.json",
		"apps/web/src/App.tsx",
		"apps/web/public/locales/en/common.json",
		"apps/web/public/locales/en/dashboard.json",
		"apps/web/public/locales/fr/common.json",
		"apps/api/package.json",
		"apps/api/main.go",
		"packages/ui/package.json",
		"packages/ui/src/button.tsx",
	}
	det := DetectRepoContent(paths, DetectOptions{})

	assert.Equal(t, []string{"pnpm-workspace.yaml", "turbo.json"}, det.MonorepoMarkers)
	assert.Equal(t, []string{"apps/api", "apps/web", "packages/ui"}, det.Workspaces)

	require.Len(t, det.Signals, 1)
	sig := det.Signals[0]
	assert.Equal(t, "react-i18next", sig.ID)
	assert.Equal(t, "i18next catalogs", sig.Label)
	assert.Equal(t, "apps/web/public/locales", sig.Dir)
	assert.Equal(t, 3, sig.Files)

	assert.Equal(t, "apps/web/public/locales/**/*.json", det.ProposedPatterns)
	assert.Equal(t, 3, det.MatchCount)
	assert.Contains(t, det.MatchPreview, "apps/web/public/locales/en/common.json")
	assert.NotContains(t, det.MatchPreview, "apps/web/package.json",
		"the proposal must not sweep up package manifests")
	assert.False(t, det.Truncated)
}

// A plain docs repository (Docusaurus): the docs signal wins and the proposal
// targets the markdown content.
func TestDetectRepoContent_DocsRepo(t *testing.T) {
	paths := []string{
		"docusaurus.config.ts",
		"package.json",
		"README.md",
		"docs/intro.md",
		"docs/guides/setup.mdx",
		"blog/2026-07-01-launch.md",
		"src/pages/index.tsx",
	}
	det := DetectRepoContent(paths, DetectOptions{})

	assert.Empty(t, det.MonorepoMarkers)
	require.Len(t, det.Signals, 1)
	assert.Equal(t, "docusaurus", det.Signals[0].ID)
	assert.Equal(t, "Docusaurus site", det.Signals[0].Label)
	assert.Equal(t, 3, det.Signals[0].Files)

	patterns := strings.Split(det.ProposedPatterns, ",")
	assert.Contains(t, patterns, "docs/**/*.md")
	assert.Contains(t, patterns, "docs/**/*.mdx")
	assert.Contains(t, patterns, "blog/**/*.md")
	assert.Equal(t, 3, det.MatchCount)
}

// A README-only repository — the exact drill case. No framework signal, but
// the markdown fallback keeps the proposal from silently matching nothing.
func TestDetectRepoContent_ReadmeOnlyRepo(t *testing.T) {
	paths := []string{"README.md", "LICENSE", "CONTRIBUTING.md"}
	det := DetectRepoContent(paths, DetectOptions{})

	require.Len(t, det.Signals, 1)
	assert.Equal(t, "markdown", det.Signals[0].ID)
	assert.Equal(t, 2, det.Signals[0].Files)
	assert.Equal(t, 2, det.MatchCount)
	assert.ElementsMatch(t, []string{"README.md", "CONTRIBUTING.md"}, det.MatchPreview)
}

// An empty (or all-code, no-content) repository: no signals, the default
// proposal, zero matches — the wizard shows its zero-match warning off this.
func TestDetectRepoContent_EmptyRepo(t *testing.T) {
	det := DetectRepoContent(nil, DetectOptions{})
	assert.Empty(t, det.Signals)
	assert.Equal(t, strings.Join(DefaultContentPatterns(), ","), det.ProposedPatterns)
	assert.Zero(t, det.MatchCount)
	assert.Empty(t, det.MatchPreview)

	code := DetectRepoContent([]string{"main.go", "go.sum", "internal/db/db.go"}, DetectOptions{})
	assert.Empty(t, code.Signals)
	assert.Zero(t, code.MatchCount, "a pure-code repo matches nothing — the wizard must warn")
}

// Scoping to a monorepo workspace confines both the signals and the proposal.
func TestDetectRepoContent_Scope(t *testing.T) {
	paths := []string{
		"pnpm-workspace.yaml",
		"apps/web/public/locales/en/common.json",
		"apps/docs/docs/intro.md",
		"apps/docs/docusaurus.config.ts",
		"apps/docs/package.json",
		"apps/web/package.json",
	}
	det := DetectRepoContent(paths, DetectOptions{Scope: "apps/web"})
	require.Len(t, det.Signals, 1)
	assert.Equal(t, "react-i18next", det.Signals[0].ID)
	assert.Equal(t, "apps/web/public/locales/**/*.json", det.ProposedPatterns)
	assert.Equal(t, 1, det.MatchCount)

	// A scope with no catalogs falls back to the defaults *under the scope*.
	empty := DetectRepoContent(paths, DetectOptions{Scope: "apps/api"})
	assert.Empty(t, empty.Signals)
	for pat := range strings.SplitSeq(empty.ProposedPatterns, ",") {
		assert.True(t, strings.HasPrefix(pat, "apps/api/"), "scoped default pattern: %s", pat)
	}
	assert.Zero(t, empty.MatchCount)
}

// A patterns override drives the match count/preview — live feedback for the
// wizard's editable patterns input.
func TestDetectRepoContent_PatternsOverride(t *testing.T) {
	paths := []string{
		"src/locales/en.json",
		"src/locales/fr.json",
		"docs/intro.md",
	}
	det := DetectRepoContent(paths, DetectOptions{PatternsOverride: "docs/**/*.md"})
	// The proposal still reflects what was detected …
	require.NotEmpty(t, det.Signals)
	assert.Equal(t, "locale-dir", det.Signals[0].ID)
	assert.Equal(t, "src/locales", det.Signals[0].Dir)
	// … but the count follows the override.
	assert.Equal(t, 1, det.MatchCount)
	assert.Equal(t, []string{"docs/intro.md"}, det.MatchPreview)

	// A zero-match override reports zero, never silently falls back.
	none := DetectRepoContent(paths, DetectOptions{PatternsOverride: "missing/**/*.po"})
	assert.Zero(t, none.MatchCount)
}

// Catalog signals across ecosystems: the table mirrors the i18n framework
// registry (cli/skills/data/kapi/references/i18n/frameworks.yaml).
func TestDetectRepoContent_FrameworkTable(t *testing.T) {
	tests := []struct {
		name    string
		paths   []string
		wantID  string
		wantDir string
	}{
		{"flutter arb", []string{"pubspec.yaml", "lib/l10n/app_en.arb", "lib/l10n/app_de.arb"}, "flutter", "lib/l10n"},
		{"rails yaml", []string{"Gemfile", "config/locales/en.yml", "config/locales/nb.yml"}, "rails", "config/locales"},
		{"gettext po", []string{"po/de.po", "po/messages.pot"}, "gettext", "po"},
		{"xcode xcstrings", []string{"App/Localizable.xcstrings"}, "ios-xcstrings", "App"},
		{"android res", []string{"app/src/main/res/values/strings.xml", "app/src/main/res/values-de/strings.xml"}, "android", "app/src/main/res"},
		{"jvm properties", []string{"src/main/resources/messages.properties", "src/main/resources/messages_de.properties"}, "jvm", "src/main/resources"},
		{"generic locale dir", []string{"src/locales/en.json", "src/locales/de.json"}, "locale-dir", "src/locales"},
		{"transloco", []string{"src/assets/i18n/en.json"}, "transloco", "src/assets/i18n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			det := DetectRepoContent(tt.paths, DetectOptions{})
			require.NotEmpty(t, det.Signals)
			assert.Equal(t, tt.wantID, det.Signals[0].ID)
			assert.Equal(t, tt.wantDir, det.Signals[0].Dir)
			assert.Positive(t, det.MatchCount, "the proposal must match the catalogs it detected")
		})
	}
}

// Committed dependency trees never fabricate signals.
func TestDetectRepoContent_IgnoresVendoredTrees(t *testing.T) {
	det := DetectRepoContent([]string{
		"node_modules/some-lib/locales/en.json",
		"vendor/pkg/i18n/de.json",
		"main.go",
	}, DetectOptions{})
	assert.Empty(t, det.Signals)
}
