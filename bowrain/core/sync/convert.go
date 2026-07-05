// Package sync provides converters between the Go content model and the
// sync protocol protobuf types (Bowrain AD-009).
//
// The run/segment payloads are the canonical content-model wire schema
// (core/proto/content/v1, converted by core/plugin/protoconvert — see
// AD-034); this package only owns the sync-specific envelope (SyncBlock,
// hashes, the *_json escapes) and the Merkle hash helpers.
package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"

	pb "github.com/neokapi/neokapi/bowrain/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/protoconvert"
	contentv1 "github.com/neokapi/neokapi/core/proto/content/v1"
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

	// Source authoring state (authored → checked → approved) rides as a reserved
	// block property, symmetric with how a Target's status rides in its segment
	// properties — keeping the round-trip lossless without a wire-shape change.
	// Copy-on-write so we never mutate the caller's Properties map.
	if b.SourceStatus != "" {
		props := make(map[string]string, len(b.Properties)+1)
		maps.Copy(props, b.Properties)
		props[propSourceStatus] = string(b.SourceStatus)
		sb.Properties = props
	}

	// Source content — the flat run sequence rides as a single wire segment.
	if len(b.Source) > 0 {
		sb.Source = []*contentv1.SegmentMessage{runsToSegment("", b.Source)}
	}

	// Targets per variant. The variant key serializes to its text form
	// (locale-only is the common case, e.g. "fr-FR"); the run sequence rides
	// as a single wire segment carrying any target status/origin/score in
	// segment properties so the round-trip is lossless.
	if len(b.Targets) > 0 {
		sb.Targets = make(map[string]*pb.SyncSegmentList, len(b.Targets))
		for key, target := range b.Targets {
			if target == nil {
				continue
			}
			keyText, err := key.MarshalText()
			if err != nil {
				continue
			}
			sb.Targets[string(keyText)] = &pb.SyncSegmentList{
				Segments: []*contentv1.SegmentMessage{targetToSegment(target)},
			}
		}
	}

	// Annotations (serialized as type-discriminated JSON so the polymorphic
	// any interface can be reconstructed on decode).
	if am := b.AnnoMap(); len(am) > 0 {
		if data, err := marshalAnnotations(am); err == nil {
			sb.AnnotationsJson = data
		}
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
// Returns an error if any of the optional JSON extension fields (Annotations,
// Skeleton, DisplayHint, ContentRef) cannot be decoded; all other fields are
// still populated in the returned block.
func ProtoToBlock(sb *pb.SyncBlock) (*model.Block, error) {
	b := &model.Block{
		ID:                 sb.Id,
		Name:               sb.Name,
		Type:               sb.Type,
		MimeType:           sb.MimeType,
		Translatable:       sb.Translatable,
		PreserveWhitespace: sb.PreserveWhitespace,
		Properties:         sb.Properties,
	}

	// Restore the source authoring state from its reserved property and strip the
	// key so it never leaks back out as a real block property. Copy-on-write so we
	// don't mutate the proto's Properties map.
	if status, ok := sb.Properties[propSourceStatus]; ok {
		b.SourceStatus = model.SourceStatus(status)
		props := make(map[string]string, len(sb.Properties))
		for k, v := range sb.Properties {
			if k == propSourceStatus {
				continue
			}
			props[k] = v
		}
		if len(props) == 0 {
			props = nil
		}
		b.Properties = props
	}

	// Source content — concatenate the runs of every wire segment back into the
	// block's flat run sequence.
	for _, seg := range sb.Source {
		b.Source = append(b.Source, protoconvert.ProtoToRuns(seg.Runs)...)
	}

	// If no structured source but source_text is set, create a simple run.
	if len(b.Source) == 0 && sb.SourceText != "" {
		b.SetSourceText(sb.SourceText)
	}

	// Targets — one Target per variant, runs concatenated from the wire
	// segments, status/origin/score restored from the first segment's props.
	if len(sb.Targets) > 0 {
		b.Targets = make(map[model.VariantKey]*model.Target, len(sb.Targets))
		for keyText, list := range sb.Targets {
			var key model.VariantKey
			if err := key.UnmarshalText([]byte(keyText)); err != nil {
				continue
			}
			var runs []model.Run
			var first *contentv1.SegmentMessage
			for _, seg := range list.Segments {
				if first == nil {
					first = seg
				}
				runs = append(runs, protoconvert.ProtoToRuns(seg.Runs)...)
			}
			b.Targets[key] = segmentToTarget(runs, first)
		}
	}

	// Annotations.
	if len(sb.AnnotationsJson) > 0 {
		anns, err := unmarshalAnnotations(sb.AnnotationsJson)
		if err != nil {
			return b, fmt.Errorf("decode annotations: %w", err)
		}
		for k, v := range anns {
			b.SetAnno(k, v)
		}
	}

	// Skeleton.
	if len(sb.SkeletonJson) > 0 {
		b.Skeleton = &model.Skeleton{}
		if err := json.Unmarshal(sb.SkeletonJson, b.Skeleton); err != nil {
			return b, fmt.Errorf("decode skeleton: %w", err)
		}
	}

	// Display hint.
	if len(sb.DisplayHintJson) > 0 {
		b.DisplayHint = &model.DisplayHint{}
		if err := json.Unmarshal(sb.DisplayHintJson, b.DisplayHint); err != nil {
			return b, fmt.Errorf("decode display hint: %w", err)
		}
	}

	// Content ref.
	if len(sb.ContentRefJson) > 0 {
		b.ContentRef = &model.ContentRef{}
		if err := json.Unmarshal(sb.ContentRefJson, b.ContentRef); err != nil {
			return b, fmt.Errorf("decode content ref: %w", err)
		}
	}

	return b, nil
}

// runsToSegment wraps a flat run sequence in a single canonical wire segment,
// delegating run conversion to protoconvert.
func runsToSegment(id string, runs []model.Run) *contentv1.SegmentMessage {
	return &contentv1.SegmentMessage{
		Id:   id,
		Runs: protoconvert.RunsToProto(runs),
	}
}

// propSourceStatus is the reserved block-property key carrying a Block's source
// authoring state (authored → checked → approved) across the sync protocol — the
// block-level counterpart of the per-target __status segment property.
const propSourceStatus = "__source_status"

// Wire-segment property keys carrying Target metadata across the protocol.
const (
	propTargetStatus = "__status"
	propTargetScore  = "__score"
	propOriginKind   = "__origin_kind"
	propOriginEngine = "__origin_engine"
	propOriginTool   = "__origin_tool"
	propOriginRef    = "__origin_reference"
	propOriginTime   = "__origin_timestamp"
)

// targetToSegment encodes a committed Target as a single wire segment,
// stashing status/origin/score in segment properties so the protocol shape
// stays unchanged while the round-trip remains lossless.
func targetToSegment(t *model.Target) *contentv1.SegmentMessage {
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
	return &contentv1.SegmentMessage{
		Runs:       protoconvert.RunsToProto(t.Runs),
		Properties: props,
	}
}

// segmentToTarget rebuilds a Target from concatenated runs plus the first
// wire segment's metadata properties.
func segmentToTarget(runs []model.Run, first *contentv1.SegmentMessage) *model.Target {
	t := &model.Target{Runs: runs}
	if first == nil {
		return t
	}
	props := first.Properties
	if props == nil {
		return t
	}
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

// annotationEnvelope carries an annotation's concrete type alongside its JSON
// payload so the polymorphic any interface can be reconstructed.
// A plain json.Marshal of map[string]any cannot round-trip,
// because json.Unmarshal has no way to pick the concrete type for an interface.
type annotationEnvelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// marshalAnnotations encodes a block's annotations as type-discriminated
// envelopes.
func marshalAnnotations(anns map[string]model.Payload) ([]byte, error) {
	env := make(map[string]annotationEnvelope, len(anns))
	for k, a := range anns {
		if a == nil {
			continue
		}
		data, err := json.Marshal(a)
		if err != nil {
			return nil, fmt.Errorf("marshal annotation %q: %w", k, err)
		}
		env[k] = annotationEnvelope{Type: model.PayloadTypeName(a), Data: data}
	}
	return json.Marshal(env)
}

// unmarshalAnnotations reconstructs typed annotations from the discriminated
// envelopes written by marshalAnnotations, falling back to GenericAnnotation
// for unregistered types.
func unmarshalAnnotations(data []byte) (map[string]model.Payload, error) {
	var env map[string]annotationEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	out := make(map[string]model.Payload, len(env))
	for k, e := range env {
		a, ok := model.NewPayload(e.Type)
		if !ok {
			a = &model.GenericAnnotation{Kind: e.Type}
		}
		if len(e.Data) > 0 {
			if err := json.Unmarshal(e.Data, a); err != nil {
				return nil, fmt.Errorf("decode annotation %q (%s): %w", k, e.Type, err)
			}
		}
		out[k] = a
	}
	return out, nil
}
