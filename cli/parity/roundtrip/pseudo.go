//go:build parity

package roundtrip

import (
	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/model"
)

// extLatinOldChars / extLatinNewChars mirror Okapi's
// TextModificationStep with TYPE_EXTREPLACE + SCRIPT_EXT_LATIN
// (oldChars[0] / newChars[0] in the Java source). Each rune at index i
// in the "old" string maps to the rune at index i in the "new" string.
// Runes not in the table pass through unchanged. The character set is
// chosen so every replaced glyph is visually distinct from its source
// while staying within commonly-supported Latin-Extended Unicode blocks.
const (
	extLatinOldChars = "AaBbCcDdEeFfGgHhIiJjKkLlNnOoPpQqRrSsTtUuWwYyZz"
	extLatinNewChars = "ÀàßƀĆćĎďĒēƑƒĜĝĤĥ" +
		"ĨĩĵĴĶķĹĺŃńŌōƤƥǪǫŔŕ" +
		"ŚśŢţŨũŴŵŶŷŹź"
)

// extLatinMap is the precomputed substitution table. Built from the
// strings above so the source stays grep-able against the upstream
// TextModificationStep.java.
var extLatinMap = func() map[rune]rune {
	oldRunes := []rune(extLatinOldChars)
	newRunes := []rune(extLatinNewChars)
	if len(oldRunes) != len(newRunes) {
		panic("ext-latin map length mismatch — pseudo.go is out of sync with Okapi TextModificationStep")
	}
	m := make(map[rune]rune, len(oldRunes))
	for i, r := range oldRunes {
		m[r] = newRunes[i]
	}
	return m
}()

// pseudoText applies the SCRIPT_EXT_LATIN substitution to a plain text
// string rune-by-rune. Runes not in the map pass through.
func pseudoText(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if mapped, ok := extLatinMap[r]; ok {
			out = append(out, mapped)
		} else {
			out = append(out, r)
		}
	}
	return string(out)
}

// applyPseudoToBlock mirrors Okapi's TextModificationStep with
// TYPE_EXTREPLACE + SCRIPT_EXT_LATIN: for each translatable Block,
// every TextRun has its text rune-substituted via extLatinMap.
// Non-text runs (Ph, PcOpen, PcClose, Sub, Plural, Select) pass
// through unchanged so inline codes survive the round-trip.
//
// Target preference matches TextModificationStep semantics: if the
// Block already has a target for the locale (e.g. a PO file's
// existing msgstr, an XLIFF target, or a TMX tuv), the substitution
// runs against THAT target, mirroring Okapi's "modify the existing
// translation if one exists, otherwise use the source." Without this
// preference the bridge would always pseudo the English source while
// the okapi reference engine pseudos the existing French
// translation — same fixture, divergent outputs.
//
// The Block is shared with the native writer (NativeEngine) and
// echoed back over gRPC for the bridge writer (BridgeEngine), so the
// same target shape drives both engines.
func applyPseudoToBlock(b *model.Block, spec PseudoSpec) {
	applyPseudoToBlockOpts(b, spec, false)
}

// applyPseudoToBlockOpts is applyPseudoToBlock with a forceSourceBase
// flag. When true, the pseudo always operates on the source segments
// even if a target already exists — used for formats (xliff2) where
// okapi unconditionally overwrites the existing target.
func applyPseudoToBlockOpts(b *model.Block, spec PseudoSpec, forceSourceBase bool) {
	if !b.Translatable {
		return
	}
	tgt := model.LocaleID(spec.TgtLocale())
	var base []*model.Segment
	if forceSourceBase && len(b.Source) > 0 {
		base = b.Source
	} else {
		base = pickPseudoBase(b, tgt)
	}
	if base == nil {
		return
	}

	// Ignorable segments (xliff2 <ignorable> elements) are not
	// pseudo-translated — okapi's TextModificationStep only
	// operates on segments, never ignorables. Build a lookup from
	// source segments so we can skip them regardless of whether
	// base points at source or target.
	srcIgnorable := make(map[string]bool)
	for _, s := range b.Source {
		if s != nil && s.Properties != nil && s.Properties["xliff2:ignorable"] == "yes" {
			srcIgnorable[s.ID] = true
		}
	}

	// Existing target lookup so ignorable segments preserve their
	// original target verbatim (e.g. an authored French translation
	// inside <ignorable>).
	existingByID := make(map[string]*model.Segment)
	if existing, ok := b.Targets[tgt]; ok {
		for _, s := range existing {
			if s != nil {
				existingByID[s.ID] = s
			}
		}
	}

	targetSegs := make([]*model.Segment, 0, len(base))
	for _, srcSeg := range base {
		if srcSeg == nil {
			continue
		}

		// Skip pseudo-translation for ignorable segments —
		// preserve existing target or clone source verbatim.
		if srcIgnorable[srcSeg.ID] {
			if existing, ok := existingByID[srcSeg.ID]; ok {
				targetSegs = append(targetSegs, existing)
			} else {
				clonedRuns := make([]model.Run, len(srcSeg.Runs))
				copy(clonedRuns, srcSeg.Runs)
				targetSegs = append(targetSegs, &model.Segment{
					ID:          srcSeg.ID,
					Runs:        clonedRuns,
					Properties:  srcSeg.Properties,
					Annotations: srcSeg.Annotations,
				})
			}
			continue
		}

		newRuns := make([]model.Run, 0, len(srcSeg.Runs))
		for _, r := range srcSeg.Runs {
			if r.Text != nil {
				newRuns = append(newRuns, model.Run{Text: &model.TextRun{Text: pseudoText(r.Text.Text)}})
			} else {
				newRuns = append(newRuns, r)
			}
		}
		newSeg := &model.Segment{ID: srcSeg.ID, Runs: newRuns, Properties: srcSeg.Properties}
		// xliff2 carries inline-code fidelity (<ph> attributes,
		// <pc>/<sc>/<ec> structure, <data> refs) on a side-channel
		// SegmentInlineAnnotation. The writer's freshness check
		// requires the annotation's flat text to equal the segment's
		// Runs' flat text — so when we pseudo source as the new
		// target, clone the IR and pseudo-translate its Text nodes
		// in place. Without this the writer falls back to
		// RenderRunsWithData and emits target text without inline
		// codes, e.g. "Ĥōŵ ĩś ŷōũŕ ďàŷ?" instead of
		// "Ĥōŵ ĩś <ph .../>ŷōũŕ<ph .../> ďàŷ?".
		if ann, ok := srcSeg.Annotations[(&xliff2.SegmentInlineAnnotation{}).AnnotationType()]; ok {
			if sia, ok := ann.(*xliff2.SegmentInlineAnnotation); ok && sia != nil && sia.Content != nil {
				cloned := clonePseudoInlines(sia.Content.Inlines)
				if newSeg.Annotations == nil {
					newSeg.Annotations = make(map[string]model.Annotation)
				}
				newSeg.Annotations[(&xliff2.SegmentInlineAnnotation{}).AnnotationType()] = &xliff2.SegmentInlineAnnotation{
					Content: &xliff2.Content{Inlines: cloned},
				}
			}
		}
		targetSegs = append(targetSegs, newSeg)
	}
	if b.Targets == nil {
		b.Targets = make(map[model.LocaleID][]*model.Segment)
	}
	b.Targets[tgt] = targetSegs
}

// pickPseudoBase returns the segments the pseudo transform should
// operate on: the existing target for the locale when it carries any
// text content, falling back to the source. Returns nil when neither
// is usable (no source, no target).
//
// "Has text content" matters because some filters (e.g. okf_xliff for a
// trans-unit with only alt-trans alternatives) hand back a one-segment
// empty target placeholder. Treating that as the base would yield an
// empty pseudo result; instead we want the source so the round-trip
// matches Okapi's TextModificationStep with applyToBlankEntries=true.
func pickPseudoBase(b *model.Block, tgt model.LocaleID) []*model.Segment {
	if existing, ok := b.Targets[tgt]; ok && segmentsHaveText(existing) {
		return existing
	}
	// Fall back to any existing target. Bilingual formats (ts, tmx,
	// xliff with existing translation, po with existing msgstr in a
	// non-test locale) carry an existing translation under a locale
	// other than the test target. Okapi's TextModificationStep takes
	// the file's target language as the base in that case, so picking
	// any non-empty target keeps native and okapi in agreement.
	for _, segs := range b.Targets {
		if segmentsHaveText(segs) {
			return segs
		}
	}
	if len(b.Source) > 0 {
		return b.Source
	}
	return nil
}

// clonePseudoInlines deep-clones an xliff2 Inline tree and pseudo-
// translates every Text node's Content via pseudoText. Inline-code
// nodes (Ph/Sc/Ec/Pc/Mrk/Sm/Em) and their attributes are preserved
// verbatim — pseudo affects only translatable text.
func clonePseudoInlines(inls []xliff2.Inline) []xliff2.Inline {
	out := make([]xliff2.Inline, len(inls))
	for i, in := range inls {
		switch {
		case in.Text != nil:
			out[i] = xliff2.Inline{Text: &xliff2.Text{Content: pseudoText(in.Text.Content)}}
		case in.Pc != nil:
			pc := *in.Pc
			pc.Children = clonePseudoInlines(in.Pc.Children)
			out[i] = xliff2.Inline{Pc: &pc}
		case in.Mrk != nil:
			mrk := *in.Mrk
			mrk.Children = clonePseudoInlines(in.Mrk.Children)
			out[i] = xliff2.Inline{Mrk: &mrk}
		case in.Ph != nil:
			ph := *in.Ph
			out[i] = xliff2.Inline{Ph: &ph}
		case in.Sc != nil:
			sc := *in.Sc
			out[i] = xliff2.Inline{Sc: &sc}
		case in.Ec != nil:
			ec := *in.Ec
			out[i] = xliff2.Inline{Ec: &ec}
		case in.Sm != nil:
			sm := *in.Sm
			out[i] = xliff2.Inline{Sm: &sm}
		case in.Em != nil:
			em := *in.Em
			out[i] = xliff2.Inline{Em: &em}
		}
	}
	return out
}

func segmentsHaveText(segs []*model.Segment) bool {
	for _, s := range segs {
		if s == nil {
			continue
		}
		for _, r := range s.Runs {
			if r.Text == nil {
				continue
			}
			// Treat whitespace-only text as empty so we mirror okapi's
			// TextModificationStep, which falls back to source when the
			// existing target carries no real translation. Some xliff
			// fixtures (e.g. MQ-12-Test01.xlf) have placeholder
			// `<target> </target>` tags that contain just a space —
			// pseudo-translating the space leaves an effectively empty
			// target, while okapi pseudos the source instead.
			for _, ch := range r.Text.Text {
				if ch != ' ' && ch != '\t' && ch != '\n' && ch != '\r' {
					return true
				}
			}
		}
	}
	return false
}
