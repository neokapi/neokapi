package project

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/neokapi/neokapi/core/ignore"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// walkTree is a project shaped like a monorepo: source folders, a nested
// package with its own node_modules, build output and a state directory.
func walkTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range []string{
		"README.md", "notes.txt", "kapi.yaml", "a.yaml", ".hidden.yml",
		"core/x.go", "core/sub/y.go", "core/sub/z.sh", "core/sub/w.py",
		"cli/main.go", "cli/doc.md",
		"web/src/app.ts", "web/src/app.css", "web/src/deep/x.tsx",
		"web/build/out.js", "web/dist/bundle.js",
		"web/node_modules/pkg/index.js", "web/node_modules/pkg/sub/a.ts",
		"node_modules/top/readme.md",
		"harness/tool.sh", "harness/.kapi/data/legal/cldr.md",
		"docs/a b/c{d}.md", "docs/[x].md", "docs/star*.md",
		"store/pkg/lib.json",
	} {
		createFile(t, dir, f, "x")
	}
	return dir
}

func doublestarExpand(t *testing.T, root, pattern string, excludes []string) []string {
	t.Helper()
	matches, err := doublestar.Glob(os.DirFS(root), pattern, doublestar.WithNoFollow())
	require.NoError(t, err)
	var out []string
	for _, m := range matches {
		if !slices.ContainsFunc(excludes, func(e string) bool { return MatchGlob(e, m) }) {
			out = append(out, m)
		}
	}
	slices.Sort(out)
	return out
}

// TestExpandGlobs_MatchesDoublestar holds the one-walk resolver to doublestar's
// own answers, pattern by pattern, with and without excludes.
func TestExpandGlobs_MatchesDoublestar(t *testing.T) {
	dir := walkTree(t)
	patterns := []string{
		"*.md",
		"**/*.md",
		"**",
		"core/**",
		"core/**/*.go",
		"{core,cli}/**/*.{go,sh,py}",
		"{core/sub,web/src}/*.{go,ts}",
		"web/{src/**/*.{ts,tsx},build/*.js}",
		"web/**/*.{ts,tsx,js,css}",
		"{*,.*}.{yaml,yml}",
		"{bench,core,harness,scripts}/**/*.{sh,py,rs,c,h}",
		"harness/**/*.{md,mdx,html}",
		"docs/*.md",
		`docs/\[x\].md`,
		`docs/star\*.md`,
		"docs/a b/*.md",
		"docs/[a-z] b/*",
		"web/*/**",
		"nothing/**/*.go",
		"*/sub/*",
		"**/sub/**/*.go",
	}
	excludeSets := [][]string{
		nil,
		{"**/node_modules/**", "web/build/**", "**/dist/**"},
		{"web/build", "**/*.sh"},
	}
	for ei, excludes := range excludeSets {
		for _, p := range patterns {
			t.Run(fmt.Sprintf("%d/%s", ei, p), func(t *testing.T) {
				want := doublestarExpand(t, dir, p, excludes)
				got, err := ExpandGlob(dir, p, excludes...)
				require.NoError(t, err)
				slices.Sort(got)
				assert.Equal(t, want, got)
			})
		}
	}

	// All of them in one walk give each pattern the same answer.
	res, err := ExpandGlobs(dir, patterns, GlobOptions{Excludes: excludeSets[1]})
	require.NoError(t, err)
	for i, p := range patterns {
		got := slices.Clone(res[i])
		slices.Sort(got)
		assert.Equal(t, doublestarExpand(t, dir, p, excludeSets[1]), got, p)
	}
}

func TestExpandBraces(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"a/*.go", []string{"a/*.go"}},
		{"{a,b}/x", []string{"a/x", "b/x"}},
		{"*.{a,b,c}", []string{"*.a", "*.b", "*.c"}},
		{"{a/{b,c},d}/*", []string{"a/b/*", "a/c/*", "d/*"}},
		{"{a,b}/{c,d}", []string{"a/c", "a/d", "b/c", "b/d"}},
		{`\{a,b\}`, []string{`\{a,b\}`}},
		{"[{]x", []string{"[{]x"}},
		{"a}b", []string{"a}b"}},
		{"{,x}y", []string{"y", "xy"}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, expandBraces(tt.in))
		})
	}
}

func TestExpandGlobs_BadPattern(t *testing.T) {
	_, err := ExpandGlobs(t.TempDir(), []string{"ok/*.md", "bad/[a-.json"}, GlobOptions{})
	require.ErrorIs(t, err, doublestar.ErrBadPattern)
}

// TestExpandGlobs_PrunesIgnoredAndExcludedDirectories: an excluded or ignored
// directory is never read, so nothing under it is returned and no walk count
// is spent in it.
func TestExpandGlobs_PrunesIgnoredAndExcludedDirectories(t *testing.T) {
	dir := walkTree(t)
	ig := ignore.New()
	ig.AddPattern("web/dist/")

	tests := []struct {
		name     string
		pattern  string
		excludes []string
		ignore   *ignore.Matcher
		want     []string
	}{
		{
			name:     "nested node_modules excluded",
			pattern:  "web/**/*.{ts,js}",
			excludes: []string{"**/node_modules/**", "web/build/**"},
			want:     []string{"web/dist/bundle.js", "web/src/app.ts"},
		},
		{
			name:     "ignored directory",
			pattern:  "web/**/*.js",
			excludes: []string{"**/node_modules/**"},
			ignore:   ig,
			want:     []string{"web/build/out.js"},
		},
		{
			name:    "default .kapi rule prunes a nested state directory",
			pattern: "harness/**/*.{md,sh}",
			ignore:  ignore.New(),
			want:    []string{"harness/tool.sh"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := ExpandGlobs(dir, []string{tt.pattern}, GlobOptions{
				Excludes: tt.excludes, Ignore: tt.ignore, FilesOnly: true,
			})
			require.NoError(t, err)
			got := res[0]
			slices.Sort(got)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestExpandGlobs_ReadsOnlyDirectoriesAPatternCanReach counts directory reads:
// a brace set costs one walk, and an excluded node_modules is never entered.
func TestExpandGlobs_ReadsOnlyDirectoriesAPatternCanReach(t *testing.T) {
	dir := walkTree(t)
	for i := range 50 {
		createFile(t, dir, fmt.Sprintf("web/node_modules/p%d/lib/index.js", i), "x")
	}
	count := func(patterns []string, excludes []string) int {
		n := 0
		_, err := ExpandGlobs(dir, patterns, GlobOptions{Excludes: excludes, OnDir: func(int) { n++ }})
		require.NoError(t, err)
		return n
	}
	// web, web/src, web/src/deep, web/build, web/dist: node_modules pruned.
	assert.Equal(t, 6, count([]string{"web/**/*.{ts,tsx,js,mjs,cjs,css}"}, []string{"**/node_modules/**"}))
	// doublestar reads every alternative's tree, node_modules included.
	fsys := countingFS{FS: os.DirFS(dir).(statReadDirFS)}
	_, err := doublestar.Glob(&fsys, "web/**/*.{ts,tsx,js,mjs,cjs,css}", doublestar.WithNoFollow())
	require.NoError(t, err)
	assert.Greater(t, fsys.dirs, 6*50, "the per-alternative walk this resolver replaces")
	// A root-only pattern reads the root and nothing else.
	assert.Equal(t, 1, count([]string{"{*,.*}.{yaml,yml}"}, nil))
	// Naming folders reads those folders, once each, however many patterns
	// and alternatives point at them: the root, core, core/sub and cli.
	assert.Equal(t, 4, count([]string{"{core,cli}/**/*.go", "{core,cli}/**/*.{sh,py}", "core/**/*.md"}, nil))
}

// TestResolveContent_DanglingLinkInIgnoredDirectory is the `kapi status` fault:
// a dangling link under a nested `.kapi/` matched a recipe pattern and failed
// resolution before the ignore rules were applied.
func TestResolveContent_DanglingLinkInIgnoredDirectory(t *testing.T) {
	dir := t.TempDir()
	createFile(t, dir, "harness/guide.md", "# Guide")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "harness", ".kapi", "legal"), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(dir, "missing.md"), filepath.Join(dir, "harness", ".kapi", "legal", "cldr.md")))

	reg := registry.NewFormatRegistry()
	registerBuiltIn(reg, "markdown", ".md")
	proj := &KapiProject{
		Version:     CurrentVersion,
		Collections: []Collection{{Name: "Docs", Content: []ContentItem{{Path: "harness/**/*.{md,mdx,html}"}}}},
	}
	ctx := NewProjectContext(proj, filepath.Join(dir, "kapi.yaml"))

	res, err := ctx.ResolveContentReport(reg)
	require.NoError(t, err)
	require.Len(t, res.Files, 1)
	assert.Equal(t, filepath.Join("harness", "guide.md"), res.Files[0].Relative)
	assert.Empty(t, res.Unreadable, "nothing under an ignored directory is looked at")
}

// benchTree is a project whose content sits beside a large node_modules that
// the recipe excludes.
func benchTree(b *testing.B) string {
	b.Helper()
	dir := b.TempDir()
	write := func(rel string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	for i := range 40 {
		write(fmt.Sprintf("core/p%d/a.go", i))
		write(fmt.Sprintf("scripts/s%d/run.sh", i))
		write(fmt.Sprintf("harness/h%d/x.ts", i))
	}
	for i := range 400 {
		for j := range 5 {
			write(fmt.Sprintf("harness/node_modules/m%d/d%d/index.js", i, j))
		}
	}
	return dir
}

var benchPatterns = []string{
	"{core,scripts,harness}/**/*.{sh,py,rs,c,h,cs,java,rb}",
	"{core,scripts,harness}/**/*.{ts,tsx,js,mjs,cjs,css}",
	"{core,harness}/**/*.{md,mdx,html}",
	"core/**/*.go",
}

var benchExcludes = []string{"**/node_modules/**", "**/dist/**"}

// BenchmarkExpand compares a glob per pattern (doublestar walks once per
// brace alternative and filters excludes afterwards) with one pruned walk.
// dirs/op is the number of directories read.
func BenchmarkExpand(b *testing.B) {
	dir := benchTree(b)
	b.Run("per-pattern-doublestar", func(b *testing.B) {
		fsys := countingFS{FS: os.DirFS(dir).(statReadDirFS)}
		for b.Loop() {
			for _, p := range benchPatterns {
				matches, err := doublestar.Glob(&fsys, p, doublestar.WithNoFollow())
				if err != nil {
					b.Fatal(err)
				}
				_ = matches
			}
		}
		b.ReportMetric(float64(fsys.dirs)/float64(b.N), "dirs/op")
	})
	b.Run("one-walk", func(b *testing.B) {
		dirs := 0
		for b.Loop() {
			_, err := ExpandGlobs(dir, benchPatterns, GlobOptions{Excludes: benchExcludes, OnDir: func(int) { dirs++ }})
			if err != nil {
				b.Fatal(err)
			}
		}
		b.ReportMetric(float64(dirs)/float64(b.N), "dirs/op")
	})
}

type statReadDirFS interface {
	fs.StatFS
	fs.ReadDirFS
}

// countingFS counts the directories doublestar reads.
type countingFS struct {
	FS   statReadDirFS
	dirs int
}

func (c *countingFS) Open(name string) (fs.File, error)          { return c.FS.Open(name) }
func (c *countingFS) Stat(name string) (fs.FileInfo, error)      { return c.FS.Stat(name) }
func (c *countingFS) ReadDir(name string) ([]fs.DirEntry, error) { c.dirs++; return c.FS.ReadDir(name) }
