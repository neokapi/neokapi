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
version: v1
name: app
ship_gate: { translated: 100, reviewed: 100 }
`)
	require.True(t, p.HasShipGates())
	rs, err := p.BuildShipGates()
	require.NoError(t, err)

	// A catch-all gate applies to any (collection, locale).
	g, ok := rs.Resolve("docs", "nb")
	require.True(t, ok)
	assert.Equal(t, gate.Gate{"translated": 100, "reviewed": 100}, g)
}

func TestShipGates_RuleList_MostSpecificWins(t *testing.T) {
	p := loadProject(t, `
version: v1
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
	assert.Equal(t, gate.Gate{"translated": 100, "reviewed": 50}, g)

	g, _ = rs.Resolve("legal", "nb")
	assert.Equal(t, gate.Gate{"signed-off": 100}, g, "2-axis rule wins")

	g, _ = rs.Resolve("ui", "ja")
	assert.Equal(t, gate.Gate{"translated": 100, "reviewed": 0}, g)

	g, _ = rs.Resolve("ui", "de")
	assert.Equal(t, gate.Gate{"translated": 100, "reviewed": 100}, g, "falls to default")
}

func TestShipGates_NamedRegistryReference(t *testing.T) {
	p := loadProject(t, `
version: v1
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
	assert.Equal(t, gate.Gate{"translated": 100, "reviewed": 0}, g, "name expands to registry gate")
	g, _ = rs.Resolve("docs", "ko")
	assert.Equal(t, gate.Gate{"translated": 100, "reviewed": 0}, g)
}

func TestShipGates_UnknownRegistryName(t *testing.T) {
	p := loadProject(t, `
version: v1
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
version: v1
name: app
ship_gate: { bogus: 100 }
`)
	_, err := p.BuildShipGates()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown state")
}

func TestShipGates_AbsentIsEmpty(t *testing.T) {
	p := loadProject(t, `
version: v1
name: app
`)
	assert.False(t, p.HasShipGates())
	rs, err := p.BuildShipGates()
	require.NoError(t, err)
	_, ok := rs.Resolve("docs", "nb")
	assert.False(t, ok, "no gate configured")
}

func TestShipGates_RoundTrip(t *testing.T) {
	src := `version: v1
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
	assert.Equal(t, gate.Gate{"translated": 100, "reviewed": 0}, g)
}
