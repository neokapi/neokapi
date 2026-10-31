package flow

import (
	"errors"
	"fmt"
	"slices"
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
//     egresses source to a remote sink, or hands it to a program outside kapi
//     (process-exec); otherwise unprotected source leaks
//     before redaction applies. The step(s) producing the inputs its detection
//     consumes are exempt: a cloud NER feeding entity-driven redaction is the
//     documented AD-020 detection trade-off, while a local detector never
//     carries the egress effect in the first place.
//
//   - Error: a step that egresses source to a remote sink, or hands it to a
//     program outside kapi, must not follow a step that restores the withheld
//     values (unredact) unless a redact step runs between them; otherwise the
//     restored originals leave kapi.
//
//   - Warning: a transformer placed later than its earliest valid slot (right
//     after the last step producing a port it consumes) forces every overlay
//     produced in between to be rebased across its rewrite; an earlier slot
//     avoids the work and the precision loss. A transformer that sends source
//     out of kapi counts a redact step ahead of it as one of its inputs.

// PlacementSeverity grades a placement diagnostic.
type PlacementSeverity string

const (
	PlacementError   PlacementSeverity = "error"
	PlacementWarning PlacementSeverity = "warning"
)

// Placement rule identifiers, stable for programmatic handling (CLI output,
// flow-editor rendering).
const (
	RuleTransformerAfterTarget       = "transformer-after-target"
	RuleTransformerAfterEgress       = "transformer-after-remote-egress"
	RuleTransformerLate              = "transformer-late-placement"
	RuleRedactionMissingBeforeEgress = "redaction-missing-before-remote-egress"
	RuleEgressAfterRestore           = "egress-after-restore"
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

		// A transformer that sends source out of kapi belongs after a redact
		// step, so a recoverable step ahead of it counts as one of its inputs
		// and the overlays it produces are not reported as rebased.
		sendsOut := sendsSourceOut(info)

		lastInputIdx := -1 // index of the last upstream step producing a consumed port
		for j := range i {
			up := infos[j]
			if up == nil {
				continue
			}
			if feedsAny(up, consumed) || (sendsOut && up.Recoverable) {
				lastInputIdx = j
			}

			if !producesTarget && producesPort(up, schema.PortTarget, model.SideTarget) {
				diags = append(diags, PlacementDiagnostic{
					Severity: PlacementError,
					Rule:     RuleTransformerAfterTarget,
					NodeID:   n.ID,
					Tool:     n.Name,
					Message: fmt.Sprintf("transformer %q follows %q, which produces targets: rewriting source orphans the targets that anchor to it. Move the transformer before any target-producing step",
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
					Message: fmt.Sprintf("%q must run before %q, which sends source to a remote sink: unprotected content leaks before redaction applies. Move %q earlier, or use a local provider for %q",
						n.Name, ordered[j].Name, n.Name, ordered[j].Name),
				})
			} else if info.Recoverable && hasSideEffect(up, schema.SideEffectProcessExec) {
				// A program kapi runs feeds no port a transformer reads, so
				// no exemption applies.
				diags = append(diags, PlacementDiagnostic{
					Severity: PlacementError,
					Rule:     RuleTransformerAfterEgress,
					NodeID:   n.ID,
					Tool:     n.Name,
					Message: fmt.Sprintf("%q must run before %q, which hands source to a program outside kapi: unprotected content leaks before redaction applies. Move %q earlier",
						n.Name, ordered[j].Name, n.Name),
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
				Message: fmt.Sprintf("transformer %q is placed later than needed: the %s overlay(s) produced before it must be rebased across its rewrite. Move it right after its last required input",
					n.Name, strings.Join(rebased, ", ")),
			})
		}
	}
	return append(diags, egressAfterRestore(ordered, infos)...), nil
}

// egressAfterRestore reports each step that sends source out of kapi after a
// step that restores the withheld values (one that consumes the
// redaction.secret port, unredact) with no recoverable step between them.
func egressAfterRestore(ordered []FlowNode, infos []*registry.ToolInfo) []PlacementDiagnostic {
	var diags []PlacementDiagnostic
	restoredBy := ""
	for i, n := range ordered {
		info := infos[i]
		if info == nil {
			continue
		}
		if info.Recoverable {
			restoredBy = ""
		}
		if restoredBy != "" && sendsSourceOut(info) {
			diags = append(diags, PlacementDiagnostic{
				Severity: PlacementError,
				Rule:     RuleEgressAfterRestore,
				NodeID:   n.ID,
				Tool:     n.Name,
				Message: fmt.Sprintf("%q sends source out of kapi after %q restored the values redaction withholds, with no redact step between them. Add a redact step ahead of %q, or move %q ahead of %q",
					n.Name, restoredBy, n.Name, n.Name, restoredBy),
			})
		}
		if consumesPortType(info, secretPort) {
			restoredBy = n.Name
		}
	}
	return diags
}

// secretPort is the port type that carries the withheld values from redact to
// unredact (redaction.SecretAnnotationKey).
const secretPort = "redaction.secret"

func consumesPortType(info *registry.ToolInfo, portType string) bool {
	for _, c := range info.Consumes {
		if c.Type == portType {
			return true
		}
	}
	return false
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

// CheckRedactionCoverage rejects a flow that sends source to a remote sink, or
// hands it to a program outside kapi, without a recoverable transformer when
// requireRedaction is set.
//
// ValidatePlacement checks the order of existing redact steps, including the
// AD-020 exemption for remote detection inputs. This check covers flows with no
// redact step. A nil registry or unset requireRedaction disables it.
func (d *FlowDefinition) CheckRedactionCoverage(reg *registry.ToolRegistry, requireRedaction bool) error {
	if reg == nil || !requireRedaction {
		return nil
	}
	ordered, err := d.toolNodeRefs()
	if err != nil {
		return err
	}
	hasRedact := false
	var egressTool, execTool string
	for _, n := range ordered {
		info := reg.ResolveToolInfo(registry.ToolID(n.Name), n.Config)
		if info == nil {
			continue
		}
		if info.Recoverable {
			hasRedact = true
		}
		if egressTool == "" && hasSideEffect(info, schema.SideEffectRemoteSourceEgress) {
			egressTool = n.Name
		}
		if execTool == "" && hasSideEffect(info, schema.SideEffectProcessExec) {
			execTool = n.Name
		}
	}
	if execTool != "" && !hasRedact && egressTool == "" {
		return fmt.Errorf("flow %q: the project declares a redaction policy for the content it runs over, but %q hands source to a program outside kapi and the flow has no redact step. Add a redact step before it",
			d.Name, execTool)
	}
	if egressTool != "" && !hasRedact {
		return fmt.Errorf("flow %q: the project declares a redaction policy for the content it runs over, but %q sends source to a remote sink and the flow has no redact step. Add a redact step before it, or run it on a local provider",
			d.Name, egressTool)
	}
	return nil
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
	return slices.Contains(info.SideEffects, effect)
}

// sendsSourceOut reports whether a tool hands source to something outside
// kapi that a redaction policy covers: a remote sink (remote-source-egress)
// or a program kapi runs (process-exec).
func sendsSourceOut(info *registry.ToolInfo) bool {
	return hasSideEffect(info, schema.SideEffectRemoteSourceEgress) || hasSideEffect(info, schema.SideEffectProcessExec)
}

// isPseudoPort reports whether a port type names a pseudo-port (target/source)
// rather than a stand-off layer; pseudo-ports are not run-anchored, so they
// are not "rebased" by a transformer's rewrite.
func isPseudoPort(t string) bool {
	return t == schema.PortTarget || t == schema.PortSource
}
