package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// qaFindings returns the unified check findings recorded on a block under the
// quality.findings annotation (the model every checker now writes).
func qaFindings(b *model.Block) []check.Finding {
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

func TestQACheckTool(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	assert.Equal(t, "qa", tl.Name())
	assert.Contains(t, tl.Description(), "quality")
}

func TestQACheckToolPassingBlock(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Empty(t, qaFindings(resultBlock), "a clean block records no findings")
}

func TestQACheckToolEmptyTarget(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	// No target set.
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	findings := qaFindings(resultBlock)
	require.Len(t, findings, 1)
	assert.Equal(t, "empty-target", findings[0].Category)
	assert.Equal(t, check.SeverityMajor, findings[0].Severity)
}

func TestQACheckToolLeadingWhitespace(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "  Bonjour le monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	f, found := findFinding(qaFindings(resultBlock), "leading-whitespace")
	require.True(t, found, "Expected leading-whitespace finding")
	assert.Equal(t, check.SeverityMinor, f.Severity)
}

func TestQACheckToolTrailingWhitespace(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde  ")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	f, found := findFinding(qaFindings(resultBlock), "trailing-whitespace")
	require.True(t, found, "Expected trailing-whitespace finding")
	assert.Equal(t, check.SeverityMinor, f.Severity)
}

func TestQACheckToolDoubleSpaces(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour  le  monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	f, found := findFinding(qaFindings(resultBlock), "double-spaces")
	require.True(t, found, "Expected double-spaces finding")
	assert.Equal(t, check.SeverityMinor, f.Severity)
}

func TestQACheckToolTargetSameAsSource(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	f, found := findFinding(qaFindings(resultBlock), "target-same-as-source")
	require.True(t, found, "Expected target-same-as-source finding")
	assert.Equal(t, check.SeverityMinor, f.Severity)
}

func TestQACheckToolMultipleIssues(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "  Hello  world ")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	findings := qaFindings(resultBlock)
	// Should have at least leading whitespace, double spaces, and trailing whitespace findings.
	assert.GreaterOrEqual(t, len(findings), 2, "Expected multiple findings, got %d", len(findings))
}

func TestQACheckToolSkipsNonTranslatable(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Empty(t, qaFindings(resultBlock))
}

func TestQACheckToolDisabledChecks(t *testing.T) {
	t.Parallel()
	cfg := &tools.QACheckConfig{
		TargetLocale:            model.LocaleFrench,
		CheckLeadingWhitespace:  false,
		CheckTrailingWhitespace: false,
		CheckDoubleSpaces:       false,
		CheckEmptyTarget:        false,
		CheckTargetSameAsSource: false,
	}
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "  Hello  world ")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Empty(t, qaFindings(resultBlock))
}

func TestQACheckConfigValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cfg     tools.QACheckConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing target locale",
			cfg:     tools.QACheckConfig{},
			wantErr: true,
			errMsg:  "TargetLocale",
		},
		{
			name: "valid config",
			cfg:  tools.QACheckConfig{TargetLocale: model.LocaleFrench},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestQACheckToolNonDeletableSpanMissing(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	// Source has a non-deletable break placeholder.
	sourceRuns := []model.Run{
		{Text: &model.TextRun{Text: "Hello"}},
		{Ph: &model.PlaceholderRun{
			ID: "1", Type: "struct:break", Data: "<br/>",
			Constraints: &model.RunConstraints{Deletable: false},
		}},
		{Text: &model.TextRun{Text: "world"}},
	}
	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       sourceRuns,
		Properties:   make(map[string]string),
	}
	// Target is missing the break placeholder.
	block.SetTargetText(model.LocaleFrench, "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	f, found := findFinding(qaFindings(resultBlock), "non-deletable-span-missing")
	require.True(t, found, "Expected non-deletable-span-missing finding")
	assert.Equal(t, check.SeverityMajor, f.Severity)
	assert.Contains(t, f.Message, "struct:break")
}

func TestQACheckToolNonCloneableSpanDuplicated(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	nonCloneable := func() *model.PlaceholderRun {
		return &model.PlaceholderRun{
			ID: "1", Type: "code:variable", Data: "{name}",
			Constraints: &model.RunConstraints{Cloneable: false},
		}
	}

	// Source has one non-cloneable variable placeholder.
	sourceRuns := []model.Run{
		{Text: &model.TextRun{Text: "Hello "}},
		{Ph: nonCloneable()},
		{Text: &model.TextRun{Text: " world"}},
	}
	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       sourceRuns,
		Properties:   make(map[string]string),
	}
	// Target duplicates the variable placeholder.
	targetRuns := []model.Run{
		{Text: &model.TextRun{Text: "Bonjour "}},
		{Ph: nonCloneable()},
		{Text: &model.TextRun{Text: " le "}},
		{Ph: nonCloneable()},
		{Text: &model.TextRun{Text: " monde"}},
	}
	block.SetTargetRuns(model.LocaleFrench, targetRuns)
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	f, found := findFinding(qaFindings(resultBlock), "non-cloneable-span-duplicated")
	require.True(t, found, "Expected non-cloneable-span-duplicated finding")
	assert.Equal(t, check.SeverityMajor, f.Severity)
	assert.Contains(t, f.Message, "code:variable")
}

func TestQACheckToolDeletableSpanMissingNoConstraintError(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	// Source has a deletable bold pair.
	deletable := &model.RunConstraints{Deletable: true}
	sourceRuns := []model.Run{
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: "<b>", Constraints: deletable}},
		{Text: &model.TextRun{Text: "Hello"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "</b>"}},
	}
	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       sourceRuns,
		Properties:   make(map[string]string),
	}
	// Target is missing the bold pair (which are deletable).
	block.SetTargetText(model.LocaleFrench, "Bonjour")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, found := findFinding(qaFindings(resultBlock), "non-deletable-span-missing")
	assert.False(t, found, "Should not flag deletable span as non-deletable")
}

func TestQACheckToolSpanConstraintsDisabled(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	cfg.CheckSpanConstraints = false
	tl := tools.NewQACheckTool(cfg)

	// Source has a non-deletable break placeholder.
	sourceRuns := []model.Run{
		{Text: &model.TextRun{Text: "Hello"}},
		{Ph: &model.PlaceholderRun{
			ID: "1", Type: "struct:break", Data: "<br/>",
			Constraints: &model.RunConstraints{Deletable: false},
		}},
		{Text: &model.TextRun{Text: "world"}},
	}
	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       sourceRuns,
		Properties:   make(map[string]string),
	}
	// Target is missing the break placeholder, but check is disabled.
	block.SetTargetText(model.LocaleFrench, "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, found := findFinding(qaFindings(resultBlock), "non-deletable-span-missing")
	assert.False(t, found, "Should not check span constraints when disabled")
}

func TestQACheckToolEmptyTargetText(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	findings := qaFindings(resultBlock)
	require.Len(t, findings, 1)
	assert.Equal(t, "empty-target", findings[0].Category)
}
