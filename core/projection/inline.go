package projection

import "github.com/neokapi/neokapi/core/model"

// InlineSink receives the structurally-decoded inline events of a run sequence.
// A serializer implements it to map the format-neutral run vocabulary (fmt:bold,
// link:hyperlink, media:image, …) to its own markup, without re-implementing run
// decoding and pairing. This is the shared substrate that replaces the four
// independent balanced-tag stacks in the HTML, Markdown, AsciiDoc and DocLang
// writers.
//
// Events arrive in document order. Open/Close are balanced and properly nested
// for well-formed run sequences (PcOpen/PcClose pairs); Close carries the
// closing run's own Type so a sink need not track the open stack itself.
type InlineSink interface {
	// Text is a literal text chunk (already the source/target text, unescaped).
	Text(s string)
	// Open begins a paired inline construct (a PcOpenRun): bold, italic, a
	// hyperlink, etc. typ is the canonical run Type; attrs are the canonical
	// keys (model.AttrHref/AttrSrc/AttrAlt/AttrTitle) and may be nil.
	Open(typ string, attrs map[string]string)
	// Close ends the most recently opened paired construct. typ is the closing
	// run's Type (mirrors the matching Open's typ for well-formed input).
	Close(typ string)
	// Placeholder is a self-closing inline run (a PlaceholderRun): an image, a
	// variable, a <br/>, an icon. equiv is the run's human-readable equivalent
	// (Ph.Equiv); attrs are canonical and may be nil.
	Placeholder(typ, equiv string, attrs map[string]string)
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
	for _, r := range runs {
		switch {
		case r.Text != nil:
			sink.Text(r.Text.Text)
		case r.PcOpen != nil:
			sink.Open(r.PcOpen.Type, r.PcOpen.Attrs)
		case r.PcClose != nil:
			sink.Close(r.PcClose.Type)
		case r.Ph != nil:
			sink.Placeholder(r.Ph.Type, r.Ph.Equiv, r.Ph.Attrs)
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
