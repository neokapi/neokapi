package project

import (
	"testing"

	"github.com/neokapi/neokapi/core/gate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func loadProject(t *testing.T, src string) *KapiProject {
	t.Helper()
	var p KapiProject
	require.NoError(t, yaml.Unmarshal([]byte(src), &p))
	return &p
}

func TestShipGate_SingleMap(t *testing.T) {
	p := loadProject(t, `
version: v2
name: app
ship_gate: { translated: 100, reviewed: 100 }
`)
	require.True(t, p.HasShipGates())
	rs, err := p.BuildShipGates()
	require.NoError(t, err)

	// A catch-all gate applies to any (collection, locale).
	g, ok := rs.Resolve("docs", "nb")
	require.True(t, ok)
	assert.Equal(t, gate.Gate{"translated": {Pct: 100}, "reviewed": {Pct: 100}}, g)
}

func TestShipGates_RuleList_MostSpecificWins(t *testing.T) {
	p := loadProject(t, `
version: v2
name: app
ship_gates:
  - when: { collections: [docs] }
    gate: { translated: 100, reviewed: 50 }
  - when: { locales: [ja] }
    gate: { translated: 100, reviewed: 0 }
  - when: { collections: [legal], locales: [nb] }
    gate: { signed-off: 100 }
  - gate: { translated: 100, reviewed: 100 }
`)
	rs, err := p.BuildShipGates()
	require.NoError(t, err)

	g, _ := rs.Resolve("docs", "nb")
	assert.Equal(t, gate.Gate{"translated": {Pct: 100}, "reviewed": {Pct: 50}}, g)

	g, _ = rs.Resolve("legal", "nb")
	assert.Equal(t, gate.Gate{"signed-off": {Pct: 100}}, g, "2-axis rule wins")

	g, _ = rs.Resolve("ui", "ja")
	assert.Equal(t, gate.Gate{"translated": {Pct: 100}, "reviewed": {Pct: 0}}, g)

	g, _ = rs.Resolve("ui", "de")
	assert.Equal(t, gate.Gate{"translated": {Pct: 100}, "reviewed": {Pct: 100}}, g, "falls to default")
}

func TestShipGates_NamedRegistryReference(t *testing.T) {
	p := loadProject(t, `
version: v2
name: app
gates:
  machine: { translated: 100, reviewed: 0 }
ship_gates:
  - when: { locales: [ja, ko] }
    gate: machine
  - gate: { translated: 100, reviewed: 100 }
`)
	rs, err := p.BuildShipGates()
	require.NoError(t, err)

	g, _ := rs.Resolve("docs", "ja")
	assert.Equal(t, gate.Gate{"translated": {Pct: 100}, "reviewed": {Pct: 0}}, g, "name expands to registry gate")
	g, _ = rs.Resolve("docs", "ko")
	assert.Equal(t, gate.Gate{"translated": {Pct: 100}, "reviewed": {Pct: 0}}, g)
}

func TestShipGates_UnknownRegistryName(t *testing.T) {
	p := loadProject(t, `
version: v2
name: app
ship_gates:
  - gate: missing
`)
	_, err := p.BuildShipGates()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown registry gate")
}

func TestShipGates_InvalidStateRejected(t *testing.T) {
	p := loadProject(t, `
version: v2
name: app
ship_gate: { bogus: 100 }
`)
	_, err := p.BuildShipGates()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown state")
}

func TestShipGates_AbsentIsEmpty(t *testing.T) {
	p := loadProject(t, `
version: v2
name: app
`)
	assert.False(t, p.HasShipGates())
	rs, err := p.BuildShipGates()
	require.NoError(t, err)
	_, ok := rs.Resolve("docs", "nb")
	assert.False(t, ok, "no gate configured")
}

func TestShipGates_RoundTrip(t *testing.T) {
	src := `version: v2
name: app
gates:
    machine: {translated: 100, reviewed: 0}
ship_gates:
    - when: {locales: [ja]}
      gate: machine
    - gate: {translated: 100, reviewed: 100}
`
	p := loadProject(t, src)
	out, err := yaml.Marshal(p)
	require.NoError(t, err)
	// Re-load the round-tripped recipe and confirm the gates still resolve.
	var p2 KapiProject
	require.NoError(t, yaml.Unmarshal(out, &p2))
	rs, err := p2.BuildShipGates()
	require.NoError(t, err)
	g, ok := rs.Resolve("docs", "ja")
	require.True(t, ok)
	assert.Equal(t, gate.Gate{"translated": {Pct: 100}, "reviewed": {Pct: 0}}, g)
}

func TestShipGate_ApproverClassExtendedForm(t *testing.T) {
	p := loadProject(t, `
version: v2
name: app
gates:
  ship: { translated: 100, reviewed: { pct: 100, by: human } }
ship_gates:
  - gate: ship
  - when: { locales: [ja] }
    gate: { reviewed: { pct: 80, by: any } }
`)
	rs, err := p.BuildShipGates()
	require.NoError(t, err)

	g, ok := rs.Resolve("", "nb")
	require.True(t, ok)
	assert.Equal(t, gate.Gate{
		"translated": {Pct: 100},
		"reviewed":   {Pct: 100, By: gate.ByHuman},
	}, g, "registry gate carries the extended-form approver class")

	g, _ = rs.Resolve("", "ja")
	assert.Equal(t, gate.Gate{"reviewed": {Pct: 80, By: gate.ByAny}}, g)
}

func TestShipGate_UnknownApproverClassRejected(t *testing.T) {
	p := loadProject(t, `
version: v2
name: app
ship_gate: { reviewed: { pct: 100, by: robot } }
`)
	_, err := p.BuildShipGates()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "approver class")
}

// ── Verified gate: the second bar, resolved exactly like the ship gate ──

func TestVerifiedGate_SingleMap(t *testing.T) {
	p := loadProject(t, `
version: v2
name: app
verified_gate: { reviewed: 100 }
`)
	require.True(t, p.HasVerifiedGates())
	rs, err := p.BuildVerifiedGates()
	require.NoError(t, err)

	g, ok := rs.Resolve("docs", "nb")
	require.True(t, ok)
	assert.Equal(t, gate.Gate{"reviewed": {Pct: 100}}, g)
}

func TestVerifiedGates_RuleList_MostSpecificWins(t *testing.T) {
	p := loadProject(t, `
version: v2
name: app
verified_gates:
  - when: { locales: [ja] }
    gate: { signed-off: 100 }
  - gate: { reviewed: 100 }
`)
	rs, err := p.BuildVerifiedGates()
	require.NoError(t, err)

	g, _ := rs.Resolve("docs", "ja")
	assert.Equal(t, gate.Gate{"signed-off": {Pct: 100}}, g, "locale rule wins over the catch-all")

	g, _ = rs.Resolve("docs", "de")
	assert.Equal(t, gate.Gate{"reviewed": {Pct: 100}}, g, "falls to the catch-all default")
}

func TestVerifiedGates_NamedRegistryReference(t *testing.T) {
	p := loadProject(t, `
version: v2
name: app
gates:
  human: { reviewed: { pct: 100, by: human } }
verified_gates:
  - gate: human
`)
	rs, err := p.BuildVerifiedGates()
	require.NoError(t, err)
	g, ok := rs.Resolve("docs", "ja")
	require.True(t, ok)
	assert.Equal(t, gate.Gate{"reviewed": {Pct: 100, By: gate.ByHuman}}, g,
		"a verified gate resolves registry names against the shared gates: map")
}

func TestVerifiedGates_UnknownRegistryName(t *testing.T) {
	p := loadProject(t, `
version: v2
name: app
verified_gates:
  - gate: missing
`)
	_, err := p.BuildVerifiedGates()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown registry gate")
	assert.Contains(t, err.Error(), "verified_gates", "error names the verified field")
}

func TestVerifiedGates_InvalidStateRejected(t *testing.T) {
	p := loadProject(t, `
version: v2
name: app
verified_gate: { bogus: 100 }
`)
	_, err := p.BuildVerifiedGates()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown state")
}

func TestVerifiedGates_AbsentIsEmpty(t *testing.T) {
	// The default: no verified gate configured → nothing resolves a verified
	// gate → nothing is verified. Ship-gate resolution is untouched.
	p := loadProject(t, `
version: v2
name: app
ship_gate: { translated: 100 }
`)
	assert.False(t, p.HasVerifiedGates())
	require.True(t, p.HasShipGates(), "ship gate is independent of the verified gate")

	vs, err := p.BuildVerifiedGates()
	require.NoError(t, err)
	_, ok := vs.Resolve("docs", "nb")
	assert.False(t, ok, "no verified gate configured — nothing is verified")

	rs, err := p.BuildShipGates()
	require.NoError(t, err)
	_, ok = rs.Resolve("docs", "nb")
	assert.True(t, ok, "the ship gate still resolves")
}

func TestVerifiedGates_RoundTrip(t *testing.T) {
	src := `version: v2
name: app
verified_gates:
    - when: {locales: [ja]}
      gate: {signed-off: 100}
    - gate: {reviewed: 100}
`
	p := loadProject(t, src)
	out, err := yaml.Marshal(p)
	require.NoError(t, err)
	var p2 KapiProject
	require.NoError(t, yaml.Unmarshal(out, &p2))
	rs, err := p2.BuildVerifiedGates()
	require.NoError(t, err)
	g, ok := rs.Resolve("docs", "ja")
	require.True(t, ok)
	assert.Equal(t, gate.Gate{"signed-off": {Pct: 100}}, g)
}
