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
