package projection

import "github.com/neokapi/neokapi/core/model"

// InlineSink receives the structurally-decoded inline events of a run sequence.
// A serializer implements it to map the format-neutral run vocabulary (fmt:bold,
// link:hyperlink, media:image, …) to its own markup, without re-implementing run
// decoding and plural/select resolution. This is the shared substrate that
// replaces the independent balanced-tag stacks in the HTML, Markdown, AsciiDoc
// and DocLang writers.
//
// Events arrive in document order. Open/Close are balanced and properly nested
// for well-formed run sequences (PcOpen/PcClose pairs). The full run is handed
// to the sink (Type, SubType, Attrs, Data, Equiv) so a serializer can reproduce
// format-specialized behavior — raw-markup passthrough via Data, alt/href from
// Attrs — and manage its own open stack (the close behavior of a paired code
// depends on what was opened, not on the close run's own type).
type InlineSink interface {
	// Text is a literal text chunk (the source/target text, unescaped).
	Text(s string)
	// Open begins a paired inline construct (bold, italic, a hyperlink, …).
	Open(r *model.PcOpenRun)
	// Close ends the most recently opened paired construct.
	Close(r *model.PcCloseRun)
	// Placeholder is a self-closing inline run (an image, a variable, a <br/>).
	Placeholder(r *model.PlaceholderRun)
}

// WalkInline decodes a run sequence into structural inline events and drives
// them to sink. Plural / Select runs recurse into their "other" branch (or the
// first branch present), matching model.RenderRunsWithData semantics, so a
// serializer sees a single resolved inline stream. Sub runs are skipped here —
// sub-document content is projected as its own node by the stream walker.
//
// WalkInline is deliberately allocation-free beyond the sink's own work: it does
// not build an intermediate tree, so per-block inline rendering stays cheap
// enough to call from a streaming writer or a per-block preview.
func WalkInline(runs []model.Run, sink InlineSink) {
	for i := range runs {
		r := &runs[i]
		switch {
		case r.Text != nil:
			sink.Text(r.Text.Text)
		case r.PcOpen != nil:
			sink.Open(r.PcOpen)
		case r.PcClose != nil:
			sink.Close(r.PcClose)
		case r.Ph != nil:
			sink.Placeholder(r.Ph)
		case r.Plural != nil:
			WalkInline(pluralBranch(r.Plural), sink)
		case r.Select != nil:
			WalkInline(selectBranch(r.Select), sink)
		}
	}
}

// pluralBranch returns the "other" plural form, or the first form present.
func pluralBranch(p *model.PluralRun) []model.Run {
	if form, ok := p.Forms[model.PluralOther]; ok {
		return form
	}
	for _, form := range p.Forms {
		return form
	}
	return nil
}

// selectBranch returns the "other" select case, or the first case present.
func selectBranch(s *model.SelectRun) []model.Run {
	if c, ok := s.Cases["other"]; ok {
		return c
	}
	for _, c := range s.Cases {
		return c
	}
	return nil
}
