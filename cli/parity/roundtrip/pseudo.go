//go:build parity

package roundtrip

import "github.com/neokapi/neokapi/core/model"

// applyPseudoToBlock writes spec.Wrap(block.SourceText()) into the
// block's target for spec.TgtLocale(). Used by the bridge engine and
// the tikal engine; the native engine relies on the registered
// PseudoTranslate tool which produces an equivalent target. We do
// NOT preserve inline runs (paired-codes, placeholders) here — every
// block becomes a flat text target. That keeps the harness's
// comparison simple: re-extract the merged output through the
// reference reader and the resulting Block source-text stream
// should equal `spec.Wrap(original_source_text)` for every block.
func applyPseudoToBlock(b *model.Block, spec PseudoSpec) {
	if !b.Translatable {
		return
	}
	src := b.SourceText()
	if src == "" {
		return
	}
	b.SetTargetText(model.LocaleID(spec.TgtLocale()), spec.Wrap(src))
}
