package host

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/profile"
	coretools "github.com/neokapi/neokapi/core/tools"
)

// A prose-scoped pattern rule reads the same text under `kapi check` (a parsed
// Markdown block) and under `kapi voice check` (the file read as one string): an
// inline code span is out of its reach in both, and plain prose is not.
func TestVoicePatternRulesSkipInlineCodeInBothCommands(t *testing.T) {
	const voice = "id: v\nname: V\nstyle:\n  prohibited_patterns:\n" +
		"    - regex: '(?i)\\brule\\s+prefer'\n      scope: prose\n      description: anthropomorphism\n      fails: true\n"

	tests := []struct {
		name     string
		markdown string
		fails    bool
	}{
		{"inline code is not prose", "The lint rule `prefer-t-for-label-expr` flags the render site.\n", false},
		{"plain prose is", "The lint rule prefer-t-for-label-expr flags the render site.\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateCheckExecution(t)
			dir := t.TempDir()
			src := writeCheckInput(t, dir, "s.md", tt.markdown)
			profilePath := writeCheckInput(t, dir, "v.yaml", voice)

			// kapi check
			cmd := executionCommand(t)
			cmd.Flags().String("profile-file", profilePath, "")
			report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, []string{src})
			require.NoError(t, err)
			var voiceFails []check.Diagnostic
			for _, d := range report.Findings {
				if d.Check == "voice" && d.Fails {
					voiceFails = append(voiceFails, d)
				}
			}
			if tt.fails {
				require.Len(t, voiceFails, 1, "kapi check")
				assert.Equal(t, "voice.style", voiceFails[0].Rule)
			} else {
				assert.Empty(t, voiceFails, "kapi check")
			}

			// kapi voice check
			p, err := profile.LoadProfileYAML(strings.NewReader(voice))
			require.NoError(t, err)
			findings, err := RunBlockTool(t.Context(), coretools.NewVoiceVocabCheckTool(p, nil), tt.markdown)
			require.NoError(t, err)
			if tt.fails {
				assert.Len(t, findings, 1, "kapi voice check")
			} else {
				assert.Empty(t, findings, "kapi voice check")
			}
		})
	}
}

// A Go doc comment has no inline code syntax, so its backticks reach the
// comment block as text. The pattern projection reads a backtick span in a text
// run as code, as it reads a Markdown code span.
func TestVoicePatternRulesSkipBacktickCodeInGoComments(t *testing.T) {
	const voice = "id: v\nname: V\nstyle:\n  prohibited_patterns:\n" +
		"    - regex: '(?i)\\brule\\s+prefer'\n      scope: prose\n      description: anthropomorphism\n      fails: true\n"

	tests := []struct {
		name    string
		comment string
		fails   bool
	}{
		{"backtick code is not prose", "The lint rule `prefer-t-for-label-expr` flags the render site.", false},
		{"plain prose is", "The lint rule prefer-t-for-label-expr flags the render site.", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateCheckExecution(t)
			dir := t.TempDir()
			src := writeCheckInput(t, dir, "lint.go", "package lint\n\n// Lint reports. "+tt.comment+"\nfunc Lint() {}\n")
			profilePath := writeCheckInput(t, dir, "v.yaml", voice)

			cmd := executionCommand(t)
			cmd.Flags().String("profile-file", profilePath, "")
			report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, []string{src})
			require.NoError(t, err)
			fails := 0
			for _, d := range report.Findings {
				if d.Rule == "voice.style" && d.Fails {
					fails++
				}
			}
			if tt.fails {
				assert.Equal(t, 1, fails)
			} else {
				assert.Zero(t, fails)
			}
		})
	}
}
