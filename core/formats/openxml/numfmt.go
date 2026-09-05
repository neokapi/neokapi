package openxml

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// Spreadsheet number formats (ECMA-376 Part 1 §18.8.30 numFmt, §18.8.31
// numFmts). A cell stores a number and shows it through a format code: a date
// is a serial day count, a percentage a fraction, a price a bare decimal. The
// reader keeps the stored value as the cell's content and renders the code
// into a display string carried beside it (model.PropCellDisplay), so an
// export or a preview reads the way the spreadsheet does while the round-trip
// never sees anything but the stored value.
//
// The renderer covers the code families a workbook ordinarily uses: General,
// fixed decimals, thousands grouping and scaling, percent, scientific
// notation, fractions, currency and accounting literals, the date and time
// pattern letters, elapsed time, the text placeholder, conditions, and the
// positive;negative;zero;text sections. A code it cannot render is reported,
// and the caller shows the stored value instead.

// builtInNumFmts is the implied-format table of ECMA-376 §18.8.30: a cell
// style may reference these ids without a numFmt element defining them. Ids
// 5 to 8 and 41 to 44 are locale-dependent in the standard; the codes here are
// the ones Excel writes for an en-US workbook, which is the invariant rendering
// this package applies. The East Asian and Thai calendar ids (23 to 36, 50 to
// 81) have no invariant code and are deliberately absent, so a cell using one
// shows its stored value.
var builtInNumFmts = map[int]string{
	0:  "General",
	1:  "0",
	2:  "0.00",
	3:  "#,##0",
	4:  "#,##0.00",
	5:  `"$"#,##0_);("$"#,##0)`,
	6:  `"$"#,##0_);[Red]("$"#,##0)`,
	7:  `"$"#,##0.00_);("$"#,##0.00)`,
	8:  `"$"#,##0.00_);[Red]("$"#,##0.00)`,
	9:  "0%",
	10: "0.00%",
	11: "0.00E+00",
	12: "# ?/?",
	13: "# ??/??",
	14: "mm-dd-yy",
	15: "d-mmm-yy",
	16: "d-mmm",
	17: "mmm-yy",
	18: "h:mm AM/PM",
	19: "h:mm:ss AM/PM",
	20: "h:mm",
	21: "h:mm:ss",
	22: "m/d/yy h:mm",
	37: "#,##0 ;(#,##0)",
	38: "#,##0 ;[Red](#,##0)",
	39: "#,##0.00;(#,##0.00)",
	40: "#,##0.00;[Red](#,##0.00)",
	41: `_(* #,##0_);_(* \(#,##0\);_(* "-"_);_(@_)`,
	42: `_("$"* #,##0_);_("$"* \(#,##0\);_("$"* "-"_);_(@_)`,
	43: `_(* #,##0.00_);_(* \(#,##0.00\);_(* "-"??_);_(@_)`,
	44: `_("$"* #,##0.00_);_("$"* \(#,##0.00\);_("$"* "-"??_);_(@_)`,
	45: "mm:ss",
	46: "[h]:mm:ss",
	47: "mmss.0",
	48: "##0.0E+0",
	49: "@",
}

// numFmtGeneral is the code of the General format, the one a cell without a
// style shows.
const numFmtGeneral = "General"

// builtInNumFmt returns the format code of a built-in number-format id, and
// false for an id the table does not carry.
func builtInNumFmt(id int) (string, bool) {
	code, ok := builtInNumFmts[id]
	return code, ok
}

// errNumFmtUnsupported marks a format code (or a value under it) the renderer
// does not handle; the caller falls back to the stored value.
var errNumFmtUnsupported = errors.New("number format unsupported")

func unsupportedf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errNumFmtUnsupported, fmt.Sprintf(format, args...))
}

// formatCellValue renders a cell's stored value through a number-format code.
// The second result is false, and the value returned unchanged, when the code
// or the value is one the renderer does not handle.
func formatCellValue(raw, code string, date1904 bool) (string, bool) {
	nf, err := parseNumberFormat(code)
	if err != nil {
		return raw, false
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return raw, false
	}
	out, err := nf.formatNumber(v, date1904)
	if err != nil {
		return raw, false
	}
	return out, true
}

// formatCellText renders a text value through a format code's text section
// (the fourth section, or a single section built on `@`). A code without a
// text section shows the text unchanged, as Excel does.
func formatCellText(text, code string) string {
	nf, err := parseNumberFormat(code)
	if err != nil {
		return text
	}
	return nf.formatText(text)
}

// --- parsed representation ---

type tokKind int

const (
	tokLiteral tokKind = iota // text emitted as written
	tokDigit                  // a digit placeholder: '0', '#' or '?'
	tokPoint                  // the decimal point
	tokComma                  // grouping separator or a thousands scaler (decided by position)
	tokPercent                // scales by 100 and emits '%'
	tokExp                    // scientific exponent marker ("E+", "e-", ...)
	tokSlash                  // a fraction bar between digit groups
	tokGeneral                // the General rendering of the value
	tokText                   // '@', the text value
	tokDate                   // a date or time field
	tokAMPM                   // the 12-hour clock marker, as written
	tokElapsed                // [h], [mm], [ss]: a total rather than a component
)

type dateField int

const (
	fieldYear dateField = iota
	fieldMonth
	fieldDay
	fieldHour
	fieldMinute
	fieldSecond
	fieldSecFrac
)

type fmtToken struct {
	kind  tokKind
	text  string    // literal text; the placeholder char; the exponent marker; AM/PM as written
	field dateField // for tokDate and tokElapsed
	n     int       // repeat count of a date field letter; digits of a fractional second
}

// fmtCond is a bracketed section condition such as [>=100].
type fmtCond struct {
	op    string
	value float64
}

func (c *fmtCond) matches(v float64) bool {
	switch c.op {
	case "<":
		return v < c.value
	case "<=":
		return v <= c.value
	case ">":
		return v > c.value
	case ">=":
		return v >= c.value
	case "=":
		return v == c.value
	case "<>":
		return v != c.value
	}
	return false
}

type sectionKind int

const (
	kindNumber sectionKind = iota
	kindGeneral
	kindDate
	kindText
)

// fracSpec describes a fraction section (# ?/?, ?/8, ...).
type fracSpec struct {
	intIdx   []int // token indices of the whole-number placeholders (mixed fraction)
	numIdx   []int // numerator placeholders
	denIdx   []int // denominator placeholders (empty with a fixed denominator)
	fixedDen int   // a literal denominator, 0 when the placeholders decide it
	numStart int   // index of the first numerator token (the fraction starts here)
}

// fmtSection is one of a code's up-to-four sections, analysed for rendering.
type fmtSection struct {
	tokens []fmtToken
	cond   *fmtCond
	kind   sectionKind

	// number layout
	intIdx   []int // integer-part digit placeholders, in order
	fracIdx  []int // decimal-part digit placeholders
	expIdx   []int // exponent digit placeholders
	expTok   int   // index of the exponent marker, -1 when none
	grouping bool  // a comma between integer placeholders
	scale    int   // power-of-ten scaling: +2 per '%', -3 per trailing comma
	frac     *fracSpec

	// date layout
	hasAMPM   bool
	secFrac   int  // digits of fractional seconds shown, 0 when none
	roundUnit int  // seconds the time is rounded to before display; 0 = floor to the day
	dateOnly  bool // no time field at all
	elapsedH  bool
	elapsedM  bool
	elapsedS  bool
	usesTime  bool
	hasMinute bool
}

// numberFormat is a parsed format code.
type numberFormat struct {
	code     string
	sections []*fmtSection
}

// parseNumberFormat parses a format code. The empty code is General.
func parseNumberFormat(code string) (*numberFormat, error) {
	if strings.TrimSpace(code) == "" {
		code = numFmtGeneral
	}
	raw, err := splitSections(code)
	if err != nil {
		return nil, err
	}
	if len(raw) > 4 {
		return nil, unsupportedf("%d sections", len(raw))
	}
	nf := &numberFormat{code: code}
	for _, s := range raw {
		sec, err := parseSection(s)
		if err != nil {
			return nil, err
		}
		nf.sections = append(nf.sections, sec)
	}
	return nf, nil
}

// splitSections splits a code on the ';' that sit outside quotes, brackets and
// escapes.
func splitSections(code string) ([]string, error) {
	var out []string
	var cur strings.Builder
	rs := []rune(code)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch c {
		case '"':
			j := i + 1
			for j < len(rs) && rs[j] != '"' {
				j++
			}
			if j >= len(rs) {
				return nil, unsupportedf("unterminated quote in %q", code)
			}
			cur.WriteString(string(rs[i : j+1]))
			i = j
		case '[':
			j := i + 1
			for j < len(rs) && rs[j] != ']' {
				j++
			}
			if j >= len(rs) {
				return nil, unsupportedf("unterminated bracket in %q", code)
			}
			cur.WriteString(string(rs[i : j+1]))
			i = j
		case '\\', '_', '*':
			if i+1 >= len(rs) {
				return nil, unsupportedf("dangling %q in %q", string(c), code)
			}
			cur.WriteRune(c)
			cur.WriteRune(rs[i+1])
			i++
		case ';':
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(c)
		}
	}
	out = append(out, cur.String())
	return out, nil
}

var fmtColors = map[string]bool{
	"black": true, "blue": true, "cyan": true, "green": true,
	"magenta": true, "red": true, "white": true, "yellow": true,
}

// parseSection tokenizes one section and analyses its layout.
func parseSection(s string) (*fmtSection, error) {
	sec := &fmtSection{expTok: -1}
	rs := []rune(s)
	for i := 0; i < len(rs); {
		c := rs[i]
		rest := strings.ToLower(string(rs[i:]))
		switch {
		case c == '"':
			j := i + 1
			for j < len(rs) && rs[j] != '"' {
				j++
			}
			sec.tokens = append(sec.tokens, fmtToken{kind: tokLiteral, text: string(rs[i+1 : j])})
			i = j + 1
		case c == '\\':
			sec.tokens = append(sec.tokens, fmtToken{kind: tokLiteral, text: string(rs[i+1])})
			i += 2
		case c == '_':
			// Padding the width of the next character: one space in text.
			sec.tokens = append(sec.tokens, fmtToken{kind: tokLiteral, text: " "})
			i += 2
		case c == '*':
			// Fill to the cell width: nothing in text.
			i += 2
		case c == '[':
			j := i + 1
			for j < len(rs) && rs[j] != ']' {
				j++
			}
			tok, cond, emit, err := bracketToken(string(rs[i+1 : j]))
			if err != nil {
				return nil, err
			}
			if cond != nil {
				sec.cond = cond
			}
			if emit {
				sec.tokens = append(sec.tokens, tok)
			}
			i = j + 1
		case c == '@':
			sec.tokens = append(sec.tokens, fmtToken{kind: tokText})
			i++
		case c == '0' || c == '#' || c == '?':
			sec.tokens = append(sec.tokens, fmtToken{kind: tokDigit, text: string(c)})
			i++
		case c == '.':
			sec.tokens = append(sec.tokens, fmtToken{kind: tokPoint})
			i++
		case c == ',':
			sec.tokens = append(sec.tokens, fmtToken{kind: tokComma})
			i++
		case c == '%':
			sec.tokens = append(sec.tokens, fmtToken{kind: tokPercent})
			i++
		case c == '/':
			sec.tokens = append(sec.tokens, fmtToken{kind: tokSlash})
			i++
		case (c == 'E' || c == 'e') && i+1 < len(rs) && (rs[i+1] == '+' || rs[i+1] == '-'):
			sec.tokens = append(sec.tokens, fmtToken{kind: tokExp, text: string(rs[i : i+2])})
			i += 2
		case strings.HasPrefix(rest, "general"):
			sec.tokens = append(sec.tokens, fmtToken{kind: tokGeneral})
			i += len("general")
		case strings.HasPrefix(rest, "am/pm"):
			sec.tokens = append(sec.tokens, fmtToken{kind: tokAMPM, text: string(rs[i : i+5])})
			i += 5
		case strings.HasPrefix(rest, "a/p"):
			sec.tokens = append(sec.tokens, fmtToken{kind: tokAMPM, text: string(rs[i : i+3])})
			i += 3
		case isDateLetter(c):
			j := i + 1
			for j < len(rs) && unicode.ToLower(rs[j]) == unicode.ToLower(c) {
				j++
			}
			sec.tokens = append(sec.tokens, fmtToken{kind: tokDate, field: dateFieldOf(c), n: j - i})
			i = j
		case unicode.IsLetter(c):
			// Era years, calendar switches, and letters no code family defines.
			return nil, unsupportedf("letter %q in %q", string(c), s)
		default:
			sec.tokens = append(sec.tokens, fmtToken{kind: tokLiteral, text: string(c)})
			i++
		}
	}
	if err := sec.analyse(); err != nil {
		return nil, err
	}
	return sec, nil
}

func isDateLetter(c rune) bool {
	switch unicode.ToLower(c) {
	case 'y', 'm', 'd', 'h', 's':
		return true
	}
	return false
}

func dateFieldOf(c rune) dateField {
	switch unicode.ToLower(c) {
	case 'y':
		return fieldYear
	case 'm':
		return fieldMonth
	case 'd':
		return fieldDay
	case 'h':
		return fieldHour
	default:
		return fieldSecond
	}
}

// bracketToken interprets the body of a [...] construct: a currency or locale
// tag, an elapsed-time field, a colour (dropped), or a condition.
func bracketToken(body string) (tok fmtToken, cond *fmtCond, emit bool, err error) {
	lb := strings.ToLower(body)
	switch {
	case strings.HasPrefix(body, "$"):
		// [$€-407] carries a currency symbol before the locale id; [$-409]
		// carries only the locale, which the invariant rendering ignores.
		sym := body[1:]
		if k := strings.IndexByte(sym, '-'); k >= 0 {
			sym = sym[:k]
		}
		if sym == "" {
			return fmtToken{}, nil, false, nil
		}
		return fmtToken{kind: tokLiteral, text: sym}, nil, true, nil
	case lb == "h" || lb == "hh" || lb == "m" || lb == "mm" || lb == "s" || lb == "ss":
		// Inside brackets 'm' is always minutes: elapsed months do not exist.
		field := dateFieldOf(rune(lb[0]))
		if field == fieldMonth {
			field = fieldMinute
		}
		return fmtToken{kind: tokElapsed, field: field, n: len(lb)}, nil, true, nil
	case fmtColors[lb] || strings.HasPrefix(lb, "color"):
		return fmtToken{}, nil, false, nil
	case strings.HasPrefix(body, "<") || strings.HasPrefix(body, ">") || strings.HasPrefix(body, "="):
		op := body[:1]
		if len(body) > 1 && (body[1] == '=' || body[1] == '>') {
			op = body[:2]
		}
		v, perr := strconv.ParseFloat(strings.TrimSpace(body[len(op):]), 64)
		if perr != nil {
			return fmtToken{}, nil, false, unsupportedf("condition [%s]", body)
		}
		return fmtToken{}, &fmtCond{op: op, value: v}, false, nil
	}
	return fmtToken{}, nil, false, unsupportedf("bracket [%s]", body)
}

// analyse classifies the section and records where its digits go.
func (s *fmtSection) analyse() error {
	hasDate, hasDigit, hasText, hasGeneral := false, false, false, false
	for _, t := range s.tokens {
		switch t.kind {
		case tokDate, tokAMPM, tokElapsed:
			hasDate = true
		case tokDigit:
			hasDigit = true
		case tokText:
			hasText = true
		case tokGeneral:
			hasGeneral = true
		}
	}
	switch {
	case hasDate:
		if hasText || hasGeneral {
			return unsupportedf("date section mixed with text or General")
		}
		s.kind = kindDate
		return s.analyseDate()
	case hasText:
		if hasDigit || hasGeneral {
			return unsupportedf("text section mixed with digits or General")
		}
		s.kind = kindText
		return nil
	case hasGeneral:
		if hasDigit {
			return unsupportedf("General mixed with digit placeholders")
		}
		s.kind = kindGeneral
		return nil
	default:
		s.kind = kindNumber
		return s.analyseNumber()
	}
}

// analyseDate resolves the month/minute ambiguity, folds fractional seconds,
// and settles the rounding unit.
func (s *fmtSection) analyseDate() error {
	toks := s.tokens
	// A '.' followed by zeros right after a seconds field is the fractional
	// second; any other digit placeholder has no meaning in a date.
	var out []fmtToken
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		switch t.kind {
		case tokPoint:
			prevSeconds := len(out) > 0 && (out[len(out)-1].kind == tokDate || out[len(out)-1].kind == tokElapsed) && out[len(out)-1].field == fieldSecond
			n := 0
			for i+1+n < len(toks) && toks[i+1+n].kind == tokDigit && toks[i+1+n].text == "0" {
				n++
			}
			if prevSeconds && n > 0 {
				out = append(out, fmtToken{kind: tokDate, field: fieldSecFrac, n: n})
				s.secFrac = n
				i += n
				continue
			}
			out = append(out, fmtToken{kind: tokLiteral, text: "."})
		case tokDigit:
			return unsupportedf("digit placeholder in a date section")
		case tokComma:
			out = append(out, fmtToken{kind: tokLiteral, text: ","})
		case tokSlash:
			out = append(out, fmtToken{kind: tokLiteral, text: "/"})
		case tokPercent:
			out = append(out, fmtToken{kind: tokLiteral, text: "%"})
		case tokExp:
			out = append(out, fmtToken{kind: tokLiteral, text: t.text})
		default:
			out = append(out, t)
		}
	}
	// An 'm' is a minute when the nearest time field before it is an hour or
	// the nearest after it is a second; otherwise it is a month.
	for i, t := range out {
		if t.kind != tokDate || t.field != fieldMonth {
			continue
		}
		minute := false
		for j := i - 1; j >= 0; j-- {
			if out[j].kind == tokDate || out[j].kind == tokElapsed {
				if out[j].field == fieldHour {
					minute = true
				}
				break
			}
		}
		if !minute {
			for j := i + 1; j < len(out); j++ {
				if out[j].kind == tokDate || out[j].kind == tokElapsed {
					if out[j].field == fieldSecond {
						minute = true
					}
					break
				}
			}
		}
		if minute {
			out[i].field = fieldMinute
		}
	}
	s.tokens = out

	hasSecond := false
	for _, t := range out {
		switch t.kind {
		case tokAMPM:
			s.hasAMPM = true
		case tokElapsed:
			s.usesTime = true
			switch t.field {
			case fieldHour:
				s.elapsedH = true
			case fieldMinute:
				s.elapsedM = true
				s.hasMinute = true
			case fieldSecond:
				s.elapsedS = true
				hasSecond = true
			}
		case tokDate:
			switch t.field {
			case fieldHour:
				s.usesTime = true
			case fieldMinute:
				s.usesTime = true
				s.hasMinute = true
			case fieldSecond:
				s.usesTime = true
				hasSecond = true
			}
		}
	}
	s.dateOnly = !s.usesTime
	switch {
	case s.secFrac > 0:
		s.roundUnit = 0 // rounded at the fractional-second digits instead
	case hasSecond:
		s.roundUnit = 1
	case s.hasMinute:
		s.roundUnit = 60
	default:
		s.roundUnit = 3600
	}
	return nil
}

// analyseNumber records the integer, decimal, exponent and fraction
// placeholders and the scaling a section applies.
func (s *fmtSection) analyseNumber() error {
	toks := s.tokens
	slash := -1
	for i, t := range toks {
		if t.kind == tokSlash {
			slash = i
			break
		}
	}
	if slash >= 0 {
		return s.analyseFraction(slash)
	}

	part := 0 // 0 integer, 1 decimals, 2 exponent
	lastIntDigit := -1
	for i, t := range toks {
		switch t.kind {
		case tokDigit:
			switch part {
			case 0:
				s.intIdx = append(s.intIdx, i)
				lastIntDigit = i
			case 1:
				s.fracIdx = append(s.fracIdx, i)
			default:
				s.expIdx = append(s.expIdx, i)
			}
		case tokPoint:
			if part == 0 {
				part = 1
			} else {
				toks[i] = fmtToken{kind: tokLiteral, text: "."}
			}
		case tokExp:
			if part == 2 {
				return unsupportedf("two exponents")
			}
			part = 2
			s.expTok = i
		case tokPercent:
			s.scale += 2
		}
	}
	// A comma between integer placeholders groups thousands; one after the
	// last placeholder of the number (before the point, or at its end) divides
	// by a thousand; elsewhere it is text.
	lastDigit := lastIntDigit
	if n := len(s.fracIdx); n > 0 {
		lastDigit = s.fracIdx[n-1]
	}
	for i, t := range toks {
		if t.kind != tokComma {
			continue
		}
		switch {
		case lastIntDigit >= 0 && i < lastIntDigit && i > s.intIdx[0]:
			s.grouping = true
		case lastIntDigit >= 0 && i > lastIntDigit && followsScaling(toks, lastIntDigit, i) && (len(s.fracIdx) == 0 || i < s.fracIdx[0]):
			s.scale -= 3
		case lastDigit >= 0 && i > lastDigit && followsScaling(toks, lastDigit, i):
			s.scale -= 3
		default:
			toks[i] = fmtToken{kind: tokLiteral, text: ","}
		}
	}
	return nil
}

// followsScaling reports whether every token between a digit placeholder and
// a comma is itself a scaling comma, so the comma extends the scaling run.
func followsScaling(toks []fmtToken, digit, comma int) bool {
	for j := digit + 1; j < comma; j++ {
		if toks[j].kind != tokComma {
			return false
		}
	}
	return true
}

// analyseFraction lays out a fraction section: the digit group right before
// the bar is the numerator, the group before that (if any) the whole number,
// and the denominator is either a digit group or a literal integer.
func (s *fmtSection) analyseFraction(slash int) error {
	toks := s.tokens
	f := &fracSpec{}
	// Numerator: the contiguous digit run ending at the bar.
	j := slash - 1
	for j >= 0 && toks[j].kind == tokDigit {
		f.numIdx = append([]int{j}, f.numIdx...)
		j--
	}
	if len(f.numIdx) == 0 {
		return unsupportedf("fraction without a numerator")
	}
	f.numStart = f.numIdx[0]
	for k := j; k >= 0; k-- {
		if toks[k].kind == tokDigit {
			f.intIdx = append([]int{k}, f.intIdx...)
		}
	}
	// Denominator: digit placeholders, or a literal integer.
	k := slash + 1
	for k < len(toks) && toks[k].kind == tokDigit {
		f.denIdx = append(f.denIdx, k)
		k++
	}
	if len(f.denIdx) == 0 {
		// A literal denominator (?/8) is digits written as text; they stay
		// literal tokens and render as written.
		var lit strings.Builder
		for k < len(toks) && toks[k].kind == tokLiteral && len(toks[k].text) == 1 && toks[k].text[0] >= '0' && toks[k].text[0] <= '9' {
			lit.WriteString(toks[k].text)
			k++
		}
		d, err := strconv.Atoi(lit.String())
		if err != nil || d <= 0 {
			return unsupportedf("fraction without a denominator")
		}
		f.fixedDen = d
	}
	for i, t := range toks {
		if t.kind == tokComma || t.kind == tokPoint || t.kind == tokPercent || t.kind == tokExp {
			return unsupportedf("token %d in a fraction section", i)
		}
	}
	s.frac = f
	return nil
}

// --- rendering ---

// section picks the section that renders v and whether the value is shown
// with its own sign. Positional sections are positive;negative;zero: the
// negative section shows the magnitude and supplies its own sign or
// parentheses. A conditional section applies when its condition holds; the
// first unconditional section is the fallback.
func (nf *numberFormat) section(v float64) (sec *fmtSection, showSign bool) {
	conditional := false
	for _, s := range nf.sections {
		if s.cond != nil {
			conditional = true
			break
		}
	}
	if conditional {
		for _, s := range nf.sections {
			if s.cond != nil && s.cond.matches(v) {
				// A section selecting negatives spells its own sign.
				return s, !strings.HasPrefix(s.cond.op, "<")
			}
		}
		for _, s := range nf.sections {
			if s.cond == nil {
				return s, true
			}
		}
		return nf.sections[0], true
	}
	switch n := len(nf.sections); {
	case v < 0 && n >= 2:
		return nf.sections[1], false
	case v == 0 && n >= 3:
		return nf.sections[2], true
	default:
		return nf.sections[0], true
	}
}

// formatNumber renders a numeric value.
func (nf *numberFormat) formatNumber(v float64, date1904 bool) (string, error) {
	sec, showSign := nf.section(v)
	var out string
	var err error
	switch sec.kind {
	case kindText:
		// A number under a text-only section shows the General rendering
		// where the text would go.
		out = sec.renderText(formatGeneral(v))
		return strings.TrimSpace(out), nil
	case kindGeneral:
		out = sec.renderGeneral(v, showSign)
	case kindDate:
		out, err = sec.renderDate(v, date1904)
	default:
		out = sec.renderNumber(v, showSign)
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// formatText renders a text value: through the text section when the code
// has one, unchanged otherwise.
func (nf *numberFormat) formatText(text string) string {
	var sec *fmtSection
	if len(nf.sections) == 4 {
		sec = nf.sections[3]
	} else if len(nf.sections) == 1 && nf.sections[0].kind == kindText {
		sec = nf.sections[0]
	}
	if sec == nil || sec.kind != kindText {
		return text
	}
	return strings.TrimSpace(sec.renderText(text))
}

func (s *fmtSection) renderText(text string) string {
	var b strings.Builder
	for _, t := range s.tokens {
		switch t.kind {
		case tokText:
			b.WriteString(text)
		case tokLiteral:
			b.WriteString(t.text)
		}
	}
	return b.String()
}

func (s *fmtSection) renderGeneral(v float64, showSign bool) string {
	var b strings.Builder
	g := formatGeneral(math.Abs(v))
	if showSign && v < 0 {
		g = "-" + g
	}
	for _, t := range s.tokens {
		switch t.kind {
		case tokGeneral:
			b.WriteString(g)
		case tokLiteral:
			b.WriteString(t.text)
		}
	}
	return b.String()
}

// renderNumber renders v through a number section: the value is scaled and
// rounded to the placeholders, its digits are dealt to them (integer digits
// from the right, decimals from the left), and the tokens are emitted in
// order with the digits in place.
func (s *fmtSection) renderNumber(v float64, showSign bool) string {
	x := math.Abs(v) * math.Pow(10, float64(s.scale))
	out := map[int]string{}
	var expText string

	stopAt := len(s.tokens)
	switch {
	case s.frac != nil:
		if s.dealFraction(x, out) {
			// A whole number shows no fraction part.
			stopAt = s.frac.numStart
		}
	case s.expTok >= 0:
		expText = s.dealScientific(x, out)
	default:
		s.dealFixed(x, out)
	}

	var b strings.Builder
	if showSign && v < 0 {
		b.WriteString("-")
	}
	for i, t := range s.tokens {
		if i >= stopAt {
			break
		}
		if d, ok := out[i]; ok {
			b.WriteString(d)
			continue
		}
		switch t.kind {
		case tokLiteral:
			b.WriteString(t.text)
		case tokPoint:
			b.WriteString(".")
		case tokPercent:
			b.WriteString("%")
		case tokExp:
			b.WriteString(expText)
		case tokSlash:
			b.WriteString("/")
		}
	}
	return b.String()
}

// dealFixed rounds x to the decimal placeholders and deals its digits.
func (s *fmtSection) dealFixed(x float64, out map[int]string) {
	intDigits, fracDigits := roundFixed(x, len(s.fracIdx))
	s.dealInteger(intDigits, s.intIdx, s.grouping, out)
	s.dealDecimals(fracDigits, s.fracIdx, out)
}

// roundFixed renders a magnitude with exactly `decimals` digits after the
// point, returning the integer and fraction digit strings. The sheet holds
// fifteen significant digits and rounds half away from zero on them, so 1.005
// under 0.00 is 1.01 and 2.5 under 0 is 3; a binary float formatter would
// round both down. The arithmetic is done on the decimal digits for that
// reason.
func roundFixed(x float64, decimals int) (intDigits, fracDigits string) {
	s := strconv.FormatFloat(math.Abs(x), 'e', 14, 64)
	mant, expStr, _ := strings.Cut(s, "e")
	exp, _ := strconv.Atoi(expStr)
	digits := strings.Replace(mant, ".", "", 1)
	pointPos := exp + 1
	switch {
	case pointPos <= 0:
		intDigits = "0"
		fracDigits = strings.Repeat("0", -pointPos) + digits
	case pointPos >= len(digits):
		intDigits = digits + strings.Repeat("0", pointPos-len(digits))
	default:
		intDigits = digits[:pointPos]
		fracDigits = digits[pointPos:]
	}
	if len(fracDigits) > decimals {
		up := fracDigits[decimals] >= '5'
		fracDigits = fracDigits[:decimals]
		if up {
			all := incrementDecimal(intDigits + fracDigits)
			intDigits = all[:len(all)-decimals]
			fracDigits = all[len(all)-decimals:]
		}
	} else {
		fracDigits += strings.Repeat("0", decimals-len(fracDigits))
	}
	intDigits = strings.TrimLeft(intDigits, "0")
	if intDigits == "" {
		intDigits = "0"
	}
	return intDigits, fracDigits
}

// incrementDecimal adds one to a string of decimal digits.
func incrementDecimal(digits string) string {
	b := []byte(digits)
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] < '9' {
			b[i]++
			return string(b)
		}
		b[i] = '0'
	}
	return "1" + string(b)
}

// dealScientific normalizes x to the integer placeholders (an engineering
// exponent when there are several) and deals mantissa and exponent digits.
func (s *fmtSection) dealScientific(x float64, out map[int]string) string {
	width := len(s.intIdx)
	if width == 0 {
		width = 1
	}
	exp := 0
	if x != 0 {
		// Several integer placeholders make an engineering exponent (a
		// multiple of their count), so ##0.0E+0 shows 12345 as 12.3E+3.
		exp = int(math.Floor(float64(decimalExponent(x))/float64(width))) * width
	}
	mant := x / math.Pow(10, float64(exp))
	intDigits, fracDigits := roundFixed(mant, len(s.fracIdx))
	if x != 0 && len(intDigits) > width {
		// Rounding carried into a new digit: renormalize once.
		exp += width
		mant = x / math.Pow(10, float64(exp))
		intDigits, fracDigits = roundFixed(mant, len(s.fracIdx))
	}
	s.dealInteger(intDigits, s.intIdx, false, out)
	s.dealDecimals(fracDigits, s.fracIdx, out)

	marker := s.tokens[s.expTok].text
	sign := ""
	switch {
	case exp < 0:
		sign = "-"
	case marker[1] == '+':
		sign = "+"
	}
	digits := strconv.Itoa(absInt(exp))
	for len(digits) < len(s.expIdx) {
		digits = "0" + digits
	}
	for _, i := range s.expIdx {
		out[i] = ""
	}
	if len(s.expIdx) > 0 {
		out[s.expIdx[0]] = digits
	}
	return string(marker[0]) + sign
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// dealFraction approximates x as a (mixed) fraction and deals the parts. It
// reports whether the value is a whole number under a mixed-fraction section,
// which then shows no fraction at all.
func (s *fmtSection) dealFraction(x float64, out map[int]string) (wholeOnly bool) {
	f := s.frac
	whole := 0.0
	rem := x
	if len(f.intIdx) > 0 {
		whole = math.Floor(x)
		rem = x - whole
	}
	var num, den int
	if f.fixedDen > 0 {
		den = f.fixedDen
		num = int(math.Round(rem * float64(den)))
	} else {
		maxDen := int(math.Pow(10, float64(len(f.denIdx)))) - 1
		bestErr := math.Inf(1)
		for d := 1; d <= maxDen; d++ {
			n := math.Round(rem * float64(d))
			if e := math.Abs(rem - n/float64(d)); e < bestErr-1e-12 {
				bestErr, num, den = e, int(n), d
				if e == 0 {
					break
				}
			}
		}
	}
	if den > 0 && num == den {
		whole++
		num = 0
	}
	if len(f.intIdx) > 0 {
		s.dealInteger(strconv.FormatFloat(whole, 'f', 0, 64), f.intIdx, false, out)
	}
	if num == 0 && len(f.intIdx) > 0 {
		return true
	}
	if num == 0 {
		den = 1
	}
	// The numerator is right-aligned to its placeholders, the denominator
	// left-aligned; '?' placeholders pad with spaces as in the sheet. A zero
	// numerator is a digit here (0/1), never an elided integer part.
	s.dealInteger(strconv.Itoa(num), f.numIdx, false, out)
	if num == 0 {
		out[f.numIdx[len(f.numIdx)-1]] = "0"
	}
	if len(f.denIdx) > 0 {
		ds := strconv.Itoa(den)
		out[f.denIdx[0]] = ds + strings.Repeat(" ", max(0, len(f.denIdx)-len(ds)))
		for _, i := range f.denIdx[1:] {
			out[i] = ""
		}
	}
	return false
}

// dealInteger deals an integer digit string to its placeholders from the
// right: each placeholder takes one digit (plus the grouping separator to its
// right), the leftmost takes whatever remains. A placeholder with no digit
// left renders its empty form: '0' a zero, '?' a space, '#' nothing.
func (s *fmtSection) dealInteger(digits string, idx []int, grouping bool, out map[int]string) {
	if len(idx) == 0 {
		return
	}
	zeros := 0
	for _, i := range idx {
		if s.tokens[i].text == "0" {
			zeros++
		}
	}
	// No forced digit and a zero value: the integer part is empty (#.00 of
	// 0.5 is ".50").
	if digits == "0" && zeros == 0 {
		digits = ""
	}
	for len(digits) < zeros {
		digits = "0" + digits
	}
	if grouping {
		digits = groupThousands(digits)
	}
	rem := digits
	for k := len(idx) - 1; k >= 1; k-- {
		i := idx[k]
		if rem == "" {
			out[i] = emptyPlaceholder(s.tokens[i].text)
			continue
		}
		// Take one digit and any separator that trails it.
		j := len(rem) - 1
		for j >= 0 && rem[j] == ',' {
			j--
		}
		if j < 0 {
			out[i] = rem
			rem = ""
			continue
		}
		out[i] = rem[j:]
		rem = rem[:j]
	}
	first := idx[0]
	if rem == "" {
		out[first] = emptyPlaceholder(s.tokens[first].text)
	} else {
		out[first] = rem
	}
}

// dealDecimals deals decimal digits to their placeholders from the left; a
// '#' or '?' whose digit is a trailing zero renders its empty form.
func (s *fmtSection) dealDecimals(digits string, idx []int, out map[int]string) {
	last := len(digits) - 1
	for last >= 0 && digits[last] == '0' {
		last--
	}
	for k, i := range idx {
		switch {
		case k < len(digits) && k <= last:
			out[i] = string(digits[k])
		case s.tokens[i].text == "0":
			out[i] = "0"
		default:
			out[i] = emptyPlaceholder(s.tokens[i].text)
		}
	}
}

func emptyPlaceholder(ph string) string {
	switch ph {
	case "0":
		return "0"
	case "?":
		return " "
	}
	return ""
}

// groupThousands inserts a comma every three digits from the right.
func groupThousands(digits string) string {
	if len(digits) <= 3 {
		return digits
	}
	var b strings.Builder
	head := len(digits) % 3
	if head > 0 {
		b.WriteString(digits[:head])
	}
	for i := head; i < len(digits); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

// decimalExponent is floor(log10(x)) for x > 0, read from the decimal
// representation so an exact power of ten is never off by one.
func decimalExponent(x float64) int {
	s := strconv.FormatFloat(x, 'e', -1, 64)
	_, e, _ := strings.Cut(s, "e")
	n, _ := strconv.Atoi(e)
	return n
}

// formatGeneral renders v the way the General format does: up to ten
// significant digits in fixed notation, scientific notation with up to five
// decimals beyond eleven characters, and no trailing zeros.
func formatGeneral(v float64) string {
	if v == 0 {
		return "0"
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return strconv.FormatFloat(v, 'g', -1, 64)
	}
	neg := v < 0
	a := math.Abs(v)
	// A cell holds fifteen significant digits; anything beyond is noise from
	// the stored representation.
	a, _ = strconv.ParseFloat(strconv.FormatFloat(a, 'e', 14, 64), 64)
	e := decimalExponent(a)
	var s string
	switch {
	case e >= -4 && e <= -1:
		s = joinFixed(roundFixed(a, 9))
	case e >= 0 && e <= 9:
		s = joinFixed(roundFixed(a, 9-e))
	case e == 10:
		s = joinFixed(roundFixed(a, 0))
	default:
		s = generalScientific(a)
	}
	s = trimFractionZeros(s)
	if !strings.Contains(s, "E") && len(s) > 11 {
		s = generalScientific(a)
	}
	if neg {
		s = "-" + s
	}
	return s
}

// generalScientific renders a magnitude as General's scientific form: a
// mantissa of up to five decimals and a signed two-digit exponent (1.23457E+11).
func generalScientific(a float64) string {
	s := strconv.FormatFloat(a, 'e', 5, 64)
	mant, exp, _ := strings.Cut(s, "e")
	n, _ := strconv.Atoi(exp)
	return trimFractionZeros(mant) + fmt.Sprintf("E%+03d", n)
}

// joinFixed joins roundFixed's digit strings into a decimal.
func joinFixed(intDigits, fracDigits string) string {
	if fracDigits == "" {
		return intDigits
	}
	return intDigits + "." + fracDigits
}

// trimFractionZeros drops trailing zeros of a fixed decimal, and the point
// when nothing follows it.
func trimFractionZeros(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}
