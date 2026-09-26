package server

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPromoteEntityToConcept (RV-F piece 3) proves the entity→concept promotion:
// a marked entity becomes a real terms concept (not merely a term candidate),
// carrying the entity text as an approved source-locale term, AND fires
// concept.created — the event RV-E/RV-F's re-check subscribes to — so a promoted
// entity flows into the governed terminology loop automatically.
func TestPromoteEntityToConcept(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	ctx := context.Background()

	// Capture concept.created off the bus to prove the re-check trigger fires.
	var mu sync.Mutex
	var created []platev.Event
	s.EventBus.Subscribe(knowledge.EventConceptCreated, func(ev platev.Event) {
		mu.Lock()
		created = append(created, ev)
		mu.Unlock()
	})

	entity := &model.EntityAnnotation{
		Text:   "Acme Corp",
		Type:   model.EntityType("organization"),
		Source: model.ExtractionSourceManual,
		Locale: "en",
	}

	concept, err := s.promoteEntityToConcept(ctx, "rc", wsID, "curator-1", "proj-x", "main", entity)
	require.NoError(t, err)
	require.NotEmpty(t, concept.ID)

	// A real terms concept with the entity text as an approved source-locale term.
	tb, err := s.wsStores.getTerms("rc")
	require.NoError(t, err)
	got, ok, err := tb.GetConcept(ctx, concept.ID)
	require.NoError(t, err)
	require.True(t, ok, "a real terms concept was created")
	require.Len(t, got.Terms, 1)
	assert.Equal(t, "Acme Corp", got.Terms[0].Text)
	assert.Equal(t, model.LocaleID("en"), got.Terms[0].Locale)
	assert.Equal(t, model.TermApproved, got.Terms[0].Status)
	assert.Equal(t, "organization", got.Domain)

	// concept.created fired with the new concept id → RV-E/RV-F re-checks it.
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, ev := range created {
			if ev.Data["concept_id"] == concept.ID {
				return true
			}
		}
		return false
	}, 5*time.Second, 20*time.Millisecond, "promotion fires concept.created for the re-check loop")
}

// TestPromoteEntityToConcept_DoNotTranslateIsProposed: an entity marked
// do-not-translate becomes a governed proposal, a submitted change-set whose
// concept.create carries the flag, and no concept exists until it merges. Once
// approved and merged, the concept is enforced: a target that translates the
// entity fails term-check.
func TestPromoteEntityToConcept_DoNotTranslateIsProposed(t *testing.T) {
	s, wsID, owner := newRecheckHarness(t)
	fake := newFakeKnowledgeStore()
	s.KnowledgeStore = fake
	ctx := context.Background()

	entity := &model.EntityAnnotation{Text: "kubectl", Type: model.EntityType("product"), DNT: true, Locale: "en"}
	concept, err := s.promoteEntityToConcept(ctx, "rc", wsID, "curator-1", "proj-x", "main", entity)
	require.NoError(t, err)
	require.NotEmpty(t, concept.ID)

	tb, err := s.wsStores.getTerms("rc")
	require.NoError(t, err)
	_, ok, err := tb.GetConcept(ctx, concept.ID)
	require.NoError(t, err)
	assert.False(t, ok, "nothing is written before review")

	sets, err := fake.ListChangeSets(ctx, wsID, knowledge.ChangeSetInReview)
	require.NoError(t, err)
	require.Len(t, sets, 1, "the promotion is proposed for review")
	ops, err := fake.ListOps(ctx, wsID, sets[0].ID)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	require.Equal(t, knowledge.OpConceptCreate, ops[0].Op)
	var p knowledge.ConceptCreatePayload
	require.NoError(t, json.Unmarshal(ops[0].Payload, &p))
	assert.Equal(t, concept.ID, p.Concept.ID)
	assert.True(t, p.Concept.DoNotTranslate)
	assert.NotContains(t, p.Concept.Properties, "translatability", "the flag replaces the property")

	require.NoError(t, fake.AddReview(ctx, &knowledge.ChangeSetReview{
		WorkspaceID: wsID, ChangesetID: sets[0].ID, Reviewer: owner, Verdict: knowledge.VerdictApprove,
	}))
	require.NoError(t, fake.SetChangeSetStatus(ctx, wsID, sets[0].ID, knowledge.ChangeSetApproved))
	cs, err := fake.GetChangeSet(ctx, wsID, sets[0].ID)
	require.NoError(t, err)
	_, err = knowledge.NewEngine(nil, tb, fake).MergeChangeSet(ctx, wsID, fake, *cs)
	require.NoError(t, err)

	merged, ok, err := tb.GetConcept(ctx, concept.ID)
	require.NoError(t, err)
	require.True(t, ok)
	assert.True(t, merged.DoNotTranslate)
	rule, ok := terms.RuleForConcept(merged, "en", "fr")
	require.True(t, ok)
	errs, _ := coretools.TermCheckViolations(&coretools.TermCheckConfig{
		TermRules: []coreprofile.TermRule{rule}, SourceLocale: "en", TargetLocale: "fr",
	}, "Run kubectl apply", "Exécutez kubectell apply")
	assert.NotEmpty(t, errs, "the merged concept is enforced")
}
