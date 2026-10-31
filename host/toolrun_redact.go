package host

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
)

// A run of one registry tool (`kapi exec <tool>`, and the MCP tools that run
// one registry tool over a text) reads each file under the redaction policy
// that covers it (RedactionFor); a text in no file reads under the defaults.
// When the tool sends blocks out (toolSendsBlocksOut), redact runs ahead of
// it and unredact after it, so the provider or program reads placeholders and
// the output carries the originals. Between the two steps the originals ride on the blocks, as in the
// secure-translate flow, and the run writes none of them to disk.

// toolRedaction is the redaction policy one run of a tool applies.
type toolRedaction struct {
	spec   *project.RedactionSpec
	root   string
	locale model.LocaleID
}

// toolRunPolicy is what a declared policy asks of one run of a tool over one
// file: the redaction it applies (nil when the tool sends no blocks out), and
// whether a policy governs the file at all, which decides what the coverage
// check requires.
type toolRunPolicy struct {
	redaction *toolRedaction
	declared  bool
}

// toolRunPolicyFor reads the policy the project at recipe declares for a run
// of toolName with config over file. An empty recipe (no project) declares
// none; an empty file, or one outside the project, is read under
// `defaults.redaction`.
func (a *App) toolRunPolicyFor(recipe string, proj *project.KapiProject, file, toolName string, config map[string]any) (toolRunPolicy, error) {
	if recipe == "" {
		return toolRunPolicy{}, nil
	}
	resolved := proj.RedactionForPath(redactionPath(filepath.Dir(recipe), file))
	// governing is the policy a tool with no known contract is refused under:
	// the file's, or for a package, which holds files of every collection, the
	// first policy the recipe declares (as checkPackagePolicy holds it).
	governing := resolved
	if governing.Spec == nil && IsKpzPath(file) {
		if declared := proj.DeclaredRedactions(); len(declared) > 0 {
			governing = declared[0]
		}
	}
	if governing.Spec != nil && !a.toolContractKnown(toolName) {
		return toolRunPolicy{}, fmt.Errorf("kapi refused to run %s: the project declares a redaction policy at %s, and %s has no contract that kapi knows, so kapi cannot tell whether it sends blocks to a provider. Declare its side effects in the toolMeta of the schema its plugin manifest names, or run it in a flow with a redact step ahead of it", toolName, governing.At, toolName)
	}
	spec := resolved.Spec
	if spec == nil {
		return toolRunPolicy{}, nil
	}
	pol := toolRunPolicy{declared: true}
	if !a.toolSendsBlocksOut(toolName, config) {
		return pol, nil
	}
	if hasDetector(spec.Detectors, "entities") {
		// Entity detection reads the entity annotations an extraction step
		// leaves on a block, and a run of one tool has none, so the names the
		// policy withholds would reach the provider.
		return toolRunPolicy{}, fmt.Errorf("the project's redaction policy (%s) detects entities, which needs an entity-extract step that a run of one tool does not have, so %s would send the values the policy withholds. Run it in a flow with entity-extract and redact ahead of it, or set %s.detectors to [rules]", resolved.At, toolName, resolved.At)
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

// toolContractKnown reports whether the registry holds the tool's contract:
// an entry whose schema carries a toolMeta. Under a declared policy a tool
// without one does not run, because nothing says whether it reaches a
// provider. Every built-in tool carries one. A tool registered without a
// schema, or with a schema that has no toolMeta, has none.
func (a *App) toolContractKnown(toolName string) bool {
	if a.ToolReg == nil {
		return false
	}
	s := a.ToolReg.Schema(registry.ToolID(toolName))
	return s != nil && s.ToolMeta != nil
}

// toolSendsBlocksOut reports whether a run of the tool with config hands
// blocks to something outside kapi: its contract, refined for the config,
// carries an API call (a provider) or process-exec (a program kapi runs, such
// as external-command's). toolRunPolicyFor asks only about a tool whose
// contract the registry holds (toolContractKnown).
//
// The registry fills in the configured default provider only for a tool
// whose contract, resolved for the same config, requires credentials, so the
// answer for the config the command sets is the answer for the tool it builds.
func (a *App) toolSendsBlocksOut(toolName string, config map[string]any) bool {
	if a.ToolReg == nil {
		return false
	}
	info := a.ToolReg.ResolveToolInfo(registry.ToolID(toolName), config)
	return info != nil && (slices.Contains(info.SideEffects, schema.SideEffectAPICall) ||
		slices.Contains(info.SideEffects, schema.SideEffectProcessExec))
}

// toolExecsProcess reports whether the tool's contract declares process-exec:
// it hands block text to a program kapi runs. Under a redaction policy such a
// tool reads each withheld value as its visible token, and records a failing
// finding on a block whose program output changes one
// (tools.RulePlaceholder).
func (a *App) toolExecsProcess(toolName string) bool {
	if a.ToolReg == nil {
		return false
	}
	s := a.ToolReg.Schema(registry.ToolID(toolName))
	return s != nil && s.ToolMeta != nil && slices.Contains(s.ToolMeta.SideEffects, schema.SideEffectProcessExec)
}

// checksRedactionTokens reports whether a run of toolName under the policies
// in runCfgs can record the placeholder finding: the tool hands blocks to a
// program, and a redact step runs ahead of it over at least one file.
func (a *App) checksRedactionTokens(toolName string, runCfgs map[string]ToolRunConfig) bool {
	if !a.toolExecsProcess(toolName) {
		return false
	}
	for _, c := range runCfgs {
		if c.policy.redaction != nil {
			return true
		}
	}
	return false
}

// chainChecksRedactionTokens is checksRedactionTokens for a flow's chain: it
// holds a redact step (a recoverable transformer, as CheckRedactionCoverage
// reads one) and a tool that hands blocks to a program.
func (a *App) chainChecksRedactionTokens(chain []tool.Tool) bool {
	if a.ToolReg == nil {
		return false
	}
	redacts, execs := false, false
	for _, t := range chain {
		if t == nil {
			continue
		}
		if info := a.ToolReg.ResolveToolInfo(registry.ToolID(t.Name()), nil); info != nil && info.Recoverable {
			redacts = true
		}
		if a.toolExecsProcess(t.Name()) {
			execs = true
		}
	}
	return redacts && execs
}

// placeholderRule is the rule id a report gives the finding a tool records
// when the output of the program it runs changes a redaction placeholder.
var placeholderRule = check.RuleID(string(tools.ExternalCommand), tools.RulePlaceholder)

// specChecksRedactionTokens is chainChecksRedactionTokens for a flow's steps
// before they are built: a redact step and a tool that hands blocks to a
// program.
func (a *App) specChecksRedactionTokens(spec *flow.StepsSpec) bool {
	if a.ToolReg == nil || spec == nil {
		return false
	}
	redacts, execs := false, false
	for _, step := range spec.Steps {
		if info := a.ToolReg.ResolveToolInfo(registry.ToolID(step.Tool), step.Config); info != nil && info.Recoverable {
			redacts = true
		}
		if a.toolExecsProcess(step.Tool) {
			execs = true
		}
	}
	return redacts && execs
}

// checkRedactEntities refuses a flow's redact step that detects entities when
// no earlier step of the flow produces the entity port. Entity detection reads
// the entity annotations an extraction step leaves on a block, so such a step
// would withhold nothing it detects by entity, and a provider later in the
// flow would read the values. The wording follows the refusal a run of one
// tool gives (toolRunPolicyFor).
func (a *App) checkRedactEntities(flowName string, t tool.Tool, entitiesProduced bool) error {
	if entitiesProduced || t == nil || t.Name() != "redact" {
		return nil
	}
	cfg, ok := t.Config().(*tools.RedactConfig)
	if !ok || !hasDetector(cfg.Detectors, tools.DetectEntities) {
		return nil
	}
	if resolved := a.flowRedactionPolicy(); resolved.Spec != nil && hasDetector(resolved.Spec.Detectors, tools.DetectEntities) {
		return fmt.Errorf("the project's redaction policy (%s) detects entities, which needs an entity-extract step that flow %q does not have ahead of its redact step, so the flow would send the values the policy withholds. Add entity-extract ahead of redact, or set %s.detectors to [rules]", resolved.At, flowName, resolved.At)
	}
	return fmt.Errorf("the redact step of flow %q detects entities, which needs an entity-extract step that the flow does not have ahead of it, so the flow would send the values the step is meant to withhold. Add entity-extract ahead of redact, or set the step's detectors to [rules]", flowName)
}

// placeholderBlocks counts the blocks a report holds a failing placeholder
// finding on.
func placeholderBlocks(r findingsReport) int {
	blocks := map[string]bool{}
	for _, d := range r.Findings {
		if d.Fails && d.Rule == placeholderRule {
			blocks[d.Location.File+"\x00"+d.Location.Block] = true
		}
	}
	return len(blocks)
}

// placeholderGateError is the error a run returns once its report is
// printed, when the report holds a failing placeholder finding. Each such
// block was left unchanged, so the run did not do what it was asked to, and
// it exits with the code a failed check uses (ExitGate).
func placeholderGateError(data any) error {
	r, ok := data.(findingsReport)
	if !ok {
		return nil
	}
	n := placeholderBlocks(r)
	if n == 0 {
		return nil
	}
	return WithExitCode(ExitGate, fmt.Errorf("%d block(s): the program's output changed its redaction placeholders, so kapi left each such block as it was", n))
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
		// The tool reads the whole block, so the redaction covers it: blocks
		// marked not translatable, and every translation beside the source.
		rcfg := redactToolConfig(r.spec, r.root, "", r.locale)
		rcfg.WholeBlock = true
		rt, err := tools.NewRedactTool(rcfg)
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

// checkChainCoverage holds a run's chain to the rule a flow is held to
// (FlowDefinition.CheckRedactionCoverage, then CheckPlacement): under a
// declared policy, a tool that sends source to a remote provider runs only
// after a redact step. The definition it checks is built from the tools the
// run applies, so a run that left the redact step out fails here.
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
	// A run builds its own chain, so a failure here is kapi's fault and not
	// the project's: the error says so rather than asking for a step the
	// person running one tool has no way to add.
	if err := def.CheckRedactionCoverage(a.ToolReg, true); err != nil {
		return fmt.Errorf("kapi refused to run %s: the project declares a redaction policy for the content, and %s would send source to a remote provider with no redact step ahead of it. This is a fault in kapi; report it with the command you ran", toolName, toolName)
	}
	if err := def.CheckPlacement(a.ToolReg); err != nil {
		return fmt.Errorf("kapi refused to run %s: the redaction steps around it are out of order (%w). This is a fault in kapi; report it with the command you ran", toolName, err)
	}
	return nil
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
