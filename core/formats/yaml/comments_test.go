package yaml

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	yamlv3 "gopkg.in/yaml.v3"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/commenttest"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const commentFixture = `# yaml-language-server: $schema=x.json
# Document head comment.

# Head of title.
title: Hello # beside title
subtitle: 'quoted # is content'
body: | # on the indicator line
  # is content inside a literal block
  text
narration:
  # Head of the first item.
  - text: One
  - text: Two # beside the second item
#
# Foot of the document.
`

func TestLocateCommentsFindsEveryCommentWithItsSpan(t *testing.T) {
	src := []byte(commentFixture)
	got, err := LocateComments(src)
	require.NoError(t, err)

	type located struct {
		subject string
		text    string
		lines   format.LineRange
		bytes   string
	}
	var comments []located
	for _, c := range got.Comments {
		comments = append(comments, located{c.Subject, model.RunsText(c.Runs), c.Lines, string(src[c.Start:c.End])})
	}
	assert.Equal(t, []located{
		{"comment/title", "Document head comment.", format.LineRange{First: 2, Last: 2}, "# Document head comment."},
		{"comment/title", "Head of title.", format.LineRange{First: 4, Last: 4}, "# Head of title."},
		{"comment/title", "beside title", format.LineRange{First: 5, Last: 5}, "# beside title"},
		{"comment/body", "on the indicator line", format.LineRange{First: 7, Last: 7}, "# on the indicator line"},
		{"comment/narration.[0].text", "Head of the first item.", format.LineRange{First: 11, Last: 11}, "# Head of the first item."},
		{"comment/narration.[1].text", "beside the second item", format.LineRange{First: 13, Last: 13}, "# beside the second item"},
		{"comment/document", "Foot of the document.", format.LineRange{First: 15, Last: 15}, "# Foot of the document."},
	}, comments)

	var excluded []string
	for _, e := range got.Excluded {
		excluded = append(excluded, string(e.Reason)+" "+e.Form+" "+string(src[e.Start:e.End]))
	}
	assert.Equal(t, []string{
		"directive yaml-language-server # yaml-language-server: $schema=x.json",
		"blank  #",
	}, excluded)
}

func TestLocateCommentsGroupsConsecutiveLines(t *testing.T) {
	src := []byte("# one\n# two\nkey: value\n\n# three\nother: value\n")
	got, err := LocateComments(src)
	require.NoError(t, err)
	require.Len(t, got.Comments, 2)
	assert.Equal(t, "one\ntwo", model.RunsText(got.Comments[0].Runs))
	assert.Equal(t, format.LineRange{First: 1, Last: 2}, got.Comments[0].Lines)
	assert.Equal(t, "comment/key", got.Comments[0].Subject)
	assert.Equal(t, "comment/other", got.Comments[1].Subject)
}

func TestLocateCommentsNeverReadsContentAsAComment(t *testing.T) {
	src := []byte("a: \"double # quoted\"\nb: 'single # quoted'\nc: |\n  # literal\nd: >-\n  # folded\ne: url#fragment\nf: [x, \"y # z\"]\n")
	got, err := LocateComments(src)
	require.NoError(t, err)
	assert.Empty(t, got.Comments)
	assert.Empty(t, got.Excluded)
}

func TestLocateCommentsRefusesADisagreement(t *testing.T) {
	w := &commentWalk{src: []byte("key: value # real\n"), parsed: []string{"# real", "# missing"}}
	err := w.agree(w.scan())
	require.Error(t, err)
	require.ErrorIs(t, err, ErrCommentsUnlocated)
	require.ErrorIs(t, err, comment.ErrUnlocated, "the host treats any provider's unlocated file the same way")
}

// Every comment in the repository's YAML files is located, or the file is
// counted as refused; the counts are logged so a change in either is visible.
func TestLocateCommentsOverTheRepositoryYAML(t *testing.T) {
	root := yamlRepoRoot(t)
	cmd := exec.CommandContext(t.Context(), "git", "ls-files", "-z", "*.yaml", "*.yml")
	cmd.Dir = root
	listing, err := cmd.Output()
	require.NoError(t, err)

	files, comments, refused, unparsed := 0, 0, 0, 0
	var refusals []string
	for rel := range strings.SplitSeq(strings.TrimRight(string(listing), "\x00"), "\x00") {
		if rel == "" || strings.Contains(rel, "node_modules/") {
			continue
		}
		src, rerr := os.ReadFile(filepath.Join(root, rel))
		require.NoError(t, rerr)
		got, lerr := LocateComments(src)
		switch {
		case errors.Is(lerr, ErrCommentsUnlocated):
			refused++
			refusals = append(refusals, rel+": "+lerr.Error())
			continue
		case lerr != nil:
			unparsed++
			continue
		}
		files++
		comments += len(got.Comments)
		units, serr := yamlUnits(rel, src)
		require.NoError(t, serr, rel)
		assert.NoError(t, commenttest.Err(commenttest.Account(src, got, units)), rel)
	}
	t.Logf("yaml-comments files=%d comments=%d refused=%d unparsed=%d", files, comments, refused, unparsed)
	for _, r := range refusals {
		t.Log(r)
	}
	assert.GreaterOrEqual(t, files, 200, "the scan read too few YAML files")
}

func yamlRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, "no go.work above the package")
		dir = parent
	}
}

func TestCommentProviderConformance(t *testing.T) {
	commenttest.Run(t, yamlSuite(CommentProvider{}))
}

func TestCommentProviderConformanceCatchesABrokenProvider(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    comment.Provider
		want commenttest.Property
	}{
		{"a span one byte short", brokenYAML{edit: func(_ []byte, f *comment.File) { f.Comments[0].End-- }}, commenttest.PropSpan},
		{"a comment dropped", brokenYAML{edit: func(_ []byte, f *comment.File) { f.Comments = f.Comments[1:] }}, commenttest.PropAccount},
		{"a marker inside a string counted", brokenYAML{edit: countQuotedMarker}, commenttest.PropLiteral},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failures := commenttest.Verify(yamlSuite(tc.p))
			require.NotEmpty(t, failures)
			assert.Contains(t, commenttest.Properties(failures), tc.want, "%v", commenttest.Err(failures))
		})
	}
}

func yamlSuite(p comment.Provider) commenttest.Suite {
	return commenttest.Suite{
		Provider: p,
		Scan:     yamlUnits,
		Fixtures: []commenttest.Fixture{
			{Name: "comments.yaml", Source: commentFixture, Literals: []string{"'quoted # is content'", "# is content inside a literal block"}},
			{Name: "groups.yaml", Source: "# one\n# two\nkey: value\n\n# three\nother: value\n"},
			{Name: "crlf.yaml", Source: "# Head.\r\n# Second.\r\nkey: value # beside\r\n"},
			{
				Name:     "content.yaml",
				Source:   "a: \"double # quoted\"\nb: 'single # quoted'\nc: |\n  # literal\nd: >-\n  # folded\ne: url#fragment\nf: [x, \"y # z\"]\n# The one comment.\n",
				Literals: []string{`"double # quoted"`, "'single # quoted'", "# literal", "# folded", "url#fragment", `"y # z"`},
			},
		},
	}
}

// brokenYAML damages what the YAML provider locates in every document with a
// comment, except the canary.
type brokenYAML struct {
	CommentProvider
	edit func(src []byte, f *comment.File)
}

func (b brokenYAML) Locate(name string, src []byte) (*comment.File, error) {
	f, err := b.CommentProvider.Locate(name, src)
	if err == nil && name != "canary" && len(f.Comments) > 0 {
		b.edit(src, f)
	}
	return f, err
}

// countQuotedMarker reports the `#` inside a quoted scalar as a comment.
func countQuotedMarker(src []byte, f *comment.File) {
	at := bytes.Index(src, []byte("# is content'"))
	if at < 0 {
		return
	}
	end := at + len("# is content")
	f.Comments = append(f.Comments, comment.Comment{
		Start: at, End: end, Lines: format.NewLineIndex(src).Range(at, end),
		Style: comment.StyleLine, Subject: "comment/subtitle", Runs: []model.Run{model.TextR("is content")},
	})
	slices.SortFunc(f.Comments, func(a, b comment.Comment) int { return a.Start - b.Start })
}

// yamlUnits is the conformance suite's scan for YAML, with the parser as its
// only authority: a `#` begins a comment exactly when cutting its line there
// leaves every document's data as it was and removes that one comment line from
// the lines the parser reports. The provider's own scan is never consulted.
func yamlUnits(_ string, src []byte) ([]commenttest.Unit, error) {
	data, comments, err := decodeDocuments(src)
	if err != nil {
		return nil, err
	}
	var units []commenttest.Unit
	group, prevFull := 0, -1
	for line, lineStart := 0, 0; lineStart < len(src); line++ {
		end := len(src)
		next := end
		if i := bytes.IndexByte(src[lineStart:], '\n'); i >= 0 {
			end, next = lineStart+i, lineStart+i+1
		}
		if end > lineStart && src[end-1] == '\r' {
			end--
		}
		for j := lineStart; j < end; j++ {
			if src[j] != '#' {
				continue
			}
			text := bytes.TrimRight(src[j:end], " \t")
			cutData, cutComments, err := decodeDocuments(slices.Concat(src[:j], src[end:]))
			if err != nil || !reflect.DeepEqual(cutData, data) || !removesOne(comments, cutComments, string(text)) {
				continue
			}
			full := len(bytes.TrimLeft(src[lineStart:j], " \t")) == 0
			if !full || prevFull != line-1 {
				group++
			}
			if full {
				prevFull = line
			}
			_, directive := yamlDirective(string(text))
			units = append(units, commenttest.Unit{Start: j, End: j + len(text), Open: 1, Group: group, Directive: directive})
			break
		}
		lineStart = next
	}
	return units, nil
}

// decodeDocuments decodes every document in src and lists the comment lines
// the parser attaches to its nodes.
func decodeDocuments(src []byte) ([]any, []string, error) {
	dec := yamlv3.NewDecoder(bytes.NewReader(src))
	var data []any
	var comments []string
	for {
		var doc yamlv3.Node
		if err := dec.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				return data, comments, nil
			}
			return nil, nil, err
		}
		var v any
		if err := doc.Decode(&v); err != nil {
			return nil, nil, err
		}
		data = append(data, v)
		var walk func(n *yamlv3.Node)
		walk = func(n *yamlv3.Node) {
			for _, field := range []string{n.HeadComment, n.LineComment, n.FootComment} {
				for l := range strings.SplitSeq(field, "\n") {
					if l = strings.TrimSpace(l); l != "" {
						comments = append(comments, l)
					}
				}
			}
			for _, c := range n.Content {
				walk(c)
			}
		}
		walk(&doc)
	}
}

// removesOne reports whether after is before with exactly one line, text,
// taken out.
func removesOne(before, after []string, text string) bool {
	if len(after) != len(before)-1 {
		return false
	}
	rest := slices.Clone(before)
	i := slices.Index(rest, text)
	if i < 0 {
		return false
	}
	rest = slices.Delete(rest, i, i+1)
	slices.Sort(rest)
	sorted := slices.Sorted(slices.Values(after))
	return slices.Equal(rest, sorted)
}
