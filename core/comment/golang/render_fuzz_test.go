package golang

import (
	"bytes"
	"go/format"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/comment"
)

// FuzzRewrite rewrites a doc comment, a comment in a body and a trailing
// comment with arbitrary text. Each rewrite is either refused with a reason or
// leaves every byte outside the comment identical in a file gofmt agrees with,
// and rendering the rewritten comment's own prose changes nothing.
func FuzzRewrite(f *testing.F) {
	for _, seed := range []string{
		"Parse ends at */ and resumes.",
		"Parse handles // inside a line comment.",
		"Before " + strings.Repeat("x", 200) + " after.",
		"Combining marks: e\u0301 a\u0308 \u0301alone.",
		"\u05E9\u05DC\u05D5\u05DD \u05E2\u05D5\u05DC\u05DD and \u0645\u0631\u062D\u0628\u0627 \u0628\u0627\u0644\u0639\u0627\u0644\u0645 in one line.",
		"A lone\rcarriage return.",
		"Paragraph.\n\n\tcode := 1\n\nAfter.",
		"Cases:\n  - one\n  - two",
		"nolint",
		"Deprecated: use New.",
		"[io.Reader] reads.",
		"\u202Eoverride",
		"",
	} {
		f.Add(seed)
	}
	src := []byte(rewriteFixture)
	located, err := Provider{}.Locate("demo.go", src)
	if err != nil {
		f.Fatal(err)
	}
	blocks := located.Blocks()
	targets := []string{"type/Block", "func/Parse/comment", "type/Block/ID"}
	for _, id := range targets {
		found := false
		for _, b := range blocks {
			found = found || b.ID == id
		}
		if !found {
			f.Fatalf("the fixture has no comment %s", id)
		}
	}

	f.Fuzz(func(t *testing.T, text string) {
		for _, id := range targets {
			i := -1
			for j, b := range blocks {
				if b.ID == id {
					i = j
				}
			}
			c := located.Comments[i]
			r, err := comment.Rewrite(Provider{}, "demo.go", src, nil, comment.Target{ID: id}, text, comment.RenderOptions{})
			if err != nil {
				if _, ok := comment.AsRefusal(err); !ok {
					t.Fatalf("%s: %q: an error that is not a refusal: %v", id, text, err)
				}
				continue
			}
			after := r.Source
			end := c.End + len(after) - len(src)
			if !bytes.Equal(src[:c.Start], after[:c.Start]) || end < c.Start || !bytes.Equal(src[c.End:], after[end:]) {
				t.Fatalf("%s: %q: a byte outside the comment changed", id, text)
			}
			formatted, err := format.Source(after)
			if err != nil || !bytes.Equal(formatted, after) {
				t.Fatalf("%s: %q: gofmt disagrees with the rewrite: %v", id, text, err)
			}
			prose, err := Provider{}.Prose(after, r.After)
			if err != nil {
				t.Fatalf("%s: %q: %v", id, text, err)
			}
			again, err := comment.Rewrite(Provider{}, "demo.go", after, nil, comment.Target{ID: id}, prose, comment.RenderOptions{})
			if err != nil || again.Changed {
				t.Fatalf("%s: %q: rendering the rewritten comment's own prose is not a no-op: %v", id, text, err)
			}
		}
	})
}
