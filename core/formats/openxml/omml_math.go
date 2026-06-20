package openxml

import xmath "github.com/neokapi/neokapi/core/math"

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
