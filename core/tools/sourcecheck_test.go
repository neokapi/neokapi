package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
)

// runSourceCheck seeds a block, runs source-check over it, and returns the
// stamped source status.
func runSourceCheck(t *testing.T, cfg *tools.SourceCheckConfig, seed func(b *model.Block)) model.SourceStatus {
	t.Helper()
	block := model.NewBlock("tu1", "Our product is the best.")
	if seed != nil {
		seed(block)
	}
	tl := tools.NewSourceCheckTool(cfg)
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)
	return result.Resource.(*model.Block).SourceStatus
}

func TestSourceCheck_CleanSourceIsChecked(t *testing.T) {
	t.Parallel()
	// A block with no findings clears its checks → checked.
	got := runSourceCheck(t, &tools.SourceCheckConfig{}, nil)
	assert.Equal(t, model.SourceStatusChecked, got)
}

func TestSourceCheck_BrandFindingStaysAuthored(t *testing.T) {
	t.Parallel()
	// A blocking brand-voice finding (critical) keeps the source at authored.
	got := runSourceCheck(t, &tools.SourceCheckConfig{}, func(b *model.Block) {
		b.SetAnno("brand-voice", &brand.BrandVoiceAnnotation{
			Findings: []brand.BrandVoiceFinding{{
				Category: "vocabulary",
				Severity: check.SeverityCritical,
				Message:  "competitor term",
			}},
		})
	})
	assert.Equal(t, model.SourceStatusAuthored, got)
}

func TestSourceCheck_UnifiedFindingStaysAuthored(t *testing.T) {
	t.Parallel()
	// A major finding in the unified check annotation (e.g. terminology) blocks.
	got := runSourceCheck(t, &tools.SourceCheckConfig{}, func(b *model.Block) {
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

func TestSourceCheck_MinorFindingToleratedByDefault(t *testing.T) {
	t.Parallel()
	// Default threshold is major, so a minor style nit does not block readiness.
	got := runSourceCheck(t, &tools.SourceCheckConfig{}, func(b *model.Block) {
		b.SetAnno("brand-voice", &brand.BrandVoiceAnnotation{
			Findings: []brand.BrandVoiceFinding{{
				Category: "style",
				Severity: check.SeverityMinor,
				Message:  "soft preference",
			}},
		})
	})
	assert.Equal(t, model.SourceStatusChecked, got)
}

func TestSourceCheck_MinorFindingBlocksWhenStrict(t *testing.T) {
	t.Parallel()
	// With blockSeverity=minor, even a style nit keeps the source authored.
	got := runSourceCheck(t, &tools.SourceCheckConfig{BlockSeverity: "minor"}, func(b *model.Block) {
		b.SetAnno("brand-voice", &brand.BrandVoiceAnnotation{
			Findings: []brand.BrandVoiceFinding{{
				Category: "style",
				Severity: check.SeverityMinor,
				Message:  "soft preference",
			}},
		})
	})
	assert.Equal(t, model.SourceStatusAuthored, got)
}

func TestSourceCheck_CleanReCheckKeepsApproval(t *testing.T) {
	t.Parallel()
	// A clean re-check must never downgrade a human sign-off.
	got := runSourceCheck(t, &tools.SourceCheckConfig{}, func(b *model.Block) {
		b.SourceStatus = model.SourceStatusApproved
	})
	assert.Equal(t, model.SourceStatusApproved, got)
}

func TestSourceCheck_BlockingFindingRegressesApproval(t *testing.T) {
	t.Parallel()
	// An approved source that now trips a blocking check falls to authored —
	// the source changed (or a rule did) and no longer clears its checks.
	got := runSourceCheck(t, &tools.SourceCheckConfig{}, func(b *model.Block) {
		b.SourceStatus = model.SourceStatusApproved
		b.SetAnno("brand-voice", &brand.BrandVoiceAnnotation{
			Findings: []brand.BrandVoiceFinding{{
				Severity: check.SeverityCritical,
				Message:  "forbidden term",
			}},
		})
	})
	assert.Equal(t, model.SourceStatusAuthored, got)
}

func TestSourceCheck_NonTranslatableUntouched(t *testing.T) {
	t.Parallel()
	block := &model.Block{ID: "x", Translatable: false, Source: []model.Run{{Text: &model.TextRun{Text: "code"}}}}
	tl := tools.NewSourceCheckTool(&tools.SourceCheckConfig{})
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)
	assert.Empty(t, result.Resource.(*model.Block).SourceStatus, "non-translatable source must not be stamped")
}

func TestSourceCheckConfig_Validate(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (&tools.SourceCheckConfig{}).Validate())
	assert.NoError(t, (&tools.SourceCheckConfig{BlockSeverity: "critical"}).Validate())
	assert.Error(t, (&tools.SourceCheckConfig{BlockSeverity: "nonsense"}).Validate())
}
