package mdspike_test

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/profile/mdspike"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func load(t *testing.T, src string) (*profile.VoiceProfile, error) {
	t.Helper()
	fsys := fstest.MapFS{"p.md": &fstest.MapFile{Data: []byte(src)}}
	return mdspike.LoadFS(context.Background(), fsys, "p.md", mdspike.Options{})
}

// TestFrontmatterRejectsProse is what keeps the split from collapsing. The
// frontmatter struct has no `guidelines` and no `cultural_notes`, and is decoded
// with KnownFields, so an author who puts prose back into the structured half
// gets an error instead of a document whose prose nobody reads in review.
func TestFrontmatterRejectsProse(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "tone guidelines in frontmatter",
			src:  "---\nname: x\ntone:\n  guidelines: state things plainly\n---\n",
			want: "field guidelines not found",
		},
		{
			name: "cultural notes in frontmatter",
			src:  "---\nname: x\nlocales:\n  nb:\n    cultural_notes: address the reader as du\n---\n",
			want: "field cultural_notes not found",
		},
		{
			name: "typo'd key",
			src:  "---\nname: x\nstyle:\n  activ_voice: true\n---\n",
			want: "field activ_voice not found",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := load(t, tc.src)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// TestBodyRejectsUnknownSections answers the standing objection to markdown as a
// config format — that prose degrades silently. A heading the reader is
// obviously treating as meaningful, but that maps to no field, fails the load.
func TestBodyRejectsUnknownSections(t *testing.T) {
	_, err := load(t, "---\nname: x\n---\n\n## Guidlines\n\nplain prose\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown section "Guidlines"`)
}

// TestExampleRequiresBothSides keeps a half-written example from loading as a
// silently empty one. ValidateProfile would also catch it, but the reader can
// name the section rather than an index.
func TestExampleRequiresBothSides(t *testing.T) {
	_, err := load(t, "---\nname: x\n---\n\n## Example: tone\n\n**Before:** a hype sentence\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "both **Before:** and **After:** are required")

	_, err = load(t, "---\nname: x\n---\n\n## Example: tone\n\nnot a labelled paragraph\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not labelled")
}

// A voice profile is prose, so the paragraph quoted back in an error is
// routinely non-ASCII — typographic punctuation, a Norwegian vowel, CJK. The
// quote is shortened to fit the message, and a cut measured in bytes lands
// mid-rune: %q renders the orphaned byte as a \xNN escape, so the author is
// shown machine noise where their own sentence should end.
func TestUnlabelledParagraphIsQuotedWholeRunes(t *testing.T) {
	tests := []struct {
		name string
		para string
	}{
		{"norwegian vowels", "Skriv så leseren forstår hva hen får ut av å bruke løsningen vår her"},
		{"typographic punctuation", "“Vi leverer” — og det er ikke et løfte, det er en beskrivelse av arbeidet"},
		{"cjk", "これはドキュメントの文章であり、読者に向けて書かれたものである。長さは十分にある。"},
		{"emoji straddling the cut", strings.Repeat("a", 39) + "🙂 trailing"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := load(t, "---\nname: x\n---\n\n## Example: tone\n\n"+tc.para+"\n")
			require.Error(t, err)
			assert.True(t, utf8.ValidString(err.Error()),
				"error message %q is not valid UTF-8", err.Error())
			assert.NotContains(t, err.Error(), `\x`,
				"error message %q quoted the paragraph mid-rune", err.Error())
		})
	}
}

// TestHardWrappedProseNormalizes is what lets a markdown paragraph and a YAML
// folded scalar compare equal: the author's line breaks are presentation, not
// content.
func TestHardWrappedProseNormalizes(t *testing.T) {
	p, err := load(t, "---\nname: x\n---\n\n# Title\n\nOne sentence\nwrapped over\nthree lines.\n\n## Guidelines\n\nAlso\nwrapped.\n")
	require.NoError(t, err)
	assert.Equal(t, "One sentence wrapped over three lines.", p.Description)
	assert.Equal(t, "Also wrapped.", p.Tone.Guidelines)
}

// TestMultiParagraphProseKeepsItsBreaks confirms the normalization stops at the
// paragraph: a description with two paragraphs stays two paragraphs, which the
// single-line YAML form cannot hold without an explicit literal block.
func TestMultiParagraphProseKeepsItsBreaks(t *testing.T) {
	p, err := load(t, "---\nname: x\n---\n\nFirst para.\n\nSecond para.\n")
	require.NoError(t, err)
	assert.Equal(t, "First para.\n\nSecond para.", p.Description)
}

func TestMalformedDocuments(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"no frontmatter", "# Just prose\n\nnothing structured here\n", "does not start with a `---` frontmatter fence"},
		{"unclosed fence", "---\nname: x\n", "frontmatter fence is never closed"},
		{"empty", "", "empty document"},
		{"locale with no id", "---\nname: x\n---\n\n## Locale:\n\nnotes\n", "names no locale"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := load(t, tc.src)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// TestExtendsCycleIsDiagnosed keeps a composition mistake from hanging the
// loader.
func TestExtendsCycleIsDiagnosed(t *testing.T) {
	fsys := fstest.MapFS{
		"a.md": &fstest.MapFile{Data: []byte("---\nname: a\nextends: b.md\n---\n")},
		"b.md": &fstest.MapFile{Data: []byte("---\nname: b\nextends: a.md\n---\n")},
	}
	_, err := mdspike.LoadFS(context.Background(), fsys, "a.md", mdspike.Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "possibly a cycle")
}

// TestFromTermsWithoutStoreFails keeps a sourced ban list from evaporating into
// an empty vocabulary when the store is missing.
func TestFromTermsWithoutStoreFails(t *testing.T) {
	_, err := load(t, "---\nname: x\nvocabulary:\n  from_terms:\n    forbid: [deprecated]\n---\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no terms store was supplied")
}
