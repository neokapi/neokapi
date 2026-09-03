package backend

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

func TestVoiceOverlay_ContainmentSuppression(t *testing.T) {
	profile := &coreprofile.VoiceProfile{
		Vocabulary: coreprofile.VocabularyRules{
			ForbiddenTerms: []coreprofile.TermRule{{Term: "cart", Replacement: "shopping cart"}},
		},
	}
	src := "Add it to your shopping cart"
	runs := []model.Run{{Text: &model.TextRun{Text: src}}}
	if ov := voiceOverlay(profile, runs, src); ov != nil {
		t.Fatalf("shopping cart should suppress the inner cart, got overlay %+v", ov)
	}
	bare := "Empty your cart now"
	runs2 := []model.Run{{Text: &model.TextRun{Text: bare}}}
	if ov := voiceOverlay(profile, runs2, bare); ov == nil || len(ov.Spans) != 1 {
		t.Fatalf("bare cart should flag once, got %+v", ov)
	}
}
