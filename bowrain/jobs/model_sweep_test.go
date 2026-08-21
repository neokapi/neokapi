package jobs

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sweepStored(id, text string) *venue.StoredBlock {
	return &venue.StoredBlock{Block: model.NewBlock(id, text)}
}

func sweepTestContext() *SweepContext {
	return &SweepContext{
		Profile: &coreprofile.VoiceProfile{
			ID: "prof-1", Name: "Voice", Version: 3,
			Vocabulary: coreprofile.VocabularyRules{
				CompetitorTerms: []coreprofile.TermRule{{Term: "Localizely"}},
				ForbiddenTerms:  []coreprofile.TermRule{{Term: "utilize", Replacement: "use"}},
			},
		},
		Glossary: map[string]string{"save": "enregistrer"},
		DNT:      []string{"Kapiflow"},
	}
}

func TestDeriveSweepFixtures_TrapClassificationAndDeterminism(t *testing.T) {
	sc := sweepTestContext()
	blocks := []*venue.StoredBlock{
		sweepStored("b3", "Just plain prose with no traps at all"),
		sweepStored("b1", "Save your work before leaving"),        // glossary trap
		sweepStored("b2", "Better than Localizely in every test"), // voice trap (competitor in source)
		sweepStored("b4", "Open Kapiflow and continue"),           // dnt trap
		sweepStored("b5", "Please utilize the sidebar"),           // voice trap (forbidden in source)
	}

	fixtures := DeriveSweepFixtures(blocks, sc)
	require.Len(t, fixtures, 4)

	byID := map[string]SweepFixture{}
	for _, f := range fixtures {
		byID[f.BlockID] = f
	}
	assert.Equal(t, "glossary", byID["b1"].Trap)
	assert.Equal(t, "voice", byID["b2"].Trap)
	assert.Equal(t, "dnt", byID["b4"].Trap)
	assert.Equal(t, "voice", byID["b5"].Trap)
	assert.NotContains(t, byID, "b3", "trapless prose must not be selected")

	// Deterministic: shuffled input yields the identical fixture set.
	shuffled := []*venue.StoredBlock{blocks[4], blocks[2], blocks[0], blocks[3], blocks[1]}
	again := DeriveSweepFixtures(shuffled, sc)
	assert.Equal(t, fixtures, again)
}

func TestDeriveSweepFixtures_CapsAtBudget(t *testing.T) {
	sc := sweepTestContext()
	var blocks []*venue.StoredBlock
	for i := range 40 {
		blocks = append(blocks, sweepStored(
			// Two-letter IDs so lexicographic block-ID order is stable.
			string(rune('a'+i/26))+string(rune('a'+i%26)),
			"Save item now")) // all glossary traps
	}
	fixtures := DeriveSweepFixtures(blocks, sc)
	assert.Len(t, fixtures, maxSweepFixtures,
		"a project rich in one trap kind refills spare capacity up to the budget")
	for _, f := range fixtures {
		assert.Equal(t, "glossary", f.Trap)
	}
}

func TestDeriveSweepFixtures_QuotasBalanceMixedTraps(t *testing.T) {
	sc := sweepTestContext()
	var blocks []*venue.StoredBlock
	// 30 glossary traps with IDs sorting BEFORE the dnt traps: without the
	// balanced first pass they would crowd out every other kind.
	for i := range 30 {
		blocks = append(blocks, sweepStored(fmt.Sprintf("a-gloss-%02d", i), "Save item now"))
	}
	for i := range 6 {
		blocks = append(blocks, sweepStored(fmt.Sprintf("z-dnt-%02d", i), "Open Kapiflow now"))
	}
	fixtures := DeriveSweepFixtures(blocks, sc)
	require.Len(t, fixtures, maxSweepFixtures)
	byTrap := map[string]int{}
	for _, f := range fixtures {
		byTrap[f.Trap]++
	}
	assert.Equal(t, sweepDNTQuota, byTrap["dnt"],
		"the DNT quota is reserved even when earlier-sorting glossary traps abound")
	assert.Equal(t, maxSweepFixtures-sweepDNTQuota, byTrap["glossary"],
		"glossary refills the remaining budget")
}

func TestSweepFixtureDigest_SensitiveToContentAndContext(t *testing.T) {
	sc := sweepTestContext()
	fixtures := []SweepFixture{{BlockID: "b1", SourceText: "Save your work", Trap: "glossary"}}

	base := SweepFixtureDigest("p1", "fr", fixtures, sc)
	assert.Equal(t, base, SweepFixtureDigest("p1", "fr", fixtures, sc), "digest must be deterministic")

	// Content change → new digest.
	changed := []SweepFixture{{BlockID: "b1", SourceText: "Save your work twice", Trap: "glossary"}}
	assert.NotEqual(t, base, SweepFixtureDigest("p1", "fr", changed, sc))

	// Profile version bump → new digest.
	sc2 := sweepTestContext()
	sc2.Profile.Version = 4
	assert.NotEqual(t, base, SweepFixtureDigest("p1", "fr", fixtures, sc2))

	// Terms-derived glossary change → new digest.
	sc3 := sweepTestContext()
	sc3.Glossary["dashboard"] = "tableau de bord"
	assert.NotEqual(t, base, SweepFixtureDigest("p1", "fr", fixtures, sc3))

	// Locale is part of the key.
	assert.NotEqual(t, base, SweepFixtureDigest("p1", "de", fixtures, sc))
}

func TestScoreSweepBlocks_RealCheckToolSemantics(t *testing.T) {
	sc := sweepTestContext()
	locale := model.LocaleID("fr")

	adherent := model.NewBlock("ok", "Save your work in Kapiflow")
	adherent.SetTargetText(locale, "Enregistrer votre travail dans Kapiflow")

	termMiss := model.NewBlock("term", "Save your work")
	termMiss.SetTargetText(locale, "Sauvegarder votre travail") // mandated rendering missing

	dntMiss := model.NewBlock("dnt", "Open Kapiflow now")
	dntMiss.SetTargetText(locale, "Ouvrir Kapiflou maintenant") // product name translated

	offVoice := model.NewBlock("voice", "A better tool")
	offVoice.SetTargetText(locale, "Un meilleur outil que Localizely") // competitor term in target (critical)

	noTarget := model.NewBlock("missing", "Anything")

	score, err := scoreSweepBlocks(t.Context(),
		[]*model.Block{adherent, termMiss, dntMiss, offVoice, noTarget}, locale, sc)
	require.NoError(t, err)
	assert.Equal(t, 5, score.Total)
	assert.Equal(t, 1, score.Adherent, "only the fully adherent block passes term-check, dnt-check, and the voice bar")
	assert.InDelta(t, 0.2, score.Rate(), 1e-9)
}

func TestRecommendModel_PolicyAndTieBreak(t *testing.T) {
	rows := []*ModelSweepResult{
		{Model: "m-low-adherence", FixtureCount: 20, Adherence: 0.5, Lift: 0.9},   // below floor
		{Model: "m-few-fixtures", FixtureCount: 3, Adherence: 0.95, Lift: 0.9},    // too few fixtures
		{Model: "m-winner", FixtureCount: 12, Adherence: 0.9, Lift: 0.4},          //
		{Model: "m-lower-lift", FixtureCount: 12, Adherence: 0.99, Lift: 0.3},     //
		{Model: "a-tied-but-later", FixtureCount: 12, Adherence: 0.9, Lift: 0.4},  // ties with m-winner
		{Model: "z-tied-but-later", FixtureCount: 12, Adherence: 0.85, Lift: 0.4}, // tied lift, lower adherence
	}
	best := RecommendModel(rows)
	require.NotNil(t, best)
	assert.Equal(t, "a-tied-but-later", best.Model,
		"tie on lift+adherence breaks to lexicographic model id")

	assert.Nil(t, RecommendModel(nil))
	assert.Nil(t, RecommendModel([]*ModelSweepResult{
		{Model: "m", FixtureCount: 2, Adherence: 0.99, Lift: 0.9},
	}), "a measurement below the minimum fixture count backs no recommendation")
}

// fakeSweepStore serves canned latest-measurement rows.
type fakeSweepStore struct {
	rows []*ModelSweepResult
	err  error
}

func (f *fakeSweepStore) UpsertModelSweepResult(context.Context, *ModelSweepResult) error { return nil }
func (f *fakeSweepStore) LatestModelSweepResults(context.Context, string, string) ([]*ModelSweepResult, error) {
	return f.rows, f.err
}
func (f *fakeSweepStore) ListModelSweepResults(context.Context, string) ([]*ModelSweepResult, error) {
	return f.rows, f.err
}

// fakeSweepSettings is a canned platform-config gate.
type fakeSweepSettings struct {
	enabled bool
	models  []string
}

func (f *fakeSweepSettings) ModelSweepsEnabled() bool  { return f.enabled }
func (f *fakeSweepSettings) AIEnabledModels() []string { return f.models }
func (f *fakeSweepSettings) IsModelEnabled(id string) bool {
	return slices.Contains(f.models, id)
}

func TestSweepModelRecommender_Gates(t *testing.T) {
	fresh := []*ModelSweepResult{{
		Model: "m1", FixtureCount: 12, Adherence: 0.9, Lift: 0.4,
		MeasuredAt: time.Now().UTC().Add(-time.Hour),
	}}
	ctx := t.Context()

	// Flag off → no recommendation, regardless of data.
	r := &SweepModelRecommender{
		Store:    &fakeSweepStore{rows: fresh},
		Settings: &fakeSweepSettings{enabled: false, models: []string{"m1"}},
	}
	_, ok := r.RecommendedModel(ctx, "p1", "fr")
	assert.False(t, ok, "model_sweeps.enabled=false must gate consumption")

	// Flag on + fresh + enabled model → recommendation applies.
	r.Settings = &fakeSweepSettings{enabled: true, models: []string{"m1"}}
	got, ok := r.RecommendedModel(ctx, "p1", "fr")
	require.True(t, ok)
	assert.Equal(t, "m1", got)

	// Stale measurement → no recommendation.
	stale := []*ModelSweepResult{{
		Model: "m1", FixtureCount: 12, Adherence: 0.9, Lift: 0.4,
		MeasuredAt: time.Now().UTC().Add(-RecommendMaxAge - time.Hour),
	}}
	r.Store = &fakeSweepStore{rows: stale}
	_, ok = r.RecommendedModel(ctx, "p1", "fr")
	assert.False(t, ok, "a stale measurement must not steer live jobs")

	// Model no longer in the enabled set → no recommendation.
	r.Store = &fakeSweepStore{rows: fresh}
	r.Settings = &fakeSweepSettings{enabled: true, models: []string{"other"}}
	_, ok = r.RecommendedModel(ctx, "p1", "fr")
	assert.False(t, ok, "an offboarded model must not be recommended")
}
