package profile

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repoVoicePacks are the voice profiles this repo governs its own prose with,
// relative to the repo root. The bowrain profile restates the house rules
// because a profile cannot yet inherit from another, so every judgement
// asserted here is asserted of both — a rule narrowed in one pack and left
// alone in the other is the drift those two files are meant to avoid.
var repoVoicePacks = []string{
	filepath.Join(".kapi", "voice.yaml"),
	filepath.Join(".kapi", "profiles", "bowrain", "voice.yaml"),
}

// repoRoot locates the checkout this test file lives in, so the packs are read
// from the tree under test rather than from a developer's working directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller must resolve the test file path")
	// file = <root>/core/profile/repopacks_test.go
	root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	require.FileExists(t, filepath.Join(root, "go.work"), "repo root detection")
	return root
}

func loadRepoPack(t *testing.T, rel string) *VoiceProfile {
	t.Helper()
	f, err := os.Open(filepath.Join(repoRoot(t), rel))
	require.NoError(t, err, "the repo's own voice pack must be readable")
	t.Cleanup(func() { _ = f.Close() })
	p, err := DecodeProfileStrict(f)
	require.NoError(t, err, "the repo's own voice pack must decode under the strict schema")
	return p
}

// magicFindings narrows a pack's whole deterministic verdict on text down to
// what it says about the word "magic", so a sample that happens to trip an
// unrelated house rule cannot be mistaken for a verdict on the register.
func magicFindings(p *VoiceProfile, text string) []VoiceFinding {
	var out []VoiceFinding
	for _, f := range Findings(p, text, nil) {
		if strings.Contains(strings.ToLower(f.OriginalText), "magic") {
			out = append(out, f)
		}
	}
	return out
}

func TestRepoVoicePacksAreValid(t *testing.T) {
	for _, rel := range repoVoicePacks {
		t.Run(rel, func(t *testing.T) {
			assert.Empty(t, ValidateProfile(loadRepoPack(t, rel)))
		})
	}
}

// TestRepoVoicePacksJudgeMagicByRegister pins both halves of the "magic" rule:
// the word as a claim about the product is a major finding, and the technical
// compound that names a file-format signature is not a finding at all. RE2 has
// no lookahead, so the exclusion cannot be written as `\bmagic\b(?!\s+bytes)`
// and the marketing register is written positively instead — which makes the
// clean half of this table the load-bearing half.
func TestRepoVoicePacksJudgeMagicByRegister(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		flagged bool
	}{
		{
			name:    "the simile that closes a sentence",
			text:    "It just works, like magic.",
			flagged: true,
		},
		{
			name:    "a copular claim about the product",
			text:    "The first run is pure magic!",
			flagged: true,
		},
		{
			name:    "a contracted copular claim",
			text:    "It's magic.",
			flagged: true,
		},
		{
			name:    "a typographic apostrophe reads the same",
			text:    "That’s magic, and the reader never sees the wiring.",
			flagged: true,
		},
		{
			name:    "the claim closing a block needs no punctuation",
			text:    "Point kapi at the repository and this is magic",
			flagged: true,
		},
		{
			name:    "the adjective has no technical sense",
			text:    "A magical experience for the whole team.",
			flagged: true,
		},
		{
			name:    "the adverb has no technical sense either",
			text:    "The catalogs update magically.",
			flagged: true,
		},
		{
			name:    "the stock invitation to the demo",
			text:    "Open the preview to watch where the magic happens.",
			flagged: true,
		},
		{
			name:    "the register is case-insensitive",
			text:    "IT JUST WORKS, LIKE MAGIC.",
			flagged: true,
		},
		{
			name:    "magic bytes name a file signature",
			text:    "Detection reads the magic bytes at the head of the file.",
			flagged: false,
		},
		{
			name:    "magic bytes in a list item",
			text:    "Magic bytes / content sniffing",
			flagged: false,
		},
		{
			name:    "a magic number is a constant, not a claim",
			text:    "Batching intent, not a magic number.",
			flagged: false,
		},
		{
			name:    "a magic string identifies a schema",
			text:    "The kind field is the magic string on the document root.",
			flagged: false,
		},
		{
			name:    "a simile whose object is the compound stays clean",
			text:    "Content sniffing works like magic bytes do in libmagic.",
			flagged: false,
		},
		{
			name:    "the hyphenated compound stays clean",
			text:    "This is magic-byte sniffing at the head of the stream.",
			flagged: false,
		},
		{
			name:    "the plural compound stays clean",
			text:    "The cascade compares magic numbers before it reads the body.",
			flagged: false,
		},
	}

	for _, rel := range repoVoicePacks {
		p := loadRepoPack(t, rel)
		for _, tt := range tests {
			t.Run(rel+"/"+tt.name, func(t *testing.T) {
				got := magicFindings(p, tt.text)
				if !tt.flagged {
					assert.Empty(t, got, "the technical sense of %q is not a voice defect", tt.text)
					return
				}
				require.NotEmpty(t, got, "the marketing sense of %q must still be caught", tt.text)
				for _, f := range got {
					assert.Equal(t, SeverityMajor, f.Severity)
				}
			})
		}
	}
}
