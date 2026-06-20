package math

import "strings"

// latexString renders an expression to LaTeX (no $ delimiters).
func latexString(e Exp) string {
	switch v := e.(type) {
	case nil:
		return ""
	case Number:
		return v.Text
	case Ident:
		return v.Text
	case Operator:
		return latexOp(v.Text)
	case Text:
		return `\text{` + latexEscapeText(v.Content) + "}"
	case Raw:
		return `\text{` + latexEscapeText(v.Content) + "}"
	case Row:
		var parts []string
		for _, it := range v.Items {
			parts = append(parts, latexString(it))
		}
		return strings.Join(parts, "")
	case Fraction:
		if v.NoBar {
			return "{" + latexString(v.Num) + `\atop ` + latexString(v.Den) + "}"
		}
		return `\frac{` + latexString(v.Num) + "}{" + latexString(v.Den) + "}"
	case Superscript:
		return latexGroup(v.Base) + "^{" + latexString(v.Sup) + "}"
	case Subscript:
		return latexGroup(v.Base) + "_{" + latexString(v.Sub) + "}"
	case SubSup:
		return latexGroup(v.Base) + "_{" + latexString(v.Sub) + "}^{" + latexString(v.Sup) + "}"
	case Radical:
		if v.Degree == nil {
			return `\sqrt{` + latexString(v.Body) + "}"
		}
		return `\sqrt[` + latexString(v.Degree) + "]{" + latexString(v.Body) + "}"
	case Nary:
		op := naryLaTeX[v.Chr]
		if op == "" {
			op = latexSymbol(v.Chr)
		}
		var b strings.Builder
		b.WriteString(op)
		if v.Sub != nil {
			b.WriteString("_{" + latexString(v.Sub) + "}")
		}
		if v.Sup != nil {
			b.WriteString("^{" + latexString(v.Sup) + "}")
		}
		if v.Body != nil {
			b.WriteString(latexString(v.Body))
		}
		return b.String()
	case Delimited:
		return `\left` + latexFence(v.Open) + latexString(v.Body) + `\right` + latexFence(v.Close)
	case Function:
		name := latexFunc(v.Name)
		return name + " " + latexGroup(v.Arg)
	case Matrix:
		var rows []string
		for _, r := range v.Rows {
			var cells []string
			for _, c := range r {
				cells = append(cells, latexString(c))
			}
			rows = append(rows, strings.Join(cells, " & "))
		}
		return `\begin{matrix}` + strings.Join(rows, ` \\ `) + `\end{matrix}`
	case Accent:
		cmd := accentLaTeX[v.Accent]
		if cmd == "" {
			cmd = `\hat`
		}
		return cmd + "{" + latexString(v.Body) + "}"
	case Bar:
		if v.Top {
			return `\overline{` + latexString(v.Body) + "}"
		}
		return `\underline{` + latexString(v.Body) + "}"
	case GroupChr:
		if v.Pos == "top" {
			return `\overbrace{` + latexString(v.Body) + "}"
		}
		return `\underbrace{` + latexString(v.Body) + "}"
	default:
		return ""
	}
}

// latexGroup renders e wrapped in {} unless it is a single atomic token, so it
// can serve as the base of a script or a function argument.
func latexGroup(e Exp) string {
	switch e.(type) {
	case Number, Ident:
		return latexString(e)
	case Delimited, Radical, Fraction:
		return latexString(e) // already self-delimiting
	default:
		return "{" + latexString(e) + "}"
	}
}

// latexOp renders an operator glyph, mapping known Unicode symbols to commands.
func latexOp(s string) string {
	if v, ok := symbolLaTeX[s]; ok {
		return v + " "
	}
	return s
}

// latexFence maps a delimiter glyph to a LaTeX fence (escaping braces; "" → ".").
func latexFence(s string) string {
	switch s {
	case "":
		return "."
	case "{":
		return `\{`
	case "}":
		return `\}`
	case "|":
		return "|"
	case "⟨":
		return `\langle`
	case "⟩":
		return `\rangle`
	case "⌊":
		return `\lfloor`
	case "⌋":
		return `\rfloor`
	case "⌈":
		return `\lceil`
	case "⌉":
		return `\rceil`
	default:
		return s
	}
}

// knownFuncs are function names rendered with a backslash command in LaTeX.
var knownFuncs = map[string]bool{
	"sin": true, "cos": true, "tan": true, "cot": true, "sec": true, "csc": true,
	"arcsin": true, "arccos": true, "arctan": true,
	"sinh": true, "cosh": true, "tanh": true,
	"log": true, "ln": true, "lg": true, "exp": true,
	"lim": true, "limsup": true, "liminf": true, "max": true, "min": true,
	"sup": true, "inf": true, "det": true, "gcd": true, "deg": true, "dim": true,
	"ker": true, "arg": true, "Pr": true,
}

func latexFunc(name string) string {
	if knownFuncs[strings.ToLower(name)] {
		return `\` + name
	}
	return `\operatorname{` + name + "}"
}

// latexEscapeText escapes LaTeX-special characters inside \text{}.
func latexEscapeText(s string) string {
	r := strings.NewReplacer(
		`\`, `\textbackslash{}`, "&", `\&`, "%", `\%`, "$", `\$`,
		"#", `\#`, "_", `\_`, "{", `\{`, "}", `\}`, "~", `\textasciitilde{}`, "^", `\textasciicircum{}`,
	)
	return r.Replace(s)
}
