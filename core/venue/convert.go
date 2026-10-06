// Package sync provides converters between the Go content model and the
// sync protocol protobuf types (Bowrain AD-009).
//
// The run/segment payloads are the canonical content-model wire schema
// (core/proto/content/v1, converted by core/plugin/protoconvert — see
// AD-034); this package only owns the sync-specific envelope (SyncBlock,
// hashes, the *_json escapes) and the Merkle hash helpers.
package venue

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/protoconvert"
	contentv1 "github.com/neokapi/neokapi/core/proto/content/v1"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/ref"
)

// BlockToProto converts a model.Block to a SyncBlock protobuf message.
func BlockToProto(b *model.Block, itemName string) *pb.SyncBlock {
	sb := &pb.SyncBlock{
		Id:                 b.ID,
		ItemName:           itemName,
		Name:               b.Name,
		Unit:               b.Key,
		Type:               b.Type,
		MimeType:           b.MimeType,
		Translatable:       b.Translatable,
		SourceText:         b.SourceText(),
		PreserveWhitespace: b.PreserveWhitespace,
		Properties:         b.Properties,
		SourceLocale:       string(b.SourceLocale),
		IsReferent:         b.IsReferent,
	}

	// Source authoring state (written→established) rides as a reserved
	// block property, symmetric with how a Target's status rides in its segment
	// properties — keeping the round-trip lossless without a wire-shape change.
	// Copy-on-write so we never mutate the caller's Properties map.
	auth := b.Authoritative(model.AuthorityPolicy{})
	src, _ := b.Edition(auth)
	if src.Status != "" {
		props := make(map[string]string, len(b.Properties)+1)
		maps.Copy(props, b.Properties)
		props[propSourceStatus] = string(src.Status)
		sb.Properties = props
	}

	// Source content: the authoritative edition's flat run sequence rides as a
	// single wire segment.
	if len(src.Runs) > 0 {
		sb.Source = []*contentv1.SegmentMessage{runsToSegment("", src.Runs)}
	}

	// Every other edition rides as a target. The edition key serializes to its
	// text form (locale-only is the common case, e.g. "fr-FR"); the run
	// sequence rides as a single wire segment carrying any status/origin/score
	// in segment properties so the round-trip is lossless. Editions can list a
	// key that Edition does not reach (a target stored under a non-canonical
	// key); that edition is left out, because an empty target would replace
	// the translation the receiver holds.
	for _, key := range b.EditionKeys() {
		if key == auth {
			continue
		}
		e, ok := b.Edition(key)
		if !ok {
			continue
		}
		keyText, err := key.MarshalText()
		if err != nil {
			continue
		}
		if sb.Targets == nil {
			sb.Targets = make(map[string]*pb.SyncSegmentList)
		}
		sb.Targets[string(keyText)] = &pb.SyncSegmentList{
			Segments: []*contentv1.SegmentMessage{targetToSegment(e)},
			Derived:  DerivationToProto(e.Derived),
		}
	}

	// Annotations (serialized as type-discriminated JSON so the polymorphic
	// any interface can be reconstructed on decode).
	if am := b.AnnoMap(); len(am) > 0 {
		if data, err := marshalAnnotations(am); err == nil {
			sb.AnnotationsJson = data
		}
	}

	// Skeleton (polymorphic parts ride a discriminated codec so they survive).
	if b.Skeleton != nil {
		if data, err := MarshalSkeleton(b.Skeleton); err == nil {
			sb.SkeletonJson = data
		}
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

	// Overlays — every run-anchored stand-off layer (segmentation, term, entity,
	// qa, alignment, term-candidate, and any plugin-defined type) rides as a typed
	// OverlayMessage via the canonical protoconvert overlay codec, so the type,
	// run-index anchors, props, variant, and typed span value all survive. Unlike
	// protoconvert.BlockToProto (which reconstructs segmentation from its
	// multi-segment source/target boundaries and so excludes it), a SyncBlock
	// carries source/target as a single wire segment — segmentation is not
	// reconstructable from segment boundaries here, so it rides explicitly too.
	for i := range b.Overlays {
		sb.Overlays = append(sb.Overlays, protoconvert.OverlayToProto(b.Overlays[i]))
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
		Key:                sb.Unit,
		Type:               sb.Type,
		MimeType:           sb.MimeType,
		Translatable:       sb.Translatable,
		PreserveWhitespace: sb.PreserveWhitespace,
		Properties:         sb.Properties,
		SourceLocale:       model.LocaleID(sb.SourceLocale),
		IsReferent:         sb.IsReferent,
	}

	// Restore the source authoring state from its reserved property and strip the
	// key so it never leaks back out as a real block property. Copy-on-write so we
	// don't mutate the proto's Properties map.
	//
	// The status is stored as a reader stores what it read: SetEdition records
	// a write to the authoritative edition as an edit of it, and a status that
	// arrives with the block is part of the block as read, so it goes through
	// SetEditionStatus, which changes the status alone.
	if status, ok := sb.Properties[propSourceStatus]; ok {
		b.SetEditionStatus(model.EditionKey{}, model.Status(status))
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

	// Source content: concatenate the runs of every wire segment back into the
	// flat run sequence of the edition the block was read in.
	var srcRuns []model.Run
	for _, seg := range sb.Source {
		srcRuns = append(srcRuns, protoconvert.ProtoToRuns(seg.Runs)...)
	}
	b.SetSourceRuns(srcRuns)

	// If no structured source but source_text is set, create a simple run.
	if len(srcRuns) == 0 && sb.SourceText != "" {
		b.SetSourceText(sb.SourceText)
	}

	// Targets: one edition per variant, runs concatenated from the wire
	// segments, status/origin/score restored from the first segment's props.
	// Each is filed as a target whatever its key (SetTargetEdition), so a
	// target under the source language stays a target, and so does one under
	// no language.
	for keyText, list := range sb.Targets {
		var key model.EditionKey
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
		e := segmentToEdition(runs, first)
		e.Derived = ProtoToDerivation(list.Derived)
		b.SetTargetEdition(key, e)
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

	// Skeleton (discriminated codec — see MarshalSkeleton).
	if len(sb.SkeletonJson) > 0 {
		skel, err := UnmarshalSkeleton(sb.SkeletonJson)
		if err != nil {
			return b, fmt.Errorf("decode skeleton: %w", err)
		}
		b.Skeleton = skel
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

	// Overlays — reconstruct every stand-off layer via the canonical protoconvert
	// overlay codec. An unregistered/future overlay kind degrades to a
	// GenericAnnotation span value rather than panicking or being dropped.
	protoconvert.ApplyProtoOverlays(b, sb.Overlays)

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
// authoring state (written→established) across the sync protocol — the
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
	propOriginConf   = "__origin_confidence"
	propOriginProf   = "__origin_profile"
	propOriginProfV  = "__origin_profile_version"
	propOriginCtxFP  = "__origin_context_fingerprint"
)

// DerivationToProto encodes the basis a derived edition records, nil for none.
func DerivationToProto(d *model.Derivation) *pb.SyncDerivation {
	if d == nil {
		return nil
	}
	from, err := d.From.MarshalText()
	if err != nil {
		return nil
	}
	return &pb.SyncDerivation{From: string(from), Rev: d.Rev}
}

// ProtoToDerivation decodes a wire derivation, nil for none or for a key that
// does not parse.
func ProtoToDerivation(d *pb.SyncDerivation) *model.Derivation {
	if d == nil {
		return nil
	}
	var from model.EditionKey
	if err := from.UnmarshalText([]byte(d.From)); err != nil {
		return nil
	}
	return &model.Derivation{From: from, Rev: d.Rev}
}

// targetToSegment encodes a derived edition as a single wire segment,
// stashing status/origin/score in segment properties so the protocol shape
// stays unchanged while the round-trip remains lossless.
func targetToSegment(t model.Edition) *contentv1.SegmentMessage {
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
	if t.Origin.Confidence != 0 {
		props[propOriginConf] = strconv.FormatFloat(t.Origin.Confidence, 'g', -1, 64)
	}
	if t.Origin.Profile != "" {
		props[propOriginProf] = t.Origin.Profile
	}
	if t.Origin.ProfileVersion != "" {
		props[propOriginProfV] = t.Origin.ProfileVersion
	}
	if t.Origin.ContextFingerprint != "" {
		props[propOriginCtxFP] = t.Origin.ContextFingerprint
	}
	if len(props) == 0 {
		props = nil
	}
	return &contentv1.SegmentMessage{
		Runs:       protoconvert.RunsToProto(t.Runs),
		Properties: props,
	}
}

// segmentToEdition rebuilds a derived edition from concatenated runs plus the
// first wire segment's metadata properties.
func segmentToEdition(runs []model.Run, first *contentv1.SegmentMessage) model.Edition {
	t := model.Edition{Runs: runs}
	if first == nil {
		return t
	}
	props := first.Properties
	if props == nil {
		return t
	}
	t.Status = model.Status(props[propTargetStatus])
	if s := props[propTargetScore]; s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			t.Score = v
		}
	}
	t.Origin = model.Origin{
		Kind:               props[propOriginKind],
		Engine:             props[propOriginEngine],
		Tool:               props[propOriginTool],
		Reference:          props[propOriginRef],
		Timestamp:          props[propOriginTime],
		Profile:            props[propOriginProf],
		ProfileVersion:     props[propOriginProfV],
		ContextFingerprint: props[propOriginCtxFP],
	}
	if s := props[propOriginConf]; s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			t.Origin.Confidence = v
		}
	}
	return t
}

// ComputeItemHash computes the Merkle hash for an item by hashing
// all its block content hashes in sorted order.
func ComputeItemHash(blockHashes map[string]string) string {
	return ref.Fold(blockHashes)
}

// ComputeRootHash computes the project root hash from item hashes.
func ComputeRootHash(itemHashes map[string]string) string {
	return ref.Fold(itemHashes)
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
// envelopes written by marshalAnnotations, through model.DecodePayload — the
// same decode the server store's hydration uses, so an annotation means one
// thing whether it arrives over the wire or off a row.
func unmarshalAnnotations(data []byte) (map[string]model.Payload, error) {
	var env map[string]annotationEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	out := make(map[string]model.Payload, len(env))
	for k, e := range env {
		typeName := e.Type
		if typeName == "" {
			typeName = k
		}
		out[k] = model.DecodePayload(typeName, e.Data)
	}
	return out, nil
}

// BlockPropertyKeys is the set of property keys a producer's readers emit,
// declared by a push so the far side knows what this producer is authoritative
// about.
//
// It scopes DELETION, not transfer — what to send is decided by the record hash
// (RecordHash), per block, and needs no declaration.
//
// The problem it solves is a fleet that is not one version. A CI runner pinned
// to an older kapi reads the same files with a reader that records less, and
// because the sync cache is not committed a fresh checkout declares every item,
// so that producer really does push the whole corpus. Storing what it sent as
// the whole truth would delete a property it has never heard of, and two
// vintages would take turns adding and removing it. Declaring the keys it knows
// lets the far side keep the rest.
//
// Computed over every block the producer read, not the changed ones: a key is
// declared because this reader emits it, and a push that carries one edited
// string still knows what a note is.
func BlockPropertyKeys(blocks []*model.Block) []string {
	var keys []string
	for _, b := range blocks {
		if b == nil {
			continue
		}
		for key := range b.Properties {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return slices.Compact(keys)
}

// ItemBlockKeys declares the complete block-key set for each item the producer
// read. The receiver uses this declaration to remove obsolete blocks; transfer
// selection is handled separately.
//
// Changed-block payloads alone cannot express deletion. The declaration must be
// computed from the full scan, rather than a diff against the uncommitted sync
// cache, so deletion also works in a fresh CI checkout.
//
// Only scanned items are included. An omitted item is outside the producer's
// scope and must be preserved; an included item with no keys is empty.
func ItemBlockKeys(blocksByItem map[string][]*model.Block) map[string][]string {
	if len(blocksByItem) == 0 {
		return nil
	}
	out := make(map[string][]string, len(blocksByItem))
	for item, blocks := range blocksByItem {
		if item == "" {
			continue
		}
		keys := make([]string, 0, len(blocks))
		for _, b := range blocks {
			if b == nil {
				continue
			}
			if key := convergence.BlockKey(b); key != "" {
				keys = append(keys, key)
			}
		}
		slices.Sort(keys)
		// An item that read to nothing still declares itself, with an empty
		// set. A file whose last translatable string was deleted is exactly the
		// case this exists for, and dropping it here would leave the emptiest
		// item the only one that never gets cleaned.
		out[item] = slices.Compact(keys)
	}
	return out
}
