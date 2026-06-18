package pluginhost

import (
	"context"
	"os"
	"strconv"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/structure"
	"github.com/neokapi/neokapi/core/vision"
)

// tier3Reader wraps a plugin-format daemon reader with the host-side vision
// tier-3 pass. When the format config requests "tier3" and the kapi-vision
// layout engine is available, the plugin renders each page to a raster (emitted
// as a Media part marked vision.PageRasterProperty) and this wrapper runs the
// layout model over it to produce authoritative structure (multi-column reading
// order, borderless tables, figure/caption regions) over the page's own
// positioned blocks. Without vision it strips the tier3 request so the plugin
// falls back to its geometric structure (tier 2); when tier3 isn't requested it
// is a transparent pass-through, so wrapping every plugin reader is free.
//
// It embeds *daemonReader, promoting Open/Signature/Config/Close so config still
// flows to the shared mapConfig; only Read is overridden.
type tier3Reader struct {
	*daemonReader
}

func newTier3Reader(inner *daemonReader) *tier3Reader { return &tier3Reader{daemonReader: inner} }

func (r *tier3Reader) tier3Requested() bool {
	cfg, ok := r.Cfg.(*mapConfig)
	return ok && cfg.params["tier3"] == "true"
}

// Read drives the inner daemon reader and, when tier-3 is active, restructures
// each page from its raster + blocks via the vision layout model.
func (r *tier3Reader) Read(ctx context.Context) <-chan model.PartResult {
	if !r.tier3Requested() {
		return r.daemonReader.Read(ctx)
	}
	if !vision.Available("") {
		// No layout engine: don't make the plugin render rasters it can't use —
		// drop the request so it falls back to tier-2.
		if cfg, ok := r.Cfg.(*mapConfig); ok {
			delete(cfg.params, "tier3")
		}
		return r.daemonReader.Read(ctx)
	}

	out := make(chan model.PartResult, 64)
	go func() {
		defer close(out)
		var le vision.LayoutEngine
		if eng, err := vision.Open(""); err == nil {
			defer func() { _ = eng.Close() }()
			le, _ = eng.(vision.LayoutEngine)
		}
		enrichTier3(ctx, r.daemonReader.Read(ctx), out, le)
	}()
	return out
}

// enrichTier3 forwards the part stream, replacing each page's raster + raw blocks
// with vision tier-3 structure. Pages are the depth-2 layers (depth-1 is the
// document root); their Media raster is consumed (and deleted) and their blocks
// are restructured. Everything else passes through untouched.
func enrichTier3(ctx context.Context, in <-chan model.PartResult, out chan<- model.PartResult, le vision.LayoutEngine) {
	emit := func(pr model.PartResult) bool {
		select {
		case out <- pr:
			return true
		case <-ctx.Done():
			return false
		}
	}

	depth := 0
	var blocks []*model.Block
	var raster *model.Media
	counter, groupCounter := 0, 0

	for res := range in {
		if res.Error != nil {
			if !emit(res) {
				return
			}
			continue
		}
		p := res.Part
		if p == nil {
			continue
		}
		switch p.Type {
		case model.PartLayerStart:
			depth++
			if depth == 2 {
				blocks, raster = nil, nil
			}
			if !emit(res) {
				return
			}
		case model.PartLayerEnd:
			if depth == 2 {
				for _, pp := range structurePage(ctx, le, raster, blocks, &counter, &groupCounter) {
					if !emit(model.PartResult{Part: pp}) {
						return
					}
				}
				if raster != nil && raster.URI != "" {
					_ = os.Remove(raster.URI)
				}
				blocks, raster = nil, nil
			}
			depth--
			if !emit(res) {
				return
			}
		case model.PartMedia:
			if depth == 2 {
				if m, ok := p.Resource.(*model.Media); ok && m.Properties[vision.PageRasterProperty] == "page" {
					raster = m // consume the raster; not forwarded
					continue
				}
			}
			if !emit(res) {
				return
			}
		case model.PartBlock:
			if depth == 2 {
				if b, ok := p.Resource.(*model.Block); ok {
					blocks = append(blocks, b)
					continue
				}
			}
			if !emit(res) {
				return
			}
		default:
			if !emit(res) {
				return
			}
		}
	}
}

// structurePage builds a page's structured parts: tier-3 from the layout model
// over the raster when all of (engine, raster, blocks) are present, otherwise the
// geometric tier-2 over the same blocks (exactly what the plugin would have
// emitted without tier3).
func structurePage(ctx context.Context, le vision.LayoutEngine, raster *model.Media, blocks []*model.Block, counter, groupCounter *int) []*model.Part {
	if len(blocks) == 0 {
		return nil
	}
	if le != nil && raster != nil && raster.URI != "" {
		w, _ := strconv.Atoi(raster.Properties["width"])
		h, _ := strconv.Atoi(raster.Properties["height"])
		if parts, err := vision.StructureFromLayout(ctx, le, raster.URI, blocks, w, h, vision.LayoutOptions{}, counter, groupCounter); err == nil && parts != nil {
			return parts
		}
	}
	return structure.ToParts(structure.Analyze(blocks), groupCounter)
}
