package host

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
)

// A run of one registry tool (`kapi exec <tool>`, and the MCP tools that run
// one registry tool over a text) applies the redaction policy its project
// declares, as a flow does. When the tool calls a provider, redact runs ahead
// of it and unredact after it: the provider reads the placeholders, and the
// output carries the originals again. The originals ride on the blocks between
// the two steps, as they do in the secure-translate flow, so the run writes
// none of them to disk.
//
// The tools a run applies are held to the rule every flow is held to
// (FlowDefinition.CheckRedactionCoverage, then CheckPlacement), built from the
// tools the run applies, so a run that left the redact step out fails the same
// check a flow without one fails.

// toolRedaction is the redaction policy one run of a tool applies.
type toolRedaction struct {
	spec   *project.RedactionSpec
	root   string
	locale model.LocaleID
}

// toolRunPolicy is what a declared policy asks of one run of a tool: the
// redaction it applies (nil when the tool calls no provider), and whether the
// project declares a policy at all, which decides what the coverage check
// requires.
type toolRunPolicy struct {
	redaction *toolRedaction
	declared  bool
}

// toolRunPolicyFor reads the policy the project at recipe declares for a run
// of toolName with config. An empty recipe (no project) declares none.
func (a *App) toolRunPolicyFor(recipe string, proj *project.KapiProject, toolName string, config map[string]any) (toolRunPolicy, error) {
	spec := ProjectRedaction(proj)
	if recipe == "" || spec == nil {
		return toolRunPolicy{}, nil
	}
	pol := toolRunPolicy{declared: true}
	if !a.toolCallsProvider(toolName, config) {
		return pol, nil
	}
	if err := checkRedactionRules(spec); err != nil {
		return toolRunPolicy{}, err
	}
	locale := proj.Defaults.SourceLanguage
	if locale.IsEmpty() {
		locale = model.LocaleID(a.SourceLocale())
	}
	pol.redaction = &toolRedaction{spec: spec, root: filepath.Dir(recipe), locale: locale}
	return pol, nil
}

// toolCallsProvider reports whether a run of the tool with config reaches a
// provider: its contract, refined for the config, carries an API call. A
// tool whose contract the registry does not hold calls none it declares.
func (a *App) toolCallsProvider(toolName string, config map[string]any) bool {
	if a.ToolReg == nil {
		return false
	}
	info := a.ToolReg.ResolveToolInfo(registry.ToolID(toolName), config)
	return info != nil && slices.Contains(info.SideEffects, schema.SideEffectAPICall)
}

// toolChain is the tools one run applies to a file: the tool, and the steps
// the redaction policy puts around it.
type toolChain struct {
	before []tool.Tool
	main   tool.Tool
	after  []tool.Tool
}

// tools returns the chain in order with main in the tool's place, so a caller
// that wraps the tool (block parallelism, a collector tap, a trace) keeps the
// steps around it.
func (c toolChain) tools(main tool.Tool) []tool.Tool {
	out := make([]tool.Tool, 0, len(c.before)+1+len(c.after))
	out = append(out, c.before...)
	out = append(out, main)
	return append(out, c.after...)
}

// chainFor puts the policy's steps around t and checks the result against the
// redaction rule. With no policy the chain is t alone.
func (a *App) chainFor(pol toolRunPolicy, toolName string, config map[string]any, t tool.Tool) (toolChain, error) {
	c := toolChain{main: t}
	if r := pol.redaction; r != nil {
		rt, err := tools.NewRedactTool(redactToolConfig(r.spec, r.root, "", r.locale))
		if err != nil {
			return toolChain{}, fmt.Errorf("redaction: %w", err)
		}
		ut, err := tools.NewUnredactTool(&tools.UnredactConfig{})
		if err != nil {
			return toolChain{}, fmt.Errorf("redaction: %w", err)
		}
		c.before = []tool.Tool{rt}
		c.after = []tool.Tool{ut}
	}
	if err := a.checkChainCoverage(toolName, config, c, pol.declared); err != nil {
		return toolChain{}, err
	}
	return c, nil
}

// checkChainCoverage holds a run's chain to the rule a flow is held to: under
// a declared policy, a tool that sends source to a remote provider runs only
// after a redact step.
func (a *App) checkChainCoverage(toolName string, config map[string]any, c toolChain, declared bool) error {
	if !declared {
		return nil
	}
	def := flow.FlowDefinition{ID: toolName, Name: toolName}
	for i, t := range c.tools(c.main) {
		n := flow.FlowNode{ID: strconv.Itoa(i), Type: flow.NodeTool, Name: t.Name()}
		if i == len(c.before) {
			n.Name, n.Config = toolName, config
		}
		def.Nodes = append(def.Nodes, n)
		if i > 0 {
			def.Edges = append(def.Edges, flow.FlowEdge{ID: "e" + n.ID, Source: strconv.Itoa(i - 1), Target: n.ID})
		}
	}
	if err := def.CheckRedactionCoverage(a.ToolReg, true); err != nil {
		return err
	}
	return def.CheckPlacement(a.ToolReg)
}

// runChain runs each tool of a chain over parts in turn and returns what the
// last one emits.
func runChain(ctx context.Context, chain []tool.Tool, parts []*model.Part) ([]*model.Part, error) {
	for _, t := range chain {
		in := make(chan *model.Part, len(parts))
		for _, p := range parts {
			in <- p
		}
		close(in)
		out := make(chan *model.Part)
		errc := make(chan error, 1)
		go func() {
			defer close(out)
			errc <- t.Process(ctx, in, out)
		}()
		var next []*model.Part
		for p := range out {
			next = append(next, p)
		}
		if err := <-errc; err != nil {
			return nil, err
		}
		parts = next
	}
	return parts, nil
}
