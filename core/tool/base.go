package tool

import (
	"context"
	"fmt"
	"hash/fnv"
	"sort"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
)

// PartHandler is the function signature for handling a single Part.
type PartHandler func(part *model.Part) (*model.Part, error)

// EnforceImmutability gates the source/target mutation guard in handleBlock.
// On by default: a tool that changes a Block's source or target content
// without declaring the matching capability (WritesSource / WritesTarget) is
// rejected, and a source-transform that mutates source while a stand-off
// overlay is attached is rejected (overlays anchor to runs and would be
// invalidated). Can be disabled in perf-sensitive embeddings.
var EnforceImmutability = true

// BaseTool provides default pass-through behavior and event dispatch.
// Embed in concrete tools and override Handle* methods as needed.
type BaseTool struct {
	ToolName        string
	ToolDescription string
	Cfg             ToolConfig

	// Content-write capabilities (immutability model, AD-002). DEPRECATED:
	// these are the transitional runtime flags used only by the legacy
	// HandleBlockFn path. Migrated tools express capability by which typed
	// block handler they set (Annotate / Translate / Transform) instead.
	WritesSource bool // may rewrite Block.Source (source-transform; runs before overlays)
	WritesTarget bool // may write Block.Targets (translation / target edits)

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
	//                          may register recovery for a later restore phase.
	//
	// Each is 1→1 (mutate in place) or 1→0 (call view.Drop()). A tool needing
	// 1→N, cross-block state, batching, or stream control overrides Process.
	Annotate  func(BlockView) error
	Translate func(TargetView) error
	Transform func(SourceView) error

	// HandleBlockFn is the legacy untyped block handler. DEPRECATED: superseded
	// by Annotate / Translate / Transform. Retained only until all tools are
	// migrated off it. When set, it takes precedence over the typed handlers.
	HandleBlockFn PartHandler

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
			result, err := b.dispatch(part)
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

// Apply runs a single Part through the tool's per-part dispatch — the same
// routing (and immutability backstop) Process uses — returning the result Part,
// or nil if the handler dropped it. For callers that apply a BaseTool across an
// in-memory slice of parts rather than driving the streaming pipeline.
func (b *BaseTool) Apply(part *model.Part) (*model.Part, error) {
	return b.dispatch(part)
}

// hasBlockHandler reports whether the tool set any per-block handler — one of
// the capability-typed handlers or the legacy HandleBlockFn. Wrappers
// (parallel, retry) use it to decide whether per-block treatment is possible.
func (b *BaseTool) hasBlockHandler() bool {
	return b.Annotate != nil || b.Translate != nil || b.Transform != nil || b.HandleBlockFn != nil
}

func (b *BaseTool) dispatch(part *model.Part) (*model.Part, error) {
	switch part.Type {
	case model.PartBlock:
		return b.handleBlock(part)
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

// handleBlock dispatches a Block part to whichever block handler the tool set,
// picking the typed (capability-scoped) handlers first and falling back to the
// legacy HandleBlockFn. The typed paths run a dev/test backstop (gated by
// EnforceImmutability) that catches in-place edits the read-only view type
// can't prevent — a tool that mutates the live run slices it was handed.
func (b *BaseTool) handleBlock(part *model.Part) (*model.Part, error) {
	switch {
	case b.Transform != nil:
		return b.runTransform(part)
	case b.Translate != nil:
		return b.runTranslate(part)
	case b.Annotate != nil:
		return b.runAnnotate(part)
	case b.HandleBlockFn != nil:
		return b.handleBlockLegacy(part)
	default:
		return part, nil
	}
}

// runAnnotate drives a read-only annotation handler. Backstop: neither source
// nor target content may change (only overlays/annotations/properties).
func (b *BaseTool) runAnnotate(part *model.Part) (*model.Part, error) {
	block, _ := part.Resource.(*model.Block)
	if block == nil {
		return part, nil
	}
	v := newBlockView(block)
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
func (b *BaseTool) runTranslate(part *model.Part) (*model.Part, error) {
	block, _ := part.Resource.(*model.Block)
	if block == nil {
		return part, nil
	}
	v := newBlockView(block)
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
func (b *BaseTool) runTransform(part *model.Part) (*model.Part, error) {
	block, _ := part.Resource.(*model.Block)
	if block == nil {
		return part, nil
	}
	v := newBlockView(block)
	if !EnforceImmutability {
		if err := b.Transform(v); err != nil {
			return nil, err
		}
		return v.result(part), nil
	}
	hadOverlay := block.HasSourceOverlays()
	srcBefore := blockSourceSig(block)
	if err := b.Transform(v); err != nil {
		return nil, err
	}
	if hadOverlay && blockSourceSig(block) != srcBefore {
		return nil, fmt.Errorf("immutability: transform tool %q changed source of block %q while a stand-off overlay was attached — source transforms must run before overlays", b.ToolName, block.ID)
	}
	return v.result(part), nil
}

// handleBlockLegacy is the transitional untyped path (WritesSource/WritesTarget
// runtime flags). Removed once every tool moves to the typed handlers.
func (b *BaseTool) handleBlockLegacy(part *model.Part) (*model.Part, error) {
	block, _ := part.Resource.(*model.Block)
	if !EnforceImmutability || block == nil {
		return b.HandleBlockFn(part)
	}

	srcBefore := blockSourceSig(block)
	hadSourceOverlay := b.WritesSource && block.HasSourceOverlays()
	var tgtBefore uint64
	if !b.WritesTarget {
		tgtBefore = blockTargetsSig(block)
	}

	result, err := b.HandleBlockFn(part)
	if err != nil {
		return result, err
	}
	// Only diff when the same Block came back (in-place mutation, the norm);
	// a tool that replaces or drops the part isn't meaningfully comparable.
	if result == nil || result.Resource != part.Resource {
		return result, nil
	}
	if blockSourceSig(block) != srcBefore {
		if !b.WritesSource {
			return nil, fmt.Errorf("immutability: tool %q changed source of block %q but does not declare WritesSource", b.ToolName, block.ID)
		}
		if hadSourceOverlay {
			return nil, fmt.Errorf("immutability: tool %q changed source of block %q while a stand-off overlay was attached — source transforms must run before overlays", b.ToolName, block.ID)
		}
	}
	if !b.WritesTarget && blockTargetsSig(block) != tgtBefore {
		return nil, fmt.Errorf("immutability: tool %q changed target of block %q but does not declare WritesTarget", b.ToolName, block.ID)
	}
	return result, nil
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
	keys := make([]string, 0, len(b.Targets))
	index := make(map[string]model.VariantKey, len(b.Targets))
	for k := range b.Targets {
		kt, _ := k.MarshalText()
		keys = append(keys, string(kt))
		index[string(kt)] = k
	}
	sort.Strings(keys)
	h := fnv.New64a()
	for _, kt := range keys {
		_, _ = h.Write([]byte(kt))
		_, _ = h.Write([]byte{0})
		if t := b.Targets[index[kt]]; t != nil {
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
