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
		SourceLanguage: sourceLocale,
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

			for _, locale := range block.TargetLocales() {
				if text := block.TargetText(locale); text != "" {
					b.Targets[string(locale)] = text
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

// renderFragmentHTML renders a block's source content with inline
// markup as HTML. Walks the block's Runs directly: TextRun content
// is emitted verbatim; Ph / PcOpen / PcClose / Sub runs emit their
// raw Data payload; Plural / Select recurse through their forms or
// cases (using the 'other' branch, then any branch if 'other' is
// absent) so flattened HTML stays non-empty.
func renderFragmentHTML(block *model.Block) string {
	if block == nil {
		return ""
	}
	runs := block.SourceRuns()
	if len(runs) == 0 {
		return ""
	}
	var buf strings.Builder
	renderRunsHTML(&buf, runs)
	return buf.String()
}

func renderRunsHTML(buf *strings.Builder, runs []model.Run) {
	for _, r := range runs {
		switch {
		case r.Text != nil:
			buf.WriteString(r.Text.Text)
		case r.Ph != nil:
			buf.WriteString(r.Ph.Data)
		case r.PcOpen != nil:
			buf.WriteString(r.PcOpen.Data)
		case r.PcClose != nil:
			buf.WriteString(r.PcClose.Data)
		case r.Sub != nil:
			// Subblock references render as their equiv label.
			buf.WriteString(r.Sub.Equiv)
		case r.Plural != nil:
			if form, ok := r.Plural.Forms[model.PluralOther]; ok {
				renderRunsHTML(buf, form)
				continue
			}
			for _, form := range r.Plural.Forms {
				renderRunsHTML(buf, form)
				break
			}
		case r.Select != nil:
			if form, ok := r.Select.Cases["other"]; ok {
				renderRunsHTML(buf, form)
				continue
			}
			for _, form := range r.Select.Cases {
				renderRunsHTML(buf, form)
				break
			}
		}
	}
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
