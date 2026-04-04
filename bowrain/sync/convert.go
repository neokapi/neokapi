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
	ps := &pb.SyncSegment{
		Id:         seg.ID,
		Properties: seg.Properties,
	}
	if seg.Content != nil {
		ps.Text = seg.Content.Text()
		ps.CodedText = seg.Content.CodedText
		for _, span := range seg.Content.Spans {
			ps.Spans = append(ps.Spans, &pb.SyncSpan{
				Id:       span.ID,
				Type:     span.Type,
				SubType:  span.SubType,
				SpanType: span.SpanType.String(),
				Data:     span.Data,
			})
		}
	}
	return ps
}

// protoToSegment converts a proto segment to model.Segment.
func protoToSegment(ps *pb.SyncSegment) *model.Segment {
	seg := &model.Segment{
		ID:         ps.Id,
		Properties: ps.Properties,
	}
	frag := &model.Fragment{
		CodedText: ps.CodedText,
	}
	if frag.CodedText == "" {
		frag.CodedText = ps.Text
	}
	for _, sp := range ps.Spans {
		frag.Spans = append(frag.Spans, &model.Span{
			ID:       sp.Id,
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
