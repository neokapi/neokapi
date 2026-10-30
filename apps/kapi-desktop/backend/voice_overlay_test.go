package backend

import (
	"reflect"
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

// entityRun is a character reference as the HTML reader keeps it.
func entityRun(id, data string) model.Run {
	return model.PhR(model.PlaceholderRun{ID: id, Type: "code:entity", Data: data})
}

// The word-rule overlay finds a term beside a character reference the way
// `kapi check` does (coreprofile.MatchTermRulesInRuns), anchored in the term's
// own text run, and a term a reference spells an edge of carries the page's
// spelling.
func TestVoiceOverlay_CharacterReferences(t *testing.T) {
	words := []coreprofile.TermRuleSet{{Rules: []coreprofile.TermRule{
		{Term: "utilize", Replacement: "use"},
		{Term: "next", Replacement: "following"},
		{Term: "café", Replacement: "coffee shop"},
	}}}
	cases := []struct {
		name    string
		runs    []model.Run
		spelled string
	}{
		{"before a reference", []model.Run{model.TextR("utilize"), entityRun("1", "&amp;"), model.TextR("more")}, ""},
		{"after a reference", []model.Run{model.TextR("word"), entityRun("1", "&nbsp;"), model.TextR("next")}, ""},
		{"between references", []model.Run{entityRun("1", "&amp;"), model.TextR("utilize"), entityRun("2", "&nbsp;")}, ""},
		{"a reference spells the last letter", []model.Run{model.TextR("caf"), entityRun("1", "&eacute;")}, "café"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := model.RunsText(tc.runs)
			hits := coreprofile.MatchTermRulesInRuns(words, source, tc.runs)
			if len(hits) != 1 {
				t.Fatalf("core/profile finds %d hits, want 1", len(hits))
			}
			ov := voiceOverlay(words, nil, tc.runs, source)
			if ov == nil || len(ov.Spans) != 1 {
				t.Fatalf("want one span, got %+v", ov)
			}
			span := ov.Spans[0]
			if span.Props["term"] != hits[0].Term || span.Props["spelled"] != tc.spelled {
				t.Fatalf("span props = %v, want term %q spelled %q", span.Props, hits[0].Term, tc.spelled)
			}
			if want := model.RangeAnchorForBytes(tc.runs, hits[0].Start, hits[0].End); !reflect.DeepEqual(span.Range, want) {
				t.Fatalf("span range = %+v, want %+v", span.Range, want)
			}
		})
	}
}
