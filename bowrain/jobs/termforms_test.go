package jobs

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/termcandidate"
	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	fwterms "github.com/neokapi/neokapi/terms"
)

// formsTestProvider names a stand-in for the platform AI that answers the
// term-forms prompt with the Norwegian forms of "varsel", and reports token
// usage the way a real provider does.
const formsTestProvider aiprovider.ProviderID = "forms-test"

var registerFormsProvider sync.Once

func useFormsProvider(t *testing.T, f *contextScanFixture) {
	t.Helper()
	registerFormsProvider.Do(func() {
		aiprovider.RegisterProvider(aiprovider.ProviderInfo{Name: formsTestProvider, Label: "Forms test", Local: true},
			func(aiprovider.Config) aiprovider.LLMProvider {
				mock := aiprovider.NewMockProvider()
				mock.ChatStructuredFunc = func(_ context.Context, messages []aiprovider.Message, _ aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
					type expansion struct {
						Term  string   `json:"term"`
						Forms []string `json:"forms"`
					}
					var out struct {
						Terms []expansion `json:"terms"`
					}
					last := messages[len(messages)-1].Text()
					for term := range strings.SplitSeq(strings.TrimSpace(last), "\n") {
						e := expansion{Term: term}
						if strings.EqualFold(term, "varsel") {
							e.Forms = []string{"varsler", "varselet", "varslene"}
						}
						out.Terms = append(out.Terms, e)
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
	f.deps.Platform = &PlatformProviderConfig{Provider: string(formsTestProvider)}
}

// TestTermForms_WritesFormsOntoTheKeptTerm: a term-forms job asks about the
// target terms of the concepts it names, writes the forms onto each term,
// leaves the concept's revision time alone and records the tokens it used.
// A concept it does not name, and a term with forms already, are not asked
// about.
func TestTermForms_WritesFormsOntoTheKeptTerm(t *testing.T) {
	f := newContextScanFixture(t)
	useFormsProvider(t, f)
	ctx := t.Context()
	cs, err := bstore.NewPostgresStoreFromDB(f.db)
	require.NoError(t, err)
	const (
		projectID = "proj-forms"
		wsID      = "ws-demo"
	)
	f.deps.ContentStore = cs
	f.deps.TermsResolver = TermsResolverFunc(func(slug string) (fwterms.Terminology, error) {
		return sqlterms.NewPostgresStoreFromDB(f.db, slug)
	})
	require.NoError(t, cs.CreateProject(ctx, &store.Project{
		ID: projectID, Name: "Forms", DefaultSourceLanguage: "en", WorkspaceID: wsID,
		TargetLanguages: []model.LocaleID{"nb"}, DefaultStream: "main",
	}))
	tb, err := sqlterms.NewPostgresStoreFromDB(f.db, "acme")
	require.NoError(t, err)
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	add := func(id string, ts ...fwterms.Term) {
		require.NoError(t, tb.AddConcept(ctx, fwterms.Concept{
			ID: id, Terms: ts, CreatedAt: at, UpdatedAt: at,
			Properties: map[string]string{termcandidate.PropProject: projectID},
		}))
	}
	add("alert",
		fwterms.Term{Text: "alert", Locale: "en", Status: model.TermPreferred},
		fwterms.Term{Text: "varsel", Locale: "nb", Status: model.TermPreferred})
	add("other",
		fwterms.Term{Text: "notice", Locale: "en", Status: model.TermPreferred},
		fwterms.Term{Text: "varsel", Locale: "nb", Status: model.TermPreferred})

	job := f.seedJob(t, wsID, ContextScanRequest{Project: projectID, TermForms: &TermFormsRequest{
		Concepts: []string{"alert"}, Locales: []model.LocaleID{"nb"},
	}})
	require.NoError(t, processContextScanJob(ctx, f.deps, job.ID))
	got, err := f.deps.Store.GetContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, ContextScanStatusCompleted, got.Status, "error: %s", got.Error)
	var result ContextScanResult
	require.NoError(t, json.Unmarshal(got.Result, &result))
	require.NotNil(t, result.TermForms)
	assert.Equal(t, TermFormsSummary{Asked: 1, Written: 1}, *result.TermForms)
	assert.Equal(t, 52, got.TokensUsed, "the usage of the call is recorded")

	alert, ok, err := tb.GetConcept(ctx, "alert")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, []string{"varsler", "varselet", "varslene"}, alert.Terms[1].Forms)
	assert.True(t, alert.UpdatedAt.Equal(at), "writing forms leaves the revision time alone")
	other, _, err := tb.GetConcept(ctx, "other")
	require.NoError(t, err)
	assert.Empty(t, other.Terms[1].Forms, "a concept the job does not name is left alone")

	again := f.seedJob(t, wsID, ContextScanRequest{Project: projectID, TermForms: &TermFormsRequest{Concepts: []string{"alert"}}})
	require.NoError(t, processContextScanJob(ctx, f.deps, again.ID))
	got, err = f.deps.Store.GetContextScanJob(ctx, again.ID)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(got.Result, &result))
	assert.Equal(t, TermFormsSummary{}, *result.TermForms, "a term with forms is not asked about again")
	assert.Zero(t, got.TokensUsed)
}

// TestTermForms_NamesNoConcept: a job naming no concept fails rather than
// expanding the whole terms store.
func TestTermForms_NamesNoConcept(t *testing.T) {
	f := newContextScanFixture(t)
	job := f.seedJob(t, "ws-demo", ContextScanRequest{Project: "p", TermForms: &TermFormsRequest{}})
	require.Error(t, processContextScanJob(t.Context(), f.deps, job.ID))
	got, err := f.deps.Store.GetContextScanJob(t.Context(), job.ID)
	require.NoError(t, err)
	assert.Equal(t, ContextScanStatusFailed, got.Status)
}
