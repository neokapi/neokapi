// Package jsonobject adds and removes members of the objects in a JSON
// document while keeping every other byte. The JSON and ARB formats use it to
// write insert_block and delete_block: in a key-value catalog a block's shell
// is its member, the key and the value beside it, and the separators around
// it follow the layout the document already uses.
//
// A format hands over the tokens its own scanner read, each with the
// whitespace and comments before it, so the document is read exactly as the
// format reads it (JSON5 comments, single quotes and bare keys included).
package jsonobject

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Kind is the kind of a token.
type Kind int

// The token kinds.
const (
	ObjectStart Kind = iota
	ObjectEnd
	ArrayStart
	ArrayEnd
	Colon
	Comma
	// String is a string, or a JSON5 bare key.
	String
	// Scalar is a number, true, false or null.
	Scalar
	EOF
)

// Token is one token as a format's scanner read it.
type Token struct {
	Kind Kind
	// Prefix is the whitespace and comments before the token; Raw is the
	// token's own bytes.
	Prefix string
	Raw    string
	// Text is a string's decoded value.
	Text string
}

// Doc is a document read into its objects and their members.
type Doc struct {
	toks []Token
	// at[i] is the offset of token i's Raw in the document.
	at []int
	// Root is the top-level object, nil when the document holds another
	// value.
	Root *Object
	// members lists every member of every object, in document order.
	members []*Member
}

// Object is one object of the document.
type Object struct {
	// Path is the key path of the object, as a reader names it: keys joined
	// with dots, an array element as [i].
	Path    string
	open    int
	close   int
	Members []*Member
}

// Member is one key and its value.
type Member struct {
	Key string
	// Path is the member's key path: the object's path and the key.
	Path   string
	Object *Object
	// key, colon, valStart and valEnd are token indexes; comma is the comma
	// after the value, -1 when there is none.
	key, colon, valStart, valEnd, comma int
}

// Parse reads toks, the tokens of src, into a Doc. The tokens must spell src
// exactly, and form one value.
func Parse(src []byte, toks []Token) (*Doc, error) {
	d := &Doc{toks: toks, at: make([]int, len(toks))}
	off := 0
	for i, t := range toks {
		off += len(t.Prefix)
		d.at[i] = off
		off += len(t.Raw)
	}
	if off != len(src) {
		return nil, fmt.Errorf("the tokens spell %d bytes of a %d-byte document", off, len(src))
	}
	p := &parser{d: d}
	if len(toks) == 0 {
		return d, nil
	}
	if toks[0].Kind == ObjectStart {
		o, err := p.object(0, "")
		if err != nil {
			return nil, err
		}
		d.Root = o
		return d, nil
	}
	if _, err := p.value(0, ""); err != nil {
		return nil, err
	}
	return d, nil
}

type parser struct{ d *Doc }

// value reads the value at token i and returns the index of its last token.
func (p *parser) value(i int, path string) (int, error) {
	toks := p.d.toks
	if i >= len(toks) {
		return 0, errors.New("the document ends where a value was expected")
	}
	switch toks[i].Kind {
	case ObjectStart:
		o, err := p.object(i, path)
		if err != nil {
			return 0, err
		}
		return o.close, nil
	case ArrayStart:
		return p.array(i, path)
	case String, Scalar:
		return i, nil
	}
	return 0, fmt.Errorf("unexpected %q where a value was expected", toks[i].Raw)
}

func (p *parser) object(i int, path string) (*Object, error) {
	toks := p.d.toks
	o := &Object{Path: path, open: i}
	i++
	for i < len(toks) {
		switch toks[i].Kind {
		case ObjectEnd:
			o.close = i
			return o, nil
		case Comma:
			i++
			continue
		case String:
		default:
			return nil, fmt.Errorf("unexpected %q where a key was expected", toks[i].Raw)
		}
		m := &Member{Key: toks[i].Text, Path: Join(path, toks[i].Text), Object: o, key: i, comma: -1}
		if i+1 >= len(toks) || toks[i+1].Kind != Colon {
			return nil, fmt.Errorf("the key %q has no colon after it", m.Key)
		}
		m.colon = i + 1
		m.valStart = i + 2
		end, err := p.value(m.valStart, m.Path)
		if err != nil {
			return nil, err
		}
		m.valEnd = end
		i = end + 1
		if i < len(toks) && toks[i].Kind == Comma {
			m.comma = i
			i++
		}
		o.Members = append(o.Members, m)
		p.d.members = append(p.d.members, m)
	}
	return nil, errors.New("an object is not closed")
}

func (p *parser) array(i int, path string) (int, error) {
	toks := p.d.toks
	i++
	n := 0
	for i < len(toks) {
		switch toks[i].Kind {
		case ArrayEnd:
			return i, nil
		case Comma:
			i++
			continue
		}
		end, err := p.value(i, path+"["+strconv.Itoa(n)+"]")
		if err != nil {
			return 0, err
		}
		n++
		i = end + 1
	}
	return 0, errors.New("an array is not closed")
}

// Join is the key path of key in the object at path, as the JSON reader
// builds it.
func Join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// Find returns the members whose key path is path.
func (d *Doc) Find(path string) []*Member {
	var out []*Member
	for _, m := range d.members {
		if m.Path == path {
			out = append(out, m)
		}
	}
	return out
}

// Member returns the member of o with key, or nil.
func (o *Object) Member(key string) *Member {
	for _, m := range o.Members {
		if m.Key == key {
			return m
		}
	}
	return nil
}

// Next returns the member after m in its object, or nil.
func (m *Member) Next() *Member {
	for i, x := range m.Object.Members {
		if x == m && i+1 < len(m.Object.Members) {
			return m.Object.Members[i+1]
		}
	}
	return nil
}

// Prev returns the member before m in its object, or nil.
func (m *Member) Prev() *Member {
	for i, x := range m.Object.Members {
		if x == m && i > 0 {
			return m.Object.Members[i-1]
		}
	}
	return nil
}

// IsObject reports whether m's value is an object.
func (d *Doc) IsObject(m *Member) bool { return d.toks[m.valStart].Kind == ObjectStart }

// IsString reports whether m's value is a string.
func (d *Doc) IsString(m *Member) bool {
	return m.valStart == m.valEnd && d.toks[m.valStart].Kind == String
}

// start and end are the offsets of token i's own bytes; prefixStart is the
// offset of the whitespace before it.
func (d *Doc) start(i int) int       { return d.at[i] }
func (d *Doc) end(i int) int         { return d.at[i] + len(d.toks[i].Raw) }
func (d *Doc) prefixStart(i int) int { return d.at[i] - len(d.toks[i].Prefix) }

// src rebuilds the document.
func (d *Doc) src() string {
	var b strings.Builder
	for _, t := range d.toks {
		b.WriteString(t.Prefix)
		b.WriteString(t.Raw)
	}
	return b.String()
}

// splice is a change to the document: the bytes [from, to) replaced by text.
type splice struct {
	from, to int
	text     string
}

// apply returns the document with the splices made; they do not overlap and
// are in order.
func (d *Doc) apply(ss ...splice) []byte {
	src := d.src()
	var b strings.Builder
	at := 0
	for _, s := range ss {
		b.WriteString(src[at:s.from])
		b.WriteString(s.text)
		at = s.to
	}
	b.WriteString(src[at:])
	return []byte(b.String())
}

// lineStart is the offset in token i's prefix where the line holding the
// token begins, at the line break before it (a CRLF taken whole), or -1 when
// the prefix holds no line break.
func (d *Doc) lineStart(i int) int {
	p := d.toks[i].Prefix
	nl := strings.LastIndexByte(p, '\n')
	if nl < 0 {
		return -1
	}
	if nl > 0 && p[nl-1] == '\r' {
		nl--
	}
	return d.prefixStart(i) + nl
}

// Delete returns the document without m: its key, its value, and the
// separator that kept it apart from its neighbours. Comments stay; the
// indentation of m's line goes with it.
func (d *Doc) Delete(m *Member) []byte {
	from := d.lineStart(m.key)
	if from < 0 {
		from = d.prefixStart(m.key)
	}
	if m.comma >= 0 {
		to := d.end(m.comma)
		if d.lineStart(m.key) < 0 && m.Prev() == nil {
			// The first member on the brace's line: the space after the
			// brace stays, and the separator before the next member goes.
			from = d.start(m.key)
			if next := d.toks[m.comma+1].Prefix; !strings.ContainsAny(next, "\r\n") {
				to += len(leadingSpace(next))
			}
		}
		return d.apply(splice{from: from, to: to})
	}
	if prev := m.Prev(); prev != nil && prev.comma >= 0 {
		return d.apply(
			splice{from: d.start(prev.comma), to: d.end(prev.comma)},
			splice{from: from, to: d.end(m.valEnd)},
		)
	}
	return d.apply(splice{from: from, to: d.end(m.valEnd)})
}

// spelling is how the members of m's object are laid out: the separator
// before a member's key from its line break on, and the spacing around a
// colon. The separator is read from a member after the first where there is
// one, since the first member's follows the brace.
func (d *Doc) spelling(m *Member) (sep, beforeColon, afterColon string) {
	ref := m
	if m.Prev() == nil {
		if next := m.Next(); next != nil {
			ref = next
		}
	}
	p := d.toks[ref.key].Prefix
	if nl := strings.LastIndexByte(p, '\n'); nl >= 0 {
		eol := "\n"
		if nl > 0 && p[nl-1] == '\r' {
			eol = "\r\n"
		}
		sep = eol + leadingSpace(p[nl+1:])
	} else {
		sep = trailingSpace(p)
	}
	beforeColon = d.toks[m.colon].Prefix
	if strings.TrimSpace(beforeColon) != "" || strings.ContainsAny(beforeColon, "\r\n") {
		beforeColon = ""
	}
	afterColon = d.toks[m.valStart].Prefix
	if strings.TrimSpace(afterColon) != "" || strings.ContainsAny(afterColon, "\r\n") {
		afterColon = " "
	}
	if ref == m && m.Prev() == nil && sep == "" {
		// The only member, right after the brace: members are spaced the
		// way its colon is.
		sep = afterColon
	}
	return sep, beforeColon, afterColon
}

// trailingSpace is the whitespace s ends with.
func trailingSpace(s string) string {
	return s[len(strings.TrimRightFunc(s, unicode.IsSpace)):]
}

// leadingSpace is the whitespace s starts with.
func leadingSpace(s string) string {
	return s[:len(s)-len(strings.TrimLeftFunc(s, unicode.IsSpace))]
}

// sameLineEnd is the offset where the line after token i's own bytes ends
// before its line break, when what follows the token on that line is a
// comment; otherwise the end of the token.
func (d *Doc) sameLineEnd(i int) int {
	end := d.end(i)
	if i+1 >= len(d.toks) {
		return end
	}
	line, _, found := strings.Cut(d.toks[i+1].Prefix, "\n")
	if !found {
		return end
	}
	line = strings.TrimSuffix(line, "\r")
	if strings.TrimSpace(line) == "" {
		return end
	}
	return end + len(line)
}

// member spells a new member in anchor's layout.
func (d *Doc) member(anchor *Member, keyRaw, valueRaw string) (sep, raw string) {
	sep, bc, ac := d.spelling(anchor)
	return sep, keyRaw + bc + ":" + ac + valueRaw
}

// InsertAfter returns the document with a new member right after m, in its
// object, laid out as m is. keyRaw and valueRaw are the key and value as the
// format spells them. A comment on m's line stays with m.
func (d *Doc) InsertAfter(m *Member, keyRaw, valueRaw string) []byte {
	sep, raw := d.member(m, keyRaw, valueRaw)
	if m.comma >= 0 {
		at := d.sameLineEnd(m.comma)
		return d.apply(splice{from: at, to: at, text: sep + raw + ","})
	}
	end := d.end(m.valEnd)
	at := d.sameLineEnd(m.valEnd)
	if at == end {
		return d.apply(splice{from: end, to: end, text: "," + sep + raw})
	}
	return d.apply(splice{from: end, to: end, text: ","}, splice{from: at, to: at, text: sep + raw})
}

// InsertBefore returns the document with a new member right before m, on a
// line of its own when m has one, laid out as m's object is.
func (d *Doc) InsertBefore(m *Member, keyRaw, valueRaw string) []byte {
	sep, raw := d.member(m, keyRaw, valueRaw)
	p := d.toks[m.key].Prefix
	if nl := strings.LastIndexByte(p, '\n'); nl >= 0 {
		eol := "\n"
		if nl > 0 && p[nl-1] == '\r' {
			eol = "\r\n"
		}
		at := d.prefixStart(m.key) + nl + 1
		return d.apply(splice{from: at, to: at, text: leadingSpace(p[nl+1:]) + raw + "," + eol})
	}
	at := d.start(m.key)
	return d.apply(splice{from: at, to: at, text: raw + "," + sep})
}

// Append returns the document with a new member last in o. In an object with
// members the new one is laid out as the last is; in an empty object it goes
// on a line of its own, indented two spaces past the object.
func (d *Doc) Append(o *Object, keyRaw, valueRaw string) []byte {
	if n := len(o.Members); n > 0 {
		return d.InsertAfter(o.Members[n-1], keyRaw, valueRaw)
	}
	base := d.indentOf(o.open)
	from := d.end(o.open)
	to := d.start(o.close)
	inner := d.src()[from:to]
	if strings.TrimSpace(inner) != "" {
		// A comment sits between the braces; it stays, before the member.
		inner = strings.TrimRightFunc(inner, unicode.IsSpace)
	} else {
		inner = ""
	}
	return d.apply(splice{from: from, to: to, text: inner + "\n" + base + "  " + keyRaw + ": " + valueRaw + "\n" + base})
}

// indentOf is the indentation of the line token i sits on.
func (d *Doc) indentOf(i int) string {
	src := d.src()
	start := d.start(i)
	ls := strings.LastIndexByte(src[:start], '\n') + 1
	line := src[ls:start]
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}
