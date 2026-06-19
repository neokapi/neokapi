package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/imageops"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/vision"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// RefuseToken is what the model is told to return when a slice is unreadable,
// instead of guessing. It maps to a review flag, never to fabricated source.
const RefuseToken = "[illegible]"

// PropNeedsReview marks a block whose source the refinement tier rewrote or
// could not read — the least-verified tier, surfaced for human review.
const PropNeedsReview = "kapi-needs-review"

// MediaRef points at the source raster a MediaSlicer slices, by reference — a
// path or already-resolved bytes, never the whole asset forced through the
// pipeline (AD-030).
type MediaRef struct {
	Path     string
	Data     []byte
	MimeType string
}

func (r MediaRef) bytes() ([]byte, error) {
	if len(r.Data) > 0 {
		return r.Data, nil
	}
	if r.Path != "" {
		return os.ReadFile(r.Path)
	}
	return nil, errors.New("media-refine: no source raster (path or bytes) available")
}

// MediaSlicer turns a block's anchor facet into a bounded media content part for
// the refinement LLM. ImageSlicer crops the geometry bbox; an AudioCutter /
// VideoClipper (Phases 6–7) cut a timing span.
type MediaSlicer interface {
	Slice(ctx context.Context, src MediaRef, b *model.Block) (aiprovider.ContentPart, error)
}

// ImageSlicer crops the block's spatial (geometry) anchor out of the source
// raster and returns it as an inline image part.
type ImageSlicer struct{}

func (ImageSlicer) Slice(_ context.Context, src MediaRef, b *model.Block) (aiprovider.ContentPart, error) {
	g, ok := b.Geometry()
	if !ok || g == nil || (g.BBox.W <= 0 && g.BBox.H <= 0) {
		return aiprovider.ContentPart{}, fmt.Errorf("media-refine: block %q has no spatial anchor to crop", b.ID)
	}
	raster, err := src.bytes()
	if err != nil {
		return aiprovider.ContentPart{}, err
	}
	crop, err := imageops.Crop(raster, int(g.BBox.X), int(g.BBox.Y), int(g.BBox.W), int(g.BBox.H))
	if err != nil {
		return aiprovider.ContentPart{}, err
	}
	return aiprovider.MediaPart(aiprovider.ContentImage, &model.Media{
		Data:     crop,
		MimeType: "image/png",
	}), nil
}

// MediaRefineTool re-reads low-confidence extracted blocks with a configurable
// multimodal LLM (AD-030). It is a source-Transform: it rewrites source, gated
// on the source Origin confidence, sending only the bounded slice to the
// provider. It overrides Process because it needs the source raster, which a
// per-block view does not carry.
type MediaRefineTool struct {
	tool.BaseTool
	provider  aiprovider.LLMProvider
	slicer    MediaSlicer
	threshold float64
	src       MediaRef
}

// Capability marks this as a source transform so the flow placement pass runs it
// in the leading source-transform stage (AD-006), even though it overrides
// Process rather than setting the Transform handler.
func (t *MediaRefineTool) Capability() tool.Capability { return tool.CapTransform }

// MediaRefineConfig configures the tool. Provider is selected explicitly — there
// is no implicit fallback.
type MediaRefineConfig struct {
	Provider  string  `json:"provider,omitempty"  schema:"title=AI Provider,description=Multimodal AI provider,default=anthropic,group=provider"`
	APIKey    string  `json:"apiKey,omitempty"    schema:"title=API Key,description=API key for the AI provider,group=provider"`
	Model     string  `json:"model,omitempty"     schema:"title=Model,description=Multimodal model name,group=provider"`
	Threshold float64 `json:"threshold,omitempty" schema:"title=Confidence Threshold,description=Re-read extracted lines whose confidence is below this,default=0.85"`
	// Source is the path to the source raster (the image the blocks were OCR'd
	// from). When empty, the tool uses a page-raster Media part from the stream.
	Source string `json:"source,omitempty" schema:"-"`
}

const defaultRefineThreshold = 0.85

// NewMediaRefineTool builds the tool from a provider + config.
func NewMediaRefineTool(provider aiprovider.LLMProvider, cfg MediaRefineConfig) *MediaRefineTool {
	thr := cfg.Threshold
	if thr <= 0 {
		thr = defaultRefineThreshold
	}
	t := &MediaRefineTool{
		provider:  provider,
		slicer:    ImageSlicer{},
		threshold: thr,
		src:       MediaRef{Path: cfg.Source},
	}
	t.ToolName = "media-refine"
	t.ToolDescription = "Re-read low-confidence extracted text with a multimodal LLM"
	return t
}

// MediaRefineSchema returns the tool's config schema.
func MediaRefineSchema() *schema.ComponentSchema {
	s := schema.FromStruct(&MediaRefineConfig{}, schema.ToolMeta{
		ID:          "media-refine",
		Category:    schema.CategoryTextProcessing,
		DisplayName: "Media Refine",
		Description: "Re-read low-confidence OCR/ASR lines with a configurable multimodal LLM",
		Tags:        []string{"ai-powered", "vision"},
		Requires:    []string{schema.RequiresCredentials},
		SideEffects: []schema.SideEffect{schema.SideEffectAPICall, schema.SideEffectRemoteSourceEgress},
	})
	injectProviderOptions(s)
	return s
}

// NewMediaRefineFromConfig is the config-factory entry point.
func NewMediaRefineFromConfig(config map[string]any, _ string) (tool.Tool, error) {
	var cfg MediaRefineConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("media-refine config: %w", err)
	}
	p, err := ProviderFromConfig(cfg.Provider, aiprovider.Config{APIKey: cfg.APIKey, Model: cfg.Model})
	if err != nil {
		return nil, err
	}
	return NewMediaRefineTool(p, cfg), nil
}

// Process buffers the page's parts, refines the gated blocks against the source
// raster, then emits every part in its original order.
func (t *MediaRefineTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	var parts []*model.Part
	src := t.src
	for p := range in {
		// Pick up a page raster from the stream if no explicit source is set.
		if src.Path == "" && len(src.Data) == 0 && p.Type == model.PartMedia {
			if m, ok := p.Resource.(*model.Media); ok && m.Properties[vision.PageRasterProperty] == "page" {
				src = MediaRef{Path: m.URI, Data: m.Data, MimeType: m.MimeType}
			}
		}
		parts = append(parts, p)
	}

	blocks := blockResources(parts)
	for i, b := range blocks {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !t.gated(b) {
			continue
		}
		if err := t.refine(ctx, b, blocks, i, src); err != nil {
			// A refinement failure must not drop the block — keep the original
			// OCR text and flag it for review.
			setProp(b, PropNeedsReview, "refine-error: "+err.Error())
		}
	}

	for _, p := range parts {
		select {
		case out <- p:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// gated reports whether a block should be re-read: it was produced by OCR and its
// confidence is below the threshold.
func (t *MediaRefineTool) gated(b *model.Block) bool {
	o, ok := b.SourceOrigin()
	if !ok || o.Kind != model.OriginOCR {
		return false
	}
	return o.Confidence < t.threshold
}

func (t *MediaRefineTool) refine(ctx context.Context, b *model.Block, all []*model.Block, idx int, src MediaRef) error {
	// Capability check: the chosen provider must accept the slice's modality.
	if !modalitySupported(t.provider, aiprovider.ModalityImage) {
		return fmt.Errorf("provider %q does not accept image input", t.provider.Name())
	}
	part, err := t.slicer.Slice(ctx, src, b)
	if err != nil {
		return err
	}

	msgs := []aiprovider.Message{
		aiprovider.TextMessage("system",
			"You transcribe a single line cropped from a document image. "+
				"Return only the exact text you read, with no commentary. "+
				"If the crop is unreadable, return "+RefuseToken+"."),
		{Role: "user", Parts: []aiprovider.ContentPart{
			aiprovider.TextPart(refineContext(all, idx)),
			part,
		}},
	}

	resp, err := t.provider.Chat(ctx, msgs)
	if err != nil {
		return err
	}
	refined := strings.TrimSpace(resp.Content)
	original := strings.TrimSpace(b.SourceText())

	if refined == "" || refined == RefuseToken {
		setProp(b, PropNeedsReview, "illegible")
		return nil
	}

	// Record provenance: the source is now LLM-produced. Flag for review when the
	// LLM disagrees with the original guess — the least-verified tier.
	if refined != original {
		b.SetSourceText(refined)
		setProp(b, PropNeedsReview, "llm-rewrite")
	}
	b.SetSourceOrigin(&model.Origin{
		Kind:       model.OriginOCR,
		Engine:     "llm:" + string(t.provider.Name()),
		Confidence: 1,
	})
	return nil
}

// refineContext gathers the immediately neighbouring block texts as a plain-text
// hint, so the LLM has the language prior without shipping the whole page.
func refineContext(all []*model.Block, idx int) string {
	var b strings.Builder
	b.WriteString("Surrounding lines for context:\n")
	for i := idx - 2; i <= idx+2; i++ {
		if i < 0 || i >= len(all) || i == idx {
			continue
		}
		if txt := strings.TrimSpace(all[i].SourceText()); txt != "" {
			b.WriteString("- ")
			b.WriteString(txt)
			b.WriteByte('\n')
		}
	}
	b.WriteString("\nTranscribe the highlighted line in the image:")
	return b.String()
}

// blockResources extracts the Block resources from a part slice, in order.
func blockResources(parts []*model.Part) []*model.Block {
	var out []*model.Block
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if b, ok := p.Resource.(*model.Block); ok {
				out = append(out, b)
			}
		}
	}
	return out
}

// setProp sets a block property, initializing the map on first use.
func setProp(b *model.Block, key, val string) {
	if b.Properties == nil {
		b.Properties = map[string]string{}
	}
	b.Properties[key] = val
}

// modalitySupported reports whether the provider accepts the given modality.
func modalitySupported(p aiprovider.LLMProvider, m aiprovider.Modality) bool {
	return slices.Contains(p.InputModalities(), m)
}
