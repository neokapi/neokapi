package yaml

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/comment"
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
		for _, c := range got.Comments {
			require.True(t, strings.HasPrefix(string(src[c.Start:c.End]), "#"), "%s: a comment span starts on its marker", rel)
		}
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
