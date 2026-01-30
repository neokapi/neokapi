package kaz

import (
	"strings"

	"github.com/gokapi/gokapi/core/model"
)

// ReconstructParts rebuilds a Part stream from a BlockIndex.
// This allows document reconstruction WITHOUT the original source item.
// Uses document_order + blocks (with skeletons) + data_parts + layers.
func ReconstructParts(index *BlockIndex) []*model.Part {
	// Build lookup maps
	blockMap := make(map[string]*Block, len(index.Blocks))
	for i := range index.Blocks {
		blockMap[index.Blocks[i].ID] = &index.Blocks[i]
	}

	dataMap := make(map[string]*DataPart, len(index.DataParts))
	for i := range index.DataParts {
		dataMap[index.DataParts[i].ID] = &index.DataParts[i]
	}

	layerMap := make(map[string]*LayerInfo, len(index.Layers))
	for i := range index.Layers {
		layerMap[index.Layers[i].ID] = &index.Layers[i]
	}

	var parts []*model.Part

	for _, ref := range index.DocumentOrder {
		colonIdx := strings.IndexByte(ref, ':')
		if colonIdx < 0 {
			continue
		}
		refType := ref[:colonIdx]
		refID := ref[colonIdx+1:]

		switch refType {
		case "layer_start":
			li := layerMap[refID]
			if li == nil {
				continue
			}
			parts = append(parts, &model.Part{
				Type: model.PartLayerStart,
				Resource: &model.Layer{
					ID:       li.ID,
					Name:     li.Name,
					Format:   li.Format,
					Locale:   model.LocaleID(li.Locale),
					Encoding: li.Encoding,
				},
			})

		case "layer_end":
			li := layerMap[refID]
			if li == nil {
				continue
			}
			parts = append(parts, &model.Part{
				Type: model.PartLayerEnd,
				Resource: &model.Layer{
					ID:       li.ID,
					Name:     li.Name,
					Format:   li.Format,
					Locale:   model.LocaleID(li.Locale),
					Encoding: li.Encoding,
				},
			})

		case "block":
			b := blockMap[refID]
			if b == nil {
				continue
			}
			block := &model.Block{
				ID:           b.ID,
				Translatable: b.Translatable,
				Source:       []*model.Segment{{ID: "s1", Content: model.NewFragment(b.Source)}},
				Targets:      make(map[model.LocaleID][]*model.Segment),
				Properties:   copyProps(b.Properties),
				Annotations:  make(map[string]model.Annotation),
				Skeleton:     reconstructSkeleton(b.Skeleton),
			}

			for locale, text := range b.Targets {
				block.Targets[model.LocaleID(locale)] = []*model.Segment{
					{ID: "s1", Content: model.NewFragment(text)},
				}
			}

			parts = append(parts, &model.Part{
				Type:     model.PartBlock,
				Resource: block,
			})

		case "data":
			dp := dataMap[refID]
			if dp == nil {
				continue
			}
			data := &model.Data{
				ID:         dp.ID,
				Name:       dp.Name,
				Properties: copyProps(dp.Properties),
				Skeleton:   reconstructSkeleton(dp.Skeleton),
			}
			parts = append(parts, &model.Part{
				Type:     model.PartData,
				Resource: data,
			})
		}
	}

	return parts
}

// reconstructSkeleton converts serialized SkeletonData back to a model.Skeleton.
func reconstructSkeleton(sd *SkeletonData) *model.Skeleton {
	if sd == nil {
		return nil
	}

	strategy := model.SkeletonFragmentBased
	if sd.Strategy == "reparse" {
		strategy = model.SkeletonReparse
	}

	skel := &model.Skeleton{
		Strategy: strategy,
	}

	for _, sp := range sd.Parts {
		switch sp.Type {
		case "text":
			skel.Parts = append(skel.Parts, &model.SkeletonText{Text: sp.Text})
		case "ref":
			skel.Parts = append(skel.Parts, &model.SkeletonRef{
				ResourceID: sp.ResourceID,
				Property:   sp.Property,
			})
		}
	}

	return skel
}
