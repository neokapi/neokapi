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
// the target is set to a copy of the source's segments where every
// TextRun has its text rune-substituted via extLatinMap. Non-text
// runs (Ph, PcOpen, PcClose, Sub, Plural, Select) are copied as-is
// so inline codes survive the round-trip.
//
// The Block is shared with the native writer (NativeEngine) and
// echoed back over gRPC for the bridge writer (BridgeEngine), so the
// same target shape drives both engines and matches what
// TextModificationStep would produce on the okapi-pseudo path.
func applyPseudoToBlock(b *model.Block, spec PseudoSpec) {
	if !b.Translatable {
		return
	}
	if len(b.Source) == 0 {
		return
	}
	tgt := model.LocaleID(spec.TgtLocale())
	targetSegs := make([]*model.Segment, 0, len(b.Source))
	for _, srcSeg := range b.Source {
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
