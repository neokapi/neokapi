package math

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

// mathNS is the OMML namespace (ECMA-376 Part 1 §22.1).
const mathNS = "http://schemas.openxmlformats.org/officeDocument/2006/math"

type parser struct{ dec *xml.Decoder }

// wprNS is the WordprocessingML namespace (run/paragraph props embedded in OMML).
const wprNS = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

// parseOMML decodes an <m:oMath>/<m:oMathPara> fragment into a Math AST. An OMML
// subtree captured from a .docx carries no namespace declarations of its own
// (they sit on an ancestor), so we wrap it in a root that binds the m: and w:
// prefixes before decoding — otherwise the prefixes are unbound and nothing
// resolves to the math namespace.
func parseOMML(raw []byte) (*Math, error) {
	wrapped := `<ommlRoot xmlns:m="` + mathNS + `" xmlns:w="` + wprNS + `">` + string(raw) + `</ommlRoot>`
	p := &parser{dec: xml.NewDecoder(bytes.NewReader([]byte(wrapped)))}
	for {
		tok, err := p.dec.Token()
		if errors.Is(err, io.EOF) {
			return &Math{Body: Row{}}, nil
		}
		if err != nil {
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Space != mathNS {
			continue
		}
		switch se.Name.Local {
		case "oMathPara":
			items, err := p.seq(se.Name)
			if err != nil {
				return nil, err
			}
			return &Math{Body: row(items), Block: true}, nil
		case "oMath":
			items, err := p.seq(se.Name)
			if err != nil {
				return nil, err
			}
			return &Math{Body: row(items)}, nil
		}
	}
}

// seq reads a sequence of math child nodes until the matching end element,
// skipping property/control/non-math elements.
func (p *parser) seq(end xml.Name) ([]Exp, error) {
	var out []Exp
	for {
		tok, err := p.dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space != mathNS {
				p.skip(t.Name) // foreign (w:rPr etc.) — consume
				continue
			}
			n, err := p.node(t)
			if err != nil {
				return nil, err
			}
			if n != nil {
				out = append(out, n)
			}
		case xml.EndElement:
			if t.Name == end {
				return out, nil
			}
		}
	}
}

// arg reads one argument element's content (e/num/den/sub/sup/deg/fName/lim …)
// as a single expression.
func (p *parser) arg(end xml.Name) (Exp, error) {
	items, err := p.seq(end)
	if err != nil {
		return nil, err
	}
	return row(items), nil
}

// skip consumes tokens until the matching end element.
func (p *parser) skip(end xml.Name) {
	depth := 1
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name == end {
				depth++
			}
		case xml.EndElement:
			if t.Name == end {
				depth--
				if depth == 0 {
					return
				}
			}
		}
	}
}

// props reads a property element (…Pr) and returns its direct children's m:val
// attributes keyed by child local name (e.g. naryPr → {chr:"∑"}), plus presence
// flags for valueless children (e.g. {noBar:""} would appear if present).
func (p *parser) props(end xml.Name) map[string]string {
	m := map[string]string{}
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return m
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space == mathNS {
				val := ""
				for _, a := range t.Attr {
					if a.Name.Local == "val" {
						val = a.Value
					}
				}
				m[t.Name.Local] = val
			}
			if t.Name != end {
				p.skip(t.Name)
			}
		case xml.EndElement:
			if t.Name == end {
				return m
			}
		}
	}
}

// mn makes an element name in the math namespace (for matching ends).
func mn(local string) xml.Name { return xml.Name{Space: mathNS, Local: local} }

// node parses one math structural element (the decoder is positioned just after
// its StartElement) and returns its Exp.
func (p *parser) node(start xml.StartElement) (Exp, error) {
	end := start.Name
	switch start.Name.Local {
	case "oMath":
		// An <m:oMath> nested inside <m:oMathPara>: parse its sequence.
		items, err := p.seq(end)
		if err != nil {
			return nil, err
		}
		return row(items), nil
	case "r":
		return p.run(end)
	case "f":
		return p.fraction(end)
	case "sSup":
		return p.script(end, "sup")
	case "sSub":
		return p.script(end, "sub")
	case "sSubSup":
		return p.script(end, "subsup")
	case "rad":
		return p.radical(end)
	case "nary":
		return p.nary(end)
	case "d":
		return p.delim(end)
	case "func":
		return p.function(end)
	case "m":
		return p.matrix(end)
	case "acc":
		return p.accent(end)
	case "bar":
		return p.bar(end)
	case "groupChr":
		return p.groupChr(end)
	case "limLow", "limUpp":
		return p.limit(end, start.Name.Local)
	case "box", "borderBox", "phant":
		// Wrappers: parse the inner <m:e> as the content.
		return p.arg(end)
	case "eqArr":
		return p.eqArr(end)
	case "sPre":
		return p.sPre(end)
	default:
		// Unmodeled element: consume and drop (best-effort).
		p.skip(end)
		return nil, nil
	}
}

// run parses <m:r>: optional <m:rPr> (carries <m:nor/> normal-text flag) +
// <m:t> text. The text is tokenized into number/identifier/operator nodes,
// unless it is normal text (prose) which is kept whole and translatable.
func (p *parser) run(end xml.Name) (Exp, error) {
	normal := false
	var text strings.Builder
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return row(nil), err //nolint:nilerr // best-effort on truncation
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch {
			case t.Name.Space == mathNS && t.Name.Local == "rPr":
				if _, ok := p.props(t.Name)["nor"]; ok {
					normal = true
				}
			case t.Name.Space == mathNS && t.Name.Local == "t":
				s, _ := p.text(t.Name)
				text.WriteString(s)
			default:
				p.skip(t.Name)
			}
		case xml.EndElement:
			if t.Name == end {
				s := text.String()
				if normal {
					return Text{Content: s, Normal: true}, nil
				}
				return row(classifyRunText(s)), nil
			}
		}
	}
}

// text reads the character data of a simple element until its end.
func (p *parser) text(end xml.Name) (string, error) {
	var b strings.Builder
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return b.String(), err
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.EndElement:
			if t.Name == end {
				return b.String(), nil
			}
		}
	}
}

func (p *parser) fraction(end xml.Name) (Exp, error) {
	var num, den Exp
	noBar := false
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return Fraction{Num: num, Den: den, NoBar: noBar}, nil //nolint:nilerr // best-effort: keep the partial fraction on truncation
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch {
			case t.Name == mn("fPr"):
				if p.props(t.Name)["type"] == "noBar" {
					noBar = true
				}
			case t.Name == mn("num"):
				num, _ = p.arg(t.Name)
			case t.Name == mn("den"):
				den, _ = p.arg(t.Name)
			default:
				p.skip(t.Name)
			}
		case xml.EndElement:
			if t.Name == end {
				return Fraction{Num: num, Den: den, NoBar: noBar}, nil
			}
		}
	}
}

// script handles sSup/sSub/sSubSup (kind = sup|sub|subsup).
func (p *parser) script(end xml.Name, kind string) (Exp, error) {
	var base, sub, sup Exp
	for {
		tok, err := p.dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name {
			case mn("e"):
				base, _ = p.arg(t.Name)
			case mn("sub"):
				sub, _ = p.arg(t.Name)
			case mn("sup"):
				sup, _ = p.arg(t.Name)
			default:
				p.skip(t.Name)
			}
		case xml.EndElement:
			if t.Name == end {
				goto done
			}
		}
	}
done:
	switch kind {
	case "sup":
		return Superscript{Base: base, Sup: sup}, nil
	case "sub":
		return Subscript{Base: base, Sub: sub}, nil
	default:
		return SubSup{Base: base, Sub: sub, Sup: sup}, nil
	}
}

func (p *parser) radical(end xml.Name) (Exp, error) {
	var deg, body Exp
	degHide := false
	for {
		tok, err := p.dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name {
			case mn("radPr"):
				if _, ok := p.props(t.Name)["degHide"]; ok {
					degHide = true
				}
			case mn("deg"):
				deg, _ = p.arg(t.Name)
			case mn("e"):
				body, _ = p.arg(t.Name)
			default:
				p.skip(t.Name)
			}
		case xml.EndElement:
			if t.Name == end {
				goto done
			}
		}
	}
done:
	if degHide || isEmpty(deg) {
		deg = nil
	}
	return Radical{Degree: deg, Body: body}, nil
}

func (p *parser) nary(end xml.Name) (Exp, error) {
	chr := "∫" // OMML default n-ary char is the integral
	var sub, sup, body Exp
	subHide, supHide := false, false
	for {
		tok, err := p.dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name {
			case mn("naryPr"):
				pr := p.props(t.Name)
				if v, ok := pr["chr"]; ok {
					chr = v
				}
				if _, ok := pr["subHide"]; ok {
					subHide = true
				}
				if _, ok := pr["supHide"]; ok {
					supHide = true
				}
			case mn("sub"):
				sub, _ = p.arg(t.Name)
			case mn("sup"):
				sup, _ = p.arg(t.Name)
			case mn("e"):
				body, _ = p.arg(t.Name)
			default:
				p.skip(t.Name)
			}
		case xml.EndElement:
			if t.Name == end {
				goto done
			}
		}
	}
done:
	if subHide {
		sub = nil
	}
	if supHide {
		sup = nil
	}
	return Nary{Chr: chr, Sub: sub, Sup: sup, Body: body}, nil
}

func (p *parser) delim(end xml.Name) (Exp, error) {
	open, close := "(", ")"
	var parts []Exp
	for {
		tok, err := p.dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name {
			case mn("dPr"):
				pr := p.props(t.Name)
				if v, ok := pr["begChr"]; ok {
					open = v
				}
				if v, ok := pr["endChr"]; ok {
					close = v
				}
			case mn("e"):
				e, _ := p.arg(t.Name)
				parts = append(parts, e)
			default:
				p.skip(t.Name)
			}
		case xml.EndElement:
			if t.Name == end {
				goto done
			}
		}
	}
done:
	var body Exp
	if len(parts) == 1 {
		body = parts[0]
	} else {
		// Multiple operands are separated by '|' (or the dPr sepChr); join simply.
		var items []Exp
		for i, e := range parts {
			if i > 0 {
				items = append(items, Operator{Text: "|"})
			}
			items = append(items, e)
		}
		body = row(items)
	}
	return Delimited{Open: open, Close: close, Body: body}, nil
}

func (p *parser) function(end xml.Name) (Exp, error) {
	var name, arg Exp
	for {
		tok, err := p.dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name {
			case mn("fName"):
				name, _ = p.arg(t.Name)
			case mn("e"):
				arg, _ = p.arg(t.Name)
			default:
				p.skip(t.Name)
			}
		case xml.EndElement:
			if t.Name == end {
				goto done
			}
		}
	}
done:
	return Function{Name: flatText(name), Arg: arg}, nil
}

func (p *parser) matrix(end xml.Name) (Exp, error) {
	var rows [][]Exp
	for {
		tok, err := p.dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name == mn("mr") {
				rows = append(rows, p.matrixRow(t.Name))
			} else {
				p.skip(t.Name)
			}
		case xml.EndElement:
			if t.Name == end {
				return Matrix{Rows: rows}, nil
			}
		}
	}
	return Matrix{Rows: rows}, nil
}

func (p *parser) matrixRow(end xml.Name) []Exp {
	var cells []Exp
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return cells
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name == mn("e") {
				e, _ := p.arg(t.Name)
				cells = append(cells, e)
			} else {
				p.skip(t.Name)
			}
		case xml.EndElement:
			if t.Name == end {
				return cells
			}
		}
	}
}

func (p *parser) accent(end xml.Name) (Exp, error) {
	chr := "̂" // combining circumflex (default OMML accent)
	var body Exp
	for {
		tok, err := p.dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name {
			case mn("accPr"):
				if v, ok := p.props(t.Name)["chr"]; ok {
					chr = v
				}
			case mn("e"):
				body, _ = p.arg(t.Name)
			default:
				p.skip(t.Name)
			}
		case xml.EndElement:
			if t.Name == end {
				return Accent{Accent: chr, Body: body}, nil
			}
		}
	}
	return Accent{Accent: chr, Body: body}, nil
}

func (p *parser) bar(end xml.Name) (Exp, error) {
	top := true
	var body Exp
	for {
		tok, err := p.dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name {
			case mn("barPr"):
				if p.props(t.Name)["pos"] == "bot" {
					top = false
				}
			case mn("e"):
				body, _ = p.arg(t.Name)
			default:
				p.skip(t.Name)
			}
		case xml.EndElement:
			if t.Name == end {
				return Bar{Body: body, Top: top}, nil
			}
		}
	}
	return Bar{Body: body, Top: top}, nil
}

func (p *parser) groupChr(end xml.Name) (Exp, error) {
	chr, pos := "", "bot"
	var body Exp
	for {
		tok, err := p.dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name {
			case mn("groupChrPr"):
				pr := p.props(t.Name)
				if v, ok := pr["chr"]; ok {
					chr = v
				}
				if v, ok := pr["pos"]; ok {
					pos = v
				}
			case mn("e"):
				body, _ = p.arg(t.Name)
			default:
				p.skip(t.Name)
			}
		case xml.EndElement:
			if t.Name == end {
				return GroupChr{Chr: chr, Pos: pos, Body: body}, nil
			}
		}
	}
	return GroupChr{Chr: chr, Pos: pos, Body: body}, nil
}

// limit handles limLow/limUpp (a base with a limit under/over it).
func (p *parser) limit(end xml.Name, kind string) (Exp, error) {
	var base, lim Exp
	for {
		tok, err := p.dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name {
			case mn("e"):
				base, _ = p.arg(t.Name)
			case mn("lim"):
				lim, _ = p.arg(t.Name)
			default:
				p.skip(t.Name)
			}
		case xml.EndElement:
			if t.Name == end {
				goto done
			}
		}
	}
done:
	if kind == "limUpp" {
		return Superscript{Base: base, Sup: lim}, nil
	}
	return Subscript{Base: base, Sub: lim}, nil
}

// eqArr is an equation array: stack rows vertically (model as a 1-column matrix).
func (p *parser) eqArr(end xml.Name) (Exp, error) {
	var rows [][]Exp
	for {
		tok, err := p.dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name == mn("e") {
				e, _ := p.arg(t.Name)
				rows = append(rows, []Exp{e})
			} else {
				p.skip(t.Name)
			}
		case xml.EndElement:
			if t.Name == end {
				return Matrix{Rows: rows}, nil
			}
		}
	}
	return Matrix{Rows: rows}, nil
}

// sPre is a pre-sub/superscript (scripts before the base). Model it as the
// base with leading scripts rendered via a SubSup on an empty base + base.
func (p *parser) sPre(end xml.Name) (Exp, error) {
	var base, sub, sup Exp
	for {
		tok, err := p.dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name {
			case mn("e"):
				base, _ = p.arg(t.Name)
			case mn("sub"):
				sub, _ = p.arg(t.Name)
			case mn("sup"):
				sup, _ = p.arg(t.Name)
			default:
				p.skip(t.Name)
			}
		case xml.EndElement:
			if t.Name == end {
				goto done
			}
		}
	}
done:
	pre := SubSup{Base: Row{}, Sub: sub, Sup: sup}
	return row([]Exp{pre, base}), nil
}

func isEmpty(e Exp) bool {
	if e == nil {
		return true
	}
	if r, ok := e.(Row); ok {
		return len(r.Items) == 0
	}
	return false
}
