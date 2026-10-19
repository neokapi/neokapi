package profile_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
)

func ptText(s string) model.Run { return model.Run{Text: &model.TextRun{Text: s}} }

func ptCodeOpen(id string) model.Run {
	return model.Run{PcOpen: &model.PcOpenRun{ID: id, Type: model.TypeInlineCode, SubType: "md:code", Data: "`"}}
}

func ptCodeClose(id string) model.Run {
	return model.Run{PcClose: &model.PcCloseRun{ID: id, Type: model.TypeInlineCode, SubType: "md:code", Data: "`"}}
}

func ptBoldOpen(id string) model.Run {
	return model.Run{PcOpen: &model.PcOpenRun{ID: id, Type: "fmt:bold", Data: "**"}}
}

func ptBoldClose(id string) model.Run {
	return model.Run{PcClose: &model.PcCloseRun{ID: id, Type: "fmt:bold", Data: "**"}}
}

func ptPh(id string) model.Run {
	return model.Run{Ph: &model.PlaceholderRun{ID: id, Type: "code", Data: "{name}", Equiv: "name"}}
}

func TestRunsPatternText(t *testing.T) {
	tests := []struct {
		name string
		runs []model.Run
		want string
	}{
		{
			name: "plain text is unchanged",
			runs: []model.Run{ptText("The rule prefers it.")},
			want: "The rule prefers it.",
		},
		{
			name: "an inline code span is spelled with backticks",
			runs: []model.Run{ptText("The lint rule "), ptCodeOpen("1"), ptText("prefer-t"), ptCodeClose("1"), ptText(" flags it.")},
			want: "The lint rule `prefer-t` flags it.",
		},
		{
			name: "emphasis markers contribute no text",
			runs: []model.Run{ptText("The rule "), ptBoldOpen("1"), ptText("prefers"), ptBoldClose("1"), ptText(" it.")},
			want: "The rule prefers it.",
		},
		{
			name: "a placeholder contributes no text",
			runs: []model.Run{ptText("Hello "), ptPh("1"), ptText(" there.")},
			want: "Hello  there.",
		},
		{
			name: "a literal object replacement character is text",
			runs: []model.Run{ptText("a￼b")},
			want: "a￼b",
		},
		{
			name: "a plural reads its other branch",
			runs: []model.Run{{Plural: &model.PluralRun{Forms: map[model.PluralForm][]model.Run{
				model.PluralOne:   {ptText("one "), ptCodeOpen("1"), ptText("file"), ptCodeClose("1")},
				model.PluralOther: {ptText("many "), ptCodeOpen("2"), ptText("files"), ptCodeClose("2")},
			}}}},
			want: "many `files`",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pt := profile.RunsPatternText(tt.runs)
			assert.Equal(t, tt.want, pt.Text())

			// Every byte of the projection that came from a text run maps back
			// onto the same byte of RunsText.
			real := model.RunsText(tt.runs)
			for i := range len(pt.Text()) {
				off := pt.RealOffset(i)
				require.LessOrEqual(t, off, len(real))
				if pt.Text()[i] != '`' {
					assert.Equal(t, pt.Text()[i], real[off], "byte %d", i)
				}
			}
			assert.Equal(t, len(real), pt.RealOffset(len(pt.Text())))
		})
	}
}

// The projection of a block and the plain string a reader would type for it
// must answer every pattern rule alike, whatever its scope.
func TestMatchPatternsInRunsAgreesWithPlainText(t *testing.T) {
	codeRuns := []model.Run{
		ptText("The lint rule "), ptCodeOpen("1"), ptText("prefer-t-for-label-expr"), ptCodeClose("1"),
		ptText(" flags the render site."),
	}
	const codeText = "The lint rule `prefer-t-for-label-expr` flags the render site."
	plainRuns := []model.Run{ptText("The lint rule prefer-t-for-label-expr flags the render site.")}
	const plainText = "The lint rule prefer-t-for-label-expr flags the render site."

	tests := []struct {
		name     string
		scope    string
		regex    string
		runs     []model.Run
		text     string
		wantHits int
		wantSnip string
	}{
		{"prose rule skips inline code", profile.ScopeProse, `(?i)\brule\s+prefer`, codeRuns, codeText, 0, ""},
		{"prose rule matches plain prose", profile.ScopeProse, `(?i)\brule\s+prefer`, plainRuns, plainText, 1, "rule prefer"},
		{"unscoped rule does not read across a code delimiter", "", `(?i)\brule\s+prefer`, codeRuns, codeText, 0, ""},
		{"code rule matches inside inline code", profile.ScopeCode, `prefer-t`, codeRuns, codeText, 1, "prefer-t"},
		{"code rule ignores plain prose", profile.ScopeCode, `prefer-t`, plainRuns, plainText, 0, ""},
		{"prose rule still reads the prose beside code", profile.ScopeProse, `render site`, codeRuns, codeText, 1, "render site"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &profile.VoiceProfile{Style: profile.StyleRules{ProhibitedPatterns: []profile.Pattern{
				{Regex: tt.regex, Scope: tt.scope, Description: "test rule"},
			}}}
			fromRuns := profile.MatchPatternsInRuns(p, tt.runs)
			fromText := profile.MatchPatterns(p, tt.text)
			assert.Len(t, fromRuns, tt.wantHits, "runs")
			assert.Len(t, fromText, tt.wantHits, "plain text")

			findings := profile.PatternFindings(p, model.RunsText(tt.runs), tt.runs)
			require.Len(t, findings, tt.wantHits)
			if tt.wantHits > 0 {
				f := findings[0]
				assert.Equal(t, tt.wantSnip, f.OriginalText)
				assert.Equal(t, tt.wantSnip, model.RunsText(f.Position.ExtractRuns(tt.runs)))
			}
		})
	}
}
