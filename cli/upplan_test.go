package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// planFixture is convergeFixture with the flow a plan is normally read for:
// recycle, then draft the remainder through a provider — the shape of the
// built-in default. The plan prices the work the CONFIGURED flow would do, so a
// fixture whose flow neither recycles nor calls out has no leverage to report
// and no bill to quote; those cases have their own tests below (#1866).
func planFixture(t *testing.T, targets []model.LocaleID, shipGate gate.Gate) (recipe, root string) {
	t.Helper()
	recipe, root = convergeFixture(t, targets, shipGate)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.Flow = "converge"
	proj.Flows["converge"] = &flow.StepsSpec{Steps: []flow.FlowStep{
		{Tool: "recycle"},
		{Tool: "translate", Config: map[string]any{"provider": "demo", "skipMatched": true}},
	}}
	require.NoError(t, project.Save(recipe, proj))
	return recipe, root
}

// seedPlanMemory writes a project content memory with one exact entry for the fixture's
// "Hello, world." source (a.json), leaving b.json's "Goodbye." uncovered.
func seedPlanMemory(t *testing.T, root string) {
	t.Helper()
	seedPlanEntry(t, root, "seed-1", "Hello, world.", "Hei, verden.")
}

// seedPlanEntry writes one exact en-US → nb-NO pair into the project content
// memory.
func seedPlanEntry(t *testing.T, root, id, source, target string) {
	t.Helper()
	db, err := projectdb.Open(t.Context(), project.Layout{
		Root: root, StateDir: filepath.Join(root, project.StateDirName),
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	now := time.Now().UTC()
	require.NoError(t, db.Memory().Add(t.Context(), memory.Entry{
		ID: id,
		Variants: map[model.LocaleID][]model.Run{
			"en-US": {{Text: &model.TextRun{Text: source}}},
			"nb-NO": {{Text: &model.TextRun{Text: target}}},
		},
		HintSrcLang: "en-US",
		CreatedAt:   now,
		UpdatedAt:   now,
	}))
}

// TestUpPlan_MemoryLeverageAndTokenEstimate: --plan reports, per locale, the units
// missing a target split into exact content-memory hits and remaining AI work with a
// chars/4 token estimate — and writes nothing.
func TestUpPlan_MemoryLeverageAndTokenEstimate(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := planFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})
	seedPlanMemory(t, root)

	out, err := runUp(t, a, recipe, "--plan", "--json")
	require.NoError(t, err, out)

	var plan UpPlanOutput
	require.NoError(t, json.Unmarshal([]byte(out), &plan), "plan --json must be valid JSON: %s", out)
	require.Len(t, plan.Scopes, 1)
	s := plan.Scopes[0]
	assert.Equal(t, "nb-NO", s.Locale)
	assert.Equal(t, 2, s.MissingTarget, "both fixture units lack targets")
	assert.Equal(t, 1, s.MemoryExact, `"Hello, world." has an exact content-memory hit`)
	assert.Equal(t, 1, s.AIRemaining, `"Goodbye." remains for AI`)
	// "Goodbye." is 8 chars → ceil(8/4) = 2 estimated tokens.
	assert.Equal(t, 2, s.TokenEstimate)
	assert.Equal(t, plan.Totals.MissingTarget, s.MissingTarget)
	assert.NotEmpty(t, plan.Note, "the estimation method is disclosed")

	// Dry run: no targets written. The store exists here only because the test
	// seeded it — see TestUpPlan_NeverCreatesTheProjectStore for the invariant.
	_, statErr := os.Stat(filepath.Join(root, "src/locales/nb-NO", "a.json"))
	assert.True(t, os.IsNotExist(statErr), "--plan must not write targets")
}

// TestUpPlan_NeverCreatesTheProjectStore is the invariant the merged store
// sharpened. Leverage used to come from a `memory.db` a plan could decline to
// open; it now comes from `.kapi/work/store.db`, and OPENING that store creates it —
// the handle runs every subsystem's migrations at open.
//
// So a plan on a fresh project must reach its answer without opening anything,
// and leave the state directory with no store in it. Otherwise `kapi up --plan`
// would be a dry run with a side effect, and the next real `up` would find a
// store it never wrote.
func TestUpPlan_NeverCreatesTheProjectStore(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := planFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})

	storePath := project.LayoutAt(root).StorePath()
	require.NoFileExists(t, storePath, "the fixture starts with no store")

	out, err := runUp(t, a, recipe, "--plan", "--json")
	require.NoError(t, err, out)

	var plan UpPlanOutput
	require.NoError(t, json.Unmarshal([]byte(out), &plan))
	assert.Equal(t, 2, plan.Totals.AIRemaining, "with no store, every missing unit is AI work")
	assert.Zero(t, plan.Totals.MemoryExact, "no store means no leverage to report")

	assert.NoFileExists(t, storePath, "a plan must never create the project store")
	assert.NoFileExists(t, storePath+"-wal", "nor a journal for one")
}

// TestUpPlan_FreshCheckoutReportsCommittedMemoryLeverage: a clone with no
// project store still plans against the corpus git carries. The committed
// `.kapi/memory/` bundles are compiled through the same importers into a
// content memory that exists only for the call, so the plan reads what the run
// will read and the checkout is left as git wrote it — the shape of CI's leg
// and a new contributor's, and the one where the estimate is most consulted
// (#1866).
func TestUpPlan_FreshCheckoutReportsCommittedMemoryLeverage(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := compassCopy(t)

	storePath := project.LayoutAt(root).StorePath()
	require.NoFileExists(t, storePath, "the sample is committed without a store")

	// Nothing has a target yet: the sample's catalogs start short of the source.
	require.NoError(t, os.RemoveAll(filepath.Join(root, "site", "locales", "nb.json")))

	outJSON, err := runUp(t, a, recipe, "--plan", "--json")
	require.NoError(t, err, outJSON)
	var plan UpPlanOutput
	require.NoError(t, json.Unmarshal([]byte(outJSON), &plan))

	assert.Positive(t, plan.Totals.MemoryExact,
		"the committed bundles answer units git already carries reviewed wording for: %s", outJSON)
	assert.NoFileExists(t, storePath, "and reading them materialized no store")
	assert.NoFileExists(t, storePath+"-wal", "nor a journal for one")
}

// TestUpPlan_TextTable: the human rendering is a table with a totals row and
// the estimation note.
func TestUpPlan_TextTable(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := planFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})

	out, err := runUp(t, a, recipe, "--plan")
	require.NoError(t, err, out)
	assert.Contains(t, out, "dry run")
	assert.Contains(t, out, "content memory exact")
	assert.Contains(t, out, "total")
	assert.Contains(t, out, "chars / 4", "the token heuristic is disclosed")
}

// TestUpPlan_NoWritesToExistingStore: with an existing project store, --plan
// leaves its contents alone.
//
// The assertion is on CONTENT, not on mtime: opening a SQLite database
// checkpoints its WAL, so a read-only pass legitimately touches the file. What
// must not change is what the store holds — no blocks extracted, no entries
// learned, by a command that only reports.
func TestUpPlan_NoWritesToExistingStore(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := planFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})

	// Materialize the store via a real up first.
	out, err := runUp(t, a, recipe)
	require.NoError(t, err, out)

	countStore := func() (blocks, entries int) {
		t.Helper()
		db := openStore(t, root)
		sess, serr := db.BlocksAutocommit().Begin(t.Context())
		require.NoError(t, serr)
		defer sess.Close()
		for _, berr := range sess.Blocks(blockstore.BlockFilter{}) {
			require.NoError(t, berr)
			blocks++
		}
		es, eerr := db.Memory().Entries(t.Context())
		require.NoError(t, eerr)
		return blocks, len(es)
	}
	blocksBefore, entriesBefore := countStore()
	require.Positive(t, blocksBefore, "the real up must have extracted something to compare against")

	a2 := processOnlyApp(t)
	out2, err := runUp(t, a2, recipe, "--plan")
	require.NoError(t, err, out2)

	blocksAfter, entriesAfter := countStore()
	assert.Equal(t, blocksBefore, blocksAfter, "--plan must not extract into the block cache")
	assert.Equal(t, entriesBefore, entriesAfter, "--plan must not write to the content memory")
}

// TestUpPlan_ConvergedProjectHasNoWork: after a converged up, --plan reports
// nothing to do — when the corpus answers the units the targets hold. That, and
// not the presence of a target file, is what "nothing to do" means: the pass
// reads the source documents and fills from the content memory, so a unit it
// cannot recycle is drafted however finished the target file looks.
func TestUpPlan_ConvergedProjectHasNoWork(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := planFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})
	seedPlanMemory(t, root)
	seedPlanEntry(t, root, "seed-2", "Goodbye.", "Farvel.")
	out, err := runUp(t, a, recipe)
	require.NoError(t, err, out)

	a2 := processOnlyApp(t)
	out2, err := runUp(t, a2, recipe, "--plan")
	require.NoError(t, err, out2)
	assert.Contains(t, out2, "Nothing to do")
}

// TestUpPlan_ProducedUnitsTheCorpusCannotFillAreWork is the same project with an
// empty corpus: its targets exist and the next pass re-produces every one of
// them, so a plan reporting nothing to do described a run that was going to
// happen anyway — the under-pricing #1974 names.
func TestUpPlan_ProducedUnitsTheCorpusCannotFillAreWork(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := planFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})
	out, err := runUp(t, a, recipe)
	require.NoError(t, err, out)

	a2 := processOnlyApp(t)
	out2, err := runUp(t, a2, recipe, "--plan")
	require.NoError(t, err, out2)
	assert.Contains(t, out2, "unanswered")
	assert.Contains(t, out2, "the pass drafts them")
}

// TestUpPlan_RecycleOnlyFlowPricesNothingItCannotProduce: the plan reads the
// flow it is planning. A recipe whose flow is exact-match content-memory reuse
// and nothing else has no step that could produce a unit the corpus does not
// answer, so those units are reported as out of reach rather than quoted as AI
// work — and the run that follows makes no provider calls and spends nothing,
// which the plan now says (#1866).
func TestUpPlan_RecycleOnlyFlowPricesNothingItCannotProduce(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.Flow = "recycle-only"
	proj.Flows["recycle-only"] = &flow.StepsSpec{Steps: []flow.FlowStep{{Tool: "recycle"}}}
	require.NoError(t, project.Save(recipe, proj))
	// One of the two fixture units has an answer; the other never will.
	seedPlanMemory(t, root)

	outJSON, err := runUp(t, a, recipe, "--plan", "--json")
	require.NoError(t, err, outJSON)
	var plan UpPlanOutput
	require.NoError(t, json.Unmarshal([]byte(outJSON), &plan))
	assert.Equal(t, 1, plan.Totals.MemoryExact, `"Hello, world." is leverage this flow does apply`)
	assert.Zero(t, plan.Totals.AIRemaining, "and nothing here is drafted")
	assert.Equal(t, 1, plan.Totals.OutOfReach, `"Goodbye." is beyond what this flow can do`)
	assert.Zero(t, plan.Totals.TokenEstimate, "so there is nothing to price")
	assert.False(t, plan.FlowDrafts)
	assert.False(t, plan.FlowCallsProvider)
	assert.Empty(t, plan.Provider, "naming a provider for a run that never calls one is the quote itself")

	out, err := runUp(t, processOnlyApp(t), recipe, "--plan")
	require.NoError(t, err, out)
	assert.Contains(t, out, "out of reach")
	assert.Contains(t, out, "calls no provider, so this run spends nothing")
	assert.NotContains(t, out, "AI provider:")
}

// TestUpPlan_LocalDraftingFlowIsNotAProviderBill: a flow that drafts without
// reaching a provider (pseudo-translation) produces the units and bills
// nothing. They are work, not out of reach — and they carry no token estimate.
func TestUpPlan_LocalDraftingFlowIsNotAProviderBill(t *testing.T) {
	a := processOnlyApp(t)
	// convergeFixture's own flow is a single pseudo-translate step.
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})

	outJSON, err := runUp(t, a, recipe, "--plan", "--json")
	require.NoError(t, err, outJSON)
	var plan UpPlanOutput
	require.NoError(t, json.Unmarshal([]byte(outJSON), &plan))
	assert.Equal(t, 2, plan.Totals.AIRemaining, "the flow produces both units")
	assert.Zero(t, plan.Totals.OutOfReach)
	assert.Zero(t, plan.Totals.TokenEstimate, "without a provider there is no bill to estimate")
	assert.True(t, plan.FlowDrafts)
	assert.False(t, plan.FlowCallsProvider)

	out, err := runUp(t, processOnlyApp(t), recipe, "--plan")
	require.NoError(t, err, out)
	assert.Contains(t, out, "produces its drafts locally: no provider is called and nothing is spent")
}

// TestUpPlan_UnresolvableFlowIsAnError: a recipe naming a flow it does not
// define cannot run, so it cannot be planned either — the plan fails with the
// message the run gives rather than quoting figures for a run that never starts.
func TestUpPlan_UnresolvableFlowIsAnError(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.Flow = "no-such-flow"
	require.NoError(t, project.Save(recipe, proj))

	out, err := runUp(t, a, recipe, "--plan")
	require.Error(t, err, out)
	assert.Contains(t, err.Error(), "no-such-flow")
}

// TestEstimateTokens: the chars/4 heuristic rounds up and zeroes on empty.
func TestEstimateTokens(t *testing.T) {
	assert.Equal(t, 0, EstimateTokens(""))
	assert.Equal(t, 1, EstimateTokens("abc"))
	assert.Equal(t, 1, EstimateTokens("abcd"))
	assert.Equal(t, 2, EstimateTokens("abcde"))
}

// TestUpPlan_SubscriptionProviderNote: with the claude-code default provider,
// the plan swaps the metered framing for subscription wording — in the text
// table and in the JSON (provider + subscription fields).
func TestUpPlan_SubscriptionProviderNote(t *testing.T) {
	a := processOnlyApp(t)
	a.Config = config.NewAppConfig()
	a.Config.Set(config.KeyAIProvider, "claude-code")
	recipe, _ := planFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})

	out, err := runUp(t, a, recipe, "--plan")
	require.NoError(t, err, out)
	assert.Contains(t, out, "runs on your Claude subscription")
	assert.Contains(t, out, "no per-token API cost")

	a2 := processOnlyApp(t)
	a2.Config = config.NewAppConfig()
	a2.Config.Set(config.KeyAIProvider, "claude-code")
	outJSON, err := runUp(t, a2, recipe, "--plan", "--json")
	require.NoError(t, err, outJSON)
	var plan UpPlanOutput
	require.NoError(t, json.Unmarshal([]byte(outJSON), &plan))
	assert.Equal(t, "claude-code", plan.Provider)
	assert.True(t, plan.Subscription)
	assert.Contains(t, plan.Note, "Claude subscription")
}

// TestUpPlan_MeteredProviderKeepsNote: a metered provider keeps the standard
// estimation note with no subscription framing.
func TestUpPlan_MeteredProviderKeepsNote(t *testing.T) {
	a := processOnlyApp(t)
	a.Config = config.NewAppConfig()
	a.Config.Set(config.KeyAIProvider, "anthropic")
	recipe, _ := planFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})

	outJSON, err := runUp(t, a, recipe, "--plan", "--json")
	require.NoError(t, err, outJSON)
	var plan UpPlanOutput
	require.NoError(t, json.Unmarshal([]byte(outJSON), &plan))
	assert.Equal(t, "anthropic", plan.Provider)
	assert.False(t, plan.Subscription)
	assert.NotContains(t, plan.Note, "subscription")
}
