package client

import (
	"encoding/json"
	"strconv"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
)

// Wire-segment property keys carrying Target metadata across the JSON sync
// protocol so the round-trip of status/origin/score stays lossless.
const (
	propTargetStatus = "__status"
	propTargetScore  = "__score"
	propOriginKind   = "__origin_kind"
	propOriginEngine = "__origin_engine"
	propOriginTool   = "__origin_tool"
	propOriginRef    = "__origin_reference"
	propOriginTime   = "__origin_timestamp"
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

	// Source content — the flat run sequence rides as a single wire segment.
	if len(b.Source) > 0 {
		sync.Source = []SyncSegment{runsToWireSegment(b.Source)}
	}

	// Targets per variant. The variant key serializes to its text form
	// ("fr-FR" or "fr-FR;tone=…"); the run sequence rides as a single wire
	// segment whose properties carry any target status/origin/score.
	if len(b.Targets) > 0 {
		sync.Targets = make(map[string][]SyncSegment, len(b.Targets))
		for key, target := range b.Targets {
			if target == nil {
				continue
			}
			keyText, err := key.MarshalText()
			if err != nil {
				continue
			}
			sync.Targets[string(keyText)] = []SyncSegment{targetToWireSegment(target)}
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

// runsToWireSegment wraps a flat run sequence in a single wire segment.
func runsToWireSegment(runs []model.Run) SyncSegment {
	return SyncSegment{Runs: modelRunsToSync(runs)}
}

// targetToWireSegment encodes a committed Target as a single wire segment,
// stashing status/origin/score in segment properties so the protocol shape is
// unchanged while the round-trip remains lossless.
func targetToWireSegment(t *model.Target) SyncSegment {
	props := map[string]string{}
	if t.Status != "" {
		props[propTargetStatus] = string(t.Status)
	}
	if t.Score != 0 {
		props[propTargetScore] = strconv.FormatFloat(t.Score, 'g', -1, 64)
	}
	if t.Origin.Kind != "" {
		props[propOriginKind] = t.Origin.Kind
	}
	if t.Origin.Engine != "" {
		props[propOriginEngine] = t.Origin.Engine
	}
	if t.Origin.Tool != "" {
		props[propOriginTool] = t.Origin.Tool
	}
	if t.Origin.Reference != "" {
		props[propOriginRef] = t.Origin.Reference
	}
	if t.Origin.Timestamp != "" {
		props[propOriginTime] = t.Origin.Timestamp
	}
	if len(props) == 0 {
		props = nil
	}
	return SyncSegment{Runs: modelRunsToSync(t.Runs), Properties: props}
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

	// Source content — concatenate the runs of every wire segment back into the
	// block's flat run sequence.
	for _, seg := range sb.Source {
		b.Source = append(b.Source, syncRunsToModel(seg.Runs)...)
	}

	// If no structured source but source_text is set, create a simple run.
	if len(b.Source) == 0 && sb.SourceText != "" {
		b.SetSourceText(sb.SourceText)
	}

	// Targets — one Target per variant, runs concatenated from the wire
	// segments, status/origin/score restored from the first segment's props.
	if len(sb.Targets) > 0 {
		b.Targets = make(map[model.VariantKey]*model.Target, len(sb.Targets))
		for keyText, segs := range sb.Targets {
			var key model.VariantKey
			if err := key.UnmarshalText([]byte(keyText)); err != nil {
				continue
			}
			var runs []model.Run
			var first *SyncSegment
			for i := range segs {
				if first == nil {
					first = &segs[i]
				}
				runs = append(runs, syncRunsToModel(segs[i].Runs)...)
			}
			b.Targets[key] = wireSegmentToTarget(runs, first)
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

// wireSegmentToTarget rebuilds a Target from concatenated runs plus the first
// wire segment's metadata properties.
func wireSegmentToTarget(runs []model.Run, first *SyncSegment) *model.Target {
	t := &model.Target{Runs: runs}
	if first == nil || first.Properties == nil {
		return t
	}
	props := first.Properties
	t.Status = model.TargetStatus(props[propTargetStatus])
	if s := props[propTargetScore]; s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			t.Score = v
		}
	}
	t.Origin = model.Origin{
		Kind:      props[propOriginKind],
		Engine:    props[propOriginEngine],
		Tool:      props[propOriginTool],
		Reference: props[propOriginRef],
		Timestamp: props[propOriginTime],
	}
	return t
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
