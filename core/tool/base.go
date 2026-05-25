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

	// Content-write capabilities (immutability model, AD-002). Source and
	// target are read-only to a tool by default; a tool must declare its
	// intent to write them. Analysis/annotation tools (qa, term, entity,
	// word-count, segmenter) declare neither — they emit overlays,
	// annotations, and properties, which are always writable.
	WritesSource bool // may rewrite Block.Source (source-transform; runs before overlays)
	WritesTarget bool // may write Block.Targets (translation / target edits)

	// SchemaFn optionally returns the tool's parameter schema.
	// Set this to enable schema-driven CLI flags and flow editor config panels.
	SchemaFn func() *schema.ComponentSchema

	// Override these handlers in concrete tools.
	HandleBlockFn      PartHandler
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

func (b *BaseTool) handleBlock(part *model.Part) (*model.Part, error) {
	if b.HandleBlockFn == nil {
		return part, nil
	}
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
