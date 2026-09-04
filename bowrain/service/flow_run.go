package service

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/neokapi/neokapi/bowrain/analytics"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/safeio"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/venue"
)

// FlowRun is one run of a flow definition over content held in the store:
// the platform's way of running a flow, whichever surface asked for it. An
// automation rule's run_flow action and the MCP run_flow tool both build one
// of these and hand it to FlowService.RunFlow.
type FlowRun struct {
	// Definition is the resolved flow graph (see FlowCatalog.Get).
	Definition *flow.FlowDefinition
	// ProjectID scopes the blocks read and written.
	ProjectID string
	// Stream scopes the blocks read and written. Empty means main.
	Stream string
	// Items names the items to process. Empty means every item in the stream.
	Items []string
	// TargetLocales are the locales to run for, one pass each over the same
	// blocks. Empty runs a single pass with no target locale, which is what a
	// source-only tool wants and what a bilingual tool rejects at build time.
	TargetLocales []string
	// Actor is the analytics identity of whoever asked: a user id, or empty
	// when the platform started the run itself.
	Actor string
	// Source names the surface the run came from ("automation", "mcp").
	Source string
}

// FlowRunResult reports what a run touched.
type FlowRunResult struct {
	// Items is the number of items whose blocks went through the flow.
	Items int
	// Passes is the number of (item, locale) passes executed.
	Passes int
	// Blocks is the number of blocks written back to the store.
	Blocks int
}

// RunFlow runs a flow definition over a project's stored blocks and writes the
// results back, one item at a time.
//
// Each item's blocks are loaded once and streamed through the flow's tools
// with the executor's channel pipeline, one pass per target locale; the tools
// mutate the blocks in place and the pass's output is persisted under the
// item before the next locale starts. Reading per item keeps memory bounded
// by one item rather than by the project (a whole-project read is what took
// the server down once), and each item's batch draws from the same in-flight
// admission budget every other flow execution on the process shares.
//
// The flow is gated the way a CLI run gates it before any block is read: the
// definition must validate, every tool's consumed port must be produced
// upstream or carried by the content store, and transformer placement must
// hold. A flow that fails a gate, names a tool the registry lacks, or whose
// tool errors mid-pass fails the run with that error; nothing is written for
// the item that failed.
func (s *FlowService) RunFlow(ctx context.Context, run FlowRun) (FlowRunResult, error) {
	var res FlowRunResult
	if run.Definition == nil {
		return res, errors.New("flow definition is required")
	}
	if run.ProjectID == "" {
		return res, errors.New("project id is required")
	}
	if s.store == nil {
		return res, errors.New("content store not configured")
	}
	if s.toolReg == nil {
		return res, errors.New("tool registry not configured")
	}

	start := time.Now()
	def := run.Definition
	nodes, err := s.storeFlowToolNodes(def)
	if err != nil {
		s.trackDefinitionRun(run, 0, time.Since(start), "failed")
		return res, err
	}

	stream := run.Stream
	if stream == "" {
		stream = "main"
	}
	items := run.Items
	if len(items) == 0 {
		stored, err := s.store.ListItems(ctx, run.ProjectID, stream)
		if err != nil {
			return res, fmt.Errorf("list items: %w", err)
		}
		items = make([]string, 0, len(stored))
		for _, it := range stored {
			items = append(items, it.Name)
		}
	}
	locales := run.TargetLocales
	if len(locales) == 0 {
		locales = []string{""}
	}

	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		blocks, err := s.store.GetBlocks(ctx, store.BlockQuery{
			ProjectID: run.ProjectID,
			Stream:    stream,
			ItemName:  item,
		})
		if err != nil {
			return res, fmt.Errorf("load blocks for item %q: %w", item, err)
		}
		current := make([]*model.Block, 0, len(blocks))
		for _, sb := range blocks {
			if sb.Block != nil {
				current = append(current, sb.Block)
			}
		}
		if len(current) == 0 {
			continue
		}

		release, err := s.AcquireCapacity(ctx, storedBlocksWeight(blocks))
		if err != nil {
			s.trackDefinitionRun(run, res.Blocks, time.Since(start), "failed")
			return res, err
		}
		written, err := s.runItemPasses(ctx, def, nodes, run.ProjectID, stream, item, current, locales)
		release()
		if err != nil {
			s.trackDefinitionRun(run, res.Blocks, time.Since(start), "failed")
			return res, fmt.Errorf("item %q: %w", item, err)
		}
		res.Items++
		res.Passes += len(locales)
		res.Blocks += written
	}

	s.trackDefinitionRun(run, res.Blocks, time.Since(start), "completed")
	return res, nil
}

// storeFlowToolNodes applies the build-time gates a CLI run applies and
// returns the flow's tool nodes in execution order.
//
// The source binding is the content store, which carries the source and every
// persisted port, so a flow that opens on a check reads the stored targets
// rather than being rejected for consuming a port nothing upstream produced.
// A definition that pins its own source binding keeps it.
func (s *FlowService) storeFlowToolNodes(def *flow.FlowDefinition) ([]flow.FlowNode, error) {
	if err := def.Validate(); err != nil {
		return nil, err
	}
	bound := *def
	if bound.Binding == nil || bound.Binding.Source == "" {
		binding := flow.FlowBinding{Source: "store"}
		if def.Binding != nil {
			binding.Sink = def.Binding.Sink
		}
		bound.Binding = &binding
	}
	if err := bound.ValidateDataFlow(s.toolReg); err != nil {
		return nil, err
	}
	if err := bound.CheckPlacement(s.toolReg); err != nil {
		return nil, err
	}
	order, err := def.TopologicalOrder()
	if err != nil {
		return nil, err
	}
	byID := make(map[string]flow.FlowNode, len(def.Nodes))
	for _, n := range def.Nodes {
		byID[n.ID] = n
	}
	nodes := make([]flow.FlowNode, 0, len(order))
	for _, id := range order {
		if n := byID[id]; n.Type == flow.NodeTool {
			nodes = append(nodes, n)
		}
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("flow %q has no tool nodes", def.ID)
	}
	// Every tool must be constructible before a single block is read, so a
	// flow naming a tool this server lacks fails here rather than mid-item.
	for _, n := range nodes {
		if !s.toolReg.Has(registry.ToolID(n.Name)) {
			return nil, fmt.Errorf("flow %q: unknown tool %q", def.ID, n.Name)
		}
	}
	return nodes, nil
}

// runItemPasses streams one item's blocks through the flow once per locale
// and persists each pass's output under the item. The tools mutate the blocks
// in place and pass the same pointers on, so one set of blocks carries every
// pass; each pass builds a fresh tool chain because a tool holds its target
// locale in its config. It returns the number of blocks in the item's final
// state, or zero when no pass produced output.
func (s *FlowService) runItemPasses(ctx context.Context, def *flow.FlowDefinition, nodes []flow.FlowNode, projectID, stream, item string, blocks []*model.Block, locales []string) (int, error) {
	current := blocks
	written := 0
	for _, locale := range locales {
		tools, err := s.buildFlowTools(nodes, locale)
		if err != nil {
			return 0, err
		}
		out, err := runBlocksThroughTools(ctx, def.ID, tools, current)
		if err != nil {
			return 0, err
		}
		if len(out) == 0 {
			continue
		}
		if err := s.store.StoreBlocksForItem(ctx, projectID, stream, item, out); err != nil {
			return 0, fmt.Errorf("persist flow output: %w", err)
		}
		current = out
		written = len(out)
	}
	return written, nil
}

// buildFlowTools constructs the tool chain for one pass. Each node's own
// config is applied first and the pass's target locale on top, the same shape
// the CLI hands a built-in flow's tools.
func (s *FlowService) buildFlowTools(nodes []flow.FlowNode, targetLocale string) ([]tool.Tool, error) {
	tools := make([]tool.Tool, 0, len(nodes))
	for _, n := range nodes {
		cfg := make(map[string]any, len(n.Config)+1)
		maps.Copy(cfg, n.Config)
		if targetLocale != "" {
			cfg["target_locale"] = targetLocale
		}
		t, err := s.toolReg.NewToolWithConfig(registry.ToolID(n.Name), cfg, targetLocale)
		if err != nil {
			return nil, fmt.Errorf("tool %q: %w", n.Name, err)
		}
		tools = append(tools, t)
	}
	return tools, nil
}

// runBlocksThroughTools feeds blocks into the executor's channel pipeline and
// collects the blocks that come out the far end.
//
// The feeder runs beside the drain: a pipeline fed only after its output is
// read would stall past the channel buffer. The drain finishes before the
// wait, because a tool that failed stops reading and the stage above it can
// only finish once something empties the channel between them. The pass's
// own cancel releases the feeder once the wait returns, so a tool that fails
// before consuming every block leaves no goroutine behind.
func runBlocksThroughTools(ctx context.Context, name string, tools []tool.Tool, blocks []*model.Block) ([]*model.Block, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	f := &flow.Flow{Name: name, Tools: tools}
	exec := flow.NewExecutor(flow.WithFailFast(true))
	in, out, wait := exec.ExecuteWithChannels(ctx, f)

	go func() {
		defer close(in)
		for _, b := range blocks {
			select {
			case in <- &model.Part{Type: model.PartBlock, Resource: b}:
			case <-ctx.Done():
				return
			}
		}
	}()

	result := make([]*model.Block, 0, len(blocks))
	for p := range out {
		if p.Type != model.PartBlock {
			continue
		}
		if b, ok := p.Resource.(*model.Block); ok {
			result = append(result, b)
		}
	}
	if err := wait(); err != nil {
		return nil, err
	}
	return result, nil
}

// storedBlocksWeight estimates a batch's in-flight byte weight from its
// source text, as the gRPC flow route does. Never zero, so a batch always
// draws some budget.
func storedBlocksWeight(blocks []*venue.StoredBlock) int64 {
	var total int64
	for _, sb := range blocks {
		if sb.Block != nil {
			total += int64(len(sb.Block.SourceText()))
		}
	}
	if total <= 0 {
		total = safeio.UnknownFileWeight
	}
	return total
}

// trackDefinitionRun captures flow_run_completed for a store-backed run
// (fire-and-forget, nil-safe; never carries content). The actor is the
// distinct id when one is known, otherwise the project as for every other
// server-started flow.
func (s *FlowService) trackDefinitionRun(run FlowRun, blocks int, d time.Duration, outcome string) {
	if s.tracker == nil {
		return
	}
	props := analytics.Props("", run.ProjectID)
	props["flow"] = run.Definition.ID
	props["duration_bucket"] = analytics.DurationBucket(d)
	props["outcome"] = outcome
	props["part_count"] = blocks
	if run.Source != "" {
		props["source"] = run.Source
	}
	distinct := run.Actor
	if distinct == "" {
		distinct = run.ProjectID
	}
	track(s.tracker, distinct, analytics.EventFlowRunCompleted, props)
}
