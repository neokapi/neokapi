package flow_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// A step that sends source out after unredact restored the withheld values
// sends the originals, so it needs a redact step between the two.
func TestValidatePlacement_EgressAfterRestoreNeedsARedactStep(t *testing.T) {
	t.Parallel()
	reg := placementReg(t)
	cmd := map[string]any{"command": "cat", "applySource": true}

	afterRestore := chain(
		toolNode("r", "redact", nil),
		toolNode("x", "external-command", cmd),
		toolNode("u", "unredact", nil),
		toolNode("t", "translate", nil),
	)
	diags, err := afterRestore.ValidatePlacement(reg)
	require.NoError(t, err)
	assert.Equal(t, []string{flow.RuleEgressAfterRestore}, errorRules(diags))
	require.ErrorContains(t, afterRestore.CheckPlacement(reg),
		`"translate" sends source out of kapi after "unredact" restored the values redaction withholds, with no redact step between them`)

	execAfterRestore := chain(
		toolNode("r", "redact", nil),
		toolNode("u", "unredact", nil),
		toolNode("x", "external-command", cmd),
	)
	diags, err = execAfterRestore.ValidatePlacement(reg)
	require.NoError(t, err)
	assert.Contains(t, errorRules(diags), flow.RuleEgressAfterRestore, "a program after unredact reads the originals too")

	redactedAgain := chain(
		toolNode("r", "redact", nil),
		toolNode("x", "external-command", cmd),
		toolNode("u", "unredact", nil),
		toolNode("r2", "redact", nil),
		toolNode("t", "translate", nil),
		toolNode("u2", "unredact", nil),
	)
	diags, err = redactedAgain.ValidatePlacement(reg)
	require.NoError(t, err)
	assert.NotContains(t, errorRules(diags), flow.RuleEgressAfterRestore, "a redact step between them withholds the values again")

	localAfter := chain(
		toolNode("r", "redact", nil),
		toolNode("t", "translate", nil),
		toolNode("u", "unredact", nil),
		toolNode("c", "case-transform", nil),
	)
	diags, err = localAfter.ValidatePlacement(reg)
	require.NoError(t, err)
	assert.NotContains(t, errorRules(diags), flow.RuleEgressAfterRestore, "a step that keeps source in kapi may follow unredact")
}

// cloudRewriteReg is placementReg with a transformer that rewrites source by
// sending it to a remote service (remote-source-egress).
func cloudRewriteReg(t *testing.T) *registry.ToolRegistry {
	t.Helper()
	reg := placementReg(t)
	reg.RegisterWithSchema("cloud-rewrite", func() tool.Tool {
		return &tool.BaseTool{ToolName: "cloud-rewrite"}
	}, &schema.ComponentSchema{ToolMeta: &schema.ToolMeta{
		ID:          "cloud-rewrite",
		DisplayName: "Cloud Rewrite",
		Produces:    []schema.IOPort{schema.Port(schema.PortSource, model.SideSource)},
		SideEffects: []schema.SideEffect{schema.SideEffectRemoteSourceEgress},
	}})
	return reg
}

// A transformer that sends source to a remote sink belongs after a redact
// step, as a program kapi runs does, so the redact step ahead of it raises no
// late-placement warning; an overlay produced between them still does.
func TestValidatePlacement_RedactAheadOfARemoteTransformerIsExpected(t *testing.T) {
	t.Parallel()
	reg := cloudRewriteReg(t)

	redactFirst := chain(
		toolNode("r", "redact", nil),
		toolNode("c", "cloud-rewrite", nil),
		toolNode("u", "unredact", nil),
	)
	diags, err := redactFirst.ValidatePlacement(reg)
	require.NoError(t, err)
	assert.Empty(t, diags)

	between := chain(
		toolNode("r", "redact", nil),
		toolNode("s", "segmentation", nil),
		toolNode("c", "cloud-rewrite", nil),
	)
	diags, err = between.ValidatePlacement(reg)
	require.NoError(t, err)
	assert.Equal(t, []string{flow.RuleTransformerLate}, warningRules(diags))
}
