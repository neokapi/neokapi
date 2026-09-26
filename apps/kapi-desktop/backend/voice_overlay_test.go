package backend

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

func TestVoiceOverlay_ContainmentSuppression(t *testing.T) {
	words := []coreprofile.TermRuleSet{{
		Rules: []coreprofile.TermRule{{Term: "cart", Replacement: "shopping cart"}},
		Kind:  coreprofile.VocabForbidden,
	}}
	src := "Add it to your shopping cart"
	runs := []model.Run{{Text: &model.TextRun{Text: src}}}
	if ov := voiceOverlay(words, nil, runs, src); ov != nil {
		t.Fatalf("shopping cart should suppress the inner cart, got overlay %+v", ov)
	}
	bare := "Empty your cart now"
	runs2 := []model.Run{{Text: &model.TextRun{Text: bare}}}
	if ov := voiceOverlay(words, nil, runs2, bare); ov == nil || len(ov.Spans) != 1 {
		t.Fatalf("bare cart should flag once, got %+v", ov)
	}
}

// A starter pack's terms ride on the profile, and the overlay shows them as
// word rules beside the voice's own patterns.
func TestVoiceOverlay_CarriedTerms(t *testing.T) {
	profile := (&coreprofile.VoiceProfile{}).Carry("pack test",
		[]coreprofile.TermRule{{Term: "Acme", Competitor: true}})
	src := "Unlike Acme, we ship today"
	runs := []model.Run{{Text: &model.TextRun{Text: src}}}
	ov := voiceOverlay(wordRuleSets(t.Context(), nil, profile, "en"), profile, runs, src)
	if ov == nil || len(ov.Spans) != 1 {
		t.Fatalf("the carried competitor term should flag once, got %+v", ov)
	}
	if got := ov.Spans[0].Props["kind"]; got != "competitor" {
		t.Fatalf("kind = %q, want competitor", got)
	}
}
