package terms

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
)

// LocateOverlay is the term overlay over a block's source: every place the
// store's terms occur in it (Locate), one span each. A surface that shows a
// document with its terms marked, such as an inspector or a preview, draws
// this rather than asking the store itself, so it marks what every other
// reader of the store finds.
//
// Each span carries a TermAnnotation and, for the wire form that keeps only
// props: the term as declared ("term"), its preferred term in target
// ("target", when target is set and the concept has one), the concept
// ("concept_id"), its domain ("domain"), and the words as the page shows them
// when a character reference spells an edge of the occurrence ("spelled").
// It returns nil when nothing occurs.
func LocateOverlay(ctx context.Context, store Terminology, runs []model.Run, source, target model.LocaleID) (*model.Overlay, error) {
	text := model.RunsText(runs)
	occs, err := Locate(ctx, LocateRequest{Text: text, Runs: runs, Store: store, Locale: source})
	if err != nil || len(occs) == 0 {
		return nil, err
	}
	spans := make([]model.Span, 0, len(occs))
	for _, o := range occs {
		props := map[string]string{"term": o.Term}
		if o.ConceptID != "" {
			props["concept_id"] = o.ConceptID
		}
		ann := &model.TermAnnotation{
			SourceTerm: o.Term,
			ConceptID:  o.ConceptID,
			Status:     o.Status,
			Score:      o.Score,
			MatchType:  o.MatchType,
		}
		if o.Concept != nil {
			if !target.IsEmpty() {
				if t := o.Concept.PreferredTerm(target); t != nil {
					props["target"] = t.Text
				}
				ann.TargetTerms = termRefs(*o.Concept, target)
			}
			if o.Concept.Domain != "" {
				props["domain"] = o.Concept.Domain
			}
		}
		// A spelled occurrence has no anchor of its own (Occurrence.Spelled):
		// its Start and End cover the words' own text, which is what a reader
		// sees marked.
		anchor := o.Anchor
		if o.Spelled {
			props["spelled"] = o.Text
			anchor = model.RangeAnchorForBytes(runs, o.Start, o.End)
		}
		spans = append(spans, model.Span{Range: anchor, Props: props, Value: ann})
	}
	return &model.Overlay{Type: model.OverlayTerm, Spans: spans}, nil
}
