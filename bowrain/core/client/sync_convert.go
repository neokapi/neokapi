package client

import (
	"encoding/json"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
)

// StoredBlockToSyncBlock converts a StoredBlock to the JSON wire type.
func StoredBlockToSyncBlock(sb *store.StoredBlock) SyncBlock {
	b := sb.Block
	sync := SyncBlock{
		ID:                 b.ID,
		ItemName:           sb.ItemName,
		Name:               b.Name,
		Type:               b.Type,
		MimeType:           b.MimeType,
		Translatable:       b.Translatable,
		SourceText:         b.SourceText(),
		PreserveWhitespace: b.PreserveWhitespace,
		Properties:         b.Properties,
		ContentHash:        sb.ContentHash,
	}

	// Source segments.
	for _, seg := range b.Source {
		sync.Source = append(sync.Source, segmentToWire(seg))
	}

	// Targets per locale.
	if len(b.Targets) > 0 {
		sync.Targets = make(map[string][]SyncSegment, len(b.Targets))
		for locale, segs := range b.Targets {
			wireSegs := make([]SyncSegment, 0, len(segs))
			for _, seg := range segs {
				wireSegs = append(wireSegs, segmentToWire(seg))
			}
			sync.Targets[string(locale)] = wireSegs
		}
	}

	// Annotations.
	if len(b.Annotations) > 0 {
		data, _ := json.Marshal(b.Annotations)
		sync.Annotations = data
	}

	// Skeleton.
	if b.Skeleton != nil {
		data, _ := json.Marshal(b.Skeleton)
		sync.Skeleton = data
	}

	// Display hint.
	if b.DisplayHint != nil {
		data, _ := json.Marshal(b.DisplayHint)
		sync.DisplayHint = data
	}

	// Content ref.
	if b.ContentRef != nil {
		data, _ := json.Marshal(b.ContentRef)
		sync.ContentRef = data
	}

	return sync
}

// BlockToSyncBlock converts a model.Block and item name to the JSON wire type.
func BlockToSyncBlock(b *model.Block, itemName string) SyncBlock {
	sb := &store.StoredBlock{
		Block:    b,
		ItemName: itemName,
	}
	identity := model.ComputeIdentity(b)
	sb.ContentHash = identity.ContentHash
	return StoredBlockToSyncBlock(sb)
}

// AssetToSyncMedia converts a store.Asset to the JSON wire type.
func AssetToSyncMedia(a *store.Asset) SyncMedia {
	return SyncMedia{
		ID:         a.ID,
		ItemName:   a.ItemName,
		MimeType:   a.MimeType,
		Filename:   a.Filename,
		AltText:    a.AltText,
		SizeBytes:  a.SizeBytes,
		BlobKey:    a.BlobKey,
		Properties: a.Properties,
	}
}

func segmentToWire(seg *model.Segment) SyncSegment {
	ws := SyncSegment{
		ID:         seg.ID,
		Properties: seg.Properties,
	}
	if seg.Content != nil {
		ws.Text = seg.Content.Text()
		ws.CodedText = seg.Content.CodedText
		for _, span := range seg.Content.Spans {
			ws.Spans = append(ws.Spans, SyncSpan{
				ID:       span.ID,
				Type:     span.Type,
				SubType:  span.SubType,
				SpanType: span.SpanType.String(),
				Data:     span.Data,
			})
		}
	}
	return ws
}

// SyncBlockToBlock converts a SyncBlock wire type back to a model.Block.
func SyncBlockToBlock(sb SyncBlock) *model.Block {
	b := &model.Block{
		ID:                 sb.ID,
		Name:               sb.Name,
		Type:               sb.Type,
		MimeType:           sb.MimeType,
		Translatable:       sb.Translatable,
		PreserveWhitespace: sb.PreserveWhitespace,
		Properties:         sb.Properties,
	}

	// Source segments.
	for _, seg := range sb.Source {
		b.Source = append(b.Source, wireToSegment(seg))
	}

	// If no structured source but source_text is set, create a simple segment.
	if len(b.Source) == 0 && sb.SourceText != "" {
		b.SetSourceText(sb.SourceText)
	}

	// Targets.
	if len(sb.Targets) > 0 {
		b.Targets = make(map[model.LocaleID][]*model.Segment, len(sb.Targets))
		for locale, segs := range sb.Targets {
			for _, seg := range segs {
				b.Targets[model.LocaleID(locale)] = append(b.Targets[model.LocaleID(locale)], wireToSegment(seg))
			}
		}
	}

	// Annotations.
	if len(sb.Annotations) > 0 {
		_ = json.Unmarshal(sb.Annotations, &b.Annotations)
	}

	// Skeleton.
	if len(sb.Skeleton) > 0 {
		b.Skeleton = &model.Skeleton{}
		_ = json.Unmarshal(sb.Skeleton, b.Skeleton)
	}

	// Display hint.
	if len(sb.DisplayHint) > 0 {
		b.DisplayHint = &model.DisplayHint{}
		_ = json.Unmarshal(sb.DisplayHint, b.DisplayHint)
	}

	// Content ref.
	if len(sb.ContentRef) > 0 {
		b.ContentRef = &model.ContentRef{}
		_ = json.Unmarshal(sb.ContentRef, b.ContentRef)
	}

	return b
}

func wireToSegment(ws SyncSegment) *model.Segment {
	seg := &model.Segment{
		ID:         ws.ID,
		Properties: ws.Properties,
	}
	frag := &model.Fragment{
		CodedText: ws.CodedText,
	}
	if frag.CodedText == "" {
		frag.CodedText = ws.Text
	}
	for _, sp := range ws.Spans {
		frag.Spans = append(frag.Spans, &model.Span{
			ID:       sp.ID,
			Type:     sp.Type,
			SubType:  sp.SubType,
			SpanType: parseSpanType(sp.SpanType),
			Data:     sp.Data,
		})
	}
	seg.Content = frag
	return seg
}

func parseSpanType(s string) model.SpanType {
	switch s {
	case "Opening":
		return model.SpanOpening
	case "Closing":
		return model.SpanClosing
	case "Placeholder":
		return model.SpanPlaceholder
	default:
		return model.SpanPlaceholder
	}
}
