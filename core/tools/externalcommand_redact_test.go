package tools_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tools"
)

// redactedBlock returns a block over sentence with "Mr Bean" redacted, as
// the redact step ahead of external-command leaves it.
func redactedBlock(t *testing.T, sentence string) *model.Block {
	t.Helper()
	redactTool, err := tools.NewRedactTool(&tools.RedactConfig{
		Detectors: []string{tools.DetectRules},
		Rules:     []redaction.Rule{{Term: "Mr Bean", Category: "person"}},
	})
	require.NoError(t, err)
	block := model.NewBlock("b1", sentence)
	block.SourceLocale = "en"
	processPart(t, redactTool, &model.Part{Type: model.PartBlock, Resource: block})
	require.NotContains(t, block.SourceText(), "Mr Bean")
	return block
}

func sourceCommand(command string, args ...string) *tools.ExternalCommandConfig {
	return &tools.ExternalCommandConfig{Command: command, Args: args, ApplySource: true, SendAsStdin: true, Timeout: 5}
}

func TestExternalCommand_DeclaresProcessExec(t *testing.T) {
	t.Parallel()
	reg := registry.NewToolRegistry()
	tools.RegisterAll(reg)
	s := reg.Schema(tools.ExternalCommand)
	require.NotNil(t, s)
	require.NotNil(t, s.ToolMeta)
	assert.Contains(t, s.ToolMeta.SideEffects, schema.SideEffectProcessExec)
	assert.NotContains(t, s.ToolMeta.SideEffects, schema.SideEffectAPICall, "a program is not a provider")
}

// The program reads each withheld value as its visible token, and unredact
// puts the original back into the program's output.
func TestExternalCommand_ProgramReadsTheTokenAndUnredactRestores(t *testing.T) {
	t.Parallel()
	block := redactedBlock(t, "Write to Mr Bean today.")
	cmd := tools.NewExternalCommandTool(sourceCommand("sed", "s/Write/Mail/"))
	processPart(t, cmd, &model.Part{Type: model.PartBlock, Resource: block})
	require.Empty(t, block.Properties[tools.PropExternalCommandError])
	assert.Equal(t, "Mail to [REDACTED:Person] today.", block.SourceText(), "the program read the token and kept it")

	unredact, err := tools.NewUnredactTool(&tools.UnredactConfig{})
	require.NoError(t, err)
	processPart(t, unredact, &model.Part{Type: model.PartBlock, Resource: block})
	assert.Equal(t, "Mail to Mr Bean today.", block.SourceText())
}

// An output that drops a token would lose the value it stands for, so the
// block is left as it was and the failure is recorded.
func TestExternalCommand_OutputThatDropsATokenLeavesTheBlock(t *testing.T) {
	t.Parallel()
	block := redactedBlock(t, "Write to Mr Bean today.")
	before := model.CanonicalRunsJSON(block.SourceRuns())
	cmd := tools.NewExternalCommandTool(sourceCommand("tr", "a-z", "A-Z"))
	processPart(t, cmd, &model.Part{Type: model.PartBlock, Resource: block})
	assert.Contains(t, block.Properties[tools.PropExternalCommandError], "drops the redaction placeholder [REDACTED:Person]")
	assert.Equal(t, string(before), string(model.CanonicalRunsJSON(block.SourceRuns())), "the redacted runs are kept")

	unredact, err := tools.NewUnredactTool(&tools.UnredactConfig{})
	require.NoError(t, err)
	processPart(t, unredact, &model.Part{Type: model.PartBlock, Resource: block})
	assert.Equal(t, "Write to Mr Bean today.", block.SourceText())
}

// A block with no redaction reads as SourceText reads it.
func TestExternalCommand_UnredactedBlockReadsItsText(t *testing.T) {
	t.Parallel()
	block := model.NewBlock("b1", "Write to Mr Bean today.")
	cmd := tools.NewExternalCommandTool(sourceCommand("sed", "s/Write/Mail/"))
	processPart(t, cmd, &model.Part{Type: model.PartBlock, Resource: block})
	assert.Equal(t, "Mail to Mr Bean today.", block.SourceText())
}
