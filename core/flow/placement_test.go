package flow_test

import (
	"testing"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// placementReg builds a registry with the built-in and AI tools — the same
// population the CLI gates validate against.
func placementReg(t *testing.T) *registry.ToolRegistry {
	t.Helper()
	reg := registry.NewToolRegistry()
	tools.RegisterAll(reg)
	aitools.RegisterAll(reg)
	return reg
}

// chain builds a sequential flow definition over the named tool nodes.
func chain(nodes ...flow.FlowNode) flow.FlowDefinition {
	def := flow.FlowDefinition{ID: "f", Name: "f", Nodes: nodes}
	for i := 1; i < len(nodes); i++ {
		def.Edges = append(def.Edges, flow.FlowEdge{
			ID:     "e" + nodes[i].ID,
			Source: nodes[i-1].ID,
			Target: nodes[i].ID,
		})
	}
	return def
}

func toolNode(id, name string, config map[string]any) flow.FlowNode {
	return flow.FlowNode{ID: id, Type: flow.NodeTool, Name: name, Config: config}
}

func errorRules(diags []flow.PlacementDiagnostic) []string {
	var rules []string
	for _, d := range diags {
		if d.Severity == flow.PlacementError {
			rules = append(rules, d.Rule)
		}
	}
	return rules
}

func warningRules(diags []flow.PlacementDiagnostic) []string {
	var rules []string
	for _, d := range diags {
		if d.Severity == flow.PlacementWarning {
			rules = append(rules, d.Rule)
		}
	}
	return rules
}

// A transformer after a target-producing step orphans the targets → error.
func TestValidatePlacement_TransformerAfterTranslate(t *testing.T) {
	t.Parallel()
	reg := placementReg(t)
	def := chain(
		toolNode("p", "pseudo-translate", nil),
		toolNode("c", "case-transform", nil),
	)
	diags, err := def.ValidatePlacement(reg)
	require.NoError(t, err)
	assert.Contains(t, errorRules(diags), flow.RuleTransformerAfterTarget)
	require.Error(t, def.CheckPlacement(reg))

	// The reverse order is fine.
	ok := chain(
		toolNode("c", "case-transform", nil),
		toolNode("p", "pseudo-translate", nil),
	)
	assert.NoError(t, ok.CheckPlacement(reg))
}

// unredact is a transformer that must follow translation (it restores
// originals into the translated targets). It produces the target port itself,
// so the transformer-after-target rule exempts it.
func TestValidatePlacement_UnredactAfterTranslateAllowed(t *testing.T) {
	t.Parallel()
	reg := placementReg(t)
	def := chain(
		toolNode("r", "redact", nil),
		toolNode("t", "translate", nil),
		toolNode("u", "unredact", nil),
	)
	assert.NoError(t, def.CheckPlacement(reg))
}

// A redacting (recoverable) transformer after a remote-egress step leaks
// unprotected source → error. redact before the egress is the valid order.
func TestValidatePlacement_RedactAfterRemoteEgress(t *testing.T) {
	t.Parallel()
	reg := placementReg(t)

	// translate egresses source; redact placed after it is both an orphaned-
	// target error and a remote-egress error.
	def := chain(
		toolNode("t", "translate", nil),
		toolNode("r", "redact", nil),
	)
	diags, err := def.ValidatePlacement(reg)
	require.NoError(t, err)
	assert.Contains(t, errorRules(diags), flow.RuleTransformerAfterEgress)

	// A rules-only redact after a remote NER step it does not consume from is
	// an avoidable leak → error. (Entity-driven redaction is the exemption,
	// tested below.)
	rulesAfterNER := chain(
		toolNode("n", "entity-extract", nil),
		toolNode("r", "redact", map[string]any{"detectors": []string{"rules"}}),
	)
	diags, err = rulesAfterNER.ValidatePlacement(reg)
	require.NoError(t, err)
	assert.Contains(t, errorRules(diags), flow.RuleTransformerAfterEgress)
}

// Entity-driven redaction consumes the entity overlay the NER step produces —
// the documented AD-020 detection trade-off — so the egress rule exempts the
// producer feeding it.
func TestValidatePlacement_EntityRedactionExemptsItsProducer(t *testing.T) {
	t.Parallel()
	reg := placementReg(t)
	def := chain(
		toolNode("n", "entity-extract", nil),
		toolNode("r", "redact", map[string]any{
			"detectors":   []string{"entities"},
			"entityTypes": []string{"person"},
		}),
	)
	assert.NoError(t, def.CheckPlacement(reg))
}

// A local provider (ollama, demo) keeps content on the machine: the AI tool's
// contract resolver strips the remote-egress effect, so even a rules-only
// redact after it passes.
func TestValidatePlacement_LocalProviderDoesNotTrip(t *testing.T) {
	t.Parallel()
	reg := placementReg(t)
	for _, provider := range []string{"ollama", "demo"} {
		def := chain(
			toolNode("n", "entity-extract", map[string]any{"provider": provider}),
			toolNode("r", "redact", map[string]any{"detectors": []string{"rules"}}),
		)
		require.NoError(t, def.CheckPlacement(reg), "provider %s is local", provider)
	}

	// A cloud provider named explicitly still trips the rule.
	cloud := chain(
		toolNode("n", "entity-extract", map[string]any{"provider": "anthropic"}),
		toolNode("r", "redact", map[string]any{"detectors": []string{"rules"}}),
	)
	require.Error(t, cloud.CheckPlacement(reg))

	// The on-device NER engine calls no provider at all: nothing leaves the
	// machine, so even a rules-only redact after it passes.
	onDevice := chain(
		toolNode("n", "entity-extract", map[string]any{"engine": "ner"}),
		toolNode("r", "redact", map[string]any{"detectors": []string{"rules"}}),
	)
	require.NoError(t, onDevice.CheckPlacement(reg))
}

// A local terms/content memory step (no remote egress declared) before redact never
// trips the egress rule.
func TestValidatePlacement_LocalAnnotatorBeforeRedactAllowed(t *testing.T) {
	t.Parallel()
	reg := placementReg(t)
	def := chain(
		toolNode("w", "word-count", nil),
		toolNode("r", "redact", nil),
	)
	assert.NoError(t, def.CheckPlacement(reg))
}

// A transformer placed later than its earliest valid slot forces overlays
// produced in between to be rebased → warning (never an error).
func TestValidatePlacement_LatePlacementWarns(t *testing.T) {
	t.Parallel()
	reg := placementReg(t)
	def := chain(
		toolNode("s", "segmentation", nil),
		toolNode("c", "case-transform", nil),
	)
	diags, err := def.ValidatePlacement(reg)
	require.NoError(t, err)
	assert.Contains(t, warningRules(diags), flow.RuleTransformerLate)
	assert.Empty(t, errorRules(diags))
	require.NoError(t, def.CheckPlacement(reg), "warnings do not block the gate")

	// The transformer placed first produces no warning.
	early := chain(
		toolNode("c", "case-transform", nil),
		toolNode("s", "segmentation", nil),
	)
	diags, err = early.ValidatePlacement(reg)
	require.NoError(t, err)
	assert.Empty(t, diags)
}

// Under a declared redaction policy, a flow that egresses source to a remote
// sink with no redact step at all is rejected — the gap ValidatePlacement can't
// see, because it only orders a redact step the flow already lists.
func TestCheckRedactionCoverage(t *testing.T) {
	t.Parallel()
	reg := placementReg(t)

	// translate egresses source; with a policy and no redact step, that leaks.
	remoteNoRedact := chain(toolNode("t", "translate", nil))
	require.Error(t, remoteNoRedact.CheckRedactionCoverage(reg, true),
		"remote egress with no redact under a policy must fail closed")
	require.NoError(t, remoteNoRedact.CheckRedactionCoverage(reg, false),
		"no policy declared → nothing to enforce")

	// A redact step ahead of the egress satisfies the policy; ordering is
	// ValidatePlacement's job and passes here too.
	redactThenTranslate := chain(
		toolNode("r", "redact", nil),
		toolNode("t", "translate", nil),
	)
	require.NoError(t, redactThenTranslate.CheckRedactionCoverage(reg, true))
	require.NoError(t, redactThenTranslate.CheckPlacement(reg))

	// Nothing egresses source remotely → a policy needs no redact step.
	localOnly := chain(toolNode("c", "case-transform", nil))
	require.NoError(t, localOnly.CheckRedactionCoverage(reg, true))

	// Entity-driven redaction (AD-020): the remote NER precedes redact, which is
	// allowed. Coverage sees a redact step and does not re-decide the ordering.
	entityDriven := chain(
		toolNode("n", "entity-extract", nil),
		toolNode("r", "redact", map[string]any{
			"detectors":   []string{"entities"},
			"entityTypes": []string{"person"},
		}),
		toolNode("t", "translate", nil),
	)
	require.NoError(t, entityDriven.CheckRedactionCoverage(reg, true))
}

// Unknown (plugin) tools without loaded contracts are skipped, and a nil
// registry disables the pass.
func TestValidatePlacement_UnknownToolsAndNilRegistry(t *testing.T) {
	t.Parallel()
	reg := placementReg(t)
	def := chain(
		toolNode("x", "some-plugin-tool", nil),
		toolNode("r", "redact", nil),
	)
	require.NoError(t, def.CheckPlacement(reg))

	diags, err := def.ValidatePlacement(nil)
	require.NoError(t, err)
	assert.Empty(t, diags)
}
