package host

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The run path — `kapi up`, `kapi run`, `kapi translate` inside a project —
// picks its voice by partitioning the input set (groupInputsByBinding) and
// resolving one binding set per partition and locale (localeBindings.at). These
// are the two calls every run makes, in that order, so what they produce IS what
// governs a file during a run.

// governanceRunProject writes a project with two source files, three voices, and
// the recipe body the caller supplies — the fixture the run-path governance
// tests share. Returns the recipe path, the root, and the two absolute source
// paths (guide, legal).
func governanceRunProject(t *testing.T, recipe string) (recipePath, root, guide, legal string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	for path, body := range map[string]string{
		"voice.yaml":         "id: house\nname: House Style\n",
		"promo-voice.yaml":   "id: promo\nname: Promo Voice\n",
		"legal-voice.yaml":   "id: legal\nname: Legal Voice\n",
		"docs/guide.md":      "The guide.\n",
		"docs/legal/eula.md": "The terms.\n",
	} {
		full := filepath.Join(dir, filepath.FromSlash(path))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
	}

	recipePath = filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipePath, []byte(recipe), 0o644))
	return recipePath, dir,
		filepath.Join(dir, "docs", "guide.md"),
		filepath.Join(dir, "docs", "legal", "eula.md")
}

// overrideRecipeBody routes one sub-tree of a collection to a second profile
// through a content item's own `channel:`.
const overrideRecipeBody = `version: v1
name: run-governance
defaults:
  source_language: en
  target_languages: [nb]
  voice:
    profile_file: voice.yaml
profiles:
  promo:
    channels: [docs]
    voice: promo-voice.yaml
  legal:
    channels: [docs]
    voice: legal-voice.yaml
collections:
  - name: docs
    channel: promo/docs
    content:
      - path: docs/legal/**/*.md
        channel: legal/docs
      - path: docs/**/*.md
`

// TestUp_ItemChannelOverrideGovernsItsFileSeparately is the per-item channel
// made real in a run: two files of one collection are partitioned apart and each
// carries the voice bound where it sits, rather than both carrying the
// collection's.
func TestUp_ItemChannelOverrideGovernsItsFileSeparately(t *testing.T) {
	recipe, root, guide, legal := governanceRunProject(t, overrideRecipeBody)
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)

	a := &App{}
	cmd := bindingsCmd(t, recipe)

	groups, err := a.groupInputsByBinding(cmd, proj, root, []string{guide, legal})
	require.NoError(t, err)
	require.Len(t, groups, 2, "the override partitions its file out of the collection")
	bindings := a.newLocaleBindings(cmd, proj, recipe)

	byInput := map[string]string{}
	for _, g := range groups {
		b, berr := bindings.at(g.Point, "nb")
		require.NoError(t, berr)
		require.NotNil(t, b)
		require.NotNil(t, b.profile)
		for _, in := range g.Inputs {
			byInput[in] = b.profile.Name
		}
	}
	assert.Equal(t, "Promo Voice", byInput[guide], "the collection's channel governs the ordinary file")
	assert.Equal(t, "Legal Voice", byInput[legal], "the item's own channel governs the file it matches")
}

// TestUp_ExpiredProfileFallsThroughWithAVisibleNote covers both boundaries of a
// validity window and the control: a profile whose window has closed, one whose
// window has not opened, and one that bounds nothing. The first two stop
// governing and say so once; the third is untouched.
func TestUp_ExpiredProfileFallsThroughWithAVisibleNote(t *testing.T) {
	tests := []struct {
		name      string
		window    string
		wantVoice string
		wantNote  string
	}{
		{
			name:      "after valid_to the profile stops governing",
			window:    "    valid_to: 2020-01-01\n",
			wantVoice: "House Style",
			wantNote:  `governance: profile "promo" expired 2020-01-01; governing with the project default`,
		},
		{
			name:      "before valid_from it does not govern yet",
			window:    "    valid_from: 2099-01-01\n",
			wantVoice: "House Style",
			wantNote:  `governance: profile "promo" is not in force until 2099-01-01; governing with the project default`,
		},
		{
			name:      "an always-valid profile is untouched",
			window:    "",
			wantVoice: "Promo Voice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recipe, root, guide, _ := governanceRunProject(t, `version: v1
name: run-validity
defaults:
  source_language: en
  target_languages: [nb]
  voice:
    profile_file: voice.yaml
profiles:
  promo:
    channels: [docs]
    voice: promo-voice.yaml
`+tt.window+`collections:
  - name: docs
    channel: promo/docs
    content:
      - path: docs/**/*.md
`)
			proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
			require.NoError(t, err)

			a := &App{}
			cmd := bindingsCmd(t, recipe)
			var stderr bytes.Buffer
			cmd.SetErr(&stderr)

			groups, err := a.groupInputsByBinding(cmd, proj, root, []string{guide})
			require.NoError(t, err)
			require.Len(t, groups, 1)
			b, berr := a.newLocaleBindings(cmd, proj, recipe).at(groups[0].Point, "nb")
			require.NoError(t, berr)
			require.NotNil(t, b)
			require.NotNil(t, b.profile)
			assert.Equal(t, tt.wantVoice, b.profile.Name)

			if tt.wantNote == "" {
				assert.Empty(t, stderr.String(), "nothing lapsed, so nothing to report")
				return
			}
			assert.Contains(t, stderr.String(), tt.wantNote,
				"the transition must be visible, not silent")
			assert.Equal(t, 1, bytes.Count(stderr.Bytes(), []byte("governance: ")),
				"a run reports the transition once, however many files it resolves")
		})
	}
}

// TestUp_ExpiredProfileReportedOncePerRun pins the dedup across files: a
// hundred files under one lapsed profile is one line, not a hundred.
func TestUp_ExpiredProfileReportedOncePerRun(t *testing.T) {
	recipe, root, guide, legal := governanceRunProject(t, `version: v1
name: run-validity-once
defaults:
  source_language: en
  target_languages: [nb]
  voice:
    profile_file: voice.yaml
profiles:
  promo:
    channels: [docs]
    voice: promo-voice.yaml
    valid_to: 2020-01-01
collections:
  - name: docs
    channel: promo/docs
    content:
      - path: docs/**/*.md
`)
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)

	a := &App{}
	cmd := bindingsCmd(t, recipe)
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	groups, err := a.groupInputsByBinding(cmd, proj, root, []string{guide, legal})
	require.NoError(t, err)
	assert.Len(t, groups, 1, "both files fell through to the same default point")
	assert.Equal(t, 1, bytes.Count(stderr.Bytes(), []byte("governance: ")))
}

// TestGovernanceInstant_IsFixedForTheRun is why a long convergence cannot govern
// its first and last file differently for no visible reason.
func TestGovernanceInstant_IsFixedForTheRun(t *testing.T) {
	a := &App{}
	first := a.GovernanceInstant()
	assert.Equal(t, first, a.GovernanceInstant(), "one run, one instant")

	worker := a.convergeWorker("nb", nil)
	assert.Equal(t, first, worker.GovernanceInstant(), "and every locale of a pass shares it")
}

// TestUp_ItemChannelOverrideSurvivesAConvergeRun drives the real convergence
// verb over the fixture: the partition the override creates is a partition of a
// working run, not a split that leaves half the content unprocessed.
func TestUp_ItemChannelOverrideSurvivesAConvergeRun(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src", "legal"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "en.json"),
		[]byte(`{"greeting":"Hello world"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "legal", "en.json"),
		[]byte(`{"terms":"Read the terms"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "voice.yaml"),
		[]byte("id: house\nname: House Style\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "legal-voice.yaml"),
		[]byte("id: legal\nname: Legal Voice\n"), 0o644))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "ConvergeGovernance",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr"},
			Flow:            "translate",
			SourceGate:      string(model.SourceGateNone),
			Voice:           &project.VoiceBinding{ProfileFile: "voice.yaml"},
		},
		Profiles: map[string]project.Profile{
			"house": {Channels: []project.Channel{{ID: "app"}}},
			"legal": {
				Channels: []project.Channel{{ID: "app"}},
				Voice:    &project.VoiceBinding{ProfileFile: "legal-voice.yaml"},
			},
		},
		Collections: []project.Collection{{
			Name:    "app",
			Channel: "house/app",
			Content: []project.ContentItem{
				{Path: "src/legal/en.json", Target: "src/legal/{lang}.json", Channel: "legal/app"},
				{Path: "src/en.json", Target: "src/{lang}.json"},
			},
		}},
		Flows: map[string]*flow.StepsSpec{
			"translate": {Steps: []flow.FlowStep{
				{Tool: "translate", Config: map[string]any{"provider": "demo"}},
			}},
		},
	}
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, project.StateDirName), 0o755))
	t.Chdir(dir)

	a := &App{}
	a.InitRegistries()
	a.SourceLang = "en"
	cmd := NewEnvCommand(t.Context(), "up")
	a.AddFlowRunFlags(cmd)

	loaded, err := project.Load(recipe)
	require.NoError(t, err)
	require.NoError(t, a.RunDefaultFlowConverge(cmd, loaded, recipe, ConvergeOptions{
		UntilGate: true, MaxPasses: 3, noExtract: true, noChecks: true,
	}))

	for _, rel := range []string{"src/fr.json", "src/legal/fr.json"} {
		body, rerr := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		require.NoError(t, rerr, "%s must have been written", rel)
		assert.Contains(t, string(body), "⟦fr⟧", "%s was translated in its own partition", rel)
	}
}
