package host

import (
	"context"
	"encoding/json"
	"maps"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dntArgs builds the argument object an agent would send to dnt-check. The
// target, when supplied, has translated the do-not-translate term, so a check
// that reads it must report a finding and a check that does not must not.
func dntArgs(t *testing.T, extra map[string]any) json.RawMessage {
	t.Helper()
	args := map[string]any{
		"text":        "Acme Cloud keeps your files safe.",
		"target_lang": "fr",
		"terms":       []string{"Acme Cloud"},
	}
	maps.Copy(args, extra)
	raw, err := json.Marshal(args)
	require.NoError(t, err)
	return raw
}

// TestTargetPolicyFor reads the policy off the tools as they are really
// registered, so a tool whose IO contract changes shows up here rather than in
// silence on the agent surface.
func TestTargetPolicyFor(t *testing.T) {
	app := &App{}
	app.InitRegistries()

	cases := []struct {
		tool string
		want targetPolicy
		why  string
	}{
		{"dnt-check", targetRequired, "consumes the target port"},
		{"term-check", targetRequired, "consumes the target port"},
		{"placeholder-check", targetRequired, "consumes the target port"},
		{"qa", targetRequired, "consumes the target port"},
		{"translate", targetWithheld, "produces the target itself"},
		{"pseudo-translate", targetWithheld, "produces the target itself"},
		{"recycle", targetWithheld, "produces the target itself"},
		{"create-target", targetWithheld, "writes the target container itself"},
		{"remove-target", targetAccepted, "bilingual, acts on an existing target"},
		{"review", targetAccepted, "bilingual, reads the target, declares no port"},
		{"whitespace-correct", targetAccepted, "bilingual, rewrites the target"},
		{"xml-validation", targetWithheld, "monolingual"},
		{"search-replace", targetWithheld, "monolingual"},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			info := app.ToolReg.ToolInfo(registry.ToolID(tc.tool))
			require.NotNil(t, info, "tool is registered")
			assert.Equal(t, tc.want, targetPolicyFor(*info), tc.why)
		})
	}
}

// TestFrameworkToolInputSchema_Target covers the input half of #1490: a
// bilingual tool that reads a target must offer somewhere to put one, and a
// tool that declares it consumes the target must demand it.
func TestFrameworkToolInputSchema_Target(t *testing.T) {
	base := &schema.ComponentSchema{Type: "object"}

	t.Run("withheld", func(t *testing.T) {
		props, required := projectSchema(t, base, targetWithheld)
		assert.NotContains(t, props, "target")
		assert.Equal(t, []any{"text"}, required)
	})

	t.Run("accepted", func(t *testing.T) {
		props, required := projectSchema(t, base, targetAccepted)
		assert.Contains(t, props, "target")
		assert.Equal(t, []any{"text"}, required, "offered, not demanded")
	})

	t.Run("required", func(t *testing.T) {
		props, required := projectSchema(t, base, targetRequired)
		assert.Contains(t, props, "target")
		assert.Equal(t, []any{"text", "target"}, required)
	})
}

// annotationsJSON flattens the block annotations a check writes into one string
// to assert over.
func annotationsJSON(out *frameworkToolOutput) string {
	var b strings.Builder
	for _, raw := range out.Annotations {
		b.Write(raw)
	}
	return b.String()
}

func projectSchema(t *testing.T, s *schema.ComponentSchema, policy targetPolicy) (map[string]any, []any) {
	t.Helper()
	raw, err := frameworkToolInputSchema(s, policy)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	props, _ := got["properties"].(map[string]any)
	required, _ := got["required"].([]any)
	return props, required
}

// TestFrameworkMCPHandler_BilingualToolReadsTarget is the regression test for
// #1490: a bilingual check given a target must actually inspect it. Before the
// fix the harness built a source-only block, so this returned a clean result
// over a target that had translated the protected term.
func TestFrameworkMCPHandler_BilingualToolReadsTarget(t *testing.T) {
	app := &App{}
	app.InitRegistries()

	handler := app.frameworkMCPHandler("dnt-check", "", targetRequired)
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{
		Arguments: dntArgs(t, map[string]any{"target": "Le Nuage Acme garde vos fichiers en securite."}),
	}}

	res, err := handler(context.Background(), req)
	require.NoError(t, err)
	out, ok := res.StructuredContent.(*frameworkToolOutput)
	require.True(t, ok)

	require.Contains(t, out.Targets, "fr", "the supplied target round-trips into the result")
	require.NotEmpty(t, out.Annotations, "dnt-check reported a finding over the supplied target")
	assert.Contains(t, annotationsJSON(out), "do-not-translate")
}

// TestFrameworkMCPHandler_BilingualToolPassesCleanTarget is the other side of
// the same coin: a target that keeps the term must not be reported. Without it
// the test above would also pass on a check that flags everything.
func TestFrameworkMCPHandler_BilingualToolPassesCleanTarget(t *testing.T) {
	app := &App{}
	app.InitRegistries()

	handler := app.frameworkMCPHandler("dnt-check", "", targetRequired)
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{
		Arguments: dntArgs(t, map[string]any{"target": "Acme Cloud garde vos fichiers en securite."}),
	}}

	res, err := handler(context.Background(), req)
	require.NoError(t, err)
	out := res.StructuredContent.(*frameworkToolOutput)
	require.Contains(t, out.Targets, "fr")
	assert.NotContains(t, annotationsJSON(out), "do-not-translate", "the term survived, so nothing to report")
}

// TestFrameworkMCPHandler_BilingualToolRequiresTarget covers the other half of
// #1490: with no target the call must fail naming what is missing, rather than
// succeeding with an empty result that reads as a clean bill of health.
func TestFrameworkMCPHandler_BilingualToolRequiresTarget(t *testing.T) {
	app := &App{}
	app.InitRegistries()

	handler := app.frameworkMCPHandler("dnt-check", "", targetRequired)
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: dntArgs(t, nil)}}

	_, err := handler(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target")
}

// TestFrameworkMCPHandler_TargetNeedsLocale refuses a target with no locale to
// file it under, which would otherwise be stored where the tool never looks.
func TestFrameworkMCPHandler_TargetNeedsLocale(t *testing.T) {
	app := &App{}
	app.InitRegistries()

	handler := app.frameworkMCPHandler("dnt-check", "", targetRequired)
	args, err := json.Marshal(map[string]any{
		"text":   "Acme Cloud keeps your files safe.",
		"target": "Le Nuage Acme garde vos fichiers.",
		"terms":  []string{"Acme Cloud"},
	})
	require.NoError(t, err)
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: args}}

	_, err = handler(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target_lang")
}

// TestFrameworkMCPHandler_TargetIsNotToolConfig keeps the new argument out of
// the config map handed to the tool factory.
func TestFrameworkMCPHandler_TargetIsNotToolConfig(t *testing.T) {
	app := &App{}
	app.InitRegistries()

	handler := app.frameworkMCPHandler("term-check", "", targetRequired)
	args, err := json.Marshal(map[string]any{
		"text":        "Open the dashboard.",
		"target":      "Ouvrez le tableau de bord.",
		"target_lang": "fr",
	})
	require.NoError(t, err)
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: args}}

	res, err := handler(context.Background(), req)
	require.NoError(t, err, "target reached the block, not the tool config")
	out := res.StructuredContent.(*frameworkToolOutput)
	assert.Equal(t, "term-check", out.Tool)
}
