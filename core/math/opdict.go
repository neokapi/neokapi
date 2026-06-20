package math

import (
	"strings"
	"unicode"
)

// classifyRunText tokenizes the text of a math run into Number / Ident /
// Operator nodes: runs of digits (with a decimal point) become a Number, runs
// of letters become an Ident, and any other non-space rune becomes an Operator.
// Whitespace is dropped (math layout supplies spacing).
func classifyRunText(s string) []Exp {
	var out []Exp
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		r := runes[i]
		switch {
		case unicode.IsSpace(r):
			i++
		case unicode.IsDigit(r) || (r == '.' && i+1 < len(runes) && unicode.IsDigit(runes[i+1])):
			j := i
			for j < len(runes) && (unicode.IsDigit(runes[j]) || runes[j] == '.') {
				j++
			}
			out = append(out, Number{Text: string(runes[i:j])})
			i = j
		case unicode.IsLetter(r):
			j := i
			for j < len(runes) && unicode.IsLetter(runes[j]) {
				j++
			}
			out = append(out, Ident{Text: string(runes[i:j])})
			i = j
		default:
			out = append(out, Operator{Text: string(r)})
			i++
		}
	}
	return out
}

// flatText flattens an expression to its plain text content (used for function
// names like "sin"/"lim" and for delimiter/operator glyphs).
func flatText(e Exp) string {
	switch v := e.(type) {
	case nil:
		return ""
	case Number:
		return v.Text
	case Ident:
		return v.Text
	case Operator:
		return v.Text
	case Text:
		return v.Content
	case Raw:
		return v.Content
	case Row:
		var b strings.Builder
		for _, it := range v.Items {
			b.WriteString(flatText(it))
		}
		return b.String()
	default:
		return ""
	}
}

// collectNormalText returns the concatenated normal-text (<m:nor/>) spans of an
// expression, in reading order, space-joined — the translatable prose embedded
// in an equation.
func collectNormalText(e Exp) string {
	var parts []string
	var walk func(Exp)
	walk = func(e Exp) {
		switch v := e.(type) {
		case Text:
			if v.Normal && strings.TrimSpace(v.Content) != "" {
				parts = append(parts, v.Content)
			}
		case Row:
			for _, it := range v.Items {
				walk(it)
			}
		case Fraction:
			walk(v.Num)
			walk(v.Den)
		case Superscript:
			walk(v.Base)
			walk(v.Sup)
		case Subscript:
			walk(v.Base)
			walk(v.Sub)
		case SubSup:
			walk(v.Base)
			walk(v.Sub)
			walk(v.Sup)
		case Radical:
			walk(v.Degree)
			walk(v.Body)
		case Nary:
			walk(v.Sub)
			walk(v.Sup)
			walk(v.Body)
		case Delimited:
			walk(v.Body)
		case Function:
			walk(v.Arg)
		case Matrix:
			for _, r := range v.Rows {
				for _, c := range r {
					walk(c)
				}
			}
		case Accent:
			walk(v.Body)
		case Bar:
			walk(v.Body)
		case GroupChr:
			walk(v.Body)
		}
	}
	walk(e)
	return strings.Join(parts, " ")
}

// naryLaTeX maps an n-ary operator glyph to its LaTeX command.
var naryLaTeX = map[string]string{
	"∑": `\sum`, "∏": `\prod`, "∐": `\coprod`,
	"∫": `\int`, "∬": `\iint`, "∭": `\iiint`, "∮": `\oint`,
	"⋃": `\bigcup`, "⋂": `\bigcap`, "⋁": `\bigvee`, "⋀": `\bigwedge`,
	"⨄": `\biguplus`, "⨆": `\bigsqcup`, "⨁": `\bigoplus`, "⨂": `\bigotimes`, "⨀": `\bigodot`,
}

// symbolLaTeX maps common math glyphs to LaTeX commands (operators/relations
// that authors type as Unicode in OMML runs).
var symbolLaTeX = map[string]string{
	"∑": `\sum`, "∏": `\prod`, "∫": `\int`, "√": `\surd`,
	"≤": `\leq`, "≥": `\geq`, "≠": `\neq`, "≈": `\approx`, "≡": `\equiv`,
	"±": `\pm`, "∓": `\mp`, "×": `\times`, "÷": `\div`, "⋅": `\cdot`, "∙": `\cdot`,
	"→": `\to`, "←": `\leftarrow`, "⇒": `\Rightarrow`, "⇐": `\Leftarrow`, "↔": `\leftrightarrow`,
	"∞": `\infty`, "∂": `\partial`, "∇": `\nabla`, "∈": `\in`, "∉": `\notin`,
	"⊂": `\subset`, "⊆": `\subseteq`, "∪": `\cup`, "∩": `\cap`, "∅": `\emptyset`,
	"∀": `\forall`, "∃": `\exists`, "¬": `\neg`, "∧": `\wedge`, "∨": `\vee`,
	"α": `\alpha`, "β": `\beta`, "γ": `\gamma`, "δ": `\delta`, "ε": `\epsilon`,
	"θ": `\theta`, "λ": `\lambda`, "μ": `\mu`, "π": `\pi`, "ρ": `\rho`,
	"σ": `\sigma`, "τ": `\tau`, "φ": `\phi`, "ω": `\omega`,
	"Γ": `\Gamma`, "Δ": `\Delta`, "Θ": `\Theta`, "Λ": `\Lambda`, "Σ": `\Sigma`,
	"Φ": `\Phi`, "Ω": `\Omega`, "Π": `\Pi`,
}

// accentLaTeX maps a (possibly combining) accent glyph to its LaTeX command.
var accentLaTeX = map[string]string{
	"̂": `\hat`, "^": `\hat`, "̃": `\tilde`, "~": `\tilde`,
	"̄": `\bar`, "‾": `\bar`, "̇": `\dot`, "̈": `\ddot`,
	"⃗": `\vec`, "→": `\vec`, "̌": `\check`, "̆": `\breve`, "́": `\acute`, "̀": `\grave`,
}

// latexSymbol returns the LaTeX rendering of an operator/identifier glyph,
// falling back to the literal text.
func latexSymbol(s string) string {
	if v, ok := symbolLaTeX[s]; ok {
		return v
	}
	return s
}
