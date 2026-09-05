package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/kmb"
)

// The governing-context fingerprint on the decision record.
//
// A decision records what governed the answer it blesses, the absorber reads
// that record for a target whose format keeps no provenance, and a re-seed
// carries the bundle's fingerprint back onto the record. Block identity
// (ContextHash) is untouched by all of it.

// governingNow resolves the fingerprint a producer running over the project's
// first unit would stamp now, through the resolver the gate and the decision
// writer share.
func governingNow(t *testing.T, a *App, recipe, root string) string {
	t.Helper()
	cmd := NewEnvCommand(context.Background(), "test")
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", recipe))
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)
	units, err := a.UnitsFromProject(proj, root, "")
	require.NoError(t, err)
	require.NotEmpty(t, units)
	fps, err := newContextFingerprints(a, cmd, proj, root)
	require.NoError(t, err)
	defer fps.close()
	g, err := fps.at(a.unitGovernancePoint(root, units[0]), units[0].Locale)
	require.NoError(t, err)
	require.NotEmpty(t, g.fingerprint, "a governed project stamps a fingerprint")
	return g.fingerprint
}

// TestApplyReviewDecision_RecordsTheGoverningContext: an approval records the
// context it was made under, beside the block identity it leaves alone, and the
// same approval under a moved context is a new decision.
func TestApplyReviewDecision_RecordsTheGoverningContext(t *testing.T) {
	root := writeStalenessProject(t)
	recipe := filepath.Join(root, "kapi.yaml")
	a := &App{}
	a.InitRegistries()
	ctx := context.Background()
	ref := ReviewUnitRef{File: "locales/fr/app.json", Key: "greeting", Locale: "fr"}

	// A record the loop left: identity signals and nothing decided.
	st, err := a.OpenProjectState(ctx, root)
	require.NoError(t, err)
	scope := a.DocumentScope(ctx, root, filepath.Join(root, "locales", "en", "app.json"))
	key := state.Key{Scope: scope, Unit: "greeting", Variant: model.Variant("fr")}
	require.NoError(t, st.Record(ctx, state.UnitState{
		Unit: "greeting", Variant: model.Variant("fr"), Scope: scope,
		Status: model.TargetStatusTranslated, ContextHash: "ctx-identity",
	}))

	want := governingNow(t, a, recipe, root)

	changed, err := a.ApplyReviewDecision(ctx, recipe, "en", ref, "approved", "")
	require.NoError(t, err)
	require.True(t, changed)

	got, ok := st.Get(ctx, key)
	require.True(t, ok)
	assert.Equal(t, want, got.GoverningFingerprint, "the decision records the context it was made under")
	assert.Equal(t, "ctx-identity", got.ContextHash, "block identity rides along untouched")
	assert.NotEqual(t, got.ContextHash, got.GoverningFingerprint, "the two are different quantities")
	assert.Equal(t, "approved", got.Decision.ReviewState)

	changed, err = a.ApplyReviewDecision(ctx, recipe, "en", ref, "approved", "")
	require.NoError(t, err)
	assert.False(t, changed, "the same decision under the same context is already recorded")

	// The voice moves. The same verdict on the same pairing is now a decision
	// under a different context, and is recorded again.
	require.NoError(t, os.WriteFile(filepath.Join(root, "voice.yaml"), []byte(`name: Staleness Voice
version: 2
tone:
  formality: formal
`), 0o644))
	moved := governingNow(t, a, recipe, root)
	require.NotEqual(t, want, moved)

	changed, err = a.ApplyReviewDecision(ctx, recipe, "en", ref, "approved", "")
	require.NoError(t, err)
	assert.True(t, changed, "a decision re-made under a moved context is a new decision")
	got, ok = st.Get(ctx, key)
	require.True(t, ok)
	assert.Equal(t, moved, got.GoverningFingerprint)
	assert.Equal(t, "ctx-identity", got.ContextHash)
}

// TestGoverningFingerprintOf_TheTargetsOwnStampWins pins the ladder the
// absorber reads: the stamp on the bytes in front of it first, the record for
// the unit next, and nothing when the record describes a different translation.
func TestGoverningFingerprintOf_TheTargetsOwnStampWins(t *testing.T) {
	const scope = "d-doc"
	stamped := &model.Block{ID: "greeting", Name: "greeting", Translatable: true}
	stamped.SetSourceText("Hello")
	stamped.SetTarget("nb", &model.Target{Runs: []model.Run{model.TextR("Hei")}, Origin: model.Origin{ContextFingerprint: "fp-file"}})
	bare := &model.Block{ID: "greeting", Name: "greeting", Translatable: true}
	bare.SetSourceText("Hello")
	bare.SetTargetText("nb", "Hei")

	recorded := reviewedIndex{byUnit: map[string]reviewedEntry{
		reviewUnitKey(scope, "greeting", "nb"): {
			status: model.TargetStatusReviewed, decided: true,
			targetHash: state.TargetHash("Hei"), governing: "fp-record",
		},
	}}
	other := reviewedIndex{byUnit: map[string]reviewedEntry{
		reviewUnitKey(scope, "greeting", "nb"): {
			status: model.TargetStatusReviewed, decided: true,
			targetHash: state.TargetHash("Hallo"), governing: "fp-record",
		},
	}}

	tests := []struct {
		name  string
		block *model.Block
		index reviewedIndex
		want  string
	}{
		{name: "the format's stamp wins", block: stamped, index: recorded, want: "fp-file"},
		{name: "the record answers when the format keeps none", block: bare, index: recorded, want: "fp-record"},
		{name: "a record about another translation says nothing", block: bare, index: other, want: ""},
		{name: "no record reads as ungoverned", block: bare, index: reviewedIndex{}, want: ""},
		{name: "no block reads as ungoverned", block: nil, index: recorded, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, governingFingerprintOf(tc.block, "nb", tc.index, scope))
		})
	}
}

// TestAbsorbCommittedRecord_ReadsTheGoverningContextFromTheRecord: a JSON
// catalog and a .properties file keep no provenance, so what governed their
// translations is read from the record for each unit: the decision's own
// fingerprint, the producer's stamp on a basis written before the field
// existed, and nothing for a translation the record does not describe.
func TestAbsorbCommittedRecord_ReadsTheGoverningContextFromTheRecord(t *testing.T) {
	tests := []struct {
		ext            string
		source, target string
	}{
		{
			ext:    ".json",
			source: `{"greeting":"Hello world","farewell":"Goodbye","ok":"All set"}`,
			target: `{"greeting":"Hei verden","farewell":"Ha det","ok":"Alt klart"}`,
		},
		{
			ext:    ".properties",
			source: "greeting=Hello world\nfarewell=Goodbye\nok=All set\n",
			target: "greeting=Hei verden\nfarewell=Ha det\nok=Alt klart\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.ext, func(t *testing.T) {
			a, root, recipe := newRecordProject(t, tc.ext)
			writeDoc(t, root, "src/en"+tc.ext, tc.source)
			writeDoc(t, root, "src/nb"+tc.ext, tc.target)
			ctx := context.Background()

			st, err := a.OpenProjectState(ctx, root)
			require.NoError(t, err)
			scope := a.DocumentScope(ctx, root, filepath.Join(root, "src", "en"+tc.ext))
			row := func(unit, source, target string) state.UnitState {
				return state.UnitState{
					Unit: unit, Variant: model.Variant("nb"), Scope: scope,
					TargetHash: state.TargetHash(target), ContentHash: state.SourceHash(source),
				}
			}
			// A decision, carrying the context it was made under.
			decided := row("greeting", "Hello world", "Hei verden")
			decided.Status = model.TargetStatusReviewed
			decided.Decision = state.Decision{ReviewState: "approved"}
			decided.GoverningFingerprint = "fp-decision"
			require.NoError(t, st.Put(ctx, decided))
			// A basis the loop wrote before the field existed: only the
			// producer's stamp says what governed it.
			produced := row("farewell", "Goodbye", "Ha det")
			produced.Status = model.TargetStatusTranslated
			produced.Origin = model.Origin{Kind: model.OriginAI, ContextFingerprint: "fp-produced"}
			require.NoError(t, st.Record(ctx, produced))
			// A decision about a translation somebody has since rewritten.
			rewritten := row("ok", "All set", "Alt i orden")
			rewritten.Status = model.TargetStatusReviewed
			rewritten.Decision = state.Decision{ReviewState: "approved"}
			rewritten.GoverningFingerprint = "fp-stale"
			require.NoError(t, st.Put(ctx, rewritten))

			res, err := a.SeedProjectContext(ctx, recipe)
			require.NoError(t, err)
			require.Equal(t, 3, res.Record.Learned)

			entries := storeEntries(t, a, root)
			fingerprint := func(source string) string {
				e, ok := entries[source]
				require.True(t, ok, "the store holds a pair for %q", source)
				require.Len(t, e.Origins, 1)
				return e.Origins[0].ContextFingerprint
			}
			assert.Equal(t, "fp-decision", fingerprint("Hello world"), "the decision's own fingerprint")
			assert.Equal(t, "fp-produced", fingerprint("Goodbye"), "the producer's stamp on a record without the field")
			assert.Empty(t, fingerprint("All set"), "a record about a different translation vouches for nothing")
		})
	}
}

// writeGoverningBundle writes the project's committed content-memory bundle with
// one entry per pair, each answering the given unit and carrying the given
// governing fingerprint, as the absorber writes them.
func writeGoverningBundle(t *testing.T, root string, fingerprint string, pairs map[string]string) {
	t.Helper()
	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var entries []memory.Entry
	for unit, target := range pairs {
		entries = append(entries, memory.Entry{
			ID:          "record:" + unit,
			HintSrcLang: "en",
			Unit:        unit,
			Variants: map[model.LocaleID][]model.Run{
				"en": {model.TextR("source of " + unit)},
				"nb": {model.TextR(target)},
			},
			Origins: []memory.Origin{{
				Source: recordOriginSource, Key: "src/nb.json", Reference: "src/en.json",
				AddedAt: stamp, AddedBy: "kapi-up", ContextFingerprint: fingerprint,
			}},
			CreatedAt: stamp,
			UpdatedAt: stamp,
		})
	}
	data, err := kmb.Marshal(kmb.FromModel(entries, nil))
	require.NoError(t, err)
	dir := project.LayoutAt(root).MemoryDir()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, kmb.ConventionalName), data, 0o644))
}

// TestSeedProjectContext_CarriesTheBundlesGoverningContextOntoTheRecord: a
// re-seed writes the bundle's fingerprint onto the record row for the unit it
// answers, when the row holds none and is about that translation, and makes
// the row durable in the committed shards.
func TestSeedProjectContext_CarriesTheBundlesGoverningContextOntoTheRecord(t *testing.T) {
	a, root, recipe := newRecordProject(t, ".json")
	writeDoc(t, root, "src/en.json", `{"greeting":"source of greeting","farewell":"source of farewell","ok":"source of ok"}`)
	ctx := context.Background()

	st, err := a.OpenProjectState(ctx, root)
	require.NoError(t, err)
	scope := a.DocumentScope(ctx, root, filepath.Join(root, "src", "en.json"))
	record := func(unit, target, fingerprint string) {
		require.NoError(t, st.Record(ctx, state.UnitState{
			Unit: unit, Variant: model.Variant("nb"), Scope: scope,
			Status: model.TargetStatusReviewed, Decision: state.Decision{ReviewState: "approved"},
			TargetHash: state.TargetHash(target), ContentHash: state.SourceHash("source of " + unit),
			GoverningFingerprint: fingerprint,
		}))
	}
	record("greeting", "Hei verden", "") // the bundle's answer, no fingerprint yet
	record("farewell", "Ha det", "")     // a different answer from the bundle's
	record("ok", "Alt klart", "fp-own")  // already carries its own
	writeGoverningBundle(t, root, "fp-bundle", map[string]string{
		"greeting": "Hei verden",
		"farewell": "Adjø",
		"ok":       "Alt klart",
	})

	_, err = a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)

	get := func(unit string) state.UnitState {
		u, ok := st.Get(ctx, state.Key{Scope: scope, Unit: unit, Variant: model.Variant("nb")})
		require.True(t, ok, "the row for %s", unit)
		return u
	}
	assert.Equal(t, "fp-bundle", get("greeting").GoverningFingerprint, "the bundle's fingerprint reaches the row for its answer")
	assert.Empty(t, get("farewell").GoverningFingerprint, "a row about a different translation is left alone")
	assert.Equal(t, "fp-own", get("ok").GoverningFingerprint, "the record's own fingerprint stands")
	assert.Equal(t, "approved", get("greeting").Decision.ReviewState, "the decision itself is untouched")

	committed, err := state.ReadCommitted(project.LayoutAt(root).UnitStateDir())
	require.NoError(t, err)
	var durable string
	for _, u := range committed {
		if u.Unit == "greeting" {
			durable = u.GoverningFingerprint
		}
	}
	assert.Equal(t, "fp-bundle", durable, "the restored fingerprint reaches the committed record")
}
