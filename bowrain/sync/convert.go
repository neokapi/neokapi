// Package sync provides converters between the Go content model and the
// sync protocol protobuf types (AD-038).
package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"

	pb "github.com/neokapi/neokapi/bowrain/proto/v1"
	"github.com/neokapi/neokapi/core/model"
)

// BlockToProto converts a model.Block to a SyncBlock protobuf message.
func BlockToProto(b *model.Block, itemName string) *pb.SyncBlock {
	sb := &pb.SyncBlock{
		Id:                 b.ID,
		ItemName:           itemName,
		Name:               b.Name,
		Type:               b.Type,
		MimeType:           b.MimeType,
		Translatable:       b.Translatable,
		SourceText:         b.SourceText(),
		PreserveWhitespace: b.PreserveWhitespace,
		Properties:         b.Properties,
	}

	// Source segments.
	for _, seg := range b.Source {
		sb.Source = append(sb.Source, segmentToProto(seg))
	}

	// Targets per locale.
	if len(b.Targets) > 0 {
		sb.Targets = make(map[string]*pb.SyncSegmentList, len(b.Targets))
		for locale, segs := range b.Targets {
			list := &pb.SyncSegmentList{}
			for _, seg := range segs {
				list.Segments = append(list.Segments, segmentToProto(seg))
			}
			sb.Targets[string(locale)] = list
		}
	}

	// Annotations (serialized as JSON bytes for extensibility).
	if len(b.Annotations) > 0 {
		data, _ := json.Marshal(b.Annotations)
		sb.AnnotationsJson = data
	}

	// Skeleton.
	if b.Skeleton != nil {
		data, _ := json.Marshal(b.Skeleton)
		sb.SkeletonJson = data
	}

	// Display hint.
	if b.DisplayHint != nil {
		data, _ := json.Marshal(b.DisplayHint)
		sb.DisplayHintJson = data
	}

	// Content ref.
	if b.ContentRef != nil {
		data, _ := json.Marshal(b.ContentRef)
		sb.ContentRefJson = data
	}

	// Content hash for diff computation.
	identity := model.ComputeIdentity(b)
	sb.ContentHash = identity.ContentHash

	return sb
}

// ProtoToBlock converts a SyncBlock protobuf message to a model.Block.
func ProtoToBlock(sb *pb.SyncBlock) *model.Block {
	b := &model.Block{
		ID:                 sb.Id,
		Name:               sb.Name,
		Type:               sb.Type,
		MimeType:           sb.MimeType,
		Translatable:       sb.Translatable,
		PreserveWhitespace: sb.PreserveWhitespace,
		Properties:         sb.Properties,
	}

	// Source segments.
	for _, seg := range sb.Source {
		b.Source = append(b.Source, protoToSegment(seg))
	}

	// If no structured source but source_text is set, create a simple segment.
	if len(b.Source) == 0 && sb.SourceText != "" {
		b.SetSourceText(sb.SourceText)
	}

	// Targets.
	if len(sb.Targets) > 0 {
		b.Targets = make(map[model.LocaleID][]*model.Segment, len(sb.Targets))
		for locale, list := range sb.Targets {
			for _, seg := range list.Segments {
				b.Targets[model.LocaleID(locale)] = append(b.Targets[model.LocaleID(locale)], protoToSegment(seg))
			}
		}
	}

	// Annotations.
	if len(sb.AnnotationsJson) > 0 {
		_ = json.Unmarshal(sb.AnnotationsJson, &b.Annotations)
	}

	// Skeleton.
	if len(sb.SkeletonJson) > 0 {
		b.Skeleton = &model.Skeleton{}
		_ = json.Unmarshal(sb.SkeletonJson, b.Skeleton)
	}

	// Display hint.
	if len(sb.DisplayHintJson) > 0 {
		b.DisplayHint = &model.DisplayHint{}
		_ = json.Unmarshal(sb.DisplayHintJson, b.DisplayHint)
	}

	// Content ref.
	if len(sb.ContentRefJson) > 0 {
		b.ContentRef = &model.ContentRef{}
		_ = json.Unmarshal(sb.ContentRefJson, b.ContentRef)
	}

	return b
}

// segmentToProto converts a model.Segment to proto.
func segmentToProto(seg *model.Segment) *pb.SyncSegment {
	return &pb.SyncSegment{
		Id:         seg.ID,
		Runs:       runsToSyncProto(seg.Runs),
		Properties: seg.Properties,
	}
}

// protoToSegment converts a proto segment to model.Segment.
func protoToSegment(ps *pb.SyncSegment) *model.Segment {
	seg := &model.Segment{
		ID:         ps.Id,
		Properties: ps.Properties,
	}
	seg.SetRuns(syncProtoToRuns(ps.Runs))
	return seg
}

// runsToSyncProto converts a run sequence into the wire form.
func runsToSyncProto(runs []model.Run) []*pb.SyncRun {
	if len(runs) == 0 {
		return nil
	}
	out := make([]*pb.SyncRun, len(runs))
	for i, r := range runs {
		out[i] = runToSyncProto(r)
	}
	return out
}

// syncProtoToRuns converts wire runs into model.Run form.
func syncProtoToRuns(msgs []*pb.SyncRun) []model.Run {
	if len(msgs) == 0 {
		return nil
	}
	out := make([]model.Run, len(msgs))
	for i, m := range msgs {
		out[i] = syncProtoToRun(m)
	}
	return out
}

func runToSyncProto(r model.Run) *pb.SyncRun {
	switch {
	case r.Text != nil:
		return &pb.SyncRun{Kind: &pb.SyncRun_Text{Text: &pb.SyncTextRun{Text: r.Text.Text}}}
	case r.Ph != nil:
		return &pb.SyncRun{Kind: &pb.SyncRun_Ph{Ph: &pb.SyncPlaceholderRun{
			Id: r.Ph.ID, Type: r.Ph.Type, SubType: r.Ph.SubType,
			Data: r.Ph.Data, Equiv: r.Ph.Equiv, Disp: r.Ph.Disp,
			Constraints: runConstraintsToSyncProto(r.Ph.Constraints),
		}}}
	case r.PcOpen != nil:
		return &pb.SyncRun{Kind: &pb.SyncRun_PcOpen{PcOpen: &pb.SyncPcOpenRun{
			Id: r.PcOpen.ID, Type: r.PcOpen.Type, SubType: r.PcOpen.SubType,
			Data: r.PcOpen.Data, Equiv: r.PcOpen.Equiv, Disp: r.PcOpen.Disp,
			Constraints: runConstraintsToSyncProto(r.PcOpen.Constraints),
		}}}
	case r.PcClose != nil:
		return &pb.SyncRun{Kind: &pb.SyncRun_PcClose{PcClose: &pb.SyncPcCloseRun{
			Id: r.PcClose.ID, Type: r.PcClose.Type, SubType: r.PcClose.SubType,
			Data: r.PcClose.Data, Equiv: r.PcClose.Equiv,
		}}}
	case r.Sub != nil:
		return &pb.SyncRun{Kind: &pb.SyncRun_Sub{Sub: &pb.SyncSubRun{
			Id: r.Sub.ID, Ref: r.Sub.Ref, Equiv: r.Sub.Equiv,
		}}}
	case r.Plural != nil:
		forms := make(map[string]*pb.SyncRunList, len(r.Plural.Forms))
		for form, runs := range r.Plural.Forms {
			forms[string(form)] = &pb.SyncRunList{Runs: runsToSyncProto(runs)}
		}
		return &pb.SyncRun{Kind: &pb.SyncRun_Plural{Plural: &pb.SyncPluralRun{
			Pivot: r.Plural.Pivot, Forms: forms,
		}}}
	case r.Select != nil:
		cases := make(map[string]*pb.SyncRunList, len(r.Select.Cases))
		for key, runs := range r.Select.Cases {
			cases[key] = &pb.SyncRunList{Runs: runsToSyncProto(runs)}
		}
		return &pb.SyncRun{Kind: &pb.SyncRun_Select{Select: &pb.SyncSelectRun{
			Pivot: r.Select.Pivot, Cases: cases,
		}}}
	}
	return nil
}

func syncProtoToRun(msg *pb.SyncRun) model.Run {
	if msg == nil {
		return model.Run{}
	}
	switch k := msg.Kind.(type) {
	case *pb.SyncRun_Text:
		return model.Run{Text: &model.TextRun{Text: k.Text.GetText()}}
	case *pb.SyncRun_Ph:
		return model.Run{Ph: &model.PlaceholderRun{
			ID: k.Ph.GetId(), Type: k.Ph.GetType(), SubType: k.Ph.GetSubType(),
			Data: k.Ph.GetData(), Equiv: k.Ph.GetEquiv(), Disp: k.Ph.GetDisp(),
			Constraints: syncProtoToRunConstraints(k.Ph.GetConstraints()),
		}}
	case *pb.SyncRun_PcOpen:
		return model.Run{PcOpen: &model.PcOpenRun{
			ID: k.PcOpen.GetId(), Type: k.PcOpen.GetType(), SubType: k.PcOpen.GetSubType(),
			Data: k.PcOpen.GetData(), Equiv: k.PcOpen.GetEquiv(), Disp: k.PcOpen.GetDisp(),
			Constraints: syncProtoToRunConstraints(k.PcOpen.GetConstraints()),
		}}
	case *pb.SyncRun_PcClose:
		return model.Run{PcClose: &model.PcCloseRun{
			ID: k.PcClose.GetId(), Type: k.PcClose.GetType(), SubType: k.PcClose.GetSubType(),
			Data: k.PcClose.GetData(), Equiv: k.PcClose.GetEquiv(),
		}}
	case *pb.SyncRun_Sub:
		return model.Run{Sub: &model.SubRun{
			ID: k.Sub.GetId(), Ref: k.Sub.GetRef(), Equiv: k.Sub.GetEquiv(),
		}}
	case *pb.SyncRun_Plural:
		forms := make(map[model.PluralForm][]model.Run, len(k.Plural.GetForms()))
		for form, runList := range k.Plural.GetForms() {
			forms[model.PluralForm(form)] = syncProtoToRuns(runList.GetRuns())
		}
		return model.Run{Plural: &model.PluralRun{Pivot: k.Plural.GetPivot(), Forms: forms}}
	case *pb.SyncRun_Select:
		cases := make(map[string][]model.Run, len(k.Select.GetCases()))
		for key, runList := range k.Select.GetCases() {
			cases[key] = syncProtoToRuns(runList.GetRuns())
		}
		return model.Run{Select: &model.SelectRun{Pivot: k.Select.GetPivot(), Cases: cases}}
	}
	return model.Run{}
}

func runConstraintsToSyncProto(c *model.RunConstraints) *pb.SyncRunConstraints {
	if c == nil {
		return nil
	}
	return &pb.SyncRunConstraints{Deletable: c.Deletable, Cloneable: c.Cloneable, Reorderable: c.Reorderable}
}

func syncProtoToRunConstraints(msg *pb.SyncRunConstraints) *model.RunConstraints {
	if msg == nil {
		return nil
	}
	return &model.RunConstraints{Deletable: msg.GetDeletable(), Cloneable: msg.GetCloneable(), Reorderable: msg.GetReorderable()}
}

// ComputeItemHash computes the Merkle hash for an item by hashing
// all its block content hashes in sorted order.
func ComputeItemHash(blockHashes map[string]string) string {
	ids := make([]string, 0, len(blockHashes))
	for id := range blockHashes {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	h := sha256.New()
	for _, id := range ids {
		h.Write([]byte(id))
		h.Write([]byte(blockHashes[id]))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ComputeRootHash computes the project root hash from item hashes.
func ComputeRootHash(itemHashes map[string]string) string {
	names := make([]string, 0, len(itemHashes))
	for name := range itemHashes {
		names = append(names, name)
	}
	slices.Sort(names)

	h := sha256.New()
	for _, name := range names {
		h.Write([]byte(name))
		h.Write([]byte(itemHashes[name]))
	}
	return hex.EncodeToString(h.Sum(nil))
}
