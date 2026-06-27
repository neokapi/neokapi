package project

import (
	"fmt"

	"github.com/neokapi/neokapi/core/gate"
	"gopkg.in/yaml.v3"
)

// ShipGateRule is one recipe `ship_gates` entry: a selector (`when:`) plus the
// gate that applies where it matches (`gate:`). A bare entry with no `when:` is
// the catch-all default.
type ShipGateRule struct {
	When *gate.Selector `yaml:"when,omitempty" json:"when,omitempty"`
	Gate GateRef        `yaml:"gate" json:"gate"`
}

// GateRef is a rule's gate value: either an inline threshold map
// (`gate: {translated: 100}`) or a name into the `gates:` registry
// (`gate: machine`).
type GateRef struct {
	Name   string    // set when the YAML value is a scalar (a registry name)
	Inline gate.Gate // set when the YAML value is a map (inline thresholds)
}

// UnmarshalYAML accepts a scalar (registry name) or a mapping (inline gate).
func (r *GateRef) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		r.Name = node.Value
		return nil
	case yaml.MappingNode:
		var m gate.Gate
		if err := node.Decode(&m); err != nil {
			return fmt.Errorf("ship gate: %w", err)
		}
		r.Inline = m
		return nil
	default:
		return fmt.Errorf("ship gate: expected a name or a {state: percent} map, got %v", node.Kind)
	}
}

// MarshalYAML re-encodes a GateRef as either its name or its inline map, so a
// recipe round-trips verbatim.
func (r GateRef) MarshalYAML() (any, error) {
	if r.Name != "" {
		return r.Name, nil
	}
	return r.Inline, nil
}

// BuildShipGates resolves the recipe's ship-gate configuration into an
// evaluatable gate.RuleSet, expanding registry references and validating every
// gate against the target lifecycle ladder. Resolution precedence:
//   - ShipGates (the rule list), if present;
//   - else ShipGate (a single catch-all gate), if present;
//   - else an empty RuleSet (no gate configured — nothing is gated).
func (p *KapiProject) BuildShipGates() (gate.RuleSet, error) {
	resolve := func(ref GateRef) (gate.Gate, error) {
		if ref.Name != "" {
			g, ok := p.Gates[ref.Name]
			if !ok {
				return nil, fmt.Errorf("references unknown registry gate %q", ref.Name)
			}
			return g, nil
		}
		return ref.Inline, nil
	}

	var rules []gate.Rule
	switch {
	case len(p.ShipGates) > 0:
		for i, r := range p.ShipGates {
			g, err := resolve(r.Gate)
			if err != nil {
				return gate.RuleSet{}, fmt.Errorf("ship_gates[%d]: %w", i, err)
			}
			var sel gate.Selector
			if r.When != nil {
				sel = *r.When
			}
			rules = append(rules, gate.Rule{When: sel, Gate: g})
		}
	case len(p.ShipGate) > 0:
		rules = append(rules, gate.Rule{Gate: p.ShipGate})
	}

	rs := gate.RuleSet{Rules: rules}
	if err := rs.Validate(gate.TargetLadder()); err != nil {
		return gate.RuleSet{}, err
	}
	return rs, nil
}

// HasShipGates reports whether the recipe configures any ship gate.
func (p *KapiProject) HasShipGates() bool {
	return len(p.ShipGates) > 0 || len(p.ShipGate) > 0
}
