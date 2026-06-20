package openxml

import (
	"fmt"

	xmath "github.com/neokapi/neokapi/core/math"
	"github.com/neokapi/neokapi/core/model"
)

// ommlNorBlockType marks a translatable block holding one <m:nor/> prose span of
// an equation. The cross-format writers skip these (the prose is already in the
// formula's LaTeX); the openxml writer resolves them back into the OMML.
const ommlNorBlockType = "omml-nor"

// writeOMathSubSkeleton writes an OMML equation to the skeleton as a
// sub-skeleton: verbatim OMML segments interleaved with skeleton refs to
// translatable nor-text blocks (the <m:nor/> prose — "where", "otherwise",
// units). On write each ref resolves to its block's target-else-source, which
// the skeleton writer XML-escapes back into the <m:t>, so an untranslated
// equation reproduces the original bytes exactly and a translated one splices the
// translation in. Returns false (emitting nothing) when the equation has no prose
// or the offsets look wrong, so the caller writes it verbatim.
func (p *wmlParser) writeOMathSubSkeleton(raw string, emitBlock func(*model.Block)) bool {
	spans := xmath.NorSpans([]byte(raw))
	if len(spans) == 0 {
		return false
	}
	// Validate offsets (monotonic, in range) before emitting anything.
	prev := 0
	for _, sp := range spans {
		if sp.Start < prev || sp.End > len(raw) || sp.Start > sp.End {
			return false
		}
		prev = sp.End
	}
	cursor := 0
	for _, sp := range spans {
		p.skelText(raw[cursor:sp.Start])
		*p.blockCounter++
		id := fmt.Sprintf("tu%d", *p.blockCounter)
		blk := model.NewBlock(id, sp.Text)
		blk.Type = ommlNorBlockType
		emitBlock(blk)
		p.skelRef(id)
		cursor = sp.End
	}
	p.skelText(raw[cursor:])
	return true
}

// ommlToMathEquiv converts a captured OMML subtree to portable math renderings
// for cross-format export. It returns:
//
//   - equiv: LaTeX wrapped in markdown math delimiters ($ inline / $$ display),
//     consumed by the markdown writer's placeholder rendering;
//   - disp:  the bare LaTeX (no delimiters), for writers that supply their own
//     math context (e.g. DocLang's <formula>).
//
// Both are stored only on the placeholder's Equiv/Disp, never on Ph.Data, so the
// byte-exact docx round-trip (which replays Ph.Data) and parity are unaffected.
// Returns ("","") when conversion yields nothing (the opaque blob still carries
// the equation).
func ommlToMathEquiv(raw string) (equiv, disp string) {
	m, err := xmath.FromOMML([]byte(raw))
	if err != nil || m == nil {
		return "", ""
	}
	latex := m.ToLaTeX()
	if latex == "" {
		return "", ""
	}
	if m.Block {
		return "$$" + latex + "$$", latex
	}
	return "$" + latex + "$", latex
}
