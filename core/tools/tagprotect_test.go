package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTagProtectTool(t *testing.T) {
	t.Parallel()
	cfg := &tools.TagProtectConfig{}
	tl := tools.NewTagProtectTool(cfg)

	assert.Equal(t, "tag-protect", tl.Name())

	block := model.NewBlock("tu1", "Hello <b>world</b>, value is {count}")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	count := resultBlock.Properties[tools.PropTagProtectCount]
	// Should find HTML tags and curly brace placeholder.
	assert.NotEqual(t, "0", count)

	// Check annotation.
	ann, ok := resultBlock.Anno("protected-tags")
	assert.True(t, ok)
	assert.NotNil(t, ann)
}

func TestTagProtectToolCustomPatterns(t *testing.T) {
	t.Parallel()
	cfg := &tools.TagProtectConfig{
		Patterns: []string{`\[\[.*?\]\]`},
	}
	tl := tools.NewTagProtectTool(cfg)

	block := model.NewBlock("tu1", "Hello [[name]], welcome to [[place]]")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "2", resultBlock.Properties[tools.PropTagProtectCount])
}

func TestTagProtectToolNoTags(t *testing.T) {
	t.Parallel()
	cfg := &tools.TagProtectConfig{
		Patterns: []string{`\[\[.*?\]\]`},
	}
	tl := tools.NewTagProtectTool(cfg)

	block := model.NewBlock("tu1", "Just plain text here")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "0", resultBlock.Properties[tools.PropTagProtectCount])
}

func TestTagProtectConfigValidation(t *testing.T) {
	t.Parallel()
	cfg := &tools.TagProtectConfig{Patterns: []string{""}}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")

	cfg = &tools.TagProtectConfig{Patterns: []string{"[invalid"}}
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")

	cfg = &tools.TagProtectConfig{Patterns: []string{`<[^>]+>`}}
	err = cfg.Validate()
	require.NoError(t, err)
}

func TestReplaceAndRestoreProtectedTags(t *testing.T) {
	t.Parallel()
	tags := []tools.ProtectedTag{
		{Text: "<b>", Offset: 6},
		{Text: "</b>", Offset: 14},
	}

	text := "Hello <b>world</b>"
	replaced, mapping := tools.ReplaceProtectedTags(text, tags)
	assert.NotContains(t, replaced, "<b>")
	assert.NotContains(t, replaced, "</b>")
	assert.Contains(t, replaced, "Hello")
	assert.Len(t, mapping, 2)

	restored := tools.RestoreProtectedTags(replaced, mapping)
	assert.Equal(t, text, restored)
}
