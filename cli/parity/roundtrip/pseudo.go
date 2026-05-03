//go:build parity

package roundtrip

import "github.com/neokapi/neokapi/core/model"

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
	if !b.Translatable {
		return
	}
	tgt := model.LocaleID(spec.TgtLocale())
	base := pickPseudoBase(b, tgt)
	if base == nil {
		return
	}
	targetSegs := make([]*model.Segment, 0, len(base))
	for _, srcSeg := range base {
		if srcSeg == nil {
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
		targetSegs = append(targetSegs, &model.Segment{ID: srcSeg.ID, Runs: newRuns, Properties: srcSeg.Properties})
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

func segmentsHaveText(segs []*model.Segment) bool {
	for _, s := range segs {
		if s == nil {
			continue
		}
		for _, r := range s.Runs {
			if r.Text != nil && r.Text.Text != "" {
				return true
			}
		}
	}
	return false
}
