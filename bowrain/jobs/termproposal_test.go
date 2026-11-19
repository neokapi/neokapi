package jobs

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/termcandidate"
	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/core/model"
	fwterms "github.com/neokapi/neokapi/terms"
)

// TestTermProposals_FilesATermPerTargetLanguage: a term-proposal job asks
// for the target term of each kept concept that has none, and files the
// answer on the concept as a proposal. A candidate, a do-not-translate
// concept and one translated already are not asked about, and a concept's
// revision time does not move.
func TestTermProposals_FilesATermPerTargetLanguage(t *testing.T) {
	f := newContextScanFixture(t)
	ctx := t.Context()
	cs, err := bstore.NewPostgresStoreFromDB(f.db)
	require.NoError(t, err)
	const (
		projectID = "proj-terms"
		wsID      = "ws-demo"
	)
	f.deps.ContentStore = cs
	f.deps.TermsResolver = TermsResolverFunc(func(slug string) (fwterms.Terminology, error) {
		return sqlterms.NewPostgresStoreFromDB(f.db, slug)
	})
	require.NoError(t, cs.CreateProject(ctx, &store.Project{
		ID: projectID, Name: "Terms", DefaultSourceLanguage: "en", WorkspaceID: wsID,
		TargetLanguages: []model.LocaleID{"nb", "de"}, DefaultStream: "main",
	}))
	require.NoError(t, cs.StoreItem(ctx, projectID, "main", &store.Item{Name: "a.md", Format: "markdown", ItemType: "file"}))
	require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", "a.md", []*model.Block{
		model.NewBlock("b1", "Open the workspace to see your files."),
		model.NewBlock("b2", "Every workspace has one owner."),
	}))

	tb, err := sqlterms.NewPostgresStoreFromDB(f.db, "acme")
	require.NoError(t, err)
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	concept := func(id string, ts []fwterms.Term, dnt bool) {
		require.NoError(t, tb.AddConcept(ctx, fwterms.Concept{
			ID: id, Terms: ts, DoNotTranslate: dnt, CreatedAt: at, UpdatedAt: at,
			Properties: map[string]string{termcandidate.PropProject: projectID},
		}))
	}
	concept("kept", []fwterms.Term{{Text: "workspace", Locale: "en", Status: model.TermPreferred}}, false)
	concept("candidate", []fwterms.Term{{Text: "dashpanel", Locale: "en", Status: model.TermProposed}}, false)
	concept("dnt", []fwterms.Term{{Text: "kapi", Locale: "en", Status: model.TermPreferred}}, true)
	concept("done", []fwterms.Term{
		{Text: "flow", Locale: "en", Status: model.TermPreferred},
		{Text: "flyt", Locale: "nb", Status: model.TermPreferred},
		{Text: "Fluss", Locale: "de", Status: model.TermPreferred},
	}, false)

	job := f.seedJob(t, wsID, ContextScanRequest{Project: projectID, TermProposals: &TermProposalRequest{}})
	require.NoError(t, processContextScanJob(ctx, f.deps, job.ID))
	got, err := f.deps.Store.GetContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, ContextScanStatusCompleted, got.Status, "error: %s", got.Error)
	var result ContextScanResult
	require.NoError(t, json.Unmarshal(got.Result, &result))
	require.NotNil(t, result.TermProposals)
	assert.Equal(t, 2, result.TermProposals.Asked, "one concept in two languages")
	assert.Equal(t, 2, result.TermProposals.Proposed)

	kept, ok, err := tb.GetConcept(ctx, "kept")
	require.NoError(t, err)
	require.True(t, ok)
	assert.NotEmpty(t, termcandidate.TranslationFor(kept, "nb"))
	assert.NotEmpty(t, termcandidate.TranslationFor(kept, "de"))
	assert.True(t, kept.UpdatedAt.Equal(at), "filing a proposal leaves the revision time alone")
	_, governs := fwterms.RuleForConcept(kept, "en", "nb")
	assert.False(t, governs, "a proposal governs nothing")

	for _, id := range []string{"candidate", "dnt", "done"} {
		cp, _, err := tb.GetConcept(ctx, id)
		require.NoError(t, err)
		assert.Empty(t, termcandidate.Translations(cp), id)
	}

	// A proposal turned down is not filed again.
	proposal := termcandidate.TranslationFor(kept, "nb")
	termcandidate.RejectTranslation(&kept, "nb")
	require.NoError(t, tb.AddConcept(ctx, kept))
	again := f.seedJob(t, wsID, ContextScanRequest{Project: projectID, TermProposals: &TermProposalRequest{Concepts: []string{"kept"}}})
	require.NoError(t, processContextScanJob(ctx, f.deps, again.ID))
	kept, _, err = tb.GetConcept(ctx, "kept")
	require.NoError(t, err)
	assert.Empty(t, termcandidate.TranslationFor(kept, "nb"), "the demo answer repeats the rejected one")
	assert.Equal(t, []string{strings.ToLower(proposal)}, termcandidate.RejectedTranslations(kept, "nb"))
}

// TestTermProposalTargets orders the pairs by reach and narrows to the
// concepts named.
func TestTermProposalTargets(t *testing.T) {
	p := &store.Project{ID: "p", DefaultSourceLanguage: "en", TargetLanguages: []model.LocaleID{"nb"}}
	mk := func(id, term string, reach string) fwterms.Concept {
		return fwterms.Concept{ID: id, Terms: []fwterms.Term{{Text: term, Locale: "en", Status: model.TermPreferred}},
			Properties: map[string]string{termcandidate.PropReachBlocks: reach}}
	}
	concepts := []fwterms.Concept{mk("a", "alpha", "2"), mk("b", "beta", "9"), mk("c", "gamma", "5")}
	concepts = append(concepts, fwterms.Concept{ID: "x", Terms: []fwterms.Term{{Text: "other", Locale: "en"}},
		Properties: map[string]string{termcandidate.PropProject: "another"}})
	ids := func(ts []termProposalTarget) []string {
		out := []string{}
		for _, t := range ts {
			out = append(out, t.concept.ID)
		}
		return out
	}
	assert.Equal(t, []string{"b", "c", "a"}, ids(termProposalTargets(concepts, p, nil)))
	assert.Equal(t, []string{"a"}, ids(termProposalTargets(concepts, p, []string{"a"})))
}
