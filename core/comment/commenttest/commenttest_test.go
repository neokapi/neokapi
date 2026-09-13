package commenttest_test

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/commenttest"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hash is a toy language: `#` runs to the end of the line outside a double-
// quoted string, consecutive full-line comments form one comment, and `#!` is a
// directive. It holds the suite to a provider that is neither Go nor YAML.
type hash struct{}

func (hash) Language() string     { return "hash" }
func (hash) Extensions() []string { return []string{".hash"} }

func (hash) Canary() comment.Canary {
	return comment.Canary{
		Name:   "a hash comment with a doubled word",
		Source: []byte("# Reads the the input.\nvalue = \"# not a comment\"\n"),
		Block:  "comment",
	}
}

func (hash) LineText(line []byte) (int, string, bool) {
	body, ok := bytes.CutPrefix(line, []byte("#"))
	return len(line), string(body), ok
}

func (hash) Locate(_ string, src []byte) (*comment.File, error) {
	units, err := scanHash("", src)
	if err != nil {
		return nil, err
	}
	idx := format.NewLineIndex(src)
	f := &comment.File{Language: "hash"}
	var run []commenttest.Unit
	flush := func() {
		if len(run) == 0 {
			return
		}
		start, end := run[0].Start, run[len(run)-1].End
		lines := make([]string, len(run))
		for i, u := range run {
			lines[i] = strings.TrimSpace(string(src[u.Start+u.Open : u.End]))
		}
		f.Comments = append(f.Comments, comment.Comment{
			Start: start, End: end, Lines: idx.Range(start, end), Style: comment.StyleLine,
			Subject: "comment", Runs: []model.Run{model.TextR(strings.Join(lines, "\n"))},
		})
		run = nil
	}
	for _, u := range units {
		if u.Directive {
			flush()
			f.Excluded = append(f.Excluded, comment.Excluded{
				Start: u.Start, End: u.End, Lines: idx.Range(u.Start, u.End), Reason: comment.ReasonDirective, Form: "shebang",
			})
			continue
		}
		if len(run) > 0 && run[0].Group != u.Group {
			flush()
		}
		run = append(run, u)
	}
	flush()
	return f, nil
}

func scanHash(_ string, src []byte) ([]commenttest.Unit, error) {
	var units []commenttest.Unit
	group, prev := 0, -1
	for line, lineStart := 0, 0; lineStart < len(src); line++ {
		end := len(src)
		next := end
		if i := bytes.IndexByte(src[lineStart:], '\n'); i >= 0 {
			end, next = lineStart+i, lineStart+i+1
		}
		quoted := false
		for j := lineStart; j < end; j++ {
			switch {
			case src[j] == '"':
				quoted = !quoted
			case src[j] == '#' && !quoted:
				full := len(bytes.TrimSpace(src[lineStart:j])) == 0
				if !full || prev != line-1 {
					group++
				}
				if full {
					prev = line
				}
				units = append(units, commenttest.Unit{
					Start: j, End: end, Open: 1, Group: group,
					Directive: bytes.HasPrefix(src[j:end], []byte("#!")),
				})
				j = end
			}
		}
		lineStart = next
	}
	return units, nil
}

const hashFixture = "#! run with hash\n# First line.\n# Second line.\nvalue = \"# not a comment\" # beside\n\n# # quotes a marker\nother = 1\n"

func suite(p comment.Provider) commenttest.Suite {
	return commenttest.Suite{
		Provider: p,
		Scan:     scanHash,
		Fixtures: []commenttest.Fixture{{Name: "fixture.hash", Source: hashFixture, Literals: []string{`"# not a comment"`}}},
	}
}

// broken wraps the toy provider and damages what it locates.
type broken struct {
	hash
	edit   func(src []byte, f *comment.File)
	canary []byte
}

func (b broken) Locate(name string, src []byte) (*comment.File, error) {
	f, err := b.hash.Locate(name, src)
	if err == nil && name != "canary" {
		b.edit(src, f)
	}
	return f, err
}

func (b broken) Canary() comment.Canary {
	c := b.hash.Canary()
	if b.canary != nil {
		c.Source = b.canary
	}
	return c
}

func TestTheSuitePassesAConformingProvider(t *testing.T) {
	commenttest.Run(t, suite(hash{}))
	got, err := hash{}.Locate("fixture.hash", []byte(hashFixture))
	require.NoError(t, err)
	assert.Len(t, got.Comments, 3, "the suite ran over located comments")
	assert.Equal(t, "# quotes a marker", model.RunsText(got.Comments[2].Runs), "prose may quote a marker")
}

// Each broken provider damages one property, and the suite must name it.
func TestTheSuiteCatchesABrokenProvider(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    comment.Provider
		want commenttest.Property
	}{
		{"a span one byte short", broken{edit: func(_ []byte, f *comment.File) { f.Comments[0].End-- }}, commenttest.PropSpan},
		{"a span one byte late", broken{edit: func(_ []byte, f *comment.File) { f.Comments[0].Start++ }}, commenttest.PropSpan},
		{"a line range one too long", broken{edit: func(_ []byte, f *comment.File) { f.Comments[0].Lines.Last++ }}, commenttest.PropLines},
		{"a comment dropped", broken{edit: func(_ []byte, f *comment.File) { f.Comments = f.Comments[1:] }}, commenttest.PropAccount},
		{"an exclusion counted as a comment too", broken{edit: func(_ []byte, f *comment.File) {
			e := f.Excluded[0]
			f.Comments = append(f.Comments, comment.Comment{Start: e.Start, End: e.End, Lines: e.Lines, Runs: []model.Run{model.TextR("run with hash")}})
		}}, commenttest.PropSpan},
		{"a directive inside a comment", broken{edit: func(_ []byte, f *comment.File) {
			e := f.Excluded[0]
			f.Excluded = nil
			f.Comments[0].Start, f.Comments[0].Lines.First = e.Start, e.Lines.First
		}}, commenttest.PropAccount},
		{"two groups merged", broken{edit: func(_ []byte, f *comment.File) {
			f.Comments[0].End, f.Comments[0].Lines.Last = f.Comments[1].End, f.Comments[1].Lines.Last
			f.Comments = slices.Delete(f.Comments, 1, 2)
		}}, commenttest.PropAccount},
		{"a marker left in the runs", broken{edit: func(src []byte, f *comment.File) {
			f.Comments[1].Runs = []model.Run{model.TextR(string(src[f.Comments[1].Start:f.Comments[1].End]))}
		}}, commenttest.PropRuns},
		{"a marker inside a string counted", broken{edit: func(src []byte, f *comment.File) {
			at := bytes.Index(src, []byte("# not a comment"))
			f.Comments = append(f.Comments, comment.Comment{Start: at, End: at + len("# not a comment"), Lines: format.LineRange{First: 4, Last: 4}, Runs: []model.Run{model.TextR("not a comment")}})
			slices.SortFunc(f.Comments, func(a, b comment.Comment) int { return a.Start - b.Start })
		}}, commenttest.PropLiteral},
		{"prose set aside as blank", broken{edit: func(_ []byte, f *comment.File) {
			for _, c := range f.Comments {
				f.Excluded = append(f.Excluded, comment.Excluded{Start: c.Start, End: c.End, Lines: c.Lines, Reason: comment.ReasonBlank})
			}
			f.Comments = nil
		}}, commenttest.PropLocated},
		{"a canary with no doubled word", broken{edit: func([]byte, *comment.File) {}, canary: []byte("# Reads the input.\n")}, commenttest.PropCanary},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failures := commenttest.Verify(suite(tc.p))
			require.NotEmpty(t, failures)
			assert.Contains(t, commenttest.Properties(failures), tc.want, "%v", commenttest.Err(failures))
		})
	}
}

// unreadLines reads no comment line as whole.
type unreadLines struct{ hash }

func (unreadLines) LineText([]byte) (int, string, bool) { return 0, "", false }

// shortLines reads every comment line one byte short.
type shortLines struct{ hash }

func (s shortLines) LineText(line []byte) (int, string, bool) {
	n, text, ok := s.hash.LineText(line)
	return n - 1, text, ok
}

func declaredSuite(p comment.Provider) commenttest.Suite {
	s := suite(p)
	s.Directives = comment.Directives{"skip:"}
	s.Fixtures = append(s.Fixtures, commenttest.Fixture{
		Name:   "declared.hash",
		Source: "# First line.\n# skip: a declared marker\n# Second line.\nvalue = 1 # skip: beside a value\n# see skip: in prose\n",
	})
	return s
}

// With directives declared, a comment line that opens with one is set aside as
// that directive, and a provider whose LineText misreads a line is caught.
func TestTheSuiteHoldsDeclaredDirectives(t *testing.T) {
	commenttest.Run(t, declaredSuite(hash{}))

	for _, tc := range []struct {
		name string
		p    comment.Provider
		want commenttest.Property
	}{
		{"a LineText that reads no line as whole", unreadLines{}, commenttest.PropDirective},
		{"a LineText that reads no line as whole, with nothing declared", unreadLines{}, commenttest.PropLineText},
		{"a LineText one byte short", shortLines{}, commenttest.PropLineText},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := declaredSuite(tc.p)
			if strings.HasSuffix(tc.name, "with nothing declared") {
				s = suite(tc.p)
			}
			failures := commenttest.Verify(s)
			require.NotEmpty(t, failures)
			assert.Contains(t, commenttest.Properties(failures), tc.want, "%v", commenttest.Err(failures))
		})
	}
}

func TestTheSuiteRefusesAWrongFixture(t *testing.T) {
	s := suite(hash{})
	s.Fixtures[0].Literals = []string{`"# absent"`, "# beside"}
	failures := commenttest.Verify(s)
	assert.Equal(t, []commenttest.Property{commenttest.PropLiteral}, commenttest.Properties(failures))
	assert.Len(t, failures, 3, "a literal that is not in the fixture, and one the scan and the provider both read as a comment: %v", commenttest.Err(failures))
}
