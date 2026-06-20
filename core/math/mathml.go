package math

import "strings"

// mathMLDocument renders a Math as a complete <math> element (Presentation
// MathML), with display="block" for block equations.
func mathMLDocument(m *Math) string {
	var b strings.Builder
	b.WriteString(`<math xmlns="http://www.w3.org/1998/Math/MathML"`)
	if m.Block {
		b.WriteString(` display="block"`)
	}
	b.WriteString(">")
	b.WriteString(mml(m.Body))
	b.WriteString("</math>")
	return b.String()
}

// mmlArg renders e as a single MathML element, wrapping a multi-item Row in
// <mrow> so it can serve as one argument of a script/fraction/root.
func mmlArg(e Exp) string {
	if e == nil {
		return "<mrow></mrow>"
	}
	if r, ok := e.(Row); ok && len(r.Items) != 1 {
		var b strings.Builder
		b.WriteString("<mrow>")
		for _, it := range r.Items {
			b.WriteString(mml(it))
		}
		b.WriteString("</mrow>")
		return b.String()
	}
	return mml(e)
}

// mml renders an expression to MathML.
func mml(e Exp) string {
	switch v := e.(type) {
	case nil:
		return ""
	case Number:
		return "<mn>" + esc(v.Text) + "</mn>"
	case Ident:
		return "<mi>" + esc(v.Text) + "</mi>"
	case Operator:
		return "<mo>" + esc(v.Text) + "</mo>"
	case Text:
		return "<mtext>" + esc(v.Content) + "</mtext>"
	case Raw:
		return "<mtext>" + esc(v.Content) + "</mtext>"
	case Row:
		var b strings.Builder
		for _, it := range v.Items {
			b.WriteString(mml(it))
		}
		return b.String()
	case Fraction:
		attr := ""
		if v.NoBar {
			attr = ` linethickness="0"`
		}
		return "<mfrac" + attr + ">" + mmlArg(v.Num) + mmlArg(v.Den) + "</mfrac>"
	case Superscript:
		return "<msup>" + mmlArg(v.Base) + mmlArg(v.Sup) + "</msup>"
	case Subscript:
		return "<msub>" + mmlArg(v.Base) + mmlArg(v.Sub) + "</msub>"
	case SubSup:
		return "<msubsup>" + mmlArg(v.Base) + mmlArg(v.Sub) + mmlArg(v.Sup) + "</msubsup>"
	case Radical:
		if v.Degree == nil {
			return "<msqrt>" + mmlArg(v.Body) + "</msqrt>"
		}
		return "<mroot>" + mmlArg(v.Body) + mmlArg(v.Degree) + "</mroot>"
	case Nary:
		op := "<mo>" + esc(v.Chr) + "</mo>"
		var big string
		switch {
		case v.Sub != nil && v.Sup != nil:
			big = "<munderover>" + op + mmlArg(v.Sub) + mmlArg(v.Sup) + "</munderover>"
		case v.Sub != nil:
			big = "<munder>" + op + mmlArg(v.Sub) + "</munder>"
		case v.Sup != nil:
			big = "<mover>" + op + mmlArg(v.Sup) + "</mover>"
		default:
			big = op
		}
		return "<mrow>" + big + mml(v.Body) + "</mrow>"
	case Delimited:
		var b strings.Builder
		b.WriteString("<mrow>")
		if v.Open != "" {
			b.WriteString(`<mo fence="true">` + esc(v.Open) + "</mo>")
		}
		b.WriteString(mml(v.Body))
		if v.Close != "" {
			b.WriteString(`<mo fence="true">` + esc(v.Close) + "</mo>")
		}
		b.WriteString("</mrow>")
		return b.String()
	case Function:
		return "<mrow><mi>" + esc(v.Name) + "</mi><mo>&#x2061;</mo>" + mmlArg(v.Arg) + "</mrow>"
	case Matrix:
		var b strings.Builder
		b.WriteString("<mtable>")
		for _, r := range v.Rows {
			b.WriteString("<mtr>")
			for _, c := range r {
				b.WriteString("<mtd>" + mml(c) + "</mtd>")
			}
			b.WriteString("</mtr>")
		}
		b.WriteString("</mtable>")
		return b.String()
	case Accent:
		return `<mover accent="true">` + mmlArg(v.Body) + "<mo>" + esc(v.Accent) + "</mo></mover>"
	case Bar:
		bar := "<mo>¯</mo>"
		if v.Top {
			return "<mover>" + mmlArg(v.Body) + bar + "</mover>"
		}
		return "<munder>" + mmlArg(v.Body) + bar + "</munder>"
	case GroupChr:
		ch := "<mo>" + esc(v.Chr) + "</mo>"
		if v.Pos == "top" {
			return "<mover>" + mmlArg(v.Body) + ch + "</mover>"
		}
		return "<munder>" + mmlArg(v.Body) + ch + "</munder>"
	default:
		return ""
	}
}

// esc XML-escapes text content for an element body.
func esc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}
