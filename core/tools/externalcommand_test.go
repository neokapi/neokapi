package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExternalCommandToolCatStdin(t *testing.T) {
	t.Parallel()
	cfg := &tools.ExternalCommandConfig{
		Command:     "cat",
		ApplySource: true,
		ApplyTarget: false,
		SendAsStdin: true,
		Timeout:     5,
	}
	tl := tools.NewExternalCommandTool(cfg)

	assert.Equal(t, "external-command", tl.Name())

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello world", resultBlock.SourceText())
	assert.Equal(t, "0", resultBlock.Properties[tools.PropExternalCommandExitCode])
}

func TestExternalCommandToolArgsPlaceholders(t *testing.T) {
	t.Parallel()
	cfg := &tools.ExternalCommandConfig{
		Command:     "echo",
		Args:        []string{"-n", "${source}"},
		ApplySource: true,
		ApplyTarget: false,
		SendAsStdin: false,
		Timeout:     5,
	}
	tl := tools.NewExternalCommandTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello", resultBlock.SourceText())
	assert.Equal(t, "0", resultBlock.Properties[tools.PropExternalCommandExitCode])
}

func TestExternalCommandToolTarget(t *testing.T) {
	t.Parallel()
	cfg := &tools.ExternalCommandConfig{
		Command:      "tr",
		Args:         []string{"a-z", "A-Z"},
		ApplySource:  false,
		ApplyTarget:  true,
		TargetLocale: model.LocaleFrench,
		SendAsStdin:  true,
		Timeout:      5,
	}
	tl := tools.NewExternalCommandTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "bonjour le monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello world", resultBlock.SourceText()) // Source unchanged.
	assert.Equal(t, "BONJOUR LE MONDE", resultBlock.TargetText(model.LocaleFrench))
	assert.Equal(t, "0", resultBlock.Properties[tools.PropExternalCommandExitCode])
}

func TestExternalCommandToolTimeout(t *testing.T) {
	t.Parallel()
	cfg := &tools.ExternalCommandConfig{
		Command:     "sleep",
		Args:        []string{"10"},
		ApplySource: true,
		ApplyTarget: false,
		SendAsStdin: false,
		Timeout:     1,
	}
	tl := tools.NewExternalCommandTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- part
	close(in)

	err := tl.Process(t.Context(), in, out)
	close(out)
	require.NoError(t, err) // Tool stores error in properties, doesn't return it.

	result := <-out
	require.NotNil(t, result)
	resultBlock := result.Resource.(*model.Block)
	// The command was killed, so exit code should be non-zero or error set.
	assert.NotEmpty(t, resultBlock.Properties[tools.PropExternalCommandExitCode])
}

func TestExternalCommandToolSkipsNonTranslatable(t *testing.T) {
	t.Parallel()
	cfg := &tools.ExternalCommandConfig{
		Command:     "echo",
		Args:        []string{"replaced"},
		ApplySource: true,
		ApplyTarget: false,
		SendAsStdin: false,
		Timeout:     5,
	}
	tl := tools.NewExternalCommandTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello world", resultBlock.SourceText()) // Unchanged.
}

func TestExternalCommandToolExitCode(t *testing.T) {
	t.Parallel()
	cfg := &tools.ExternalCommandConfig{
		Command:     "false", // Exits with code 1.
		ApplySource: true,
		ApplyTarget: false,
		SendAsStdin: false,
		Timeout:     5,
	}
	tl := tools.NewExternalCommandTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "1", resultBlock.Properties[tools.PropExternalCommandExitCode])
}

func TestExternalCommandConfigValidation(t *testing.T) {
	t.Parallel()
	// Empty command.
	cfg := &tools.ExternalCommandConfig{}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command must not be empty")

	// ApplyTarget without locale.
	cfg = &tools.ExternalCommandConfig{
		Command:     "echo",
		ApplyTarget: true,
	}
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target locale is required")

	// Valid config.
	cfg = &tools.ExternalCommandConfig{
		Command:      "echo",
		ApplyTarget:  true,
		TargetLocale: model.LocaleFrench,
	}
	err = cfg.Validate()
	require.NoError(t, err)
}

func TestExternalCommandConfigReset(t *testing.T) {
	t.Parallel()
	cfg := &tools.ExternalCommandConfig{
		Command:      "echo",
		Args:         []string{"hello"},
		ApplySource:  true,
		ApplyTarget:  false,
		TargetLocale: model.LocaleFrench,
		SendAsStdin:  false,
		Timeout:      60,
	}
	cfg.Reset()

	assert.Empty(t, cfg.Command)
	assert.Nil(t, cfg.Args)
	assert.False(t, cfg.ApplySource)
	assert.True(t, cfg.ApplyTarget)
	assert.True(t, cfg.TargetLocale.IsEmpty())
	assert.True(t, cfg.SendAsStdin)
	assert.Equal(t, 30, cfg.Timeout)
}

func TestExternalCommandConfigToolName(t *testing.T) {
	t.Parallel()
	cfg := &tools.ExternalCommandConfig{}
	assert.Equal(t, "external-command", cfg.ToolName())
}
