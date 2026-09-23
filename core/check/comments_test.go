package check

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/golang"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/segment"
	_ "github.com/neokapi/neokapi/core/segment/uax29"
)

// sentenceBreak is the UAX #29 sentence break the comment checks read, or a
// skipped test in a build without ICU.
func sentenceBreak(t *testing.T) segment.Segmenter {
	t.Helper()
	seg, err := segment.Build("uax29", segment.BaseConfig{}, nil)
	if err != nil {
		t.Skipf("the uax29 sentence break is not in this build: %v", err)
	}
	return seg
}

// commentBlock is the block the comment layer builds from one comment.
func commentBlock(subject string, doc bool, runs ...model.Run) *model.Block {
	f := comment.File{Language: "go", Comments: []comment.Comment{{Subject: subject, Doc: doc, Runs: runs}}}
	return f.Blocks()[0]
}

// words is n words, the first capitalised, as one line.
func words(n int) string {
	return strings.TrimSpace("Every" + strings.Repeat(" word", n-1))
}

func unitTexts(runs ...model.Run) []string {
	v := model.NewHygieneView(runs)
	var out []string
	for _, u := range commentUnits(v) {
		out = append(out, v.Text()[u[0]:u[1]])
	}
	return out
}

func TestCommentUnits(t *testing.T) {
	t.Run("soft line breaks stay inside a unit and a blank line ends it", func(t *testing.T) {
		assert.Equal(t, []string{"One sentence\nwrapped over lines.", "A second paragraph."},
			unitTexts(model.TextR("One sentence\nwrapped over lines.\n\nA second paragraph.")))
	})
	t.Run("a divider line ends a unit and its title is a unit of its own", func(t *testing.T) {
		assert.Equal(t, []string{"Before the rule", "Section", "After the rule", "No title follows"},
			unitTexts(model.TextR("Before the rule\n── Section ──\nAfter the rule\n----------\nNo title follows")))
	})
	t.Run("a list item or a heading written as text starts a unit", func(t *testing.T) {
		assert.Equal(t, []string{"Kinds:", "- the first\n  wraps", "2. the second", "# Heading", "text"},
			unitTexts(model.TextR("Kinds:\n- the first\n  wraps\n2. the second\n# Heading\ntext")))
	})
	t.Run("a list item placeholder starts a unit", func(t *testing.T) {
		item := func(id string) model.Run {
			return model.PhR(model.PlaceholderRun{ID: id, Type: comment.TypeListItem, SubType: "go:item", Data: "-"})
		}
		assert.Equal(t, []string{"Kinds:", "￼the first\nwraps", "￼the second"},
			unitTexts(model.TextR("Kinds:\n\n"), item("g1"), model.TextR("the first\nwraps\n"), item("g2"), model.TextR("the second")))
	})
	t.Run("a documentation tag starts a unit and a line of codes ends one", func(t *testing.T) {
		tag := model.PhR(model.PlaceholderRun{ID: "c1", Type: "code", SubType: "jsdoc:tag", Data: "@param name - "})
		code := model.PhR(model.PlaceholderRun{ID: "c2", Type: "code", SubType: "comment:code", Data: "```\nx := 1\n```"})
		assert.Equal(t, []string{"Parses the input", "￼the input to read", "and what follows"},
			unitTexts(model.TextR("Parses the input\n"), tag, model.TextR("the input to read\n"), code, model.TextR("\nand what follows")))
	})
	t.Run("a reference at the start of a line continues the unit", func(t *testing.T) {
		ref := model.PhR(model.PlaceholderRun{ID: "g1", Type: "code", SubType: "go:doclink", Data: "[Parse]"})
		assert.Equal(t, []string{"It calls\n￼ on the input."},
			unitTexts(model.TextR("It calls\n"), ref, model.TextR(" on the input.")))
	})
}

func TestProseWords(t *testing.T) {
	for _, tc := range []struct {
		text string
		want int
	}{
		{"three plain words", 3},
		{"a ￼ reference is no word", 5},
		{"glued￼together", 2},
		{"a — dash alone", 3},
		{"version 2 counts", 3},
		{"", 0},
	} {
		assert.Equal(t, tc.want, proseWords(tc.text), tc.text)
	}
}

// runeSlice is the text of the runes [start, end) of s.
func runeSlice(s string, start, end int) string {
	r := []rune(s)
	return string(r[start:end])
}

func TestCommentSentenceFindings(t *testing.T) {
	seg := sentenceBreak(t)
	ctx := context.Background()
	limits := DefaultCommentLimits()
	limits.Fails = true

	t.Run("a wrapped sentence is graded by its words, with its exact place", func(t *testing.T) {
		minor := words(51) + "."
		major := canarySentence(71, 3)
		text := "Short one. " + minor + "\n" + major + "\n\nFine."
		b := commentBlock("func/Parse", true, model.TextR(text))

		got, err := CommentSentenceFindings(ctx, seg, b, limits, "en")
		require.NoError(t, err)
		require.Len(t, got, 2)

		assert.False(t, got[0].Fails)
		assert.Equal(t, "51", got[0].Metadata["words"])
		assert.Equal(t, "50", got[0].Metadata["limit"])
		assert.Equal(t, minor, runeSlice(text, got[0].Position.Start.Offset, got[0].Position.End.Offset))

		assert.True(t, got[1].Fails)
		assert.Equal(t, "71", got[1].Metadata["words"])
		assert.Equal(t, major, runeSlice(text, got[1].Position.Start.Offset, got[1].Position.End.Offset),
			"the sentence runs across its line breaks")
		assert.Equal(t, strings.ReplaceAll(major, "\n", " "), got[1].OriginalText)
		assert.Equal(t, CategorySentenceLength, got[1].Category)
	})

	t.Run("must fail: a sentence break that ends a sentence at each line break misses the wrapped sentence", func(t *testing.T) {
		major := canarySentence(71, 3)
		var perLine []Finding
		for line := range strings.SplitSeq(major, "\n") {
			found, err := CommentSentenceFindings(ctx, seg, commentBlock("func/Parse", true, model.TextR(line)), limits, "en")
			require.NoError(t, err)
			perLine = append(perLine, found...)
		}
		assert.Empty(t, perLine, "read line by line, no sentence is over the limit")
	})

	t.Run("a placeholder is no word, and a reader sees the text it stands for", func(t *testing.T) {
		ref := model.PhR(model.PlaceholderRun{ID: "g1", Type: "code", SubType: "go:doclink", Data: "[Parse]"})
		b := commentBlock("func/Parse", true, model.TextR(words(50)+" "), ref, model.TextR(" end."))
		got, err := CommentSentenceFindings(ctx, seg, b, limits, "en")
		require.NoError(t, err)
		require.Len(t, got, 1, "50 words, the reference and one more make 51 words")
		assert.Equal(t, "51", got[0].Metadata["words"])
		assert.Equal(t, words(50)+" [Parse] end.", got[0].OriginalText)
		assert.Equal(t, model.RunPos{Run: 0, Offset: 0}, got[0].Position.Start)
		assert.Equal(t, model.RunPos{Run: 3, Offset: 0}, got[0].Position.End, "the range runs past the reference to the end of the runs")
		start, end := got[0].Position.TextSpan(b.SourceRuns())
		assert.Equal(t, [2]int{0, utf8.RuneCountInString(words(50) + "  end.")}, [2]int{start, end})

		without, err := CommentSentenceFindings(ctx, seg, commentBlock("func/Parse", true, model.TextR(words(49)+" "), ref, model.TextR(" end.")), limits, "en")
		require.NoError(t, err)
		assert.Empty(t, without, "49 words and a reference are 50 words, which the limit allows")
	})

	t.Run("a divider line ends a sentence that has no full stop", func(t *testing.T) {
		b := commentBlock("func/Parse", true, model.TextR(words(30)+"\n── Section ──\n"+words(30)))
		got, err := CommentSentenceFindings(ctx, seg, b, limits, "en")
		require.NoError(t, err)
		assert.Empty(t, got, "two sentences of 30 words, not one of 61")
	})

	t.Run("list items are sentences of their own", func(t *testing.T) {
		item := func(id string) model.Run {
			return model.PhR(model.PlaceholderRun{ID: id, Type: comment.TypeListItem, SubType: "go:item", Data: "-"})
		}
		b := commentBlock("func/Parse", true, model.TextR("It holds:\n\n"), item("g1"), model.TextR(words(30)+"\n"), item("g2"), model.TextR(words(30)))
		got, err := CommentSentenceFindings(ctx, seg, b, limits, "en")
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("a block the comment layer did not build reports nothing", func(t *testing.T) {
		got, err := CommentSentenceFindings(ctx, seg, CanaryBlock(words(80)+"."), limits, "en")
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}

// The Go provider consumes a list item's marker. Each item of a Go doc comment
// list is still a sentence of its own, however its lines wrap and whether or
// not it ends in a full stop.
func TestGoListItemsAreSentencesOfTheirOwn(t *testing.T) {
	seg := sentenceBreak(t)
	item := func(first string) string {
		return "//   - " + strings.Join(limitWords(first, 30, 15), "\n//     ")
	}
	src := "package demo\n\n// Parse reads these:\n//\n" + item("the") + "\n" + item("then") + "\nfunc Parse() {}\n"
	f, err := golang.Provider{}.Locate("demo.go", []byte(src))
	require.NoError(t, err)
	blocks := f.Blocks()
	require.Len(t, blocks, 1)

	got, err := CommentSentenceFindings(context.Background(), seg, blocks[0], DefaultCommentLimits(), "en")
	require.NoError(t, err)
	assert.Empty(t, got, "two items of 30 words each, not one sentence of 63")
}

// limitWords is n words, first among them, that repeat no word next to itself,
// cut into lines of perLine words.
func limitWords(first string, n, perLine int) []string {
	vocabulary := []string{"reads", "each", "value", "from", "the", "input", "and", "keeps", "what", "it", "finds"}
	var lines []string
	line := []string{first}
	for i := 1; i < n; i++ {
		if len(line) == perLine {
			lines = append(lines, strings.Join(line, " "))
			line = nil
		}
		line = append(line, vocabulary[(i-1)%len(vocabulary)])
	}
	return append(lines, strings.Join(line, " "))
}

func TestCommentLengthFindings(t *testing.T) {
	limits := DefaultCommentLimits()
	for _, tc := range []struct {
		name    string
		subject string
		doc     bool
		limit   int
		message string
	}{
		{"a comment that documents nothing", "func/Parse/comment", false, 100, "Comment has 101 words, over the limit of 100"},
		{"a declaration's doc comment", "func/Parse", true, 150, "Doc comment has 151 words, over the limit of 150"},
		{"a package doc comment", "package", true, 300, "Package doc comment has 301 words, over the limit of 300"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at := CommentLengthFindings(commentBlock(tc.subject, tc.doc, model.TextR(words(tc.limit))), limits)
			assert.Empty(t, at, "a comment at the limit passes")

			over := CommentLengthFindings(commentBlock(tc.subject, tc.doc, model.TextR(words(tc.limit+1))), limits)
			require.Len(t, over, 1)
			assert.Equal(t, CategoryCommentLength, over[0].Category)
			assert.Equal(t, limits.Fails, over[0].Fails)
			assert.Equal(t, tc.message, over[0].Message)
		})
	}

	t.Run("placeholders are not counted", func(t *testing.T) {
		code := model.PhR(model.PlaceholderRun{ID: "g1", Type: "code", SubType: "go:code", Data: strings.Repeat("x := 1\n", 80)})
		assert.Empty(t, CommentLengthFindings(commentBlock("func/Parse/comment", false, model.TextR(words(100)+"\n\n"), code), limits))
	})
	t.Run("a doc comment on a package-named declaration path is not the package doc", func(t *testing.T) {
		over := CommentLengthFindings(commentBlock("module/parse", true, model.TextR(words(151))), limits)
		require.Len(t, over, 1, "module/parse is a declaration, held to the doc limit")
	})
}

func TestCommentCanaries(t *testing.T) {
	seg := sentenceBreak(t)
	ctx := context.Background()
	limits := DefaultCommentLimits()
	limits.Fails = true
	sentences := func(b *model.Block) ([]Finding, error) { return CommentSentenceFindings(ctx, seg, b, limits, "en") }

	caught, err := Probe(CommentSentenceCanaries(limits), "", sentences)
	require.NoError(t, err)
	assert.Equal(t, CanaryCaught, caught.Status)
	assert.Equal(t, 2, caught.Probes)

	lineByLine := func(b *model.Block) ([]Finding, error) {
		var out []Finding
		for line := range strings.SplitSeq(model.RunsText(b.SourceRuns()), "\n") {
			found, err := sentences(commentBlock("func/Canary", true, model.TextR(line)))
			if err != nil {
				return nil, err
			}
			out = append(out, found...)
		}
		return out, nil
	}
	missed, err := Probe(CommentSentenceCanaries(limits), "", lineByLine)
	require.NoError(t, err)
	assert.Equal(t, CanaryMissed, missed.Status, "a sentence break that ends a sentence at each line break misses the wrapped canary")
	assert.Equal(t, "a sentence of 71 words wrapped over three lines", missed.Missed)

	reportOnly := func(b *model.Block) ([]Finding, error) {
		found, err := sentences(b)
		for i := range found {
			found[i].Fails = false
		}
		return found, err
	}
	graded, err := Probe(CommentSentenceCanaries(limits), "", reportOnly)
	require.NoError(t, err)
	assert.Equal(t, CanaryMissed, graded.Status, "a check that never makes a long sentence fail misses the failing canary")

	lengths := func(b *model.Block) ([]Finding, error) { return CommentLengthFindings(b, limits), nil }
	caught, err = Probe(CommentLengthCanaries(limits), "", lengths)
	require.NoError(t, err)
	assert.Equal(t, CanaryCaught, caught.Status)
	assert.Equal(t, 3, caught.Probes)

	missed, err = Probe(CommentLengthCanaries(DefaultCommentLimits()), "", func(b *model.Block) ([]Finding, error) {
		return CommentLengthFindings(b, CommentLimits{CommentWords: 1000, DocWords: 1000, PackageDocWords: 1000}), nil
	})
	require.NoError(t, err)
	assert.Equal(t, CanaryMissed, missed.Status, "a check that ignores the limits misses its canaries")
}
