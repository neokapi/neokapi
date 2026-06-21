package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContentLintToolName(t *testing.T) {
	t.Parallel()
	tl := tools.NewContentLintTool(&tools.ContentLintConfig{})
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
			tl := tools.NewContentLintTool(&tools.ContentLintConfig{})

			block := model.NewBlock("tu1", tc.source)
			part := &model.Part{Type: model.PartBlock, Resource: block}
			result := processPart(t, tl, part)

			findings := qaFindings(result.Resource.(*model.Block))
			f, ok := findFinding(findings, tc.wantCategory)
			require.Truef(t, ok, "expected a %q finding, got %v", tc.wantCategory, findings)
			assert.Equal(t, tc.wantSeverity, f.Severity)
		})
	}
}

func TestContentLintCleanSourceProducesNoFindings(t *testing.T) {
	t.Parallel()
	tl := tools.NewContentLintTool(&tools.ContentLintConfig{})

	block := model.NewBlock("tu1", "Hello world, this is clean content.")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	assert.Empty(t, qaFindings(result.Resource.(*model.Block)))
}

func TestContentLintSkipsNonTranslatable(t *testing.T) {
	t.Parallel()
	tl := tools.NewContentLintTool(&tools.ContentLintConfig{})

	block := model.NewBlock("tu1", "")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	assert.Empty(t, qaFindings(result.Resource.(*model.Block)))
}

func TestContentLintDoubledWordReportsTheWord(t *testing.T) {
	t.Parallel()
	tl := tools.NewContentLintTool(&tools.ContentLintConfig{})

	block := model.NewBlock("tu1", "a quick quick fox")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	findings := qaFindings(result.Resource.(*model.Block))
	f, ok := findFinding(findings, "doubled-word")
	require.True(t, ok)
	assert.Equal(t, "quick", f.OriginalText)
}
