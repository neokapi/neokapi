// Package commenttest is the conformance suite every comment provider passes.
//
// A provider's test supplies the provider, fixtures in its language and a scan
// of its own: a reading of where each comment sits that does not use the
// provider, typically the language's parser. The suite holds what the provider
// located to that scan:
//
//   - every comment the scan finds sits in exactly one addressable comment or
//     one exclusion;
//   - a span starts where a comment starts and ends where one ends, so its bytes
//     begin at the comment's opening marker, and it never cuts a comment, joins
//     two groups or holds a directive;
//   - the line range is the file's own lines, counted from the bytes;
//   - a comment's runs carry no marker its source lines did not hold as text;
//   - a marker inside one of the language's literal contexts is never a comment;
//   - the canary is located and its doubled word is flagged by the hygiene
//     check that reads every real comment.
//
// Verify returns what a provider broke rather than failing a test, so a test
// can hand it a provider broken on purpose and assert that it notices.
package commenttest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Unit is one comment the provider test's scan found: a line comment, or one
// delimited comment.
type Unit struct {
	// Start and End are the half-open byte span of the comment, from the first
	// byte of its opening marker to the last byte of its closing marker, or of
	// its line for a comment that runs to the end of the line.
	Start, End int
	// Open and Close are the lengths of the markers at either end of the span.
	// Close is zero for a comment that runs to the end of its line.
	Open, Close int
	// Group numbers the comments that may share one addressable comment. Two
	// units in different groups never do.
	Group int
	// Directive marks a comment a tool reads. It never sits inside an
	// addressable comment.
	Directive bool
}

// Scan reads the comments of a file without the provider under test.
type Scan func(name string, src []byte) ([]Unit, error)

// Fixture is one file the suite locates.
type Fixture struct {
	// Name is the path the provider is given.
	Name   string
	Source string
	// Literals are substrings of Source holding a comment marker as content,
	// such as a string or an attribute value. Every occurrence is checked, and
	// no comment or exclusion may overlap one.
	Literals []string
}

// Suite is what a provider's test hands the conformance suite.
type Suite struct {
	Provider comment.Provider
	Scan     Scan
	Fixtures []Fixture
}

// Property names what a failure broke.
type Property string

const (
	// PropSpan is a span that is empty, overlaps another, or does not start and
	// end where comments do.
	PropSpan Property = "span"
	// PropLines is a line range that is not the lines the span covers.
	PropLines Property = "lines"
	// PropAccount is a comment lost, counted twice, set aside as blank while it
	// holds text, or placed in a span beside what may not share it.
	PropAccount Property = "account"
	// PropRuns is a marker left in a comment's runs.
	PropRuns Property = "runs"
	// PropLiteral is a claim on a marker inside a literal context.
	PropLiteral Property = "literal"
	// PropLocated is a fixture with prose in its comments that located none and
	// set none aside for a reason covering the whole file, such as generated.
	PropLocated Property = "located"
	// PropCanary is a canary the provider or the hygiene check missed.
	PropCanary Property = "canary"
	// PropProvider is a file the provider or the scan could not read.
	PropProvider Property = "provider"
)

// Failure is one thing a provider got wrong in one file.
type Failure struct {
	Fixture  string
	Property Property
	Detail   string
}

func (f Failure) Error() string {
	if f.Fixture == "" {
		return fmt.Sprintf("%s: %s", f.Property, f.Detail)
	}
	return fmt.Sprintf("%s: %s: %s", f.Fixture, f.Property, f.Detail)
}

// Err joins failures into one error, nil when there are none.
func Err(failures []Failure) error {
	errs := make([]error, len(failures))
	for i, f := range failures {
		errs[i] = f
	}
	return errors.Join(errs...)
}

// Properties lists the distinct properties failures broke, sorted.
func Properties(failures []Failure) []Property {
	var out []Property
	for _, f := range failures {
		if !slices.Contains(out, f.Property) {
			out = append(out, f.Property)
		}
	}
	slices.Sort(out)
	return out
}

// Run verifies the suite and reports each failure on t. It fails when the
// suite has no fixture or no fixture located a comment, since a suite that
// read nothing proves nothing.
func Run(t *testing.T, s Suite) {
	t.Helper()
	if len(s.Fixtures) == 0 {
		t.Fatal("the suite has no fixture")
	}
	located := 0
	for _, fx := range s.Fixtures {
		t.Run(fx.Name, func(t *testing.T) {
			got, failures := verifyFixture(s, fx)
			for _, f := range failures {
				t.Error(f.Error())
			}
			if got != nil {
				located += len(got.Comments)
			}
		})
	}
	t.Run("canary", func(t *testing.T) {
		for _, f := range verifyCanary(s) {
			t.Error(f.Error())
		}
	})
	if located == 0 {
		t.Errorf("no fixture located a comment: the suite read nothing")
	}
}

// Verify locates every fixture and the canary and returns every failure.
func Verify(s Suite) []Failure {
	var failures []Failure
	for _, fx := range s.Fixtures {
		_, f := verifyFixture(s, fx)
		failures = append(failures, f...)
	}
	return append(failures, verifyCanary(s)...)
}

func verifyFixture(s Suite, fx Fixture) (*comment.File, []Failure) {
	src := []byte(fx.Source)
	fail := func(p Property, format string, args ...any) []Failure {
		return []Failure{{Fixture: fx.Name, Property: p, Detail: fmt.Sprintf(format, args...)}}
	}
	units, err := s.Scan(fx.Name, src)
	if err != nil {
		return nil, fail(PropProvider, "the scan could not read the fixture: %v", err)
	}
	got, err := s.Provider.Locate(fx.Name, src)
	if err != nil {
		return nil, fail(PropProvider, "locate: %v", err)
	}
	failures := Account(src, got, units)
	failures = append(failures, literals(src, got, units, fx.Literals)...)
	wholeFile := slices.ContainsFunc(got.Excluded, func(e comment.Excluded) bool {
		return e.Reason != comment.ReasonBlank && e.Reason != comment.ReasonDirective
	})
	if len(got.Comments) == 0 && !wholeFile && slices.ContainsFunc(units, func(u Unit) bool { return !u.Directive && !blank(src, u) }) {
		failures = append(failures, Failure{Property: PropLocated, Detail: fmt.Sprintf(
			"the scan finds prose in the fixture's %d comment(s) and the provider located none", len(units))})
	}
	for i := range failures {
		failures[i].Fixture = fx.Name
	}
	return got, failures
}

// blank reports whether a comment holds nothing between its markers.
func blank(src []byte, u Unit) bool {
	return len(bytes.TrimSpace(src[u.Start+u.Open:u.End-u.Close])) == 0
}

func verifyCanary(s Suite) []Failure {
	c := s.Provider.Canary()
	fail := func(format string, args ...any) []Failure {
		return []Failure{{Fixture: "canary", Property: PropCanary, Detail: fmt.Sprintf(format, args...)}}
	}
	got, err := s.Provider.Locate("canary", c.Source)
	if err != nil {
		return fail("locate %s: %v", c.Name, err)
	}
	units, err := s.Scan("canary", c.Source)
	if err != nil {
		return fail("the scan could not read %s: %v", c.Name, err)
	}
	failures := Account(c.Source, got, units)
	for i := range failures {
		failures[i].Fixture = "canary"
	}
	for _, b := range got.Blocks() {
		if b.ID != c.Block {
			continue
		}
		flagged, err := DoubledWord(context.Background(), b)
		if err != nil {
			return append(failures, fail("the hygiene check could not read block %s: %v", c.Block, err)...)
		}
		if !flagged {
			failures = append(failures, fail("the hygiene check flags no doubled word in block %s (%q)", c.Block, model.RunsText(b.SourceRuns()))...)
		}
		return failures
	}
	return append(failures, fail("%s: no block %s among %d located", c.Name, c.Block, len(got.Comments))...)
}

// DoubledWord runs the hygiene check `kapi check` runs on every comment over b
// and reports whether it flags a doubled word.
func DoubledWord(ctx context.Context, b *model.Block) (bool, error) {
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: b}
	close(in)
	errc := make(chan error, 1)
	go func() {
		defer close(out)
		errc <- check.NewContentLintTool().Process(ctx, in, out)
	}()
	for range out { //nolint:revive // drain
	}
	if err := <-errc; err != nil {
		return false, err
	}
	ann, ok := model.AnnoAs[*check.FindingsAnnotation](b, check.AnnotationKey)
	if !ok {
		return false, nil
	}
	return slices.ContainsFunc(ann.Findings, func(f check.Finding) bool { return f.Category == "doubled-word" }), nil
}

// claim is one span a provider reported: a comment or an exclusion.
type claim struct {
	start, end int
	lines      format.LineRange
	excluded   bool
	reason     comment.Reason
	comment    int
}

func (c claim) String() string {
	kind := "comment"
	if c.excluded {
		kind = "exclusion"
	}
	return fmt.Sprintf("%s %d-%d", kind, c.start, c.end)
}

// Account holds what a provider located in src to the comments a scan found
// there: spans, lines, accounting and runs. It trusts nothing the provider
// computed.
func Account(src []byte, got *comment.File, units []Unit) []Failure {
	var failures []Failure
	fail := func(p Property, format string, args ...any) {
		failures = append(failures, Failure{Property: p, Detail: fmt.Sprintf(format, args...)})
	}
	if got == nil {
		fail(PropProvider, "the provider returned no file")
		return failures
	}
	for _, u := range units {
		if u.Start < 0 || u.End > len(src) || u.Open <= 0 || u.Start+u.Open+u.Close > u.End {
			fail(PropProvider, "the scan reports an impossible comment %d-%d (markers %d, %d)", u.Start, u.End, u.Open, u.Close)
			return failures
		}
	}
	units = slices.Clone(units)
	sort.Slice(units, func(i, j int) bool { return units[i].Start < units[j].Start })

	claims := make([]claim, 0, len(got.Comments)+len(got.Excluded))
	for i, c := range got.Comments {
		claims = append(claims, claim{start: c.Start, end: c.End, lines: c.Lines, comment: i})
	}
	for _, e := range got.Excluded {
		claims = append(claims, claim{start: e.Start, end: e.End, lines: e.Lines, excluded: true, reason: e.Reason, comment: -1})
	}
	sort.SliceStable(claims, func(i, j int) bool { return claims[i].start < claims[j].start })

	for i, c := range claims {
		if c.start < 0 || c.end > len(src) || c.start >= c.end {
			fail(PropSpan, "%s is empty or outside the file", c)
			continue
		}
		if i > 0 && c.start < claims[i-1].end {
			fail(PropSpan, "%s overlaps %s", c, claims[i-1])
		}
		if want := linesOf(src, c.start, c.end); c.lines != want {
			fail(PropLines, "%s reports lines %d-%d, and its bytes sit on lines %d-%d", c, c.lines.First, c.lines.Last, want.First, want.Last)
		}
	}

	covered := make([]int, len(claims))
	first := make([]Unit, len(claims))
	last := make([]Unit, len(claims))
	for _, u := range units {
		n := 0
		for ci, c := range claims {
			if c.end <= u.Start || c.start >= u.End || c.start >= c.end {
				continue
			}
			n++
			if u.Start < c.start || u.End > c.end {
				fail(PropSpan, "the comment at %d-%d (%q) is cut by %s", u.Start, u.End, clip(src[u.Start:u.End]), c)
				continue
			}
			if c.excluded && (c.start != u.Start || c.end != u.End) {
				fail(PropSpan, "%s does not cover exactly the comment at %d-%d", c, u.Start, u.End)
			}
			if c.reason == comment.ReasonBlank && !blank(src, u) {
				fail(PropAccount, "%s sets aside the comment %q as blank", c, clip(src[u.Start:u.End]))
			}
			if covered[ci] == 0 {
				first[ci] = u
			} else if first[ci].Group != u.Group {
				fail(PropAccount, "%s merges comment group %d with group %d", c, first[ci].Group, u.Group)
			}
			covered[ci]++
			last[ci] = u
			if !c.excluded && u.Directive {
				fail(PropAccount, "%s contains the directive %q", c, clip(src[u.Start:u.End]))
			}
		}
		switch n {
		case 0:
			fail(PropAccount, "the comment at %d-%d (%q) is neither addressable nor excluded", u.Start, u.End, clip(src[u.Start:u.End]))
		case 1:
		default:
			fail(PropAccount, "the comment at %d-%d is claimed %d times", u.Start, u.End, n)
		}
	}

	for ci, c := range claims {
		if c.start >= c.end || c.start < 0 || c.end > len(src) {
			continue
		}
		if covered[ci] == 0 {
			fail(PropAccount, "%s (%q) holds no comment the scan finds", c, clip(src[c.start:c.end]))
			continue
		}
		if c.excluded {
			continue
		}
		if first[ci].Start != c.start || last[ci].End != c.end {
			fail(PropSpan, "%s does not run from a comment's opening marker (%d) to a comment's end (%d)", c, first[ci].Start, last[ci].End)
		}
		failures = append(failures, runsCarryNoMarker(src, got.Comments[c.comment], units)...)
	}
	return failures
}

// runsCarryNoMarker holds a comment's runs free of its markers. A line of the
// runs may start with an opening marker, or end with a closing one, only as
// often as a line of the comments' own text does, so prose that quotes a
// marker stays prose and a provider that leaves one in place is caught.
func runsCarryNoMarker(src []byte, c comment.Comment, units []Unit) []Failure {
	var opens, closes []string
	var content []string
	for _, u := range units {
		if u.Start < c.Start || u.End > c.End {
			continue
		}
		if o := string(src[u.Start : u.Start+u.Open]); !slices.Contains(opens, o) {
			opens = append(opens, o)
		}
		if u.Close > 0 {
			if cl := string(src[u.End-u.Close : u.End]); !slices.Contains(closes, cl) {
				closes = append(closes, cl)
			}
		}
		content = append(content, strings.Split(string(src[u.Start+u.Open:u.End-u.Close]), "\n")...)
	}
	count := func(lines []string) (opened, closed int) {
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if slices.ContainsFunc(opens, func(m string) bool { return strings.HasPrefix(l, m) }) {
				opened++
			}
			if slices.ContainsFunc(closes, func(m string) bool { return strings.HasSuffix(l, m) }) {
				closed++
			}
		}
		return opened, closed
	}
	text := strings.Split(model.RunsText(c.Runs), "\n")
	gotOpen, gotClose := count(text)
	allowOpen, allowClose := count(content)
	var failures []Failure
	if gotOpen > allowOpen || gotClose > allowClose {
		failures = append(failures, Failure{Property: PropRuns, Detail: fmt.Sprintf(
			"the runs of comment %d-%d keep a comment marker: %q", c.Start, c.End, clip([]byte(model.RunsText(c.Runs))))})
	}
	return failures
}

// literals checks that no claim, and no unit of the scan, overlaps a literal.
func literals(src []byte, got *comment.File, units []Unit, lits []string) []Failure {
	var failures []Failure
	fail := func(format string, args ...any) {
		failures = append(failures, Failure{Property: PropLiteral, Detail: fmt.Sprintf(format, args...)})
	}
	for _, lit := range lits {
		occurrences := 0
		for from := 0; ; {
			i := bytes.Index(src[from:], []byte(lit))
			if i < 0 {
				break
			}
			start := from + i
			end := start + len(lit)
			from = start + 1
			occurrences++
			for _, u := range units {
				if u.Start < end && start < u.End {
					fail("the scan reads the literal %q at %d as the comment at %d-%d; the fixture or the scan is wrong", lit, start, u.Start, u.End)
				}
			}
			for _, c := range got.Comments {
				if c.Start < end && start < c.End {
					fail("comment %d-%d overlaps the literal %q at %d", c.Start, c.End, lit, start)
				}
			}
			for _, e := range got.Excluded {
				if e.Start < end && start < e.End {
					fail("exclusion %d-%d overlaps the literal %q at %d", e.Start, e.End, lit, start)
				}
			}
		}
		if occurrences == 0 {
			fail("the literal %q does not occur in the fixture", lit)
		}
	}
	return failures
}

// linesOf counts the lines [start, end) covers from the bytes themselves: a
// line ends after its '\n'.
func linesOf(src []byte, start, end int) format.LineRange {
	firstLine := bytes.Count(src[:start], []byte("\n")) + 1
	return format.LineRange{First: firstLine, Last: firstLine + bytes.Count(src[start:end-1], []byte("\n"))}
}

func clip(b []byte) string {
	const limit = 48
	s := string(b)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i] + "..."
	}
	if len(s) > limit {
		return s[:limit] + "..."
	}
	return s
}
