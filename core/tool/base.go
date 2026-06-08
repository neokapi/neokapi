package tool

import (
	"cmp"
	"context"
	"fmt"
	"hash/fnv"
	"slices"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
)

// PartHandler is the function signature for handling a single Part.
type PartHandler func(part *model.Part) (*model.Part, error)

// EnforceImmutability gates the dev/test backstop in the typed block-handler
// dispatch. On by default: capability is enforced primarily by the handler's
// parameter type (an Annotate handler has no source/target setters), and the
// backstop additionally catches in-place edits made through the live run
// slices the read-only view hands back — including a source rewrite while a
// stand-off overlay is attached (overlays anchor to runs and would be
// invalidated). Can be disabled in perf-sensitive embeddings.
//
// Concurrency: this is a process-wide configuration knob, not per-run state.
// Set it once during initialization, before any tool starts processing. It is
// read (never written) from the concurrent per-block dispatch path, including
// ParallelBlockTool's workers, so toggling it while a flow is running is a data
// race and unsupported.
var EnforceImmutability = true

// BaseTool provides default pass-through behavior and event dispatch.
// Embed in concrete tools and override Handle* methods as needed.
type BaseTool struct {
	ToolName        string
	ToolDescription string
	Cfg             ToolConfig

	// SchemaFn optionally returns the tool's parameter schema.
	// Set this to enable schema-driven CLI flags and flow editor config panels.
	SchemaFn func() *schema.ComponentSchema

	// Process-named block handlers (immutability model, AD-006). A tool sets
	// exactly ONE, picking the verb that matches what it does — that choice
	// IS its capability declaration, enforced by the parameter type:
	//
	//   Annotate(BlockView)  — analysis/annotation: reads source+target,
	//                          writes only overlays, annotations, properties.
	//                          The default surface; no content writes exist.
	//   Translate(TargetView)— target production: reads source, writes target.
	//   Transform(SourceView)— source transformation: rewrites source (and may
	//                          write target). Runs early, before overlays exist;
	//                          may record recovery for a later restore (e.g. redact).
	//
	// Each is 1→1 (mutate in place) or 1→0 (call view.Drop()). A tool needing
	// 1→N, cross-block state, batching, or stream control overrides Process.
	Annotate  func(BlockView) error
	Translate func(TargetView) error
	Transform func(SourceView) error

	// Non-block handlers stay untyped: the immutability model governs Block
	// source/target content; Data/Media/Layer/Group parts carry no such content.
	HandleDataFn       PartHandler
	HandleMediaFn      PartHandler
	HandleLayerStartFn PartHandler
	HandleLayerEndFn   PartHandler
	HandleGroupStartFn PartHandler
	HandleGroupEndFn   PartHandler
}

// Name returns the tool's identifier.
func (b *BaseTool) Name() string { return b.ToolName }

// Description returns the tool's description.
func (b *BaseTool) Description() string { return b.ToolDescription }

// Schema returns the tool's parameter schema, if SchemaFn is set.
func (b *BaseTool) Schema() *schema.ComponentSchema {
	if b.SchemaFn != nil {
		return b.SchemaFn()
	}
	return nil
}

// Config returns the current configuration.
func (b *BaseTool) Config() ToolConfig { return b.Cfg }

// SetConfig applies a new configuration after validation.
func (b *BaseTool) SetConfig(cfg ToolConfig) error {
	if cfg == nil {
		return nil
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	b.Cfg = cfg
	return nil
}

// Process dispatches each Part to the appropriate handler.
func (b *BaseTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-in:
			if !ok {
				return nil
			}
			result, err := b.dispatch(ctx, part)
			if err != nil {
				return err
			}
			select {
			case out <- result:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

// Capability classifies what a tool's block handler may write — for flow-stage
// validation (e.g. only source-transforms belong in a Flow's source-transform
// stage). Tools that override Process without setting a typed handler report
// CapNone.
type Capability int

const (
	CapNone      Capability = iota // no typed block handler (pure Process override / pass-through)
	CapAnnotate                    // read-only: overlays/annotations/properties
	CapTranslate                   // writes target
	CapTransform                   // rewrites source (and may write target)
)

// Capable is the optional interface a Tool implements to report its write
// capability. *BaseTool implements it (and tools embedding it inherit it).
type Capable interface {
	Capability() Capability
}

// Capability reports the tool's write capability from which typed handler is set.
func (b *BaseTool) Capability() Capability {
	switch {
	case b.Transform != nil:
		return CapTransform
	case b.Translate != nil:
		return CapTranslate
	case b.Annotate != nil:
		return CapAnnotate
	default:
		return CapNone
	}
}

// IsSourceTransform reports whether t may rewrite source (CapTransform). Tools
// that don't implement Capable are treated as not source-transforms.
func IsSourceTransform(t Tool) bool {
	c, ok := t.(Capable)
	return ok && c.Capability() == CapTransform
}

// Apply runs a single Part through the tool's per-part dispatch — the same
// routing (and immutability backstop) Process uses — returning the result Part,
// or nil if the handler dropped it. For callers that apply a BaseTool across an
// in-memory slice of parts rather than driving the streaming pipeline.
func (b *BaseTool) Apply(part *model.Part) (*model.Part, error) {
	return b.dispatch(context.Background(), part)
}

// ApplyContext is Apply with an explicit context, so a block handler driven
// outside the streaming pipeline can still honour cancellation/deadlines.
func (b *BaseTool) ApplyContext(ctx context.Context, part *model.Part) (*model.Part, error) {
	return b.dispatch(ctx, part)
}

// hasBlockHandler reports whether the tool set one of the capability-typed
// block handlers. Wrappers (parallel, retry) use it to decide whether per-block
// treatment is possible.
func (b *BaseTool) hasBlockHandler() bool {
	return b.Annotate != nil || b.Translate != nil || b.Transform != nil
}

func (b *BaseTool) dispatch(ctx context.Context, part *model.Part) (*model.Part, error) {
	switch part.Type {
	case model.PartBlock:
		return b.handleBlock(ctx, part)
	case model.PartData:
		return b.handleData(part)
	case model.PartMedia:
		return b.handleMedia(part)
	case model.PartLayerStart:
		return b.handleLayerStart(part)
	case model.PartLayerEnd:
		return b.handleLayerEnd(part)
	case model.PartGroupStart:
		return b.handleGroupStart(part)
	case model.PartGroupEnd:
		return b.handleGroupEnd(part)
	default:
		return part, nil
	}
}

// handleBlock dispatches a Block part to whichever capability-typed handler the
// tool set. The handler's parameter type bounds what it may write; a dev/test
// backstop (gated by EnforceImmutability) additionally catches in-place edits
// the read-only view type can't prevent — a tool that mutates the live run
// slices it was handed.
func (b *BaseTool) handleBlock(ctx context.Context, part *model.Part) (*model.Part, error) {
	switch {
	case b.Transform != nil:
		return b.runTransform(ctx, part)
	case b.Translate != nil:
		return b.runTranslate(ctx, part)
	case b.Annotate != nil:
		return b.runAnnotate(ctx, part)
	default:
		return part, nil
	}
}

// runAnnotate drives a read-only annotation handler. Backstop: neither source
// nor target content may change (only overlays/annotations/properties).
func (b *BaseTool) runAnnotate(ctx context.Context, part *model.Part) (*model.Part, error) {
	block, _ := part.Resource.(*model.Block)
	if block == nil {
		return part, nil
	}
	v := newBlockView(ctx, block)
	if !EnforceImmutability {
		if err := b.Annotate(v); err != nil {
			return nil, err
		}
		return v.result(part), nil
	}
	srcBefore, tgtBefore := blockSourceSig(block), blockTargetsSig(block)
	if err := b.Annotate(v); err != nil {
		return nil, err
	}
	if blockSourceSig(block) != srcBefore {
		return nil, fmt.Errorf("immutability: annotate tool %q changed source of block %q — annotators write only overlays/annotations/properties", b.ToolName, block.ID)
	}
	if blockTargetsSig(block) != tgtBefore {
		return nil, fmt.Errorf("immutability: annotate tool %q changed target of block %q — use a Translate handler to write targets", b.ToolName, block.ID)
	}
	return v.result(part), nil
}

// runTranslate drives a target-writing handler. Backstop: source is read-only.
func (b *BaseTool) runTranslate(ctx context.Context, part *model.Part) (*model.Part, error) {
	block, _ := part.Resource.(*model.Block)
	if block == nil {
		return part, nil
	}
	v := newBlockView(ctx, block)
	if !EnforceImmutability {
		if err := b.Translate(v); err != nil {
			return nil, err
		}
		return v.result(part), nil
	}
	srcBefore := blockSourceSig(block)
	if err := b.Translate(v); err != nil {
		return nil, err
	}
	if blockSourceSig(block) != srcBefore {
		return nil, fmt.Errorf("immutability: translate tool %q changed source of block %q — use a Transform handler to rewrite source", b.ToolName, block.ID)
	}
	return v.result(part), nil
}

// runTransform drives a source-transform handler (source + target writable).
// Backstop: source must not change once a stand-off overlay is attached — the
// overlay's run-anchored ranges would rot. Source transforms run early.
func (b *BaseTool) runTransform(ctx context.Context, part *model.Part) (*model.Part, error) {
	block, _ := part.Resource.(*model.Block)
	if block == nil {
		return part, nil
	}
	v := newBlockView(ctx, block)
	if !EnforceImmutability {
		if err := b.Transform(v); err != nil {
			return nil, err
		}
		return v.result(part), nil
	}
	srcBefore := blockSourceSig(block)
	if err := b.Transform(v); err != nil {
		return nil, err
	}
	// A source rewrite invalidates any run-anchored source overlay. The hazard
	// is leaving such an overlay *dangling* over the new runs, so we check the
	// post-transform state: a tool that consumes a source facet and then rewrites
	// the source must drop that facet (e.g. redact consuming the entity facet),
	// leaving no stale overlay behind.
	if blockSourceSig(block) != srcBefore && block.HasSourceOverlays() {
		return nil, fmt.Errorf("immutability: transform tool %q rewrote the source of block %q while a stand-off source overlay remained attached — drop consumed source facets before rewriting source (overlays anchor to runs)", b.ToolName, block.ID)
	}
	return v.result(part), nil
}

// blockSourceSig is a cheap content signature of a Block's source runs,
// covering text and inline-code markup so in-place edits are detected.
func blockSourceSig(b *model.Block) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(model.RenderRunsWithData(b.Source)))
	return h.Sum64()
}

// blockTargetsSig is an order-independent content signature of all target
// variants (content + status), used to detect target mutation.
func blockTargetsSig(b *model.Block) uint64 {
	if len(b.Targets) == 0 {
		return 0
	}
	type variant struct {
		key string
		tgt *model.Target
	}
	entries := make([]variant, 0, len(b.Targets))
	for k, t := range b.Targets {
		mt, _ := k.MarshalText()
		entries = append(entries, variant{key: string(mt), tgt: t})
	}
	slices.SortFunc(entries, func(a, b variant) int { return cmp.Compare(a.key, b.key) })
	h := fnv.New64a()
	for _, e := range entries {
		_, _ = h.Write([]byte(e.key))
		_, _ = h.Write([]byte{0})
		if t := e.tgt; t != nil {
			_, _ = h.Write([]byte(model.RenderRunsWithData(t.Runs)))
			_, _ = h.Write([]byte{0})
			_, _ = h.Write([]byte(t.Status))
		}
		_, _ = h.Write([]byte{1})
	}
	return h.Sum64()
}

func (b *BaseTool) handleData(part *model.Part) (*model.Part, error) {
	if b.HandleDataFn != nil {
		return b.HandleDataFn(part)
	}
	return part, nil
}

func (b *BaseTool) handleMedia(part *model.Part) (*model.Part, error) {
	if b.HandleMediaFn != nil {
		return b.HandleMediaFn(part)
	}
	return part, nil
}

func (b *BaseTool) handleLayerStart(part *model.Part) (*model.Part, error) {
	if b.HandleLayerStartFn != nil {
		return b.HandleLayerStartFn(part)
	}
	return part, nil
}

func (b *BaseTool) handleLayerEnd(part *model.Part) (*model.Part, error) {
	if b.HandleLayerEndFn != nil {
		return b.HandleLayerEndFn(part)
	}
	return part, nil
}

func (b *BaseTool) handleGroupStart(part *model.Part) (*model.Part, error) {
	if b.HandleGroupStartFn != nil {
		return b.HandleGroupStartFn(part)
	}
	return part, nil
}

func (b *BaseTool) handleGroupEnd(part *model.Part) (*model.Part, error) {
	if b.HandleGroupEndFn != nil {
		return b.HandleGroupEndFn(part)
	}
	return part, nil
}

// Verify BaseTool implements Tool at compile time.
var _ Tool = (*BaseTool)(nil)
