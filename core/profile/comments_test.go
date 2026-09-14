package profile

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
)

func TestCommentRulesLimits(t *testing.T) {
	t.Run("an empty section asks for every default", func(t *testing.T) {
		p, err := LoadProfileYAML(strings.NewReader("name: Source\nstyle:\n  comments: {}\n"))
		require.NoError(t, err)
		require.NotNil(t, p.Style.Comments, "an empty mapping still asks for the comment checks")
		assert.Equal(t, check.DefaultCommentLimits(), p.Style.Comments.Limits())
	})

	t.Run("a limit the profile names replaces its default and the rest keep theirs", func(t *testing.T) {
		p, err := LoadProfileYAML(strings.NewReader(`name: Source
style:
  sentence_length: short
  comments:
    sentence_words: {major: 90}
    doc_words: 180
`))
		require.NoError(t, err)
		want := check.DefaultCommentLimits()
		want.SentenceMajor, want.DocWords = 90, 180
		assert.Equal(t, want, p.Style.Comments.Limits())
		assert.Equal(t, "short", p.Style.SentenceLength, "sentence_length stays guidance beside the limits")
	})

	t.Run("a profile with no section asks for no comment checks", func(t *testing.T) {
		p, err := LoadProfileYAML(strings.NewReader("name: Docs\nstyle:\n  sentence_length: short\n"))
		require.NoError(t, err)
		assert.Nil(t, p.Style.Comments)
	})
}

func TestCommentRulesValidation(t *testing.T) {
	for _, tc := range []struct {
		name, yaml, field string
	}{
		{"a zero limit", "name: S\nstyle:\n  comments:\n    comment_words: 0\n", "style.comments.comment_words"},
		{"a negative limit", "name: S\nstyle:\n  comments:\n    sentence_words: {minor: -5}\n", "style.comments.sentence_words.minor"},
		{"a minor limit at the major one", "name: S\nstyle:\n  comments:\n    sentence_words: {minor: 70}\n", "style.comments.sentence_words"},
		{"a limit in a channel's style", "name: S\nchannels:\n  code:\n    style:\n      comments:\n        package_doc_words: 0\n", "channels.code.style.comments.package_doc_words"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadProfileYAML(strings.NewReader(tc.yaml))
			require.Error(t, err, "the profile is refused at load")
			assert.Contains(t, err.Error(), tc.field+":", "the error names the key")

			p, derr := DecodeProfileStrict(strings.NewReader(tc.yaml))
			require.NoError(t, derr)
			var fields []string
			for _, prob := range Blocking(ValidateProfile(p)) {
				fields = append(fields, prob.Field)
			}
			assert.Contains(t, fields, tc.field)
		})
	}

	t.Run("the keys are known to the strict decoder", func(t *testing.T) {
		probs, err := UnknownKeys([]byte("name: S\nstyle:\n  comments:\n    sentence_words: {minor: 40, major: 60}\n    comment_words: 90\n    doc_words: 140\n    package_doc_words: 280\n"))
		require.NoError(t, err)
		assert.Empty(t, probs)
	})
}

func TestCommentRulesOverrideWarning(t *testing.T) {
	p, err := LoadProfileYAML(strings.NewReader(`name: S
style:
  comments: {}
channels:
  terse:
    style:
      sentence_length: short
  kept:
    style:
      comments: {doc_words: 120}
`))
	require.NoError(t, err)
	var warned []string
	for _, prob := range Advisory(ValidateProfile(p)) {
		if prob.Code == CodeOverrideDropsCommentRules {
			warned = append(warned, prob.Field)
		}
	}
	assert.Equal(t, []string{"channels.terse.style"}, warned)
	assert.Nil(t, ResolveProfile(p, "", "terse", "").Style.Comments, "the channel's style replaces the base style, as the warning says")
}

func TestCommentRulesClone(t *testing.T) {
	major := 90
	p := &VoiceProfile{Name: "S", Style: StyleRules{Comments: &CommentRules{SentenceWords: &SentenceWordLimits{Major: &major}}}}
	c := p.Clone()
	*c.Style.Comments.SentenceWords.Major = 10
	assert.Equal(t, 90, p.Style.Comments.Limits().SentenceMajor, "a clone's limits are its own")
}

func TestGuideCarriesTheCommentLimits(t *testing.T) {
	doc := 180
	p := &VoiceProfile{Name: "Source", Style: StyleRules{SentenceLength: "short", Comments: &CommentRules{DocWords: &doc}}}

	guide := RenderVoiceGuide(p)
	for _, want := range []string{
		"- Sentence length: short",
		"- Code comments:",
		"  - A sentence over 50 words is a minor finding, and over 70 words a major one",
		"  - A comment that documents no declaration: at most 100 words",
		"  - A declaration's doc comment: at most 180 words",
		"  - A package or module doc comment: at most 300 words",
	} {
		assert.Contains(t, guide, want)
	}

	compact := RenderVoiceGuideCompact(p)
	assert.Contains(t, compact, "Code comments: a sentence over 50 words is flagged (a major finding over 70), and so is a comment over 100 words, a doc comment over 180 or a package doc comment over 300.")

	assert.NotContains(t, RenderVoiceGuide(sampleProfile()), "Code comments", "a profile without comment limits renders none")
}
