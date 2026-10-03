package arb

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/icu"
	"github.com/neokapi/neokapi/core/model"
)

// This file converts an ARB message value (ICU MessageFormat text) into runs,
// and runs back into a value.
//
// ARB messages use ICU MessageFormat: simple arguments such as {name} or
// {count, number}, and pickers such as {count, plural, …}, {gender, select, …}
// and selectordinal. The syntax carries program semantics a translator, a tool
// or an agent must never alter, so it is read into runs that keep it intact:
//
//   - literal text is a text run;
//   - a simple argument is one placeholder run whose Data is its exact source;
//   - a plural or selectordinal is a plural run and a select is a select run,
//     whose branches are runs read the same way, so the words of every branch
//     are visible and an edit reaches one branch by its path. Inside a branch,
//     # (the number a plural counts) is a placeholder too.
//
// The syntax itself is read by core/icu, the framework's one reader of it.
//
// Literal text keeps its ICU quoting: an apostrophe span is copied into the
// text run as written, so a quoted brace stays quoted and a value written back
// from unchanged runs is the value read. The pieces of syntax a plural run has
// no field for (the keyword, an offset, the order of the branches and the
// whitespace between them) are taken, when a value is written, from the value
// the block was read from: see renderMessage.

// propMessage records, on a block whose message holds a plural or select, the
// message value the block was read from, so the writer can write the
// structure's syntax as it was read. Advisory (model.AdvisoryPropertyPrefix):
// the value is the block's own content, so it must not enter the context hash.
const propMessage = model.AdvisoryPropertyPrefix + "arb.message"

// placeholderType is the type of every placeholder run an ICU message reads
// into: an argument, or # inside a branch.
const placeholderType = "icu"

// message is an ICU message value as the reader reads it: its runs, and the
// shape of each plural or select among them, in order.
type icuMessage struct {
	src    string
	runs   []model.Run
	shapes []shape
}

// shape is how a plural or select was written. header runs from the opening
// brace to the first branch's keyword ("{count, plural, offset:1 "), trailer
// from the brace that closes the last branch to the brace that closes the
// structure.
type shape struct {
	// plural reports a plural or selectordinal (read as a plural run); a
	// select is read as a select run.
	plural   bool
	pivot    string
	header   string
	branches []branch
	trailer  string
}

// branch is one branch of a shape: the whitespace before its keyword, the
// keyword through the opening brace ("one {"), and its body as read.
type branch struct {
	key   string
	gap   string
	label string
	body  icuMessage
}

// runsFromValue reads an ICU message value into runs.
func runsFromValue(value string) []model.Run {
	return readMessage(value).runs
}

// readMessage reads an ICU message value. A message with no content reads as
// one empty text run, so the block still has a source.
func readMessage(value string) icuMessage {
	var ids idSource
	m := readSeq(value, &ids, nil)
	if len(m.runs) == 0 {
		m.runs = []model.Run{{Text: &model.TextRun{Text: value}}}
	}
	return m
}

// idSource numbers the placeholder runs of one message: p1, p2, …
type idSource struct{ n int }

func (s *idSource) next() string {
	s.n++
	return "p" + strconv.Itoa(s.n)
}

// readSeq reads s into runs. inside is nil at a message's top level, where
// every placeholder has an id of its own. Within a plural or select it maps a
// placeholder's source to its id, so a placeholder repeated across branches,
// as # and {count} are, keeps one id; there # is a placeholder.
func readSeq(s string, ids *idSource, inside map[string]string) icuMessage {
	m := icuMessage{src: s}
	var lit []byte
	flush := func() {
		if len(lit) > 0 {
			m.runs = append(m.runs, model.Run{Text: &model.TextRun{Text: string(lit)}})
			lit = lit[:0]
		}
	}
	placeholder := func(data string) {
		flush()
		id, ok := inside[data]
		if !ok {
			id = ids.next()
			if inside != nil {
				inside[data] = id
			}
		}
		m.runs = append(m.runs, model.Run{Ph: &model.PlaceholderRun{
			ID: id, Type: placeholderType, Data: data, Equiv: data, Disp: data,
		}})
	}

	for i := 0; i < len(s); {
		switch ch := s[i]; {
		case ch == '\'':
			// ICU quoting: the span is copied as written, so a quoted brace
			// or # neither opens syntax nor loses its quotes.
			start := i
			icu.SkipQuoted(s, &i)
			lit = append(lit, s[start:i]...)
		case ch == '#' && inside != nil:
			placeholder("#")
			i++
		case ch == '{':
			end, ok := icu.MatchBrace(s, i)
			if !ok {
				// An unbalanced brace: the rest is literal text, so the
				// value still writes back as read.
				lit = append(lit, s[i:]...)
				i = len(s)
				continue
			}
			data := s[i : end+1]
			if run, sh, ok := readStructure(data, ids, inside); ok {
				flush()
				m.runs = append(m.runs, run)
				m.shapes = append(m.shapes, sh)
			} else {
				placeholder(data)
			}
			i = end + 1
		default:
			_, size := utf8.DecodeRuneInString(s[i:])
			lit = append(lit, s[i:i+size]...)
			i += size
		}
	}
	flush()
	return m
}

// readStructure reads data, one balanced {…} group, as a plural or select
// run with its shape. It reports false for a simple argument, and for a
// picker the run model cannot hold as written (a malformed one, a branch
// keyword a plural run has no form for, or a keyword written twice), which
// then stays one placeholder.
func readStructure(data string, ids *idSource, inside map[string]string) (model.Run, shape, bool) {
	nodes, err := icu.Parse(data)
	if err != nil || len(nodes) != 1 || !nodes[0].Type.Picker() || len(nodes[0].Branches) == 0 {
		return model.Run{}, shape{}, false
	}
	n := nodes[0]
	sh := shape{plural: n.Type != icu.NodeSelect, pivot: n.ArgName}
	seen := map[string]bool{}
	starts := make([]int, len(n.Branches))
	for k, br := range n.Branches {
		if seen[br.Keyword] || (sh.plural && !pluralForm(br.Keyword)) {
			return model.Run{}, shape{}, false
		}
		seen[br.Keyword] = true
		start, ok := keywordStart(data, br)
		if !ok {
			return model.Run{}, shape{}, false
		}
		starts[k] = start
	}

	if inside == nil {
		inside = map[string]string{}
	}
	sh.header = data[:starts[0]]
	prev := starts[0]
	forms := map[string][]model.Run{}
	for k, br := range n.Branches {
		b := branch{key: br.Keyword, gap: data[prev:starts[k]], label: data[starts[k]:br.Start]}
		b.body = readSeq(data[br.Start:br.End], ids, inside)
		if b.body.runs == nil {
			b.body.runs = []model.Run{}
		}
		forms[br.Keyword] = b.body.runs
		sh.branches = append(sh.branches, b)
		prev = br.End + 1
	}
	sh.trailer = data[prev:]

	if !sh.plural {
		return model.Run{Select: &model.SelectRun{Pivot: sh.pivot, Cases: forms}}, sh, true
	}
	p := &model.PluralRun{Pivot: sh.pivot, Forms: make(map[model.PluralForm][]model.Run, len(forms))}
	for k, runs := range forms {
		p.Forms[model.PluralForm(k)] = runs
	}
	return model.Run{Plural: p}, sh, true
}

// keywordStart returns where br's keyword starts in data: before the
// whitespace that may stand between it and the opening brace of its body.
func keywordStart(data string, br icu.Branch) (int, bool) {
	j := br.Start - 1 // the opening brace
	for j > 0 && isSpace(data[j-1]) {
		j--
	}
	start := j - len(br.Keyword)
	if start < 0 || data[start:j] != br.Keyword {
		return 0, false
	}
	return start, true
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

// pluralForm reports whether key names a form a plural run holds: a CLDR
// plural category or an explicit value such as =0.
func pluralForm(key string) bool {
	switch model.PluralForm(key) {
	case model.PluralZero, model.PluralOne, model.PluralTwo, model.PluralFew, model.PluralMany, model.PluralOther:
		return true
	}
	rest, ok := strings.CutPrefix(key, "=")
	if !ok || rest == "" {
		return false
	}
	_, err := strconv.ParseFloat(rest, 64)
	return err == nil
}

// valueFromRuns writes runs as an ARB message value. original is the value
// the block was read from, when it held a plural or select, and "" otherwise.
func valueFromRuns(runs []model.Run, original string) string {
	if original == "" {
		return renderMessage(runs, nil)
	}
	read := readMessage(original)
	return renderMessage(runs, &read)
}

// renderMessage writes runs as an ICU message value in the shape of read, the
// message the value was read as, or nil when there is none.
//
// Runs equal to read's are written as read's source, byte for byte. Otherwise
// text is written as it reads and a placeholder as its data, and each plural
// or select is written in the shape of the one at the same place in read,
// where that one is the same kind of structure over the same argument: its
// keyword, offset and whitespace as written, its branches in the order
// written, each branch an edit left alone as written, an edited branch from
// its runs, a branch the runs no longer hold left out, and a branch read did
// not have added before the closing brace. A plural or select with no such
// counterpart is written in the form Flutter's tools write.
func renderMessage(runs []model.Run, read *icuMessage) string {
	if read != nil && sameRuns(runs, read.runs) {
		return read.src
	}
	var b strings.Builder
	next := 0
	for _, r := range runs {
		if r.Plural == nil && r.Select == nil {
			model.RenderRunsWith(&b, []model.Run{r}, nil)
			continue
		}
		var sh *shape
		if read != nil && next < len(read.shapes) && read.shapes[next].fits(r) {
			sh = &read.shapes[next]
		}
		next++
		renderStructure(&b, r, sh)
	}
	return b.String()
}

// fits reports whether r is the kind of structure sh is, over the same
// argument.
func (sh *shape) fits(r model.Run) bool {
	if sh.plural {
		return r.Plural != nil && r.Plural.Pivot == sh.pivot
	}
	return r.Select != nil && r.Select.Pivot == sh.pivot
}

// renderStructure writes one plural or select run in sh's shape, or in the
// form Flutter's tools write when sh is nil.
func renderStructure(b *strings.Builder, r model.Run, sh *shape) {
	pivot, keyword, branches := structureBranches(r)
	if sh == nil {
		b.WriteString("{" + pivot + ", " + keyword + ",")
		for _, key := range branchOrder(r, branches) {
			b.WriteString(" " + key + "{" + renderMessage(branches[key], nil) + "}")
		}
		b.WriteString("}")
		return
	}
	b.WriteString(sh.header)
	wrote := false
	held := map[string]bool{}
	for i := range sh.branches {
		br := &sh.branches[i]
		runs, ok := branches[br.key]
		if !ok {
			continue
		}
		held[br.key] = true
		if wrote {
			b.WriteString(br.gap)
		}
		b.WriteString(br.label)
		b.WriteString(renderMessage(runs, &br.body))
		b.WriteString("}")
		wrote = true
	}
	for _, key := range branchOrder(r, branches) {
		if held[key] {
			continue
		}
		if wrote {
			b.WriteString(" ")
		}
		b.WriteString(key + "{" + renderMessage(branches[key], nil) + "}")
		wrote = true
	}
	b.WriteString(sh.trailer)
}

// structureBranches returns a plural or select run's argument, the keyword it
// is written with when no shape gives one, and its branches by key.
func structureBranches(r model.Run) (pivot, keyword string, branches map[string][]model.Run) {
	if r.Select != nil {
		return r.Select.Pivot, "select", r.Select.Cases
	}
	branches = make(map[string][]model.Run, len(r.Plural.Forms))
	for f, runs := range r.Plural.Forms {
		branches[string(f)] = runs
	}
	return r.Plural.Pivot, "plural", branches
}

// pluralOrder is the order Flutter's tools write a plural's categories in,
// after its explicit values.
var pluralOrder = []string{"zero", "one", "two", "few", "many", "other"}

// branchOrder orders a structure's branches as Flutter's tools write them: a
// plural's explicit values (=0, =1, …) by value, then its categories in CLDR
// order; a select's cases by name, with other last.
func branchOrder(r model.Run, branches map[string][]model.Run) []string {
	keys := slices.Collect(maps.Keys(branches))
	rank := func(k string) (int, float64) {
		if r.Plural != nil {
			if v, ok := strings.CutPrefix(k, "="); ok {
				f, _ := strconv.ParseFloat(v, 64)
				return 0, f
			}
			if i := slices.Index(pluralOrder, k); i >= 0 {
				return 1, float64(i)
			}
			return 2, 0
		}
		if k == "other" {
			return 1, 0
		}
		return 0, 0
	}
	slices.SortFunc(keys, func(a, b string) int {
		ra, va := rank(a)
		rb, vb := rank(b)
		if ra != rb {
			return ra - rb
		}
		if va != vb {
			if va < vb {
				return -1
			}
			return 1
		}
		return strings.Compare(a, b)
	})
	return keys
}

// sameRuns reports whether two run sequences hold the same content.
func sameRuns(a, b []model.Run) bool {
	return string(model.CanonicalRunsJSON(a)) == string(model.CanonicalRunsJSON(b))
}

// holdsStructure reports whether runs hold a plural or select at their top
// level.
func holdsStructure(runs []model.Run) bool {
	return slices.ContainsFunc(runs, func(r model.Run) bool { return r.Plural != nil || r.Select != nil })
}

// errInvalidMessage is the refusal of a value that would not read back as
// written.
var errInvalidMessage = errors.New("arb writer: the message would not read back as written")

// checkMessage refuses value when runs hold a plural or select and value is
// not valid ICU MessageFormat: text in a branch that opens or closes a brace
// unquoted would end the branch early and change what the program shows. A
// message without a plural or select is written as it reads, as it always
// was.
func checkMessage(block *model.Block, runs []model.Run, value string) error {
	if !holdsStructure(runs) {
		return nil
	}
	if _, err := icu.Parse(value); err != nil {
		name := block.ID
		if block.Name != "" {
			name = block.Name
		}
		return fmt.Errorf("%w: message %s: %v; quote a literal brace or # in ICU style ('{', '}', '#')", errInvalidMessage, name, err)
	}
	return nil
}
