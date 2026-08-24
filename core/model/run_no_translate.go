package model

import "strings"

// RestoreNonTranslatable puts back, in target, the text that source marked
// NoTranslate — the contents of a code span, a <kbd>, a <samp>.
//
// Marking a run is only half the job on a machine-translation round trip. The
// runs are handed to the provider as semantic HTML, and whether a provider
// honours `<code>` or `translate="no"` is that provider's business: some
// translate the contents anyway, and a translated command is worse than an
// untranslated sentence. So rather than ask, this restores the source bytes
// afterwards, which holds for every provider including the ones that ignore
// every hint.
//
// Matching is by paired-code ID. ParseRunsSemanticHTML reuses the source's
// PcOpen and PcClose runs, so the brackets on both sides carry the same ids,
// and the text between them is what gets replaced.
func RestoreNonTranslatable(target, source []Run) []Run {
	protected := protectedByCode(source)
	if len(protected) == 0 {
		return target
	}

	out := make([]Run, 0, len(target))
	for i := 0; i < len(target); i++ {
		r := target[i]
		out = append(out, r)
		if r.PcOpen == nil {
			continue
		}
		text, ok := protected[r.PcOpen.ID]
		if !ok {
			continue
		}
		// Skip whatever the provider put between the brackets and write the
		// source bytes instead.
		j := i + 1
		for j < len(target) && target[j].PcClose == nil {
			j++
		}
		out = append(out, Run{Text: &TextRun{Text: text, NoTranslate: true}})
		i = j - 1
	}
	return out
}

// protectedByCode maps a paired code's id to the do-not-translate text it
// encloses. Only codes whose entire content is protected are listed: a bracket
// holding a mix is left to the ordinary path, because replacing all of it
// would drop the part that was translatable.
func protectedByCode(runs []Run) map[string]string {
	var out map[string]string
	for i, r := range runs {
		if r.PcOpen == nil {
			continue
		}
		var text strings.Builder
		allProtected := true
		sawText := false
		j := i + 1
		for ; j < len(runs) && runs[j].PcClose == nil; j++ {
			if runs[j].Text == nil {
				continue
			}
			sawText = true
			if !runs[j].Text.NoTranslate {
				allProtected = false
				break
			}
			text.WriteString(runs[j].Text.Text)
		}
		if !sawText || !allProtected {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[r.PcOpen.ID] = text.String()
	}
	return out
}
