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

// resolveGateGates turns a recipe gate configuration — a when/gate rule list or
// a single catch-all gate — into a validated gate.RuleSet over the target
// lifecycle ladder, expanding any registry-name reference against p.Gates.
// It is the shared resolver behind the ship and verified gates. Precedence:
//   - the rule list, if present;
//   - else the catch-all gate, if present;
//   - else an empty RuleSet (no gate configured — nothing matched).
//
// label names the field in error messages ("ship_gates" / "verified_gates").
func (p *KapiProject) resolveGateGates(catchAll gate.Gate, ruleList []ShipGateRule, label string) (gate.RuleSet, error) {
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
	case len(ruleList) > 0:
		for i, r := range ruleList {
			g, err := resolve(r.Gate)
			if err != nil {
				return gate.RuleSet{}, fmt.Errorf("%s[%d]: %w", label, i, err)
			}
			var sel gate.Selector
			if r.When != nil {
				sel = *r.When
			}
			rules = append(rules, gate.Rule{When: sel, Gate: g})
		}
	case len(catchAll) > 0:
		rules = append(rules, gate.Rule{Gate: catchAll})
	}

	rs := gate.RuleSet{Rules: rules}
	if err := rs.Validate(gate.TargetLadder()); err != nil {
		return gate.RuleSet{}, err
	}
	return rs, nil
}

// BuildShipGates resolves the recipe's ship-gate configuration into an
// evaluatable gate.RuleSet, expanding registry references and validating every
// gate against the target lifecycle ladder. Resolution precedence:
//   - ShipGates (the rule list), if present;
//   - else ShipGate (a single catch-all gate), if present;
//   - else an empty RuleSet (no gate configured — nothing is gated).
func (p *KapiProject) BuildShipGates() (gate.RuleSet, error) {
	return p.resolveGateGates(p.ShipGate, p.ShipGates, "ship_gates")
}

// HasShipGates reports whether the recipe configures any ship gate.
func (p *KapiProject) HasShipGates() bool {
	return len(p.ShipGates) > 0 || len(p.ShipGate) > 0
}

// BuildVerifiedGates resolves the recipe's verified-gate configuration into an
// evaluatable gate.RuleSet, mirroring BuildShipGates exactly (same precedence,
// same registry resolution, same target-ladder validation) but over the
// verified_gate/verified_gates fields. An absent configuration yields an empty
// RuleSet — no locale resolves a verified gate, so nothing is verified. This is
// the deliberate default: a project declares the human-verified bar to opt in.
func (p *KapiProject) BuildVerifiedGates() (gate.RuleSet, error) {
	return p.resolveGateGates(p.VerifiedGate, p.VerifiedGates, "verified_gates")
}

// HasVerifiedGates reports whether the recipe configures any verified gate.
func (p *KapiProject) HasVerifiedGates() bool {
	return len(p.VerifiedGates) > 0 || len(p.VerifiedGate) > 0
}

// BuildSourceGate validates and returns the recipe's source-readiness gate (a
// single gate over the source authoring ladder), or an empty gate when none is
// configured. It is the source-side counterpart of BuildShipGates.
func (p *KapiProject) BuildSourceGate() (gate.Gate, error) {
	if len(p.SourceGate) == 0 {
		return gate.Gate{}, nil
	}
	if err := p.SourceGate.Validate(gate.SourceLadder()); err != nil {
		return nil, fmt.Errorf("source_gate: %w", err)
	}
	return p.SourceGate, nil
}

// HasSourceGate reports whether the recipe configures a source-readiness gate.
func (p *KapiProject) HasSourceGate() bool { return len(p.SourceGate) > 0 }
