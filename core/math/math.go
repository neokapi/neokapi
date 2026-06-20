// Package math is a cgo-free converter between Office Math Markup Language
// (OMML, ECMA-376 Part 1 §22.1) and portable math notations — Presentation
// MathML and LaTeX — via a small intermediate AST.
//
// The shape mirrors Pandoc's texmath: read OMML into an AST of Exp nodes once,
// then serialize that AST to N target notations. The host format reader (e.g.
// OpenXML) keeps the original OMML bytes verbatim for byte-exact round-trip and
// uses this package only to produce additional, portable renderings that
// cross-format writers (markdown, DocLang, HTML) can emit.
//
// It is intentionally tolerant: an OMML element it does not model degrades to a
// best-effort row/text rather than failing, so a partial conversion never breaks
// a document read.
package math

// Exp is a node in the math AST. It is a closed interface implemented by the
// concrete node types below.
type Exp interface{ isExp() }

// Number is a numeric literal (rendered as <mn>/digits).
type Number struct{ Text string }

// Ident is an identifier — a variable or function-ish letter (rendered <mi>).
type Ident struct{ Text string }

// Operator is an operator, relation, or fence glyph (rendered <mo>).
type Operator struct{ Text string }

// Text is literal text inside an equation. Normal is true when the source
// flagged it as upright normal text (OMML <m:nor/>) — i.e. natural-language
// prose embedded in the math ("where", "otherwise", a unit), which is the
// translatable surface. Non-normal Text is math-styled literal text.
type Text struct {
	Content string
	Normal  bool
}

// Row is an ordered sequence of sub-expressions (<mrow>).
type Row struct{ Items []Exp }

// Fraction is num over den; NoBar renders without the rule (binomials / stacks).
type Fraction struct {
	Num, Den Exp
	NoBar    bool
}

// Superscript, Subscript, SubSup are scripts attached to a base.
type Superscript struct{ Base, Sup Exp }
type Subscript struct{ Base, Sub Exp }
type SubSup struct{ Base, Sub, Sup Exp }

// Radical is a root: Degree is nil for a square root.
type Radical struct{ Degree, Body Exp }

// Nary is an n-ary operator (∑ ∫ ∏ ⋃ …) with optional lower/upper limits and a
// body operand. Sub/Sup are the limits; Body is the operand.
type Nary struct {
	Chr      string
	Sub, Sup Exp
	Body     Exp
}

// Delimited is a fenced group, e.g. ( … ). Open/Close are the fence glyphs
// ("" means none / invisible).
type Delimited struct {
	Open, Close string
	Body        Exp
}

// Function is a named function application, e.g. sin(x), lim …
type Function struct {
	Name string
	Arg  Exp
}

// Matrix is a grid of cells (<mtable>).
type Matrix struct{ Rows [][]Exp }

// Accent places an accent glyph (hat, bar, vec, dot …) over the base.
type Accent struct {
	Accent string
	Body   Exp
}

// Bar is an over/under bar (OMML <m:bar>).
type Bar struct {
	Body Exp
	Top  bool // true = overbar, false = underbar
}

// GroupChr groups the base under/over a character (e.g. ⏞ overbrace).
type GroupChr struct {
	Chr  string
	Pos  string // "top" or "bot"
	Body Exp
}

// Raw is an un-modeled fragment kept as a literal string (graceful fallback).
type Raw struct{ Content string }

func (Number) isExp()      {}
func (Ident) isExp()       {}
func (Operator) isExp()    {}
func (Text) isExp()        {}
func (Row) isExp()         {}
func (Fraction) isExp()    {}
func (Superscript) isExp() {}
func (Subscript) isExp()   {}
func (SubSup) isExp()      {}
func (Radical) isExp()     {}
func (Nary) isExp()        {}
func (Delimited) isExp()   {}
func (Function) isExp()    {}
func (Matrix) isExp()      {}
func (Accent) isExp()      {}
func (Bar) isExp()         {}
func (GroupChr) isExp()    {}
func (Raw) isExp()         {}

// Math is a parsed equation plus a flag for whether the source equation was a
// display block (<m:oMathPara>) versus inline (<m:oMath>).
type Math struct {
	Body  Exp
	Block bool
}

// FromOMML parses an OMML <m:oMath> or <m:oMathPara> fragment into a Math AST.
// It never returns a fatal error for unsupported constructs — those degrade to
// Raw/Row nodes; an error is returned only for malformed XML.
func FromOMML(raw []byte) (*Math, error) {
	return parseOMML(raw)
}

// ToMathML renders the equation as Presentation MathML (a <math> element).
func (m *Math) ToMathML() string { return mathMLDocument(m) }

// ToLaTeX renders the equation as LaTeX (no $ / $$ delimiters).
func (m *Math) ToLaTeX() string { return latexString(m.Body) }

// TranslatableText returns the concatenated normal-text (<m:nor/>) spans — the
// natural-language prose embedded in the equation — in reading order. Empty when
// the equation is pure math typography.
func (m *Math) TranslatableText() string { return collectNormalText(m.Body) }

// row flattens a single-item Row to its element; otherwise returns a Row.
func row(items []Exp) Exp {
	if len(items) == 1 {
		return items[0]
	}
	return Row{Items: items}
}
