package editor

import (
	"fmt"
	"maps"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// BuildBlockIndex creates a BlockIndex from a Part stream.
// It captures blocks with skeletons, data parts, layers, and document ordering.
func BuildBlockIndex(parts []*model.Part, sourceLocale, format, itemName string) *BlockIndex {
	index := &BlockIndex{
		Version:        "1.0",
		SourceLocale:   sourceLocale,
		OriginalFormat: format,
		OriginalItem:   itemName,
		Blocks:         []Block{},
		DataParts:      []DataPart{},
		DocumentOrder:  []string{},
		Layers:         []LayerInfo{},
	}

	blockIdx := 0
	for _, part := range parts {
		switch part.Type {
		case model.PartLayerStart:
			layer, ok := part.Resource.(*model.Layer)
			if !ok {
				continue
			}
			index.Layers = append(index.Layers, LayerInfo{
				ID:       layer.ID,
				Name:     layer.Name,
				Format:   layer.Format,
				Locale:   string(layer.Locale),
				Encoding: layer.Encoding,
			})
			index.DocumentOrder = append(index.DocumentOrder, "layer_start:"+layer.ID)

		case model.PartLayerEnd:
			layer, ok := part.Resource.(*model.Layer)
			if !ok {
				continue
			}
			index.DocumentOrder = append(index.DocumentOrder, "layer_end:"+layer.ID)

		case model.PartBlock:
			block, ok := part.Resource.(*model.Block)
			if !ok {
				continue
			}

			b := Block{
				ID:           block.ID,
				Index:        blockIdx,
				Translatable: block.Translatable,
				Source:       block.SourceText(),
				SourceHTML:   renderFragmentHTML(block),
				Targets:      make(map[string]string),
				Skeleton:     convertSkeleton(block.Skeleton),
				Properties:   copyProps(block.Properties),
			}

			for locale, segs := range block.Targets {
				if len(segs) > 0 {
					var buf strings.Builder
					for _, seg := range segs {
						buf.WriteString(seg.Content.Text())
					}
					b.Targets[string(locale)] = buf.String()
				}
			}

			index.Blocks = append(index.Blocks, b)
			index.DocumentOrder = append(index.DocumentOrder, "block:"+block.ID)
			blockIdx++

		case model.PartData:
			data, ok := part.Resource.(*model.Data)
			if !ok {
				continue
			}
			dp := DataPart{
				ID:         data.ID,
				Name:       data.Name,
				Skeleton:   convertSkeleton(data.Skeleton),
				Properties: copyProps(data.Properties),
			}
			index.DataParts = append(index.DataParts, dp)
			index.DocumentOrder = append(index.DocumentOrder, "data:"+data.ID)
		}
	}

	return index
}

// renderFragmentHTML renders a block's source fragment with inline span markup as HTML.
func renderFragmentHTML(block *model.Block) string {
	if len(block.Source) == 0 {
		return ""
	}
	frag := block.Source[0].Content
	if frag == nil {
		return ""
	}
	if !frag.HasSpans() {
		return frag.CodedText
	}

	var buf strings.Builder
	spanIdx := 0
	for _, r := range frag.CodedText {
		switch r {
		case model.MarkerOpening, model.MarkerClosing, model.MarkerPlaceholder:
			if spanIdx < len(frag.Spans) {
				buf.WriteString(frag.Spans[spanIdx].Data)
				spanIdx++
			}
		default:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// convertSkeleton converts a model.Skeleton to a serializable SkeletonData.
func convertSkeleton(skel *model.Skeleton) *SkeletonData {
	if skel == nil {
		return nil
	}

	strategy := "fragment"
	if skel.Strategy == model.SkeletonReparse {
		strategy = "reparse"
	}

	sd := &SkeletonData{
		Strategy: strategy,
	}

	for _, sp := range skel.Parts {
		switch p := sp.(type) {
		case *model.SkeletonText:
			sd.Parts = append(sd.Parts, SkeletonPartData{
				Type: "text",
				Text: p.Text,
			})
		case *model.SkeletonRef:
			sd.Parts = append(sd.Parts, SkeletonPartData{
				Type:       "ref",
				ResourceID: p.ResourceID,
				Property:   p.Property,
			})
		}
	}

	return sd
}

// copyProps creates a shallow copy of a string map.
func copyProps(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(m))
	maps.Copy(out, m)
	return out
}

// BlockByID returns the block with the given ID, or nil if not found.
func (bi *BlockIndex) BlockByID(id string) *Block {
	for i := range bi.Blocks {
		if bi.Blocks[i].ID == id {
			return &bi.Blocks[i]
		}
	}
	return nil
}

// UpdateTarget sets the target translation for a block in the given locale.
func (bi *BlockIndex) UpdateTarget(blockID, locale, text string) error {
	b := bi.BlockByID(blockID)
	if b == nil {
		return fmt.Errorf("block %q not found", blockID)
	}
	if b.Targets == nil {
		b.Targets = make(map[string]string)
	}
	b.Targets[locale] = text
	return nil
}
