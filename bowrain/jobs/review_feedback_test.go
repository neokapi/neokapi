package jobs

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/venue"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// These tests drive the real worker over a real PostgreSQL store with the demo
// provider, a deterministic stub: no model is called.

// rejectWith files a reviewer's rejection of target, a draft of source, with
// its reasons and note, and clears the draft mark as the review surface does.
func (f basisFixture) rejectWith(t *testing.T, projectID, unit, source, target, note string, reasons ...string) {
	t.Helper()
	ctx := t.Context()
	_, err := f.cs.UpsertUnitDecisions(ctx, projectID, "main", []venue.UnitDecision{{
		ItemName: f.item, Unit: unit, Variant: "fr",
		Status:      string(model.TargetStatusDraft),
		Revision:    frRevision(target),
		Basis:       srcRevision(source),
		ReviewState: venue.ReviewStateRejected,
		Reasons:     reasons,
		Note:        note,
		DecidedBy:   "reviewer-1@example.com",
		AgentHost:   "claude-code",
		Updated:     "2026-02-01T00:00:00Z",
	}})
	require.NoError(t, err)
	require.NoError(t, f.cs.RecordDraftBases(ctx, projectID, "main", []store.DraftBasis{{
		ItemName: f.item, Unit: unit, Variant: "fr",
	}}))
}

// stubbornTestProvider names a stand-in for the platform AI that drafts every
// segment as the wording it is given here, whatever the prompt says about
// rejected drafts: the model a stop exists for.
const stubbornTestProvider aiprovider.ProviderID = "stubborn-test"

// stubbornWording is what the stubborn provider drafts every segment as.
const stubbornWording = "Supprimer le compte"

var registerStubbornProvider sync.Once

func useStubbornProvider() string {
	registerStubbornProvider.Do(func() {
		aiprovider.RegisterProvider(aiprovider.ProviderInfo{Name: stubbornTestProvider, Label: "Stubborn test", Local: true},
			func(aiprovider.Config) aiprovider.LLMProvider {
				mock := aiprovider.NewMockProvider()
				mock.TranslateFunc = func(context.Context, aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
					return &aiprovider.TranslateResponse{Translation: stubbornWording}, nil
				}
				mock.ChatStructuredFunc = func(_ context.Context, messages []aiprovider.Message, _ aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
					var in struct {
						Segments []struct {
							ID string `json:"id"`
						} `json:"segments"`
					}
					if err := json.Unmarshal([]byte(messages[len(messages)-1].Text()), &in); err != nil {
						return nil, err
					}
					type entry struct {
						ID   string `json:"id"`
						Text string `json:"text"`
					}
					var out struct {
						Translations []entry `json:"translations"`
					}
					for _, s := range in.Segments {
						out.Translations = append(out.Translations, entry{ID: s.ID, Text: stubbornWording})
					}
					b, err := json.Marshal(out)
					if err != nil {
						return nil, err
					}
					return &aiprovider.ChatResponse{Content: string(b), Usage: aiprovider.TokenUsage{InputTokens: 40, OutputTokens: 12}}, nil
				}
				return mock
			})
	})
	return string(stubbornTestProvider)
}

// runTranslation runs one translation job for the fixture's item in projectID,
// through the platform provider named (the demo stub when none is).
func (f basisFixture) runTranslation(t *testing.T, projectID, id string, provider ...string) *TranslationJob {
	t.Helper()
	ctx := t.Context()
	js, err := NewJobStore(f.db)
	require.NoError(t, err)
	platform := "demo"
	if len(provider) > 0 {
		platform = provider[0]
	}
	deps := &WorkerDeps{
		JobStore:      js,
		ContentStore:  f.cs,
		Platform:      &PlatformProviderConfig{Provider: platform},
		ProviderStore: &fakeProviderResolver{cfg: bstore.ProviderConfig{Type: "demo"}},
	}
	job := &TranslationJob{
		ID: id, WorkspaceSlug: "acme", ProjectID: projectID, ItemName: f.item,
		TargetLocale: "fr", ProviderConfigID: "platform", Model: "demo", Status: StatusQueued,
	}
	require.NoError(t, js.CreateJob(ctx, job))
	claimed, epoch, err := js.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, executeTranslationWithDeps(ctx, deps, job, epoch))
	done, err := js.GetJob(ctx, job.ID)
	require.NoError(t, err)
	return done
}

// recordExchanges captures every model exchange made while the test runs, as
// the platform's exchange recorder sees it, rendered as JSON.
func recordExchanges(t *testing.T) func() string {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	remove := aiprovider.AddRecorder(func(_ context.Context, ex aiprovider.Exchange) {
		raw, err := json.Marshal(ex)
		if err != nil {
			return
		}
		mu.Lock()
		seen = append(seen, string(raw))
		mu.Unlock()
	})
	t.Cleanup(remove)
	return func() string {
		mu.Lock()
		defer mu.Unlock()
		return strings.Join(seen, "\n")
	}
}

// A rejected unit's next draft is told the rejected wording, the reasons in a
// person's words and the note, and the recorder shows exactly that. The
// reviewer and the agent host they decided from are never sent.
func TestWorker_TellsTheModelWhatTheReviewerRejected(t *testing.T) {
	f := newBasisFixture(t)
	exchanges := recordExchanges(t)
	f.rejectWith(t, f.projectID, "fresh", basisFreshSource, "Supprimer le compte",
		"Say close the account, not delete it.", "meaning")

	f.runTranslation(t, f.projectID, "job-feedback-1")

	sent := exchanges()
	assert.Contains(t, sent, "Supprimer le compte", "the rejected wording")
	assert.Contains(t, sent, tool.RejectionReasonPhrase("meaning"), "the reason, as a person reads it")
	assert.Contains(t, sent, "Say close the account, not delete it.", "the note")
	assert.NotContains(t, sent, "reviewer-1@example.com", "who rejected the draft is never sent")
	assert.NotContains(t, sent, "claude-code")
}

// A rejection filed in another project, in this workspace or another, for an
// item and unit of the same ids never reaches this project's drafts.
func TestWorker_ReviewFeedbackStaysInItsOwnProject(t *testing.T) {
	f := newBasisFixture(t)
	ctx := t.Context()
	for _, other := range []store.Project{
		{ID: "proj-sibling", Name: "Sibling", WorkspaceID: "", DefaultSourceLanguage: "en"},
		{ID: "proj-elsewhere", Name: "Elsewhere", WorkspaceID: "ws-elsewhere", DefaultSourceLanguage: "en"},
	} {
		other.TargetLanguages = []model.LocaleID{"fr"}
		other.Properties = map[string]string{"translate_after": "none"}
		require.NoError(t, f.cs.CreateProject(ctx, &other))
		require.NoError(t, f.cs.StoreBlocksForItem(ctx, other.ID, "main", f.item, []*model.Block{
			derivedBlock("fresh", basisFreshSource, "Supprimer le compte", srcRevision(basisFreshSource)),
		}))
		f.rejectWith(t, other.ID, "fresh", basisFreshSource, "Supprimer le compte",
			"NOTE-FROM-"+other.ID, "tone")
	}
	exchanges := recordExchanges(t)
	f.rejectWith(t, f.projectID, "fresh", basisFreshSource, "Supprimer le compte", "NOTE-FROM-THIS-PROJECT", "meaning")

	f.runTranslation(t, f.projectID, "job-scope-1")

	sent := exchanges()
	assert.Contains(t, sent, "NOTE-FROM-THIS-PROJECT")
	assert.NotContains(t, sent, "NOTE-FROM-proj-sibling", "another project in the workspace")
	assert.NotContains(t, sent, "NOTE-FROM-proj-elsewhere", "a project in another workspace")
}

// A model told about a rejected draft drafts something else: the demo stub
// does as it is asked, which is what a local stack shows.
func TestWorker_TheDemoProviderDraftsAroundARejection(t *testing.T) {
	f := newBasisFixture(t)
	f.rejectWith(t, f.projectID, "fresh", basisFreshSource, "Supprimer le compte", "Not this.", "meaning")
	f.runTranslation(t, f.projectID, "job-demo-1")
	drafted := f.storedFor(t)[basisFreshSource].Block.TargetText("fr")
	require.NotEmpty(t, drafted)
	require.NotEqual(t, "Supprimer le compte", drafted, "the first job drafted the unit")

	f.rejectWith(t, f.projectID, "fresh", basisFreshSource, drafted, "Not this.", "tone")
	f.runTranslation(t, f.projectID, "job-demo-2")
	redrafted := f.storedFor(t)[basisFreshSource].Block.TargetText("fr")
	assert.NotEqual(t, drafted, redrafted, "the rejected wording reached the model, and the model avoided it")
}

// A draft that comes back in the wording a reviewer rejected is not kept, the
// unit stays owed, and the draft counts towards the stop. At the stop the unit
// is a person's to write: no job drafts it, and the estimate owes nothing for
// it.
func TestWorker_ARepeatedDraftCountsTowardsTheStop(t *testing.T) {
	f := newBasisFixture(t)
	ctx := t.Context()
	stubborn := useStubbornProvider()
	f.rejectWith(t, f.projectID, "fresh", basisFreshSource, stubbornWording, "Not this.", "meaning")
	count := func() int {
		t.Helper()
		tallies, err := f.cs.ListRejectionTallies(ctx, f.projectID, "main")
		require.NoError(t, err)
		require.Len(t, tallies, 1)
		return tallies[0].Count
	}
	require.Equal(t, 1, count())

	// The draft repeats the rejected one: not kept, counted, and still owed.
	first := f.runTranslation(t, f.projectID, "job-stop-1", stubborn)
	assert.Equal(t, 2, first.TotalBlocks, "the stale unit and the rejected one")
	assert.Equal(t, stubbornWording, f.storedFor(t)[basisFreshSource].Block.TargetText("fr"))
	assert.Equal(t, 2, count(), "one rejection and one repeat")
	_, mark := draftMark(t, f, "fresh")
	assert.Empty(t, mark, "a repeated draft is no draft, so the unit is not marked as drafted")

	second := f.runTranslation(t, f.projectID, "job-stop-2", stubborn)
	assert.Equal(t, 1, second.TotalBlocks, "the rejected unit is still owed")
	assert.Equal(t, tool.MaxRejections, count())

	third := f.runTranslation(t, f.projectID, "job-stop-3", stubborn)
	assert.Zero(t, third.TotalBlocks, "at the stop the unit is a person's to write")
	proj, err := f.cs.GetProject(ctx, f.projectID)
	require.NoError(t, err)
	est, err := EstimateConvergence(ctx, f.cs, nil, nil, proj)
	require.NoError(t, err)
	assert.Zero(t, est.Totals.Pending, "and the estimate owes nothing for it")
}

// draftMark returns the unit's row and its draft mark.
func draftMark(t *testing.T, f basisFixture, unit string) (venue.UnitDecision, string) {
	t.Helper()
	ctx := t.Context()
	records, err := f.cs.ListUnitDecisions(ctx, f.projectID, "main")
	require.NoError(t, err)
	var rec venue.UnitDecision
	for _, d := range records {
		if d.Unit == unit {
			rec = d
		}
	}
	drafts, err := f.cs.ListDraftBases(ctx, f.projectID, "main")
	require.NoError(t, err)
	for _, d := range drafts {
		if d.Unit == unit {
			return rec, d.Basis
		}
	}
	return rec, ""
}
