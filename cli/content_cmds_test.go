package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAddRm_LocalRecipeEditing verifies the core `kapi add`/`kapi rm` commands
// edit the local .kapi recipe's content collections / exclude list, and — the
// key boundary property — preserve a platform `bowrain:` block (unknown to the
// framework, round-tripped via Extras) across the save.
func TestAddRm_LocalRecipeEditing(t *testing.T) {
	a := processOnlyApp(t)

	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := filepath.Join(real, "app.kapi")

	const initial = `version: v1
name: AddRmTest
defaults:
  source_language: en-US
  target_languages:
    - fr-FR
bowrain:
  url: https://example.test
collections:
  - path: existing/*.json
    format:
      name: json
`
	require.NoError(t, os.WriteFile(recipe, []byte(initial), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(real, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(real, "src", "page.html"), []byte("<p>hi</p>"), 0o644))

	run := func(cmd *cobra.Command, patterns ...string) (string, error) {
		cmd.SetArgs(append(patterns, "--project", recipe))
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		err := cmd.Execute()
		return out.String(), err
	}

	// add: a new pattern, format auto-detected from the .html extension.
	out, err := run(NewAddCmd(a), "src/**/*.html")
	require.NoError(t, err)
	assert.Contains(t, out, "src/**/*.html")

	// add: an already-tracked pattern is skipped.
	out, err = run(NewAddCmd(a), "existing/*.json")
	require.NoError(t, err)
	assert.Contains(t, out, "Already tracked")

	// The recipe now tracks the new pattern with the detected format, AND the
	// platform bowrain: block survived the save (Extras round-trip).
	raw, err := os.ReadFile(recipe)
	require.NoError(t, err)
	s := string(raw)
	assert.Contains(t, s, "src/**/*.html")
	assert.Contains(t, s, "html") // detected format recorded
	assert.Contains(t, s, "bowrain:")
	assert.Contains(t, s, "https://example.test")

	// rm: a tracked bare entry removes the mapping; bowrain: still preserved.
	out, err = run(NewRmCmd(a), "src/**/*.html")
	require.NoError(t, err)
	assert.Contains(t, out, "Removed")
	raw, err = os.ReadFile(recipe)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "src/**/*.html")
	assert.Contains(t, string(raw), "bowrain:")

	// rm: a non-tracked pattern is added to the exclude list.
	out, err = run(NewRmCmd(a), "legacy/*.md")
	require.NoError(t, err)
	assert.Contains(t, out, "Excluded")
	raw, err = os.ReadFile(recipe)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "legacy/*.md")

	// No project found is an actionable error.
	noProj := NewAddCmd(a)
	noProj.SetArgs([]string{"x/*.json", "--project", filepath.Join(real, "missing.kapi")})
	noProj.SetOut(&bytes.Buffer{})
	noProj.SetErr(&bytes.Buffer{})
	require.Error(t, noProj.Execute())
}

// The `target:` a documented recipe shows has to be reachable from the command
// the get-started page routes a newcomer through, or it is only reachable by
// hand-editing the recipe — and a collection with no target is the one that
// can be made to feed itself.
func TestAdd_DeclaresACollectionTarget(t *testing.T) {
	a := processOnlyApp(t)
	real, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	recipe := filepath.Join(real, "app.kapi")

	const initial = `version: v1
name: TargetTest
defaults:
  source_language: en
  target_languages:
    - fr
collections: []
`
	require.NoError(t, os.WriteFile(recipe, []byte(initial), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(real, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(real, "docs", "index.md"), []byte("# Hi\n"), 0o644))

	add := func(args ...string) (string, error) {
		cmd := NewAddCmd(a)
		cmd.SetArgs(append(args, "--project", recipe))
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		err := cmd.Execute()
		return out.String(), err
	}

	out, err := add("docs/**/*.md", "--target", "translated/{lang}/{path}.md")
	require.NoError(t, err)
	assert.Contains(t, out, "translated/{lang}/{path}.md", "the destination is reported, not silently recorded")

	proj, err := coreproj.Load(recipe)
	require.NoError(t, err)
	require.Len(t, proj.Collections, 1)
	assert.Equal(t, "translated/{lang}/{path}.md", proj.Collections[0].Target)
}

// A target inside the pattern that produced it is the self-feeding shape: the
// glob re-tracks the output as source and the project doubles on every run.
// Refused where it is authored, not discovered later as a file count.
func TestAdd_RefusesATargetInsideItsOwnCollection(t *testing.T) {
	a := processOnlyApp(t)
	real, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	recipe := filepath.Join(real, "app.kapi")

	const initial = `version: v1
name: SelfFeeding
defaults:
  source_language: en
  target_languages:
    - fr
collections: []
`
	require.NoError(t, os.WriteFile(recipe, []byte(initial), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(real, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(real, "docs", "index.md"), []byte("# Hi\n"), 0o644))

	cmd := NewAddCmd(a)
	cmd.SetArgs([]string{"docs/**/*.md", "--target", "docs/{lang}/{path}.md", "--project", recipe})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err = cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "docs/**/*.md")
	assert.Contains(t, err.Error(), "double")

	raw, err := os.ReadFile(recipe)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "docs/**/*.md", "a refused add writes nothing")
}

// --target on a pattern the project already tracks is a request about that
// entry, never one to drop. It sets the target an entry has none of, and
// refuses, with how to change it, where the entry already declares a
// different one. A file belongs to the first entry in the recipe whose
// pattern matches it, so the target applies to the files the entry claims:
// the add counts those, names the rest with the entry that claims each, and
// refuses an entry that would claim none.
func TestAdd_TargetOnATrackedPattern(t *testing.T) {
	const target = "{lang}/{name}.{ext}"
	tests := []struct {
		name        string
		collections string
		files       []string // files beside the recipe; guide.md when empty
		args        []string // the add's arguments before --target
		target      string   // --target; the const target when empty
		wantErr     []string // substrings of the refusal; empty: accepted
		wantOut     []string // lines the output carries
		wantTarget  func(*coreproj.KapiProject) string
		wantTo      string // the target stored; target when empty
		wantEntries int    // collections after the add; 1 when zero
	}{
		{
			name:        "a bare entry with no target gets the target",
			collections: "  - path: guide.md\n",
			args:        []string{"guide.md"},
			wantOut:     []string{"Set target for guide.md → " + target + ": 1 file(s)"},
			wantTarget:  func(p *coreproj.KapiProject) string { return p.Collections[0].Target },
		},
		{
			name:        "an item in a named collection with no target gets the target",
			collections: "  - name: docs\n    content:\n      - path: guide.md\n",
			args:        []string{"guide.md"},
			wantOut:     []string{"Set target for guide.md → " + target + ": 1 file(s)"},
			wantTarget:  func(p *coreproj.KapiProject) string { return p.Collections[0].Content[0].Target },
		},
		{
			name:        "the same target again is already tracked",
			collections: "  - path: guide.md\n    target: '" + target + "'\n",
			args:        []string{"guide.md"},
			wantOut:     []string{"Already tracked: guide.md"},
			wantTarget:  func(p *coreproj.KapiProject) string { return p.Collections[0].Target },
		},
		{
			name:        "a different target is refused",
			collections: "  - path: guide.md\n    target: 'out/{lang}/{name}.{ext}'\n",
			args:        []string{"guide.md"},
			wantErr:     []string{"guide.md", "out/{lang}/{name}.{ext}", "kapi rm guide.md"},
		},
		{
			name:        "a new pattern whose files an earlier glob claims is refused",
			collections: "  - path: '*.md'\n",
			args:        []string{"guide.md"},
			wantErr:     []string{`"guide.md"`, `tracked first by "*.md"`, `kapi add "*.md" --target`},
		},
		{
			name:        "a tracked pattern whose files an earlier glob claims is refused",
			collections: "  - path: '*.md'\n  - path: guide.md\n",
			args:        []string{"guide.md"},
			wantErr:     []string{`"guide.md"`, `tracked first by "*.md"`},
		},
		{
			name:        "a new glob over files earlier entries claim takes the rest",
			collections: "  - path: b.md\n  - path: guide.md\n",
			files:       []string{"b.md", "guide.md", "c.md"},
			args:        []string{"*.md"},
			wantOut: []string{
				"Added *.md (markdown) → " + target + ": 1 file(s)",
				"  b.md is tracked first by b.md, so this target does not apply to it",
				"  guide.md is tracked first by guide.md, so this target does not apply to it",
			},
			wantTarget:  func(p *coreproj.KapiProject) string { return p.Collections[2].Target },
			wantEntries: 3,
		},
		{
			name:        "a tracked glob behind earlier entries counts what it claims",
			collections: "  - path: b.md\n  - path: guide.md\n  - path: '*.md'\n",
			files:       []string{"b.md", "guide.md", "c.md"},
			args:        []string{"*.md"},
			wantOut: []string{
				"Set target for *.md → " + target + ": 1 file(s)",
				"  b.md is tracked first by b.md, so this target does not apply to it",
			},
			wantTarget:  func(p *coreproj.KapiProject) string { return p.Collections[2].Target },
			wantEntries: 3,
		},
		{
			name:        "a target under a collection's base is kept relative to it",
			collections: "  - name: site\n    base: site\n    content:\n      - path: docs/*.md\n",
			files:       []string{"site/docs/a.md"},
			args:        []string{"site/docs/*.md"},
			target:      "site/out/{lang}/{name}.{ext}",
			wantOut:     []string{"Set target for site/docs/*.md → site/out/{lang}/{name}.{ext}: 1 file(s)"},
			wantTarget:  func(p *coreproj.KapiProject) string { return p.Collections[0].Content[0].Target },
			wantTo:      "out/{lang}/{name}.{ext}",
		},
		{
			name:        "a new item under a collection's base keeps its target relative to it",
			collections: "  - name: site\n    base: site\n    content:\n      - path: docs/*.md\n",
			files:       []string{"site/docs/a.md", "site/blog/b.md"},
			args:        []string{"site/blog/*.md", "--name", "site"},
			target:      "site/out/{lang}/{name}.{ext}",
			wantOut:     []string{"Added site/blog/*.md (markdown) → site/out/{lang}/{name}.{ext}: 1 file(s)"},
			wantTarget:  func(p *coreproj.KapiProject) string { return p.Collections[0].Content[1].Target },
			wantTo:      "out/{lang}/{name}.{ext}",
		},
		{
			name:        "a target outside a collection's base is refused",
			collections: "  - name: site\n    base: site\n    content:\n      - path: docs/*.md\n",
			files:       []string{"site/docs/a.md"},
			args:        []string{"site/docs/*.md"},
			target:      "out/{lang}/{name}.{ext}",
			wantErr:     []string{`"site"`, `"out/{lang}/{name}.{ext}"`, "site/"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := processOnlyApp(t)
			real, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			recipe := filepath.Join(real, "kapi.yaml")
			initial := "version: v1\nname: Tracked\ndefaults:\n  source_language: en\n  target_languages:\n    - fr\ncollections:\n" + tc.collections
			require.NoError(t, os.WriteFile(recipe, []byte(initial), 0o644))
			files := tc.files
			if len(files) == 0 {
				files = []string{"guide.md"}
			}
			for _, f := range files {
				require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(real, f)), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(real, f), []byte("# Doc\n"), 0o644))
			}
			give := tc.target
			if give == "" {
				give = target
			}

			cmd := NewAddCmd(a)
			cmd.SetArgs(append(append([]string{}, tc.args...), "--target", give, "--project", recipe))
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			err = cmd.Execute()

			if len(tc.wantErr) > 0 {
				require.Error(t, err)
				for _, s := range tc.wantErr {
					assert.Contains(t, err.Error(), s)
				}
				raw, rerr := os.ReadFile(recipe)
				require.NoError(t, rerr)
				assert.Equal(t, initial, string(raw), "a refused add writes nothing")
				return
			}
			require.NoError(t, err)
			for _, line := range tc.wantOut {
				assert.Contains(t, out.String(), line+"\n")
			}
			proj, lerr := coreproj.Load(recipe)
			require.NoError(t, lerr)
			want := tc.wantTo
			if want == "" {
				want = give
			}
			assert.Equal(t, want, tc.wantTarget(proj))
			entries := tc.wantEntries
			if entries == 0 {
				entries = 1
			}
			assert.Len(t, proj.Collections, entries)
		})
	}
}

// TestCoreKapi_RefusesRecipeRequiringUnregisteredPlugin proves the boundary
// gate from the core side: a recipe that declares `requires: bowrain` (as a
// server-connected project does) is refused by plain kapi, where the bowrain
// extension is not registered — so a bowrain: project does not silently work
// without the plugin.
func TestCoreKapi_RefusesRecipeRequiringUnregisteredPlugin(t *testing.T) {
	a := processOnlyApp(t)
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := filepath.Join(real, "app.kapi")
	const yaml = `version: v1
name: NeedsBowrain
requires:
  bowrain: "*"
bowrain:
  url: https://example.test
`
	require.NoError(t, os.WriteFile(recipe, []byte(yaml), 0o644))

	cmd := NewAddCmd(a)
	cmd.SetArgs([]string{"src/*.json", "--project", recipe})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err = cmd.Execute()
	require.Error(t, err, "core kapi must refuse a recipe requiring an unregistered plugin")
	assert.Contains(t, err.Error(), "bowrain")
}

// TestLs_ListsTrackedFiles verifies core `kapi ls` lists the files the project's
// content tracks, honors a path filter, and shows block/word counts with --stats.
func TestLs_ListsTrackedFiles(t *testing.T) {
	a := processOnlyApp(t)
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := filepath.Join(real, "app.kapi")

	const yaml = `version: v1
name: LsTest
defaults:
  source_language: en-US
collections:
  - path: src/*.json
    format:
      name: json
`
	require.NoError(t, os.WriteFile(recipe, []byte(yaml), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(real, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(real, "src", "a.json"), []byte(`{"greeting":"Hello there world"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(real, "src", "b.json"), []byte(`{"bye":"Goodbye"}`), 0o644))

	run := func(args ...string) (string, error) {
		cmd := NewLsCmd(a)
		cmd.SetArgs(append(args, "--project", recipe))
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		execErr := cmd.Execute()
		return out.String(), execErr
	}

	// Plain ls lists both files with their format.
	out, err := run()
	require.NoError(t, err)
	assert.Contains(t, out, "src/a.json")
	assert.Contains(t, out, "src/b.json")
	assert.Contains(t, out, "json")
	assert.Contains(t, out, "2 file(s)")

	// A path filter narrows the listing.
	out, err = run("src/a.json")
	require.NoError(t, err)
	assert.Contains(t, out, "src/a.json")
	assert.NotContains(t, out, "src/b.json")

	// --stats adds block/word columns + a totals summary.
	out, err = run("--stats")
	require.NoError(t, err)
	assert.Contains(t, out, "BLOCKS")
	assert.Contains(t, out, "WORDS")
	assert.Contains(t, out, "blocks, ")
}
