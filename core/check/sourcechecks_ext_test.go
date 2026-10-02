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
		wantFails    bool
	}{
		{"empty", "", "empty", true},
		{"whitespace only", "   \t  ", "empty", true},
		{"leading whitespace", " Hello world", "leading-whitespace", false},
		{"trailing whitespace", "Hello world ", "trailing-whitespace", false},
		{"double spaces", "Hello  world", "double-spaces", false},
		{"doubled word", "the the quick brown fox", "doubled-word", false},
		{"control char", "Hello\x07world", "control-char", false},
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
			assert.Equal(t, tc.wantFails, f.Fails)
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
	assert.True(t, f.Fails)
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

// ── source readiness ────────────────────────────────────────────────────────

// runReadiness seeds a block, settles it, and returns the stamped source
// status and whether the source fails its checks.
func runReadiness(t *testing.T, seed func(b *model.Block)) (model.SourceStatus, bool) {
	t.Helper()
	block := model.NewBlock("tu1", "Our product is the best.")
	if seed != nil {
		seed(block)
	}
	check.SettleSourceStatus(t.Context(), block)
	return block.SourceStatus, block.SourceFailing()
}

func TestSourceReadiness_CleanSourceIsWrittenAndPasses(t *testing.T) {
	t.Parallel()
	got, failing := runReadiness(t, nil)
	assert.Equal(t, model.SourceStatusWritten, got)
	assert.False(t, failing)
}

func TestSourceReadiness_VoiceFindingFails(t *testing.T) {
	t.Parallel()
	got, failing := runReadiness(t, func(b *model.Block) {
		b.SetAnno("voice", &profile.VoiceAnnotation{
			Findings: []profile.VoiceFinding{{
				Category: "vocabulary",
				Fails:    true,
				Message:  "competitor term",
			}},
		})
	})
	assert.Equal(t, model.SourceStatusWritten, got)
	assert.True(t, failing)
}

func TestSourceReadiness_UnifiedFindingFails(t *testing.T) {
	t.Parallel()
	_, failing := runReadiness(t, func(b *model.Block) {
		b.SetAnno(check.AnnotationKey, &check.FindingsAnnotation{
			Findings: []check.Finding{{
				Category: "terminology",
				Fails:    true,
				Message:  "non-preferred term",
			}},
		})
	})
	assert.True(t, failing)
}

func TestSourceReadiness_ReportingFindingTolerated(t *testing.T) {
	t.Parallel()
	_, failing := runReadiness(t, func(b *model.Block) {
		b.SetAnno("voice", &profile.VoiceAnnotation{
			Findings: []profile.VoiceFinding{{
				Category: "style",
				Message:  "soft preference",
			}},
		})
	})
	assert.False(t, failing)
}

func TestSourceReadiness_KeepsEstablished(t *testing.T) {
	t.Parallel()
	got, failing := runReadiness(t, func(b *model.Block) {
		b.SourceStatus = model.SourceStatusEstablished
		b.SetAnno("voice", &profile.VoiceAnnotation{
			Findings: []profile.VoiceFinding{{
				Fails:   true,
				Message: "forbidden term",
			}},
		})
	})
	assert.Equal(t, model.SourceStatusEstablished, got, "a check never undoes a person's decision")
	assert.True(t, failing, "a failing finding still holds the source at the gate")
}

func TestSourceReadiness_NonTranslatableUntouched(t *testing.T) {
	t.Parallel()
	block := &model.Block{ID: "x", Translatable: false, Source: []model.Run{{Text: &model.TextRun{Text: "code"}}}}
	check.SettleSourceStatus(t.Context(), block)
	assert.Empty(t, block.SourceStatus, "non-translatable source must not be stamped")
}

// The settle stamps the authoritative edition. A block read from an en-US to
// en-US file holds a target under its source language; the settle stamps the
// source, leaves the target's status alone, keeps an established source, and
// records no edit of the source.
func TestSourceReadiness_StampsTheAuthoritativeEdition(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		from model.SourceStatus
		want model.SourceStatus
	}{
		{"new", model.SourceStatusNew, model.SourceStatusWritten},
		{"written", model.SourceStatusWritten, model.SourceStatusWritten},
		{"established", model.SourceStatusEstablished, model.SourceStatusEstablished},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			block := model.NewBlock("b1", "The colour of the button.")
			block.SourceLocale = "en-US"
			block.SourceStatus = tc.from
			block.SetTargetVariant(model.Variant("en-US"), &model.Target{Runs: []model.Run{model.TextR("The color of the button.")}, Status: model.TargetStatusDraft})

			check.SettleSourceStatus(t.Context(), block)

			src, ok := block.Edition(model.EditionKey{})
			require.True(t, ok)
			assert.Equal(t, model.Status(tc.want), src.Status)
			assert.Equal(t, "The colour of the button.", model.RunsText(src.Runs))
			assert.Equal(t, model.TargetStatusDraft, block.Target("en-US").Status)
			_, edited := block.SourceAsRead()
			assert.False(t, edited)
		})
	}
}

// The settle stamps the source status and nothing else: the source-origin
// annotation stays the value it was, whatever its type, and the block records
// no copy of its source as read, so content a reader sets afterwards is still
// the source as read.
func TestSourceReadiness_StampTouchesOnlyTheStatus(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		anno model.Payload
	}{
		{"origin", &model.Origin{Kind: model.OriginHuman, Tool: "editor"}},
		{"undecoded origin", &model.RawAnnotation{Kind: model.AnnoSourceOrigin, Body: []byte(`{"kind":`)}},
		{"no origin", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			block := model.NewBlock("b1", "The colour of the button.")
			block.SourceLocale = "en-US"
			if tc.anno != nil {
				block.SetAnno(model.AnnoSourceOrigin, tc.anno)
			}

			check.SettleSourceStatus(t.Context(), block)

			assert.Equal(t, model.SourceStatusWritten, block.SourceStatus)
			got, ok := block.Anno(model.AnnoSourceOrigin)
			if tc.anno == nil {
				assert.False(t, ok)
			} else {
				require.True(t, ok)
				assert.Same(t, tc.anno, got)
			}
			block.SetSourceRuns([]model.Run{model.TextR("The colour of the link.")})
			runs, edited := block.SourceAsRead()
			assert.False(t, edited, "a status stamp records no source as read")
			assert.Equal(t, "The colour of the link.", model.RunsText(runs))
		})
	}
}

// The settle's emptiness guard is the shared run-aware presence predicate
// (model.RunsHaveContent), so a block whose only run is a placeholder is
// written source that can clear the gate.
func TestSourceReadiness_PlaceholderOnlySourceIsStamped(t *testing.T) {
	t.Parallel()
	block := &model.Block{ID: "price", Translatable: true, Source: []model.Run{
		{Ph: &model.PlaceholderRun{ID: "1", Type: "jsx:var", Data: "{p.price}", Equiv: "p.price"}},
	}}
	check.SettleSourceStatus(t.Context(), block)
	assert.Equal(t, model.SourceStatusWritten, block.SourceStatus)

	// The boundary holds: a genuinely empty source is still not stamped.
	empty := &model.Block{ID: "e", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: "  "}}}}
	check.SettleSourceStatus(t.Context(), empty)
	assert.Empty(t, empty.SourceStatus)
}
