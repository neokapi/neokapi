package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newAppForTest builds a minimally-wired App for exercising the
// init/snapshot/open commands. These commands don't touch the tool
// or plugin registries, so bare InitRegistries is sufficient.
func newAppForTest(t *testing.T) *App {
	t.Helper()
	app := &App{}
	app.InitRegistries()
	return app
}

func TestInitCmd_scaffoldsProject(t *testing.T) {
	app := newAppForTest(t)
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs", "guide"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Demo\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "install.md"), []byte("# Install\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "guide", "usage.md"), []byte("# Usage\n"), 0o644))

	cmd := NewInitCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--dir", dir, "--name", "my-app", "--source-locale", "en"})
	require.NoError(t, cmd.Execute())

	recipe := filepath.Join(dir, project.RecipeFileName)
	info, err := os.Stat(recipe)
	require.NoError(t, err)
	assert.False(t, info.IsDir())

	// `.kapi/` is this checkout's cache: no manifest, and a rule that keeps
	// all of it out of version control.
	stateDir := filepath.Join(dir, ".kapi")
	assert.DirExists(t, stateDir)
	assert.NoFileExists(t, filepath.Join(stateDir, "manifest.yaml"))
	rule, err := os.ReadFile(filepath.Join(stateDir, ".gitignore"))
	require.NoError(t, err)
	assert.Equal(t, project.StateGitignore, string(rule))
	assert.NotContains(t, out.String(), "state:", "the project's state does not live in .kapi/")

	// The recipe reads the content found in the tree, needs no flow to be
	// checked, and binds no voice.
	p, err := project.Load(recipe)
	require.NoError(t, err)
	assert.Equal(t, "en", string(p.Defaults.SourceLanguage))
	assert.Empty(t, p.Defaults.TargetLanguages)
	assert.Nil(t, p.Defaults.Voice, "a scaffolded project binds no voice")
	assert.Empty(t, p.Flows, "kapi check runs with no flow declared")
	var paths []string
	for _, c := range p.Collections {
		paths = append(paths, c.Path)
	}
	assert.Equal(t, []string{"README.md", "docs/**/*.md"}, paths)
	assert.Contains(t, out.String(), "docs/**/*.md")

	recipeText, err := os.ReadFile(recipe)
	require.NoError(t, err)
	assert.Contains(t, string(recipeText), "# docs/: 2 Markdown files")
	assert.NotContains(t, string(recipeText), "store.db")
}

// A re-run leaves the person's collections alone and prints what no
// collection reads yet, as lines to paste and as kapi add commands.
func TestInitCmd_rerunPrintsUncoveredContent(t *testing.T) {
	app := newAppForTest(t)
	dir := t.TempDir()
	recipe := filepath.Join(dir, project.RecipeFileName)
	const own = "version: v1\nname: mine\ncollections:\n  - path: README.md\n    format: markdown\n"
	require.NoError(t, os.WriteFile(recipe, []byte(own), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Demo\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "guides"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "guides", "intro.md"), []byte("# Intro\n"), 0o644))

	cmd := NewInitCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--dir", dir, "--agents", "none"})
	require.NoError(t, cmd.Execute())

	body, err := os.ReadFile(recipe)
	require.NoError(t, err)
	assert.Equal(t, own, string(body), "a re-run writes nothing into the recipe")
	assert.Contains(t, out.String(), `- path: "guides/*.md"`)
	assert.Contains(t, out.String(), "kapi add 'guides/*.md' --format markdown")
	assert.NotContains(t, out.String(), "kapi add 'README.md'", "a covered file is not proposed again")
}

func TestInitCmd_translationScaffold(t *testing.T) {
	app := newAppForTest(t)
	dir := t.TempDir()

	cmd := NewInitCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	// --target-locale opts into the translation scaffold.
	cmd.SetArgs([]string{"--dir", dir, "--name", "my-app", "--source-locale", "en", "--target-locale", "fr"})
	require.NoError(t, cmd.Execute())

	// Recipe loads with source/target locales populated under defaults: — the
	// schema the loader actually reads (not top-level sourceLocale). The
	// translation scaffold does not bind a voice pack.
	recipe := filepath.Join(dir, project.RecipeFileName)
	p, err := project.Load(recipe)
	require.NoError(t, err)
	assert.Equal(t, "en", string(p.Defaults.SourceLanguage))
	var targets []string
	for _, l := range p.Defaults.TargetLanguages {
		targets = append(targets, string(l))
	}
	assert.Contains(t, targets, "fr")
	assert.Nil(t, p.Defaults.Voice)
}

func TestInitCmd_framework(t *testing.T) {
	app := newAppForTest(t)
	dir := t.TempDir()

	cmd := NewInitCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--dir", dir, "--name", "demo", "--framework", "flutter", "--target-locale", "fr"})
	require.NoError(t, cmd.Execute())

	recipe := filepath.Join(dir, project.RecipeFileName)
	// The scaffolded recipe must parse and carry the framework's content mapping.
	p, err := project.Load(recipe)
	require.NoError(t, err)
	require.Len(t, p.Collections, 1)

	items := p.Collections[0].EffectiveItems()
	require.Len(t, items, 1)
	assert.Equal(t, "lib/l10n/app_en.arb", items[0].Path)
	require.NotNil(t, items[0].Format)
	assert.Equal(t, "arb", items[0].Format.Name, "ARB catalogs read with the arb format")
	assert.Equal(t, "lib/l10n/app_{lang}.arb", items[0].Target)
}

// Every built-in framework preset maps its catalogs to a format this build
// registers, with both a reader and a writer, so a recipe `kapi init
// --framework` scaffolds reads and writes its content.
func TestFrameworkPresets_MapToRegisteredFormats(t *testing.T) {
	app := newAppForTest(t)
	reg := preset.NewPresetRegistry()
	preset.RegisterBuiltins(reg)

	for _, p := range reg.ListFrameworkPresets() {
		for _, m := range p.Mappings {
			id := registry.FormatID(m.Format)
			assert.True(t, app.FormatReg.HasReader(id), "preset %s maps %s to %q, which has no reader", p.Name, m.Local, m.Format)
			assert.True(t, app.FormatReg.HasWriter(id), "preset %s maps %s to %q, which has no writer", p.Name, m.Local, m.Format)
		}
	}
}

func TestInitCmd_frameworkNeokapiI18nScaffoldsCleanLayout(t *testing.T) {
	app := newAppForTest(t)
	dir := t.TempDir()

	cmd := NewInitCmd(app)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--dir", dir, "--framework", "neokapi-i18n"})
	require.NoError(t, cmd.Execute())

	// The recipe is written and encodes the clean nested i18n/{lang} layout:
	// source in i18n/src/ and per-locale targets in i18n/{lang}/.
	recipe, err := project.Load(filepath.Join(dir, project.RecipeFileName))
	require.NoError(t, err)
	require.Len(t, recipe.Collections, 1)
	assert.Equal(t, "i18n/src/**/*.kbf.json", recipe.Collections[0].Path)
	assert.Equal(t, "i18n/{lang}/{path}.kbf.json", recipe.Collections[0].Target)
	// A voice and terms are bound by name once the project's store holds
	// them; the scaffold names no file for either.
	assert.Nil(t, recipe.Defaults.Voice)
}

func TestInitCmd_frameworkUnknown(t *testing.T) {
	app := newAppForTest(t)
	dir := t.TempDir()

	cmd := NewInitCmd(app)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--dir", dir, "--framework", "nope"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown framework")
}

func TestInitCmd_idempotentOnExistingRecipe(t *testing.T) {
	app := newAppForTest(t)
	dir := t.TempDir()
	// Pre-create the recipe file under the fixed name init uses (kapi.yaml).
	require.NoError(t, os.WriteFile(filepath.Join(dir, project.RecipeFileName), []byte("version: v1\nname: existing\n"), 0o644))

	cmd := NewInitCmd(app)
	cmd.SetArgs([]string{"--dir", dir, "--name", "existing"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	// Idempotent: re-running init on an existing project is not an error, so
	// plugin contributions (e.g. connecting to a server) can run on top of it.
	// The recipe filename is fixed, so an existing recipe is always adopted.
	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "already initialized")
}

// A scaffolded recipe carries a stable id, and the ordinary run says nothing
// about it: the recipe is where it is read.
func TestInitCmd_scaffoldWritesAStableID(t *testing.T) {
	app := newAppForTest(t)
	dir := t.TempDir()

	cmd := NewInitCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--dir", dir, "--name", "my-app"})
	require.NoError(t, cmd.Execute())

	p, err := project.Load(filepath.Join(dir, project.RecipeFileName))
	require.NoError(t, err)
	require.NoError(t, project.ValidateID(p.ID))
	assert.Equal(t, p.ID, p.Identity())
	assert.NotContains(t, out.String(), p.ID, "an ordinary init reports the recipe, not the id")
}

// --mint-id gives an existing recipe an id, prints it, and leaves the rest of
// the file as the author wrote it. Running it again finds the id already there.
func TestInitCmd_mintID(t *testing.T) {
	app := newAppForTest(t)
	dir := t.TempDir()
	recipePath := filepath.Join(dir, project.RecipeFileName)
	const recipe = "version: v1\n# the label\nname: existing\n\ndefaults:\n  source_language: en\n"
	require.NoError(t, os.WriteFile(recipePath, []byte(recipe), 0o644))

	run := func() string {
		cmd := NewInitCmd(app)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"--dir", dir, "--mint-id"})
		require.NoError(t, cmd.Execute())
		return out.String()
	}

	first := run()
	p, err := project.Load(recipePath)
	require.NoError(t, err)
	require.NoError(t, project.ValidateID(p.ID))
	assert.Equal(t, "existing", p.Name)
	assert.Contains(t, first, p.ID)
	assert.Contains(t, first, "written to the recipe")

	body, err := os.ReadFile(recipePath)
	require.NoError(t, err)
	assert.Contains(t, string(body), "# the label")
	assert.Contains(t, string(body), "name: existing")
	assert.Contains(t, string(body), "source_language: en")

	second := run()
	assert.Contains(t, second, "already set")
	after, err := os.ReadFile(recipePath)
	require.NoError(t, err)
	assert.Equal(t, string(body), string(after), "a second mint writes nothing")
}

// kapi init points an assistant at the voice the project binds: a recipe
// carrying one gets a CLAUDE.md section naming it, and an AGENTS.md already at
// the root takes the section instead. A scaffolded recipe binds no voice, so
// both scaffolds and --no-pointer write nothing.
func TestInitCmd_voicePointer(t *testing.T) {
	const packRecipe = "version: v1\nname: my-app\ndefaults:\n  source_language: en\n  voice:\n    pack: professional-b2b\n"

	tests := []struct {
		name string
		args []string
		// recipe pre-seeds kapi.yaml, so init adopts a project that already
		// binds a voice. Empty scaffolds one.
		recipe string
		files  map[string]string
		// wantFile is the assistant file expected to hold the section,
		// relative to the project dir; empty when none may exist.
		wantFile string
		wantOut  string
		// wantNotOut must be absent from stdout.
		wantNotOut string
	}{
		{
			name:       "a bound voice creates CLAUDE.md",
			args:       []string{"--name", "my-app"},
			recipe:     packRecipe,
			wantFile:   "CLAUDE.md",
			wantOut:    "agents: ",
			wantNotOut: "@AGENTS.md",
		},
		{
			name:     "an existing CLAUDE.md takes the section",
			args:     []string{"--name", "my-app"},
			recipe:   packRecipe,
			files:    map[string]string{"CLAUDE.md": "# Rules\n"},
			wantFile: "CLAUDE.md",
			wantOut:  "voice pointer written",
		},
		{
			name:     "an AGENTS.md alone takes the section and earns the import hint",
			args:     []string{"--name", "my-app"},
			recipe:   packRecipe,
			files:    map[string]string{"AGENTS.md": "# Agents\n"},
			wantFile: "AGENTS.md",
			wantOut:  "@AGENTS.md",
		},
		{
			name:   "--no-pointer skips it",
			args:   []string{"--name", "my-app", "--no-pointer"},
			recipe: packRecipe,
		},
		{
			name: "the content scaffold binds no voice and writes nothing",
			args: []string{"--name", "my-app"},
		},
		{
			name: "a translation scaffold binds no voice and writes nothing",
			args: []string{"--name", "my-app", "--target-locale", "fr"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newAppForTest(t)
			dir := t.TempDir()
			if tt.recipe != "" {
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, project.RecipeFileName), []byte(tt.recipe), 0o644))
			}
			for rel, body := range tt.files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644))
			}

			cmd := NewInitCmd(app)
			var out, errOut bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)
			cmd.SetArgs(append([]string{"--dir", dir}, tt.args...))
			require.NoError(t, cmd.Execute())
			assert.Empty(t, errOut.String(), "no warning on the happy path")

			if tt.wantFile == "" {
				for _, name := range []string{"CLAUDE.md", "AGENTS.md"} {
					_, err := os.Stat(filepath.Join(dir, name))
					assert.True(t, os.IsNotExist(err), "no %s is created", name)
				}
				assert.NotContains(t, out.String(), "agents:")
				return
			}
			body, err := os.ReadFile(filepath.Join(dir, tt.wantFile))
			require.NoError(t, err)
			assert.Contains(t, string(body), "voice, Professional B2B, is held by kapi")
			assert.Contains(t, string(body), "`kapi voice guide`")
			assert.Contains(t, out.String(), tt.wantOut)
			if tt.wantNotOut != "" {
				assert.NotContains(t, out.String(), tt.wantNotOut)
			}
			for rel, seed := range tt.files {
				assert.Contains(t, string(body), seed, "hand-written content in %s survives", rel)
			}
		})
	}
}

// A re-run on an initialized project keeps the section as it is and says
// nothing about it; a voice changed in between is reflected in place.
func TestInitCmd_voicePointerOnRerun(t *testing.T) {
	app := newAppForTest(t)
	dir := t.TempDir()
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipe, []byte("version: v1\nname: my-app\ndefaults:\n  source_language: en\n  voice:\n    pack: professional-b2b\n"), 0o644))

	run := func() string {
		cmd := NewInitCmd(app)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"--dir", dir, "--name", "my-app"})
		require.NoError(t, cmd.Execute())
		return out.String()
	}

	run()
	first, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	require.NoError(t, err)
	assert.Contains(t, string(first), "voice, Professional B2B, is held by kapi")

	out := run()
	assert.Contains(t, out, "already initialized")
	assert.NotContains(t, out, "agents:", "an unchanged pointer earns no line on a re-run")
	second, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second))

	require.NoError(t, os.WriteFile(recipe, []byte("version: v1\nname: my-app\ndefaults:\n  source_language: en\n  voice:\n    pack: technical-docs\n"), 0o644))
	out = run()
	assert.Contains(t, out, "voice pointer written")
	third, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	require.NoError(t, err)
	assert.Contains(t, string(third), "voice, Technical Documentation, is held by kapi")
	assert.NotContains(t, string(third), "Professional B2B")
}
