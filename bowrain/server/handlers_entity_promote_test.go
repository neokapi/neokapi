package server

import (
	"context"
	"sync"
	"testing"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPromoteEntityToConcept (RV-F piece 3) proves the entity→concept promotion:
// a marked entity becomes a real termbase concept (not merely a term candidate),
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

	// A real termbase concept with the entity text as an approved source-locale term.
	tb, err := s.wsStores.getTB("rc")
	require.NoError(t, err)
	got, ok, err := tb.GetConcept(ctx, concept.ID)
	require.NoError(t, err)
	require.True(t, ok, "a real termbase concept was created")
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

// TestPromoteEntityToConcept_DNTPreservesTranslatability records the entity's DNT
// signal as concept metadata.
func TestPromoteEntityToConcept_DNTPreservesTranslatability(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	ctx := context.Background()

	entity := &model.EntityAnnotation{Text: "kubectl", Type: model.EntityType("product"), DNT: true, Locale: "en"}
	concept, err := s.promoteEntityToConcept(ctx, "rc", wsID, "curator-1", "proj-x", "main", entity)
	require.NoError(t, err)
	assert.Equal(t, string(model.TranslatabilityDNT), concept.Properties["translatability"])
}
