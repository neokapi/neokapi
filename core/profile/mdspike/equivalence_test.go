package mdspike_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/profile/mdspike"
	"github.com/neokapi/neokapi/core/yamledit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repoRelative paths from this package to the two committed profiles. Named as
// constants so the spike never grows an absolute path.
const (
	bowrainYAML = "../../../.kapi/profiles/bowrain/voice.yaml"
	voiceYAML   = "../../../.kapi/voice.yaml"
	termsJSON   = "../../../.kapi/terms.json"
)

func loadYAML(t *testing.T, path string) *profile.VoiceProfile {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	p, err := profile.LoadProfileYAML(f)
	require.NoError(t, err)
	return p
}

func loadMarkdown(t *testing.T, name string, opts mdspike.Options) *profile.VoiceProfile {
	t.Helper()
	p, err := mdspike.LoadFile(context.Background(), "testdata/"+name, opts)
	require.NoError(t, err)
	return p
}

// TestMarkdownFormEqualsYAMLForm is the spike's central claim: the same profile
// authored as markdown-with-frontmatter, inheriting the house rules instead of
// copying them, decodes to a VoiceProfile that is field-for-field identical to
// the one .kapi/profiles/bowrain/voice.yaml decodes to.
//
// Identical means identical — the assertion is on the whole struct, not on a
// chosen subset. Nothing about the profile is lost in the move, which is why
// the note's "what does not survive" section is about the surfaces around the
// file (the store, the web editor, the AI drafting path), not about the file.
func TestMarkdownFormEqualsYAMLForm(t *testing.T) {
	fromYAML := loadYAML(t, bowrainYAML)
	fromMD := loadMarkdown(t, "bowrain-voice.md", mdspike.Options{})

	assert.Equal(t, fromYAML, fromMD)
}

// TestRenderedGuideIsByteIdentical checks the claim that matters most at
// runtime. RenderVoiceGuide is what the AI translate prompt, the voice check
// tool, `kapi voice guide` and the cloud MCP tool all send to the model. If the
// two forms render the same guide, no generation behaviour changes with the
// authoring format.
func TestRenderedGuideIsByteIdentical(t *testing.T) {
	fromYAML := loadYAML(t, bowrainYAML)
	fromMD := loadMarkdown(t, "bowrain-voice.md", mdspike.Options{})

	assert.Equal(t, profile.RenderVoiceGuide(fromYAML), profile.RenderVoiceGuide(fromMD))
	assert.Equal(t, profile.RenderVoiceGuideCompact(fromYAML), profile.RenderVoiceGuideCompact(fromMD))
}

// TestMarkdownFormValidates confirms the markdown form produces a profile the
// existing validator accepts, so `kapi voice validate` would need no new
// semantic rules — only a second decoder in front of the same checks.
func TestMarkdownFormValidates(t *testing.T) {
	p := loadMarkdown(t, "bowrain-voice.md", mdspike.Options{})
	assert.Empty(t, profile.ValidateProfile(p))
}

// TestHouseRulesAreDuplicatedToday documents the defect the inheritance answers,
// from the committed files rather than from assertion: the two profiles carry
// the same eleven prohibitions verbatim, and every one of them has to be edited
// twice — three times counting testdata/house.md, which is a third copy.
//
// The count grew from five when the fixed vocabulary moved into the profiles
// (#2175). That is the argument for composition getting stronger, not weaker.
//
// Drift is asserted at zero rather than merely counted. RenderVoiceGuide sends
// the description, not the regex, to the model, so a description that differs
// between the two profiles gives the model two instructions for one
// prohibition — which is what this test exists to notice. One had drifted
// (the emoji rule); it is now in step.
func TestHouseRulesAreDuplicatedToday(t *testing.T) {
	brand := loadYAML(t, voiceYAML)
	bowrain := loadYAML(t, bowrainYAML)

	byRegex := func(p *profile.VoiceProfile) map[string]string {
		m := map[string]string{}
		for _, pat := range p.Style.ProhibitedPatterns {
			m[pat.Regex] = pat.Description
		}
		return m
	}
	a, b := byRegex(brand), byRegex(bowrain)

	var shared, drifted []string
	for regex, descA := range a {
		descB, ok := b[regex]
		if !ok {
			continue
		}
		shared = append(shared, regex)
		if descA != descB {
			drifted = append(drifted, regex)
		}
	}

	require.Len(t, shared, 11, "the two committed profiles share eleven prohibitions verbatim")
	assert.Empty(t, drifted, "a shared prohibition must give the model one instruction, not two")
	for _, regex := range drifted {
		t.Logf("drifted prohibition %s:\n  voice.yaml:                  %q\n  profiles/bowrain/voice.yaml: %q", regex, a[regex], b[regex])
	}
}

// TestCommittedProfileSurvivesAWriteBack is the constraint that decides how far
// a new authoring format can go. `kapi apply` with a voice add-rule entry loads
// the committed profile, upserts the rule and writes it back
// (host.writeProfileYAML), so a write-back that marshalled the struct over the
// file would take the header with it — including the one recording that the
// house rules are duplicated. The write goes through core/yamledit instead: the
// value supplies the data, the document supplies its own commentary, and an
// unchanged profile re-renders byte for byte.
//
// The read side of a markdown form is easy. This is the side that is not: a
// write-back that marshals the *resolved* profile would inline every inherited
// house rule into the child and silently undo the composition.
func TestCommittedProfileSurvivesAWriteBack(t *testing.T) {
	original, err := os.ReadFile(bowrainYAML)
	require.NoError(t, err)
	require.Contains(t, string(original), "duplicated from .kapi/voice.yaml",
		"the committed file explains its own duplication in a comment")

	rewritten, err := yamledit.Marshal(original, loadYAML(t, bowrainYAML))
	require.NoError(t, err)

	assert.Contains(t, string(rewritten), "duplicated from .kapi/voice.yaml",
		"an applied rule must not drop the file's account of itself")
	assert.Equal(t, string(original), string(rewritten),
		"and a profile that did not change is not rewritten at all")
}

// TestInheritanceDeclaresHouseRulesOnce shows the resolved profile carrying the
// eleven inherited prohibitions plus the one the child declares, in that order,
// with the child's file holding only its own rule.
func TestInheritanceDeclaresHouseRulesOnce(t *testing.T) {
	house := loadMarkdown(t, "house.md", mdspike.Options{})
	child := loadMarkdown(t, "bowrain-voice.md", mdspike.Options{})

	require.Len(t, house.Style.ProhibitedPatterns, 11)
	require.Len(t, child.Style.ProhibitedPatterns, 12)

	for i, pat := range house.Style.ProhibitedPatterns {
		assert.Equal(t, pat, child.Style.ProhibitedPatterns[i], "inherited rule %d keeps its position", i)
	}
	assert.Equal(t,
		`\b(enterprise-grade|best-in-class|world-class|trusted by)\b`,
		child.Style.ProhibitedPatterns[11].Regex,
		"the child's own rule comes last")

	// The house forbidden terms are inherited whole and the child adds one.
	require.Len(t, house.Vocabulary.ForbiddenTerms, 2)
	require.Len(t, child.Vocabulary.ForbiddenTerms, 3)
	assert.Equal(t, "solution", child.Vocabulary.ForbiddenTerms[2].Term)

	// Style enums the house sets and the child does not restate survive.
	assert.True(t, child.Style.ActiveVoice)
	assert.Equal(t, "short", child.Style.SentenceLength)
	assert.Equal(t, "second", child.Style.PersonPOV)
	assert.Equal(t, "sometimes", child.Style.Contractions)
}

// TestRestatingAnInheritedRuleDoesNotDoubleIt guards the one place where the
// merge is load-bearing rather than tidy. Two copies of a prohibition would
// raise two findings for one violation and double the score penalty, which is
// how a composition feature silently changes whether content ships.
func TestRestatingAnInheritedRuleDoesNotDoubleIt(t *testing.T) {
	fsys := os.DirFS("testdata")
	p, err := mdspike.LoadFS(context.Background(), fsys, "restates-house-rule.md", mdspike.Options{})
	require.NoError(t, err)

	// The house rules, none duplicated, and the restated one carries the
	// child's severity rather than the house's.
	require.Len(t, p.Style.ProhibitedPatterns, 11)
	for _, pat := range p.Style.ProhibitedPatterns {
		if strings.Contains(pat.Regex, "powerful") {
			assert.Equal(t, "critical", pat.Severity, "the child tightened the inherited severity")
		}
	}

	require.Len(t, p.Vocabulary.ForbiddenTerms, 2)
	var simply profile.TermRule
	for _, tr := range p.Vocabulary.ForbiddenTerms {
		if tr.Term == "simply" {
			simply = tr
		}
	}
	assert.Equal(t, "major", simply.Severity, "the child raised the inherited severity in place")

	// One violation, one finding — not two.
	hits := profile.MatchVocabulary(p, "This simply works.")
	assert.Len(t, hits, 1)
}
