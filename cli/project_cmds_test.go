package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/project"
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

	cmd := NewInitCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	// No --target-locale / --framework: the default is an on-brand content project.
	cmd.SetArgs([]string{"--dir", dir, "--name", "my-app", "--source-locale", "en"})
	require.NoError(t, cmd.Execute())

	// Recipe + state dir both exist.
	recipe := filepath.Join(dir, project.RecipeFileName)
	info, err := os.Stat(recipe)
	require.NoError(t, err)
	assert.False(t, info.IsDir())

	stateDir := filepath.Join(dir, ".kapi")
	info, err = os.Stat(stateDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// State manifest was written with the project id.
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	state, err := project.LoadState(layout)
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, "my-app", state.Project.ID)

	// Default init scaffolds an on-brand content project: source language set,
	// no target languages, a voice pack bound under defaults:, and a check
	// flow on the deterministic voice-vocabulary check. Terminology needs no
	// binding — the vocabulary lives in the project's own store.
	p, err := project.Load(recipe)
	require.NoError(t, err)
	assert.Equal(t, "en", string(p.Defaults.SourceLanguage))
	assert.Empty(t, p.Defaults.TargetLanguages)

	require.NotNil(t, p.Defaults.Voice)
	assert.Equal(t, "professional-b2b", p.Defaults.Voice.Pack)

	require.Contains(t, p.Flows, "check")
	require.NotNil(t, p.Flows["check"])
	steps := p.Flows["check"].Steps
	require.NotEmpty(t, steps)
	assert.Equal(t, "voice-vocab-check", steps[0].Tool)

	// The scaffold and the registry have to agree, because the scaffold writes a
	// project with no target languages: the flow it ships must resolve to a
	// source-only run rather than to a locale it will then demand.
	infos := flow.BuildToolInfoMap(app.ToolReg)
	assert.False(t, flow.FlowNeedsTargetLanguage(p.Flows["check"], infos),
		"the scaffolded check flow must run on the project the scaffold creates")
	assert.Nil(t, flow.ResolveFlowLocales(p.Flows["check"], infos, "en", nil),
		"no target languages and an all-monolingual chain is one source-only pass")

	// The recipe's own next-step comment names a command; it has to be one that
	// works here.
	recipeText, err := os.ReadFile(recipe)
	require.NoError(t, err)
	assert.Contains(t, string(recipeText), "'kapi check' to score them")
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
	assert.Equal(t, "json", items[0].Format.Name)
	assert.Equal(t, "lib/l10n/app_{lang}.arb", items[0].Target)
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
	// source in i18n/src/, per-locale targets in i18n/{lang}/, and voice profile +
	// terms under i18n/ — no sibling i18n-<lang>/ sprawl.
	recipe, err := project.Load(filepath.Join(dir, project.RecipeFileName))
	require.NoError(t, err)
	require.Len(t, recipe.Collections, 1)
	assert.Equal(t, "i18n/src/**/*.kbf.json", recipe.Collections[0].Path)
	assert.Equal(t, "i18n/{lang}/{path}.kbf.json", recipe.Collections[0].Target)
	require.NotNil(t, recipe.Defaults.Voice)
	assert.Equal(t, "i18n/voice.yaml", recipe.Defaults.Voice.ProfileFile)
	assert.Equal(t, "i18n/terms.json", recipe.Defaults.TermsSource)
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

// kapi init points an assistant at the voice it just bound: the content
// scaffold binds a pack, so a fresh project gets an AGENTS.md section naming
// it; an existing CLAUDE.md takes the section instead; --no-pointer and a
// scaffold without a voice write nothing.
func TestInitCmd_voicePointer(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		files map[string]string
		// wantFile is the assistant file expected to hold the section,
		// relative to the project dir; empty when none may exist.
		wantFile string
		wantOut  string
	}{
		{
			name:     "content scaffold creates AGENTS.md",
			args:     []string{"--name", "my-app"},
			wantFile: "AGENTS.md",
			wantOut:  "agents: ",
		},
		{
			name:     "an existing CLAUDE.md takes the section",
			args:     []string{"--name", "my-app"},
			files:    map[string]string{"CLAUDE.md": "# Rules\n"},
			wantFile: "CLAUDE.md",
			wantOut:  "voice pointer written",
		},
		{
			name: "--no-pointer skips it",
			args: []string{"--name", "my-app", "--no-pointer"},
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
				_, err := os.Stat(filepath.Join(dir, "AGENTS.md"))
				assert.True(t, os.IsNotExist(err), "no assistant file is created")
				assert.NotContains(t, out.String(), "agents:")
				return
			}
			body, err := os.ReadFile(filepath.Join(dir, tt.wantFile))
			require.NoError(t, err)
			assert.Contains(t, string(body), "voice, Professional B2B, is held by kapi")
			assert.Contains(t, string(body), "`kapi voice guide`")
			assert.Contains(t, out.String(), tt.wantOut)
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
	first, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)

	out := run()
	assert.Contains(t, out, "already initialized")
	assert.NotContains(t, out, "agents:", "an unchanged pointer earns no line on a re-run")
	second, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second))

	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipe, []byte("version: v1\nname: my-app\ndefaults:\n  source_language: en\n  voice:\n    pack: technical-docs\n"), 0o644))
	out = run()
	assert.Contains(t, out, "voice pointer written")
	third, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)
	assert.Contains(t, string(third), "voice, Technical Documentation, is held by kapi")
	assert.NotContains(t, string(third), "Professional B2B")
}
