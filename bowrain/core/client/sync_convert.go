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
	return SyncSegment{
		ID:         seg.ID,
		Runs:       modelRunsToSync(seg.Runs),
		Properties: seg.Properties,
	}
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
	return &model.Segment{
		ID:         ws.ID,
		Runs:       syncRunsToModel(ws.Runs),
		Properties: ws.Properties,
	}
}

func modelRunsToSync(runs []model.Run) []SyncRun {
	if len(runs) == 0 {
		return nil
	}
	out := make([]SyncRun, len(runs))
	for i, r := range runs {
		out[i] = modelRunToSync(r)
	}
	return out
}

func syncRunsToModel(runs []SyncRun) []model.Run {
	if len(runs) == 0 {
		return nil
	}
	out := make([]model.Run, len(runs))
	for i, r := range runs {
		out[i] = syncRunToModel(r)
	}
	return out
}

func modelRunToSync(r model.Run) SyncRun {
	switch {
	case r.Text != nil:
		return SyncRun{Text: &SyncTextRun{Text: r.Text.Text}}
	case r.Ph != nil:
		return SyncRun{Ph: &SyncPlaceholderRun{
			ID: r.Ph.ID, Type: r.Ph.Type, SubType: r.Ph.SubType,
			Data: r.Ph.Data, Equiv: r.Ph.Equiv, Disp: r.Ph.Disp,
			Constraints: modelRunConstraintsToSync(r.Ph.Constraints),
		}}
	case r.PcOpen != nil:
		return SyncRun{PcOpen: &SyncPcOpenRun{
			ID: r.PcOpen.ID, Type: r.PcOpen.Type, SubType: r.PcOpen.SubType,
			Data: r.PcOpen.Data, Equiv: r.PcOpen.Equiv, Disp: r.PcOpen.Disp,
			Constraints: modelRunConstraintsToSync(r.PcOpen.Constraints),
		}}
	case r.PcClose != nil:
		return SyncRun{PcClose: &SyncPcCloseRun{
			ID: r.PcClose.ID, Type: r.PcClose.Type, SubType: r.PcClose.SubType,
			Data: r.PcClose.Data, Equiv: r.PcClose.Equiv,
		}}
	case r.Sub != nil:
		return SyncRun{Sub: &SyncSubRun{ID: r.Sub.ID, Ref: r.Sub.Ref, Equiv: r.Sub.Equiv}}
	case r.Plural != nil:
		forms := make(map[string][]SyncRun, len(r.Plural.Forms))
		for form, runs := range r.Plural.Forms {
			forms[string(form)] = modelRunsToSync(runs)
		}
		return SyncRun{Plural: &SyncPluralRun{Pivot: r.Plural.Pivot, Forms: forms}}
	case r.Select != nil:
		cases := make(map[string][]SyncRun, len(r.Select.Cases))
		for key, runs := range r.Select.Cases {
			cases[key] = modelRunsToSync(runs)
		}
		return SyncRun{Select: &SyncSelectRun{Pivot: r.Select.Pivot, Cases: cases}}
	}
	return SyncRun{}
}

func syncRunToModel(r SyncRun) model.Run {
	switch {
	case r.Text != nil:
		return model.Run{Text: &model.TextRun{Text: r.Text.Text}}
	case r.Ph != nil:
		return model.Run{Ph: &model.PlaceholderRun{
			ID: r.Ph.ID, Type: r.Ph.Type, SubType: r.Ph.SubType,
			Data: r.Ph.Data, Equiv: r.Ph.Equiv, Disp: r.Ph.Disp,
			Constraints: syncRunConstraintsToModel(r.Ph.Constraints),
		}}
	case r.PcOpen != nil:
		return model.Run{PcOpen: &model.PcOpenRun{
			ID: r.PcOpen.ID, Type: r.PcOpen.Type, SubType: r.PcOpen.SubType,
			Data: r.PcOpen.Data, Equiv: r.PcOpen.Equiv, Disp: r.PcOpen.Disp,
			Constraints: syncRunConstraintsToModel(r.PcOpen.Constraints),
		}}
	case r.PcClose != nil:
		return model.Run{PcClose: &model.PcCloseRun{
			ID: r.PcClose.ID, Type: r.PcClose.Type, SubType: r.PcClose.SubType,
			Data: r.PcClose.Data, Equiv: r.PcClose.Equiv,
		}}
	case r.Sub != nil:
		return model.Run{Sub: &model.SubRun{ID: r.Sub.ID, Ref: r.Sub.Ref, Equiv: r.Sub.Equiv}}
	case r.Plural != nil:
		forms := make(map[model.PluralForm][]model.Run, len(r.Plural.Forms))
		for form, runs := range r.Plural.Forms {
			forms[model.PluralForm(form)] = syncRunsToModel(runs)
		}
		return model.Run{Plural: &model.PluralRun{Pivot: r.Plural.Pivot, Forms: forms}}
	case r.Select != nil:
		cases := make(map[string][]model.Run, len(r.Select.Cases))
		for key, runs := range r.Select.Cases {
			cases[key] = syncRunsToModel(runs)
		}
		return model.Run{Select: &model.SelectRun{Pivot: r.Select.Pivot, Cases: cases}}
	}
	return model.Run{}
}

func modelRunConstraintsToSync(c *model.RunConstraints) *SyncRunConstraints {
	if c == nil {
		return nil
	}
	return &SyncRunConstraints{Deletable: c.Deletable, Cloneable: c.Cloneable, Reorderable: c.Reorderable}
}

func syncRunConstraintsToModel(c *SyncRunConstraints) *model.RunConstraints {
	if c == nil {
		return nil
	}
	return &model.RunConstraints{Deletable: c.Deletable, Cloneable: c.Cloneable, Reorderable: c.Reorderable}
}
