package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
)

// stalenessVoiceYAML is the governing profile the fixture's content is produced
// under. Editing it is what moves the context out from under a target.
const stalenessVoiceYAML = `name: Staleness Voice
version: 1
tone:
  formality: neutral
vocabulary:
  forbidden_terms:
    - term: utilize
      replacement: use
      severity: minor
`

// writeStalenessProject builds a project that binds a voice profile over one
// source file with one target locale, and returns the project root.
func writeStalenessProject(t *testing.T) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales", "en"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales", "fr"), 0o755))

	recipe := `version: v1
name: staleness
defaults:
  source_language: en
  target_languages: [fr]
  voice:
    profile_file: voice.yaml
collections:
  - path: "locales/en/*.json"
    target: "locales/{lang}/*.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "voice.yaml"), []byte(stalenessVoiceYAML), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "en", "app.json"),
		[]byte("{\n  \"greeting\": \"Hello there\"\n}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "fr", "app.json"),
		[]byte("{\n  \"greeting\": \"Bonjour\"\n}\n"), 0o644))
	return root
}

// stalenessFixture is one prepared project: the verify units, the fingerprint a
// producer running now would stamp, and a writer for the unit states.
type stalenessFixture struct {
	app  *App
	cmd  Command
	proj *project.KapiProject
	root string
	// units are the project's (source, target, locale) units, and governing is
	// the context a producer running over them now would stamp — taken from the
	// gate's own resolver, so the fixture records what a producer records
	// rather than a guess at it.
	units     []VerifyUnit
	governing governingContext
	unitKeys  []string
}

func newStalenessFixture(t *testing.T) *stalenessFixture {
	t.Helper()
	root := writeStalenessProject(t)
	t.Chdir(root)

	a := &App{SourceLang: "en"}
	a.InitRegistries()
	cmd := NewEnvCommand(context.Background(), "staleness")
	AddProjectFlag(cmd)
	AddVerifyFlags(cmd)

	proj, err := project.LoadWithOptions(filepath.Join(root, "kapi.yaml"),
		project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)

	units, err := a.UnitsFromProject(proj, root, "")
	require.NoError(t, err)
	require.NotEmpty(t, units, "the fixture must resolve at least one unit")

	fps, err := newContextFingerprints(a, cmd, proj, root)
	require.NoError(t, err)
	governing, err := fps.at(a.unitGovernancePoint(root, units[0]), units[0].Locale)
	require.NoError(t, err)
	require.NotEmpty(t, governing.fingerprint, "a governed project must stamp a fingerprint")

	blocks, err := a.readBlocks(context.Background(), units[0].SourcePath, "en")
	require.NoError(t, err)
	var keys []string
	for _, b := range blocks {
		if b.Translatable {
			keys = append(keys, blockKey(b))
		}
	}
	require.NotEmpty(t, keys)

	require.Equal(t, "1", governing.profileVersion, "the fixture's profile declares revision 1")

	return &stalenessFixture{
		app: a, cmd: cmd, proj: proj, root: root, units: units,
		governing: governing, unitKeys: keys,
	}
}

// record writes one produced target's state per unit, stamped exactly as a
// producer running under the context in force would stamp it, except for the
// fingerprint the caller names.
func (f *stalenessFixture) record(t *testing.T, fingerprint string) {
	t.Helper()
	ctx := context.Background()
	st, err := f.app.OpenProjectState(ctx, f.root)
	require.NoError(t, err)
	for _, key := range f.unitKeys {
		require.NoError(t, st.Put(ctx, state.UnitState{
			Unit:       key,
			Variant:    model.Variant(model.LocaleID(f.units[0].Locale)),
			Scope:      f.app.DocumentScope(ctx, f.root, f.units[0].SourcePath),
			Status:     model.TargetStatusTranslated,
			TargetHash: "h-" + key,
			Origin: model.Origin{
				Kind:               model.OriginAI,
				Profile:            f.governing.profileID,
				ProfileVersion:     f.governing.profileVersion,
				ContextFingerprint: fingerprint,
			},
		}))
	}
}

func (f *stalenessFixture) run(t *testing.T) (verifyGateResult, bool) {
	t.Helper()
	gate, judged, err := f.app.verifyStaleness(f.cmd, f.proj, f.root, f.units)
	require.NoError(t, err)
	return gate, judged
}

// TestStalenessGate_ThreeOutcomes holds the gate to what a stamp can support:
// a matching stamp passes, a superseded one fails, and a missing one informs
// and never fails — content that predates the stamp is not content produced
// under the wrong context.
func TestStalenessGate_ThreeOutcomes(t *testing.T) {
	t.Run("a target stamped with the context in force passes", func(t *testing.T) {
		f := newStalenessFixture(t)
		f.record(t, f.governing.fingerprint)

		gate, judged := f.run(t)
		require.True(t, judged, "a project with produced targets is judged")
		assert.True(t, gate.Pass)
		assert.Empty(t, gate.Findings)
	})

	t.Run("a target stamped under a superseded context fails", func(t *testing.T) {
		f := newStalenessFixture(t)
		f.record(t, "a-context-that-no-longer-governs")

		gate, judged := f.run(t)
		require.True(t, judged)
		assert.False(t, gate.Pass)
		require.NotEmpty(t, gate.Findings)
		assert.Equal(t, "error", gate.Findings[0].Severity)
		assert.Contains(t, gate.Findings[0].Message, "superseded context")
		assert.Contains(t, gate.Findings[0].Suggestion, "kapi up")
	})

	t.Run("an unstamped target informs and never fails", func(t *testing.T) {
		f := newStalenessFixture(t)
		f.record(t, "")

		gate, judged := f.run(t)
		require.True(t, judged)
		assert.True(t, gate.Pass, "content that predates the stamp must not fail the gate")
		require.Len(t, gate.Findings, 1)
		assert.Equal(t, "info", gate.Findings[0].Severity)
		assert.Contains(t, gate.Findings[0].Message, "no governing-context stamp")
	})

	t.Run("a project with nothing produced is not judged at all", func(t *testing.T) {
		f := newStalenessFixture(t)

		gate, judged := f.run(t)
		assert.False(t, judged, "no produced target means no provenance to report")
		assert.Empty(t, gate.Findings)
	})
}

// TestStalenessGate_MovingTheVoiceMovesTheContext is the whole-verb case:
// content produced under one context, the voice profile edited, and the gate
// failing while naming the component that moved.
func TestStalenessGate_MovingTheVoiceMovesTheContext(t *testing.T) {
	f := newStalenessFixture(t)
	f.record(t, f.governing.fingerprint)

	gate, _ := f.run(t)
	require.True(t, gate.Pass, "the fixture starts current")

	// The voice profile gains a rule and a revision — the governing context has
	// moved, and every target written under the old one is now behind it.
	moved := `name: Staleness Voice
version: 2
tone:
  formality: neutral
vocabulary:
  forbidden_terms:
    - term: utilize
      replacement: use
      severity: minor
  competitor_terms:
    - term: Globex
      replacement: our platform
      severity: critical
`
	require.NoError(t, os.WriteFile(filepath.Join(f.root, "voice.yaml"), []byte(moved), 0o644))

	// A fresh App: the fingerprints are resolved once per run, which is what
	// makes a long convergence self-consistent, and what makes a re-check a
	// new run.
	f.app = &App{SourceLang: "en"}
	f.app.InitRegistries()

	gate, judged := f.run(t)
	require.True(t, judged)
	assert.False(t, gate.Pass, "targets produced under the old voice must fail the gate")
	require.NotEmpty(t, gate.Findings)
	assert.Contains(t, gate.Findings[0].Message, "context moved")
	assert.Contains(t, gate.Findings[0].Message, "revision 2")
	assert.Equal(t, "fr", gate.Findings[0].Locale)
}

// TestStalenessGate_JoinsTheShipGateSet: `kapi check --ship` must carry the
// gate, so superseded governance blocks ship state rather than being reported
// only where someone thought to look.
func TestStalenessGate_JoinsTheShipGateSet(t *testing.T) {
	f := newStalenessFixture(t)
	f.record(t, "a-context-that-no-longer-governs")

	a := &App{}
	cmd := NewEnvCommand(context.Background(), "check")
	AddProjectFlag(cmd)
	AddVerifyFlags(cmd)
	require.NoError(t, cmd.Flags().Set("ship", "true"))

	out, err := a.computeVerify(cmd, nil)
	require.NoError(t, err)

	gate, ok := gateByName(out, gateStaleness)
	require.True(t, ok, "the ship gate set must include the staleness gate")
	assert.False(t, gate.Pass)
	assert.False(t, out.Pass, "a superseded stamp blocks ship state")
}

// TestProducedTargets_KeepsOnlyWhatCarriesProvenance: the gate asks what
// governed a target when it was written, so it reads records that answer that —
// a producer's stamp, or a person's decision. A record carrying only the basis
// the loop wrote for a translation says which source that translation renders,
// which is a different question; reading it here would report the whole project
// as produced under no context at all.
func TestProducedTargets_KeepsOnlyWhatCarriesProvenance(t *testing.T) {
	recorded := []state.UnitState{
		{Scope: "docs/index.md", Unit: "a", Variant: model.Variant("fr")},
		{Scope: "docs/index.md", Unit: "b", Variant: model.Variant("fr"), TargetHash: "h"},
		{Scope: "docs/index.md", Unit: "c", Variant: model.Variant("fr"), Origin: model.Origin{Kind: model.OriginHuman}},
		{Scope: "docs/index.md", Unit: "d", Variant: model.Variant("fr"), TargetHash: "h",
			Decision: state.Decision{ReviewState: "approved"}},
	}
	produced := producedTargets(recorded)

	assert.Len(t, produced, 2)
	assert.NotContains(t, produced, reviewUnitKey("docs/index.md", "a", "fr"),
		"a unit the project has not translated has no provenance to be stale")
	assert.NotContains(t, produced, reviewUnitKey("docs/index.md", "b", "fr"),
		"a basis the loop recorded is not a governance stamp")
	assert.Contains(t, produced, reviewUnitKey("docs/index.md", "c", "fr"))
	assert.Contains(t, produced, reviewUnitKey("docs/index.md", "d", "fr"))
}

// TestMovedComponent_AttributesTheDivergence: the fingerprint folds voice and
// terminology into one hash on purpose, so what separates them is the profile
// stamp beside it.
func TestMovedComponent_AttributesTheDivergence(t *testing.T) {
	tests := []struct {
		name    string
		stamped model.Origin
		want    governingContext
		moved   string
		detail  string
	}{
		{
			name:    "a different profile is the context component",
			stamped: model.Origin{Profile: "old", ProfileVersion: "1"},
			want:    governingContext{profileID: "new", profileVersion: "1"},
			moved:   "context",
			detail:  "the governing voice profile is now new, was old",
		},
		{
			name:    "a new revision of one profile is the context component",
			stamped: model.Origin{Profile: "brand", ProfileVersion: "1"},
			want:    governingContext{profileID: "brand", profileVersion: "4"},
			moved:   "context",
			detail:  "voice profile brand is now revision 4, was 1",
		},
		{
			name:    "an unchanged profile stamp leaves terminology as what moved",
			stamped: model.Origin{Profile: "brand", ProfileVersion: "2"},
			want:    governingContext{profileID: "brand", profileVersion: "2"},
			moved:   "terms",
			detail:  "the terminology in force has changed since they were written",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			moved, detail := movedComponent(tt.stamped, tt.want)
			assert.Equal(t, tt.moved, string(moved))
			assert.Equal(t, tt.detail, detail)
		})
	}
}
