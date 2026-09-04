package server

// Reproduces the Journey-C production drill: a GitHub-App-onboarded project
// (created via the setup wizard with target_languages ["fr-FR"]) whose forge
// ingest stored source blocks WITHOUT item bookkeeping. Its on-push convergence
// run concluded "Up to date, Passes 0, Locales —" over five untranslated
// blocks, because the derive read coverage from the ITEM-scoped dashboard
// stats (blind to item-less blocks) and treated a zero unit total as at-gate.
// Pending work must derive from the project's CONFIGURED target locales
// against the block store: a translatable block lacking a target for a
// configured locale is pending, item rows or not.

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newDeriveHarness builds a Server with a real SQLite content store + run
// store, mirroring the wizard's inline create: a project with configured
// target languages, the default collection, and the main stream.
func newDeriveHarness(t *testing.T, targets []model.LocaleID) (*Server, *sqlitestore.SQLiteStore, *bstore.ConvergenceRunStore, *platstore.Project) {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "content.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	ctx := t.Context()
	p := &platstore.Project{
		Name:                  "journey-c",
		DefaultSourceLanguage: model.LocaleEnglish,
		TargetLanguages:       targets,
	}
	require.NoError(t, cs.CreateProject(ctx, p))
	require.NoError(t, EnsureDefaultCollection(ctx, cs, p.ID))
	require.NoError(t, EnsureMainStream(ctx, cs, p.ID))

	runStore := bstore.NewConvergenceRunStore(cs.DB())
	s := &Server{ContentStore: cs, ConvergenceRunStore: runStore}
	s.convergence = newConvergenceOrchestrator(s)
	return s, cs, runStore, p
}

// seedIngestedBlocks stores n translatable source blocks the way the
// pre-fix connector ingest did: StoreBlocks with no item bookkeeping.
func seedIngestedBlocks(t *testing.T, cs *sqlitestore.SQLiteStore, projectID string, n int) {
	t.Helper()
	blocks := make([]*model.Block, 0, n)
	for i := range n {
		blocks = append(blocks, model.NewBlock(
			fmt.Sprintf("msg.%d", i), fmt.Sprintf("Welcome message number %d", i)))
	}
	require.NoError(t, cs.StoreBlocks(t.Context(), projectID, "main", blocks))
}

// TestDerive_ConnectorIngestedBlocksArePending is the direct repro: five
// ingested source blocks, configured target ["fr-FR"], zero targets — the
// derive must report fr-FR pending with five units, never an empty pending set
// ("Up to date"). Red before the fix: Pending was empty because the dashboard
// stats (item-scoped) saw zero units and total==0 read as fully covered.
func TestDerive_ConnectorIngestedBlocksArePending(t *testing.T) {
	s, cs, _, p := newDeriveHarness(t, []model.LocaleID{model.LocaleFrench})
	seedIngestedBlocks(t, cs, p.ID, 5)

	st, err := s.convergence.deriveFunc(p.ID, "main", nil)(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{string(model.LocaleFrench)}, st.Pending,
		"a configured locale with untranslated blocks is pending")
	assert.Equal(t, 5, st.UnitTotals[string(model.LocaleFrench)])
	assert.Equal(t, 0, st.Produced)
}

// TestOrchestrator_IngestedProjectRunsFirstPass drives a full run over the
// repro store with the REAL derive and a Produce that drafts targets through
// the store (what the translation jobs do): the run must execute a pass that
// produces the five fr drafts and then converge — not conclude "Up to date" in
// zero passes with nothing produced. Red before the fix: Passes was 0 and no
// block carried an fr target.
func TestOrchestrator_IngestedProjectRunsFirstPass(t *testing.T) {
	s, cs, runStore, p := newDeriveHarness(t, []model.LocaleID{model.LocaleFrench})
	seedIngestedBlocks(t, cs, p.ID, 5)

	ctx := context.Background()
	run := &bstore.ConvergenceRun{ProjectID: p.ID, Trigger: "push", State: bstore.ConvergenceRunRunning}
	require.NoError(t, runStore.CreateRun(ctx, run))

	produce := func(ctx context.Context, locale string, pass int, emit *convergence.Emitter) (convergence.PassProduction, error) {
		// Draft every pending translatable block for the locale via the store —
		// the same write path the translation worker uses.
		blocks, err := cs.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID, Stream: "main"})
		if err != nil {
			return convergence.PassProduction{}, err
		}
		var drafted []*model.Block
		for _, sb := range blocks {
			if sb.Block == nil || !sb.Block.Translatable || sb.Block.HasTarget(model.LocaleID(locale)) {
				continue
			}
			sb.Block.SetTargetText(model.LocaleID(locale), "Message de bienvenue "+sb.Block.ID)
			drafted = append(drafted, sb.Block)
		}
		if len(drafted) > 0 {
			if err := cs.StoreBlocks(ctx, p.ID, "main", drafted); err != nil {
				return convergence.PassProduction{}, err
			}
		}
		return convergence.PassProduction{Done: len(drafted), ViaAI: len(drafted)}, nil
	}

	s.convergence.driveWith(ctx, run, convergence.LoopFuncs{
		Derive:  s.convergence.deriveFunc(p.ID, "main", nil),
		Produce: produce,
	})

	got, err := runStore.GetRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, bstore.ConvergenceRunConverged, got.State)
	assert.Equal(t, 1, got.Passes, "the pending fr work must trigger a real pass")
	require.Len(t, got.Standing, 1)
	assert.Equal(t, string(model.LocaleFrench), got.Standing[0].Locale)
	assert.Equal(t, 5, got.Standing[0].Produced)

	// The drafts are in the store: every translatable block carries an fr target.
	blocks, err := cs.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID, Stream: "main"})
	require.NoError(t, err)
	drafted := 0
	for _, sb := range blocks {
		if sb.Block != nil && sb.Block.Translatable && sb.Block.HasTarget(model.LocaleFrench) {
			drafted++
		}
	}
	assert.Equal(t, 5, drafted)
}

// TestOrchestrator_NoTargetLocales_ParksNotUpToDate: a project with genuinely
// zero configured target languages must not read "Up to date" — the run parks
// on configuration with the machine-readable no_target_locales reason and the
// "no target languages configured" message (state + log + done frame). Red
// before the fix: the run converged in zero passes.
func TestOrchestrator_NoTargetLocales_ParksNotUpToDate(t *testing.T) {
	s, cs, runStore, p := newDeriveHarness(t, nil)
	seedIngestedBlocks(t, cs, p.ID, 3)

	ctx := context.Background()
	run := &bstore.ConvergenceRun{ProjectID: p.ID, Trigger: "push", State: bstore.ConvergenceRunRunning}
	require.NoError(t, runStore.CreateRun(ctx, run))

	s.convergence.driveWith(ctx, run, convergence.LoopFuncs{
		Derive: s.convergence.deriveFunc(p.ID, "main", nil),
		Produce: func(context.Context, string, int, *convergence.Emitter) (convergence.PassProduction, error) {
			t.Error("Produce must not run for a project with no target locales")
			return convergence.PassProduction{}, nil
		},
	})

	got, err := runStore.GetRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, bstore.ConvergenceRunParked, got.State, "not converged, not failed")
	assert.Equal(t, convergence.StallNoTargetLocales, got.StallReason)
	assert.Contains(t, got.Error, "no target languages configured")

	// The terminal done frame carries the reason for the SSE/UI.
	_, payloads, err := runStore.ListEvents(ctx, run.ID, 0)
	require.NoError(t, err)
	require.NotEmpty(t, payloads)
	var probe struct {
		Type        string `json:"type"`
		State       string `json:"state"`
		StallReason string `json:"stallReason"`
	}
	require.NoError(t, json.Unmarshal(payloads[len(payloads)-1], &probe))
	assert.Equal(t, "done", probe.Type)
	assert.Equal(t, "parked", probe.State)
	assert.Equal(t, "no_target_locales", probe.StallReason)
}

// TestDerive_ItemScopedProjectUnaffected: the fast item-scoped stats path stays
// authoritative for a normally-ingested project (item rows present) — the
// block-store fallback only engages when the stats see zero units.
func TestDerive_ItemScopedProjectUnaffected(t *testing.T) {
	s, cs, _, p := newDeriveHarness(t, []model.LocaleID{model.LocaleFrench})
	ctx := t.Context()

	require.NoError(t, cs.StoreItem(ctx, p.ID, "main", &platstore.Item{
		Name: "app.json", Format: "json", ItemType: "file",
	}))
	b1 := model.NewBlock("greeting", "Hello")
	b1.SetTargetText(model.LocaleFrench, "Bonjour")
	b2 := model.NewBlock("farewell", "Goodbye")
	require.NoError(t, cs.StoreBlocksForItem(ctx, p.ID, "main", "app.json", []*model.Block{b1, b2}))

	st, err := s.convergence.deriveFunc(p.ID, "main", nil)(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{string(model.LocaleFrench)}, st.Pending)
	assert.Equal(t, 2, st.UnitTotals[string(model.LocaleFrench)])
	assert.Equal(t, 1, st.Produced)
}

// seedStaleUnit stores two translated fr blocks under an item, records one of
// them in the ledger against the source as it reads now (a reviewer's approval
// when decided, the loop's own basis otherwise), and then rewrites that
// block's source, so the record grades stale and the translation renders
// wording the project no longer has. Returns the stale block's source text.
func seedStaleUnit(t *testing.T, cs *sqlitestore.SQLiteStore, projectID string, decided bool) string {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, cs.StoreItem(ctx, projectID, "main", &platstore.Item{
		Name: "app.json", Format: "json", ItemType: "file",
	}))
	stale := model.NewBlock("greeting", "Hello")
	stale.SetTargetText(model.LocaleFrench, "Bonjour")
	fresh := model.NewBlock("farewell", "Goodbye")
	fresh.SetTargetText(model.LocaleFrench, "Au revoir")
	require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", "app.json", []*model.Block{stale, fresh}))

	record := venue.UnitDecision{
		ItemName: "app.json", Unit: "greeting", Variant: string(model.LocaleFrench),
		TargetHash: state.TargetHash("Bonjour"), ContentHash: state.SourceHash("Hello"),
		Updated: "2026-01-01T00:00:00Z",
	}
	if decided {
		record.Status = string(model.TargetStatusReviewed)
		record.ReviewState = "approved"
		record.DecidedBy = "reviewer-1"
	}
	_, err := cs.UpsertUnitDecisions(ctx, projectID, "main", []venue.UnitDecision{record})
	require.NoError(t, err)

	const rewritten = "Hello there"
	require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", "app.json", []*model.Block{
		model.NewBlock("greeting", rewritten),
	}))
	return rewritten
}

// staleTargetCases are the two records a source rewrite can leave stale: the
// loop's own basis on an undecided translation, and a reviewer's approval.
var staleTargetCases = []struct {
	name    string
	decided bool
}{
	{name: "undecided translation", decided: false},
	{name: "decided translation", decided: true},
}

// TestDerive_StaleTargetIsPending: a target whose recorded basis names wording
// the block no longer holds is withheld from the produced count, so the locale
// is pending on production rather than reading fully covered over a
// translation of a sentence that is gone. The derive, the ledger's grouped
// tally and the dashboard's stale count read the same number.
func TestDerive_StaleTargetIsPending(t *testing.T) {
	for _, tc := range staleTargetCases {
		t.Run(tc.name, func(t *testing.T) {
			s, cs, _, p := newDeriveHarness(t, []model.LocaleID{model.LocaleFrench})
			ctx := t.Context()
			seedStaleUnit(t, cs, p.ID, tc.decided)

			st, err := s.convergence.deriveFunc(p.ID, "main", nil)(ctx)
			require.NoError(t, err)
			assert.Equal(t, []string{string(model.LocaleFrench)}, st.Pending,
				"a stale target is pending work, exactly as a missing one")
			assert.Equal(t, 2, st.UnitTotals[string(model.LocaleFrench)])
			assert.Equal(t, 1, st.Produced, "the stale unit is withheld from the produced count")

			basis, err := tallyDecisionBasis(ctx, cs, p.ID, "main", nil)
			require.NoError(t, err)
			counts := basis.forLocale(string(model.LocaleFrench))
			assert.Equal(t, 1, counts.Stale)
			assert.Equal(t, 1, counts.Owed)
			assert.Equal(t, st.UnitTotals[string(model.LocaleFrench)]-st.Produced, counts.Owed,
				"the derive withholds exactly the units the ledger says are owed")

			stats, err := editorGetDashboardStats(ctx, cs, p, "main")
			require.NoError(t, err)
			require.NoError(t, applyShipStates(ctx, cs, nil, p.ID, "main", nil, stats))
			require.Len(t, stats.LocaleStats, 1)
			assert.Equal(t, 1, stats.LocaleStats[0].StaleBlocks, "the dashboard grades the same record stale")
			assert.Equal(t, platstore.ShipStatePending, stats.LocaleStats[0].ShipState)
		})
	}
}

// TestOrchestrator_SourceChangeRedraftsOnce drives a full run over a project
// with one stale unit, with the REAL derive and a Produce that writes what the
// translation worker writes: the draft, the basis for an undecided unit, and
// the mark of the source the draft was made against. The run produces once and
// converges. After it, an undecided unit reads current; a decided one keeps its
// withdrawn approval, still reads stale to the dashboard, and is the reviewer's
// work rather than the loop's: a second run finds nothing to produce.
func TestOrchestrator_SourceChangeRedraftsOnce(t *testing.T) {
	for _, tc := range staleTargetCases {
		t.Run(tc.name, func(t *testing.T) {
			s, cs, runStore, p := newDeriveHarness(t, []model.LocaleID{model.LocaleFrench})
			rewritten := seedStaleUnit(t, cs, p.ID, tc.decided)
			ctx := context.Background()
			fr := string(model.LocaleFrench)

			produceCalls := 0
			var drafted []string
			produce := func(ctx context.Context, locale string, pass int, emit *convergence.Emitter) (convergence.PassProduction, error) {
				produceCalls++
				// The worker's own partition, read from the store's marks: a
				// unit is owed a draft when it has no target, or when its
				// recorded basis is stale and no draft has been marked
				// against the current source (jobs.decisionLedger.needsDraft).
				type unitKey struct{ item, unit, variant string }
				records := map[unitKey]venue.UnitDecision{}
				list, err := cs.ListUnitDecisions(ctx, p.ID, "main")
				if err != nil {
					return convergence.PassProduction{}, err
				}
				for _, d := range list {
					records[unitKey{d.ItemName, d.Unit, d.Variant}] = d
				}
				marks := map[unitKey]string{}
				bases, err := cs.ListDraftBases(ctx, p.ID, "main")
				if err != nil {
					return convergence.PassProduction{}, err
				}
				for _, d := range bases {
					marks[unitKey{d.ItemName, d.Unit, d.Variant}] = d.SourceHash
				}
				blocks, err := cs.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID, Stream: "main"})
				if err != nil {
					return convergence.PassProduction{}, err
				}
				var toStore []*model.Block
				var basisRecords []venue.UnitDecision
				var stamps []platstore.DraftBasis
				for _, sb := range blocks {
					if sb.Block == nil || !sb.Block.Translatable {
						continue
					}
					key := unitKey{sb.ItemName, sb.SourceID, locale}
					rec, recorded := records[key]
					owed := !sb.Block.HasTarget(model.LocaleID(locale))
					if !owed && recorded && rec.ContentHash != "" && rec.ContentHash != sb.ContentHash && marks[key] != sb.ContentHash {
						owed = true
					}
					if !owed {
						continue
					}
					text := "Bonjour à vous"
					sb.Block.SetTargetText(model.LocaleID(locale), text)
					toStore = append(toStore, sb.Block)
					drafted = append(drafted, sb.Block.SourceText())
					if !recorded || rec.ReviewState == "" {
						basisRecords = append(basisRecords, venue.UnitDecision{
							ItemName: sb.ItemName, Unit: sb.SourceID, Variant: locale,
							TargetHash: state.TargetHash(text), ContentHash: sb.ContentHash,
							Updated: "2026-03-01T00:00:00Z",
						})
					}
					stamps = append(stamps, platstore.DraftBasis{
						ItemName: sb.ItemName, Unit: sb.SourceID, Variant: locale, SourceHash: sb.ContentHash,
					})
				}
				if len(toStore) == 0 {
					return convergence.PassProduction{}, nil
				}
				if err := cs.StoreBlocks(ctx, p.ID, "main", toStore); err != nil {
					return convergence.PassProduction{}, err
				}
				if _, err := cs.UpsertUnitDecisions(ctx, p.ID, "main", basisRecords); err != nil {
					return convergence.PassProduction{}, err
				}
				if err := cs.RecordDraftBases(ctx, p.ID, "main", stamps); err != nil {
					return convergence.PassProduction{}, err
				}
				return convergence.PassProduction{Done: len(toStore), ViaAI: len(toStore)}, nil
			}
			drive := func() *bstore.ConvergenceRun {
				t.Helper()
				run := &bstore.ConvergenceRun{ProjectID: p.ID, Trigger: "source-change", State: bstore.ConvergenceRunRunning}
				require.NoError(t, runStore.CreateRun(ctx, run))
				s.convergence.driveWith(ctx, run, convergence.LoopFuncs{
					Derive:  s.convergence.deriveFunc(p.ID, "main", nil),
					Produce: produce,
				})
				got, err := runStore.GetRun(ctx, run.ID)
				require.NoError(t, err)
				return got
			}

			first := drive()
			assert.Equal(t, bstore.ConvergenceRunConverged, first.State)
			assert.Equal(t, 1, first.Passes, "the stale unit is pending work, so the run produces")
			assert.Equal(t, 1, produceCalls)
			assert.Equal(t, []string{rewritten}, drafted, "only the stale unit is drafted")

			blocks, err := cs.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID, Stream: "main"})
			require.NoError(t, err)
			bySource := map[string]string{}
			for _, sb := range blocks {
				bySource[sb.Block.SourceText()] = sb.Block.TargetText(model.LocaleFrench)
			}
			assert.Equal(t, "Bonjour à vous", bySource[rewritten], "the re-draft replaces the translation of the wording that is gone")
			assert.Equal(t, "Au revoir", bySource["Goodbye"], "an unchanged unit is untouched")

			// The ledger after the pass: the loop owes nothing either way; a
			// decided unit still carries its withdrawn approval and reads
			// stale to the dashboard until a person replaces it.
			basis, err := tallyDecisionBasis(ctx, cs, p.ID, "main", nil)
			require.NoError(t, err)
			counts := basis.forLocale(fr)
			assert.Zero(t, counts.Owed, "drafted against the current source")
			stats, err := editorGetDashboardStats(ctx, cs, p, "main")
			require.NoError(t, err)
			require.NoError(t, applyShipStates(ctx, cs, nil, p.ID, "main", nil, stats))
			require.Len(t, stats.LocaleStats, 1)
			assert.Equal(t, 2, stats.LocaleStats[0].TranslatedBlocks)
			if tc.decided {
				assert.Equal(t, 1, counts.Stale, "the approval blessed wording that is gone, and only a person can replace it")
				assert.Equal(t, 1, stats.LocaleStats[0].StaleBlocks)
				assert.Equal(t, platstore.ShipStatePending, stats.LocaleStats[0].ShipState,
					"re-drafted, and withheld from shipping until re-reviewed")
				records, err := cs.ListUnitDecisions(ctx, p.ID, "main")
				require.NoError(t, err)
				require.Len(t, records, 1)
				assert.Equal(t, "approved", records[0].ReviewState, "the decision is never written over")
				assert.Equal(t, state.SourceHash("Hello"), records[0].ContentHash)
			} else {
				assert.Zero(t, counts.Stale, "the loop's own basis follows its draft")
				assert.Zero(t, stats.LocaleStats[0].StaleBlocks)
			}

			// A second run has nothing to produce: the decided unit is pending
			// on the reviewer, not on production.
			second := drive()
			assert.Equal(t, bstore.ConvergenceRunConverged, second.State)
			assert.Zero(t, second.Passes)
			assert.Equal(t, 1, produceCalls, "no further pass drafts the unit again")
		})
	}
}
