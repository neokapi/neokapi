package check_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// processPart sends a single part through a tool and returns the result.
func processPart(t *testing.T, tl *tool.BaseTool, part *model.Part) *model.Part {
	t.Helper()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- part
	close(in)
	require.NoError(t, tl.Process(context.Background(), in, out))
	close(out)
	return <-out
}

// blockFindings returns the unified check findings recorded on a block.
func blockFindings(b *model.Block) []check.Finding {
	return check.Findings(tool.NewBlockView(b))
}

// findFinding returns the first finding with the given category, or false.
func findFinding(findings []check.Finding, category string) (check.Finding, bool) {
	for _, f := range findings {
		if f.Category == category {
			return f, true
		}
	}
	return check.Finding{}, false
}

// ── content-lint ─────────────────────────────────────────────────────────────

func TestContentLintToolName(t *testing.T) {
	t.Parallel()
	tl := check.NewContentLintTool()
	assert.Equal(t, "content-lint", tl.Name())
}

func TestContentLintFindings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		source       string
		wantCategory string
		wantSeverity check.Severity
	}{
		{"empty", "", "empty", check.SeverityMajor},
		{"whitespace only", "   \t  ", "empty", check.SeverityMajor},
		{"leading whitespace", " Hello world", "leading-whitespace", check.SeverityMinor},
		{"trailing whitespace", "Hello world ", "trailing-whitespace", check.SeverityMinor},
		{"double spaces", "Hello  world", "double-spaces", check.SeverityMinor},
		{"doubled word", "the the quick brown fox", "doubled-word", check.SeverityMinor},
		{"control char", "Hello\x07world", "control-char", check.SeverityMinor},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tl := check.NewContentLintTool()

			block := model.NewBlock("tu1", tc.source)
			part := &model.Part{Type: model.PartBlock, Resource: block}
			result := processPart(t, tl, part)

			findings := blockFindings(result.Resource.(*model.Block))
			f, ok := findFinding(findings, tc.wantCategory)
			require.Truef(t, ok, "expected a %q finding, got %v", tc.wantCategory, findings)
			assert.Equal(t, tc.wantSeverity, f.Severity)
		})
	}
}

func TestContentLintCleanSourceProducesNoFindings(t *testing.T) {
	t.Parallel()
	tl := check.NewContentLintTool()

	block := model.NewBlock("tu1", "Hello world, this is clean content.")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	assert.Empty(t, blockFindings(result.Resource.(*model.Block)))
}

func TestContentLintSkipsNonTranslatable(t *testing.T) {
	t.Parallel()
	tl := check.NewContentLintTool()

	block := model.NewBlock("tu1", "")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	assert.Empty(t, blockFindings(result.Resource.(*model.Block)))
}

func TestContentLintDoubledWordReportsTheWord(t *testing.T) {
	t.Parallel()
	tl := check.NewContentLintTool()

	block := model.NewBlock("tu1", "a quick quick fox")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	findings := blockFindings(result.Resource.(*model.Block))
	f, ok := findFinding(findings, "doubled-word")
	require.True(t, ok)
	assert.Equal(t, "quick", f.OriginalText)
}

// ── source length / pattern scopes (kapi check backing) ─────────────────────

func TestSourceLengthToolMaxCharsAndWords(t *testing.T) {
	t.Parallel()
	tl, err := check.NewSourceLengthTool(10, 2)
	require.NoError(t, err)

	block := model.NewBlock("tu1", "Hello world, this is long") // 25 chars, 5 words
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	findings := blockFindings(result.Resource.(*model.Block))
	f, ok := findFinding(findings, "max-chars-exceeded")
	require.True(t, ok)
	assert.Equal(t, check.SeverityMajor, f.Severity)
	assert.Contains(t, f.Message, "Source has 25")
	_, ok = findFinding(findings, "max-words-exceeded")
	assert.True(t, ok)
}

func TestSourceLengthToolRejectsNegativeLimits(t *testing.T) {
	t.Parallel()
	_, err := check.NewSourceLengthTool(-1, 0)
	require.ErrorContains(t, err, "MaxChars")
	_, err = check.NewSourceLengthTool(0, -1)
	require.ErrorContains(t, err, "MaxWords")
}

func TestSourcePatternToolForbiddenAndRequired(t *testing.T) {
	t.Parallel()
	tl, err := check.NewSourcePatternTool([]check.PatternRule{
		{Name: "forbidden-1", Pattern: `(?i)todo`, MustNotMatch: true},
		{Name: "required-1", Pattern: `\{name\}`, MustMatch: true},
	})
	require.NoError(t, err)

	block := model.NewBlock("tu1", "TODO rewrite this before launch")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	findings := blockFindings(result.Resource.(*model.Block))
	f, ok := findFinding(findings, "forbidden-pattern")
	require.True(t, ok)
	assert.Equal(t, "TODO", f.OriginalText)
	_, ok = findFinding(findings, "pattern-missing")
	assert.True(t, ok, "the required {name} pattern is absent from the source")
}

func TestSourcePatternToolValidation(t *testing.T) {
	t.Parallel()
	_, err := check.NewSourcePatternTool([]check.PatternRule{{Name: "x", Pattern: ""}})
	require.ErrorContains(t, err, "empty")
	_, err = check.NewSourcePatternTool([]check.PatternRule{{Name: "x", Pattern: "("}})
	require.ErrorContains(t, err, "invalid")
	_, err = check.NewSourcePatternTool([]check.PatternRule{{Name: "x", Pattern: "a", MustMatch: true, MustNotMatch: true}})
	require.ErrorContains(t, err, "both")
}

// ── source readiness (formerly source-check) ────────────────────────────────

// runReadiness seeds a block, runs the source-readiness stamp over it, and
// returns the stamped source status.
func runReadiness(t *testing.T, blockSeverity string, seed func(b *model.Block)) model.SourceStatus {
	t.Helper()
	block := model.NewBlock("tu1", "Our product is the best.")
	if seed != nil {
		seed(block)
	}
	tl, err := check.NewSourceReadinessTool(blockSeverity)
	require.NoError(t, err)
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)
	return result.Resource.(*model.Block).SourceStatus
}

func TestSourceReadiness_CleanSourceIsChecked(t *testing.T) {
	t.Parallel()
	got := runReadiness(t, "", nil)
	assert.Equal(t, model.SourceStatusChecked, got)
}

func TestSourceReadiness_BrandFindingStaysAuthored(t *testing.T) {
	t.Parallel()
	got := runReadiness(t, "", func(b *model.Block) {
		b.SetAnno("brand-voice", &profile.BrandVoiceAnnotation{
			Findings: []profile.BrandVoiceFinding{{
				Category: "vocabulary",
				Severity: check.SeverityCritical,
				Message:  "competitor term",
			}},
		})
	})
	assert.Equal(t, model.SourceStatusAuthored, got)
}

func TestSourceReadiness_UnifiedFindingStaysAuthored(t *testing.T) {
	t.Parallel()
	got := runReadiness(t, "", func(b *model.Block) {
		b.SetAnno(check.AnnotationKey, &check.FindingsAnnotation{
			Findings: []check.Finding{{
				Category: "terminology",
				Severity: check.SeverityMajor,
				Message:  "non-preferred term",
			}},
		})
	})
	assert.Equal(t, model.SourceStatusAuthored, got)
}

func TestSourceReadiness_MinorFindingToleratedByDefault(t *testing.T) {
	t.Parallel()
	got := runReadiness(t, "", func(b *model.Block) {
		b.SetAnno("brand-voice", &profile.BrandVoiceAnnotation{
			Findings: []profile.BrandVoiceFinding{{
				Category: "style",
				Severity: check.SeverityMinor,
				Message:  "soft preference",
			}},
		})
	})
	assert.Equal(t, model.SourceStatusChecked, got)
}

func TestSourceReadiness_MinorFindingBlocksWhenStrict(t *testing.T) {
	t.Parallel()
	got := runReadiness(t, "minor", func(b *model.Block) {
		b.SetAnno("brand-voice", &profile.BrandVoiceAnnotation{
			Findings: []profile.BrandVoiceFinding{{
				Category: "style",
				Severity: check.SeverityMinor,
				Message:  "soft preference",
			}},
		})
	})
	assert.Equal(t, model.SourceStatusAuthored, got)
}

func TestSourceReadiness_CleanReCheckKeepsApproval(t *testing.T) {
	t.Parallel()
	got := runReadiness(t, "", func(b *model.Block) {
		b.SourceStatus = model.SourceStatusApproved
	})
	assert.Equal(t, model.SourceStatusApproved, got)
}

func TestSourceReadiness_BlockingFindingRegressesApproval(t *testing.T) {
	t.Parallel()
	got := runReadiness(t, "", func(b *model.Block) {
		b.SourceStatus = model.SourceStatusApproved
		b.SetAnno("brand-voice", &profile.BrandVoiceAnnotation{
			Findings: []profile.BrandVoiceFinding{{
				Severity: check.SeverityCritical,
				Message:  "forbidden term",
			}},
		})
	})
	assert.Equal(t, model.SourceStatusAuthored, got)
}

func TestSourceReadiness_NonTranslatableUntouched(t *testing.T) {
	t.Parallel()
	block := &model.Block{ID: "x", Translatable: false, Source: []model.Run{{Text: &model.TextRun{Text: "code"}}}}
	tl, err := check.NewSourceReadinessTool("")
	require.NoError(t, err)
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)
	assert.Empty(t, result.Resource.(*model.Block).SourceStatus, "non-translatable source must not be stamped")
}

// The readiness gate's emptiness guard is the shared run-aware presence
// predicate (model.RunsHaveContent). Through SourceText() a block whose only run
// is a placeholder flattened to "" and was skipped, so it never reached `checked`
// and the source gate held it out of translation forever — fixed in #1455 but
// never pinned. It is pinned here, on the predicate every gate now shares.
func TestSourceReadiness_PlaceholderOnlySourceIsStamped(t *testing.T) {
	t.Parallel()
	block := &model.Block{ID: "price", Translatable: true, Source: []model.Run{
		{Ph: &model.PlaceholderRun{ID: "1", Type: "jsx:var", Data: "{p.price}", Equiv: "p.price"}},
	}}
	tl, err := check.NewSourceReadinessTool("")
	require.NoError(t, err)
	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})
	assert.Equal(t, model.SourceStatusChecked, result.Resource.(*model.Block).SourceStatus,
		"a placeholder-only source is authored content and must be able to reach `checked`")

	// The boundary holds: a genuinely empty source is still not stamped.
	empty := &model.Block{ID: "e", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: "  "}}}}
	result = processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: empty})
	assert.Empty(t, result.Resource.(*model.Block).SourceStatus)
}

func TestSourceReadiness_InvalidSeverityRejected(t *testing.T) {
	t.Parallel()
	_, err := check.NewSourceReadinessTool("nonsense")
	require.ErrorContains(t, err, "blockSeverity")
	_, err = check.NewSourceReadinessTool("critical")
	require.NoError(t, err)
}
