package flow

import (
	"errors"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
)

// Transformer placement validation (AD-006). Transformers are ordinary ordered
// steps — there is no structural source-transform stage — so ordering safety is
// a validation pass that runs beside the data-flow contract (ValidateDataFlow)
// at build/load, using the Capability and SideEffects each tool already
// declares (config-refined through the registry's contract resolvers, so
// plugins and config-dependent tools participate).
//
// Rules:
//
//   - Error: a transformer must not follow a step that produces a committed
//     target — rewriting source orphans the targets, which anchor to it. A
//     transformer that itself produces the target port (unredact rewrites both
//     sides coherently) is exempt.
//
//   - Error: a recoverable transformer (redact) must run before any step that
//     egresses source to a remote sink — otherwise unprotected source leaks
//     before redaction applies. The step(s) producing the inputs its detection
//     consumes are exempt: a cloud NER feeding entity-driven redaction is the
//     documented AD-020 detection trade-off, while a local detector never
//     carries the egress effect in the first place.
//
//   - Warning: a transformer placed later than its earliest valid slot (right
//     after the last step producing a port it consumes) forces every overlay
//     produced in between to be rebased across its rewrite; an earlier slot
//     avoids the work and the precision loss.

// PlacementSeverity grades a placement diagnostic.
type PlacementSeverity string

const (
	PlacementError   PlacementSeverity = "error"
	PlacementWarning PlacementSeverity = "warning"
)

// Placement rule identifiers, stable for programmatic handling (CLI output,
// flow-editor rendering).
const (
	RuleTransformerAfterTarget = "transformer-after-target"
	RuleTransformerAfterEgress = "transformer-after-remote-egress"
	RuleTransformerLate        = "transformer-late-placement"
)

// PlacementDiagnostic is one finding from the placement pass.
type PlacementDiagnostic struct {
	Severity PlacementSeverity `json:"severity"`
	Rule     string            `json:"rule"`
	NodeID   string            `json:"nodeId"`
	Tool     string            `json:"tool"`
	Message  string            `json:"message"`
}

// ValidatePlacement runs the transformer placement pass over the flow's
// ordered tool nodes, returning every diagnostic. Tools unknown to the
// registry are skipped (their contract is not loaded); a nil registry
// disables the pass. Callers that gate a build/load reject when any
// error-severity diagnostic is present (see CheckPlacement).
func (d *FlowDefinition) ValidatePlacement(reg *registry.ToolRegistry) ([]PlacementDiagnostic, error) {
	if reg == nil {
		return nil, nil
	}
	ordered, err := d.toolNodeRefs()
	if err != nil {
		return nil, err
	}

	// Resolve each node's config-refined contract once.
	infos := make([]*registry.ToolInfo, len(ordered))
	for i, n := range ordered {
		infos[i] = reg.ResolveToolInfo(registry.ToolID(n.Name), n.Config)
	}

	var diags []PlacementDiagnostic
	for i, n := range ordered {
		info := infos[i]
		if info == nil || !isTransformer(info) {
			continue
		}

		consumed := consumedPortKeys(info, true)  // incl. optional — anchors the warning
		required := consumedPortKeys(info, false) // required only — the egress exemption
		producesTarget := producesPort(info, schema.PortTarget, model.SideTarget)

		lastInputIdx := -1 // index of the last upstream step producing a consumed port
		for j := range i {
			up := infos[j]
			if up == nil {
				continue
			}
			if feedsAny(up, consumed) {
				lastInputIdx = j
			}

			if !producesTarget && producesPort(up, schema.PortTarget, model.SideTarget) {
				diags = append(diags, PlacementDiagnostic{
					Severity: PlacementError,
					Rule:     RuleTransformerAfterTarget,
					NodeID:   n.ID,
					Tool:     n.Name,
					Message: fmt.Sprintf("transformer %q follows %q, which produces targets: rewriting source orphans the targets that anchor to it — move the transformer before any target-producing step",
						n.Name, ordered[j].Name),
				})
			}

			// The exemption counts only ports the transformer's config-resolved
			// contract REQUIRES: an optional consume (rules-only redact's entity
			// port) does not justify egressing source before redaction.
			if info.Recoverable && hasSideEffect(up, schema.SideEffectRemoteSourceEgress) && !feedsAny(up, required) {
				diags = append(diags, PlacementDiagnostic{
					Severity: PlacementError,
					Rule:     RuleTransformerAfterEgress,
					NodeID:   n.ID,
					Tool:     n.Name,
					Message: fmt.Sprintf("%q must run before %q, which sends source to a remote sink: unprotected content leaks before redaction applies — move %q earlier, or use a local provider for %q",
						n.Name, ordered[j].Name, n.Name, ordered[j].Name),
				})
			}
		}

		// Warning: overlays produced between the transformer's earliest valid
		// slot and its position must all be rebased across the rewrite.
		var rebased []string
		for j := lastInputIdx + 1; j < i; j++ {
			up := infos[j]
			if up == nil {
				continue
			}
			for _, p := range up.Produces {
				if p.Side == model.SideSource && !isPseudoPort(p.Type) && !consumed[ioKey(p)] {
					rebased = append(rebased, fmt.Sprintf("%s (from %q)", p.Type, ordered[j].Name))
				}
			}
		}
		if len(rebased) > 0 {
			diags = append(diags, PlacementDiagnostic{
				Severity: PlacementWarning,
				Rule:     RuleTransformerLate,
				NodeID:   n.ID,
				Tool:     n.Name,
				Message: fmt.Sprintf("transformer %q is placed later than needed: the %s overlay(s) produced before it must be rebased across its rewrite — move it right after its last required input",
					n.Name, strings.Join(rebased, ", ")),
			})
		}
	}
	return diags, nil
}

// CheckPlacement runs ValidatePlacement and returns an error joining every
// error-severity diagnostic — the unconditional build/load gate beside
// ValidateDataFlow. Warnings are not part of the error; callers that surface
// them use ValidatePlacement directly.
func (d *FlowDefinition) CheckPlacement(reg *registry.ToolRegistry) error {
	diags, err := d.ValidatePlacement(reg)
	if err != nil {
		return err
	}
	var errs []error
	for _, diag := range diags {
		if diag.Severity == PlacementError {
			errs = append(errs, fmt.Errorf("flow %q: %s", d.Name, diag.Message))
		}
	}
	return errors.Join(errs...)
}

// isTransformer reports whether a tool may rewrite source: the capability
// probe for local tools, or a declared source pseudo-port production for
// metadata-only (plugin) tools whose handler cannot be probed.
func isTransformer(info *registry.ToolInfo) bool {
	return info.IsSourceTransform || producesPort(info, schema.PortSource, model.SideSource)
}

// consumedPortKeys returns the set of port keys a tool consumes. With
// includeOptional, optional consumes count too (they justify a producer
// preceding the transformer for the late-placement warning); without, only
// hard requirements count (the remote-egress exemption).
func consumedPortKeys(info *registry.ToolInfo, includeOptional bool) map[string]bool {
	keys := make(map[string]bool, len(info.Consumes))
	for _, c := range info.Consumes {
		if c.Optional && !includeOptional {
			continue
		}
		keys[ioKey(c)] = true
	}
	return keys
}

// feedsAny reports whether the upstream tool produces any of the consumed ports.
func feedsAny(up *registry.ToolInfo, consumed map[string]bool) bool {
	for _, p := range up.Produces {
		if consumed[ioKey(p)] {
			return true
		}
	}
	return false
}

func producesPort(info *registry.ToolInfo, portType string, side model.Side) bool {
	for _, p := range info.Produces {
		if p.Type == portType && p.Side == side {
			return true
		}
	}
	return false
}

func hasSideEffect(info *registry.ToolInfo, effect schema.SideEffect) bool {
	for _, e := range info.SideEffects {
		if e == effect {
			return true
		}
	}
	return false
}

// isPseudoPort reports whether a port type names a pseudo-port (target/source)
// rather than a stand-off layer; pseudo-ports are not run-anchored, so they
// are not "rebased" by a transformer's rewrite.
func isPseudoPort(t string) bool {
	return t == schema.PortTarget || t == schema.PortSource
}
