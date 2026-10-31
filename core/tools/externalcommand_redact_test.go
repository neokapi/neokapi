package tools_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
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

// An output whose placeholders differ from the input's would lose, repeat or
// misplace a value, so the block is left as it was and each placeholder that
// differs is a failing finding.
func TestExternalCommand_OutputThatChangesATokenLeavesTheBlock(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		cmd      *tools.ExternalCommandConfig
		kind     string
		message  string
		expected string
		found    string
	}{
		{
			name: "dropped", cmd: sourceCommand("sed", `s/\[REDACTED:Person\]/someone/`), kind: "dropped",
			message:  "sed dropped the redaction placeholder [REDACTED:Person] that the source holds; the block is unchanged",
			expected: "1", found: "0",
		},
		{
			name: "altered", cmd: sourceCommand("tr", "a-z", "A-Z"), kind: "altered",
			message:  "tr gave back [REDACTED:PERSON] where the source holds the redaction placeholder [REDACTED:Person]; the block is unchanged",
			expected: "1", found: "0",
		},
		{
			name: "duplicated", cmd: sourceCommand("sed", `s/\(\[REDACTED:Person\]\)/\1 and \1/`), kind: "duplicated",
			message:  "sed gave back the redaction placeholder [REDACTED:Person] 2 times where the source holds it 1; the block is unchanged",
			expected: "1", found: "2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			block := redactedBlock(t, "Write to Mr Bean today.")
			before := model.CanonicalRunsJSON(block.SourceRuns())
			processPart(t, tools.NewExternalCommandTool(tt.cmd), &model.Part{Type: model.PartBlock, Resource: block})
			assert.Equal(t, string(before), string(model.CanonicalRunsJSON(block.SourceRuns())), "the redacted runs are kept")
			assert.Empty(t, block.Properties[tools.PropExternalCommandError], "the finding is the record, not a property")

			ann, ok := model.AnnoAs[*check.FindingsAnnotation](block, check.AnnotationKey)
			require.True(t, ok, "the block carries the finding")
			require.Len(t, ann.Findings, 1)
			f := ann.Findings[0]
			assert.Equal(t, tools.RulePlaceholder, f.Category)
			assert.Equal(t, string(tools.ExternalCommand), f.Check)
			assert.True(t, f.Fails)
			assert.Equal(t, tt.message, f.Message)
			assert.Equal(t, "[REDACTED:Person]", f.Metadata["token"])
			assert.Equal(t, tt.kind, f.Metadata["kind"])
			assert.Equal(t, tt.expected, f.Metadata["expected"])
			assert.Equal(t, tt.found, f.Metadata["found"])

			unredact, err := tools.NewUnredactTool(&tools.UnredactConfig{})
			require.NoError(t, err)
			processPart(t, unredact, &model.Part{Type: model.PartBlock, Resource: block})
			assert.Equal(t, "Write to Mr Bean today.", block.SourceText(), "the block reads as it did before the run")
		})
	}
}

// An output that carries a placeholder of the block that the side it read
// does not hold puts a value where the input had none.
func TestExternalCommand_OutputThatAddsATokenLeavesTheBlock(t *testing.T) {
	t.Parallel()
	redactTool, err := tools.NewRedactTool(&tools.RedactConfig{
		Detectors:  []string{tools.DetectRules},
		Rules:      []redaction.Rule{{Term: "Mr Bean", Category: "person"}, {Term: "bean@example.com", Category: "email"}},
		WholeBlock: true,
	})
	require.NoError(t, err)
	block := model.NewBlock("b1", "Write to Mr Bean today.")
	block.SourceLocale = "en"
	block.SetTargetText("fr", "Écrivez à bean@example.com.")
	processPart(t, redactTool, &model.Part{Type: model.PartBlock, Resource: block})
	frText, _ := redaction.VisibleText(block.TargetRuns("fr"))
	require.Equal(t, "Écrivez à [REDACTED:Email].", frText)

	cmd := tools.NewExternalCommandTool(sourceCommand("sed", `s/today/at [REDACTED:Email]/`))
	processPart(t, cmd, &model.Part{Type: model.PartBlock, Resource: block})
	srcText, _ := redaction.VisibleText(block.SourceRuns())
	assert.Equal(t, "Write to [REDACTED:Person] today.", srcText, "the source is unchanged")
	findings := check.Findings(tool.NewBlockView(block))
	require.Len(t, findings, 1)
	assert.Equal(t, "sed gave back the redaction placeholder [REDACTED:Email], which the source does not hold; the block is unchanged", findings[0].Message)
	assert.Equal(t, "added", findings[0].Metadata["kind"])
}

// A block with no redaction reads as SourceText reads it.
func TestExternalCommand_UnredactedBlockReadsItsText(t *testing.T) {
	t.Parallel()
	block := model.NewBlock("b1", "Write to Mr Bean today.")
	cmd := tools.NewExternalCommandTool(sourceCommand("sed", "s/Write/Mail/"))
	processPart(t, cmd, &model.Part{Type: model.PartBlock, Resource: block})
	assert.Equal(t, "Mail to Mr Bean today.", block.SourceText())
}
