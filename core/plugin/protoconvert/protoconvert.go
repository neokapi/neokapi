// Package protoconvert provides conversions between core/model types and
// the v2 plugin gRPC proto types defined in core/plugin/proto/v2. The
// in-process Java bridge runner and Mode-C daemon clients use these
// helpers to ferry parts across the wire.
package protoconvert

import (
	"encoding/json"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	pb "github.com/neokapi/neokapi/core/plugin/proto/v2"
)

// jsonToMap unmarshals JSON bytes to a map[string]any, returning an empty
// map on empty input or unmarshal failure. Used by ProtoToAnnotation as
// a fallback when structured unmarshal fails due to wire-format type
// mismatches with the Go model.
func jsonToMap(data []byte) map[string]any {
	if len(data) == 0 {
		return make(map[string]any)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil || m == nil {
		return make(map[string]any)
	}
	return m
}

// ────────────────────────────────────────────────────────────────────────────
// Proto ↔ Model: Annotations
// ────────────────────────────────────────────────────────────────────────────

// AnnotationToProto converts a any to a proto AnnotationEntry.
func AnnotationToProto(a any) *pb.AnnotationEntry {
	data, err := json.Marshal(a)
	if err != nil {
		// A any that can't be JSON-encoded is a programming error
		// (a non-serializable field), not a runtime condition. Fail loudly here
		// rather than emitting a silently-empty entry that would corrupt the
		// block on the other side of the bridge.
		panic(fmt.Sprintf("protoconvert: marshal annotation %q: %v", model.PayloadTypeName(a), err))
	}
	return &pb.AnnotationEntry{
		Type: model.PayloadTypeName(a),
		Data: data,
	}
}

// ProtoToAnnotation converts a proto AnnotationEntry to a any.
func ProtoToAnnotation(e *pb.AnnotationEntry) any {
	a, ok := model.NewAnnotation(e.Type)
	if !ok {
		return &model.GenericAnnotation{
			Kind:   e.Type,
			Fields: jsonToMap(e.Data),
		}
	}
	if err := json.Unmarshal(e.Data, a); err != nil {
		// Structured unmarshal can fail when the wire format uses simpler
		// types than the Go model (e.g., the bridge sends Source/Target as
		// plain strings but the Go AltTranslation expects []Run).
		// Fall back to map-based population.
		m := jsonToMap(e.Data)
		if m != nil {
			return populateAnnotation(e.Type, a, m)
		}
	}
	return a
}

// AnnotationsToProto converts a map of model.Annotations to proto entries.
func AnnotationsToProto(anns map[string]any) map[string]*pb.AnnotationEntry {
	if len(anns) == 0 {
		return nil
	}
	result := make(map[string]*pb.AnnotationEntry, len(anns))
	for key, a := range anns {
		result[key] = AnnotationToProto(a)
	}
	return result
}

// ProtoToAnnotations converts proto annotation entries to model.Annotations.
func ProtoToAnnotations(entries map[string]*pb.AnnotationEntry) map[string]any {
	if len(entries) == 0 {
		return nil
	}
	result := make(map[string]any, len(entries))
	for key, e := range entries {
		result[key] = ProtoToAnnotation(e)
	}
	return result
}

// populateAnnotation fills a typed annotation from a raw map.
// This is used as a fallback when json.Unmarshal fails due to type mismatches
// (e.g., the bridge sends Source/Target as strings but Go expects []Run).
func populateAnnotation(typeName string, a any, m map[string]any) any {
	switch v := a.(type) {
	case *model.NoteAnnotation:
		v.Text, _ = m["text"].(string)
		v.From, _ = m["from"].(string)
		if p, ok := m["priority"].(float64); ok {
			v.Priority = int(p)
		}
		v.Annotates, _ = m["annotates"].(string)
		return v

	case *model.AltTranslation:
		// Source/Target come as plain text strings from the bridge.
		if s, ok := m["source"].(string); ok && s != "" {
			v.Source = []model.Run{{Text: &model.TextRun{Text: s}}}
		}
		if s, ok := m["target"].(string); ok && s != "" {
			v.Target = []model.Run{{Text: &model.TextRun{Text: s}}}
		}
		if loc, ok := m["locale"].(string); ok {
			v.Locale = model.LocaleID(loc)
		}
		v.Origin, _ = m["origin"].(string)
		if f, ok := m["combined_score"].(float64); ok {
			v.CombinedScore = f
		}
		if f, ok := m["fuzzy_score"].(float64); ok {
			v.FuzzyScore = f
		}
		if f, ok := m["quality_score"].(float64); ok {
			v.QualityScore = f
		}
		v.Engine, _ = m["engine"].(string)
		if mt, ok := m["match_type"].(string); ok {
			v.MatchType = model.MatchType(mt)
		}
		v.ToolID, _ = m["tool_id"].(string)
		v.AltTransType, _ = m["alt_trans_type"].(string)
		v.FromOriginal, _ = m["from_original"].(bool)
		return v

	default:
		// Unknown type — wrap as GenericAnnotation.
		return &model.GenericAnnotation{
			Kind:   typeName,
			Fields: m,
		}
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Proto ↔ Model: Skeleton
// ────────────────────────────────────────────────────────────────────────────

// SkeletonToProto converts a model.Skeleton to a proto SkeletonMessage.
func SkeletonToProto(s *model.Skeleton) *pb.SkeletonMessage {
	if s == nil {
		return nil
	}
	msg := &pb.SkeletonMessage{
		Strategy:  int32(s.Strategy),
		SourceUri: s.SourceURI,
	}
	for _, p := range s.Parts {
		switch v := p.(type) {
		case *model.SkeletonText:
			msg.Parts = append(msg.Parts, &pb.SkeletonPartMessage{Text: v.Text})
		case *model.SkeletonRef:
			msg.Parts = append(msg.Parts, &pb.SkeletonPartMessage{
				ResourceId: v.ResourceID,
				Property:   v.Property,
				Locale:     v.Locale,
			})
		}
	}
	return msg
}

// ProtoToSkeleton converts a proto SkeletonMessage to a model.Skeleton.
func ProtoToSkeleton(msg *pb.SkeletonMessage) *model.Skeleton {
	if msg == nil {
		return nil
	}
	s := &model.Skeleton{
		Strategy:  model.SkeletonStrategy(msg.Strategy),
		SourceURI: msg.SourceUri,
	}
	for _, p := range msg.Parts {
		if p.ResourceId != "" {
			s.Parts = append(s.Parts, &model.SkeletonRef{
				ResourceID: p.ResourceId,
				Property:   p.Property,
				Locale:     p.Locale,
			})
		} else {
			s.Parts = append(s.Parts, &model.SkeletonText{Text: p.Text})
		}
	}
	return s
}

// ────────────────────────────────────────────────────────────────────────────
// Proto ↔ Model: DisplayHint
// ────────────────────────────────────────────────────────────────────────────

// DisplayHintToProto converts a model.DisplayHint to a proto DisplayHintMessage.
func DisplayHintToProto(h *model.DisplayHint) *pb.DisplayHintMessage {
	if h == nil {
		return nil
	}
	return &pb.DisplayHintMessage{
		MaxLength:   int32(h.MaxLength),
		ContentType: h.ContentType,
		Context:     h.Context,
		Preview:     h.Preview,
	}
}

// ProtoToDisplayHint converts a proto DisplayHintMessage to a model.DisplayHint.
func ProtoToDisplayHint(msg *pb.DisplayHintMessage) *model.DisplayHint {
	if msg == nil {
		return nil
	}
	return &model.DisplayHint{
		MaxLength:   int(msg.MaxLength),
		ContentType: msg.ContentType,
		Context:     msg.Context,
		Preview:     msg.Preview,
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Proto ↔ Model: Run
// ────────────────────────────────────────────────────────────────────────────

// protoRunBuilder maps model runs onto the v2 plugin proto RunMessage /
// RunList types. The 7-arm dispatch and the Plural/Select recursion live in
// model.BuildRun; this builder only constructs each kind's proto payload.
type protoRunBuilder struct{}

func (protoRunBuilder) Text(t *model.TextRun) *pb.RunMessage {
	return &pb.RunMessage{Kind: &pb.RunMessage_Text{Text: &pb.TextRunMessage{Text: t.Text}}}
}

func (protoRunBuilder) Ph(p *model.PlaceholderRun) *pb.RunMessage {
	return &pb.RunMessage{Kind: &pb.RunMessage_Ph{Ph: &pb.PlaceholderRunMessage{
		Id: p.ID, Type: p.Type, SubType: p.SubType,
		Data: p.Data, Equiv: p.Equiv, Disp: p.Disp,
		Constraints: constraintsToProto(p.Constraints),
	}}}
}

func (protoRunBuilder) PcOpen(p *model.PcOpenRun) *pb.RunMessage {
	return &pb.RunMessage{Kind: &pb.RunMessage_PcOpen{PcOpen: &pb.PcOpenRunMessage{
		Id: p.ID, Type: p.Type, SubType: p.SubType,
		Data: p.Data, Equiv: p.Equiv, Disp: p.Disp,
		Constraints: constraintsToProto(p.Constraints),
	}}}
}

func (protoRunBuilder) PcClose(p *model.PcCloseRun) *pb.RunMessage {
	return &pb.RunMessage{Kind: &pb.RunMessage_PcClose{PcClose: &pb.PcCloseRunMessage{
		Id: p.ID, Type: p.Type, SubType: p.SubType,
		Data: p.Data, Equiv: p.Equiv,
	}}}
}

func (protoRunBuilder) Sub(s *model.SubRun) *pb.RunMessage {
	return &pb.RunMessage{Kind: &pb.RunMessage_Sub{Sub: &pb.SubRunMessage{
		Id: s.ID, Ref: s.Ref, Equiv: s.Equiv,
	}}}
}

func (protoRunBuilder) Plural(pivot string, forms map[string]*pb.RunList) *pb.RunMessage {
	return &pb.RunMessage{Kind: &pb.RunMessage_Plural{Plural: &pb.PluralRunMessage{
		Pivot: pivot, Forms: forms,
	}}}
}

func (protoRunBuilder) Select(pivot string, cases map[string]*pb.RunList) *pb.RunMessage {
	return &pb.RunMessage{Kind: &pb.RunMessage_Select{Select: &pb.SelectRunMessage{
		Pivot: pivot, Cases: cases,
	}}}
}

func (protoRunBuilder) List(runs []*pb.RunMessage) *pb.RunList { return &pb.RunList{Runs: runs} }
func (protoRunBuilder) Zero() *pb.RunMessage                   { return nil }

// protoRunParser is the reverse of protoRunBuilder.
type protoRunParser struct{}

func (protoRunParser) Text(m *pb.RunMessage) (*model.TextRun, bool) {
	if k, ok := m.GetKind().(*pb.RunMessage_Text); ok {
		return &model.TextRun{Text: k.Text.GetText()}, true
	}
	return nil, false
}

func (protoRunParser) Ph(m *pb.RunMessage) (*model.PlaceholderRun, bool) {
	if k, ok := m.GetKind().(*pb.RunMessage_Ph); ok {
		return &model.PlaceholderRun{
			ID: k.Ph.GetId(), Type: k.Ph.GetType(), SubType: k.Ph.GetSubType(),
			Data: k.Ph.GetData(), Equiv: k.Ph.GetEquiv(), Disp: k.Ph.GetDisp(),
			Constraints: protoToConstraints(k.Ph.GetConstraints()),
		}, true
	}
	return nil, false
}

func (protoRunParser) PcOpen(m *pb.RunMessage) (*model.PcOpenRun, bool) {
	if k, ok := m.GetKind().(*pb.RunMessage_PcOpen); ok {
		return &model.PcOpenRun{
			ID: k.PcOpen.GetId(), Type: k.PcOpen.GetType(), SubType: k.PcOpen.GetSubType(),
			Data: k.PcOpen.GetData(), Equiv: k.PcOpen.GetEquiv(), Disp: k.PcOpen.GetDisp(),
			Constraints: protoToConstraints(k.PcOpen.GetConstraints()),
		}, true
	}
	return nil, false
}

func (protoRunParser) PcClose(m *pb.RunMessage) (*model.PcCloseRun, bool) {
	if k, ok := m.GetKind().(*pb.RunMessage_PcClose); ok {
		return &model.PcCloseRun{
			ID: k.PcClose.GetId(), Type: k.PcClose.GetType(), SubType: k.PcClose.GetSubType(),
			Data: k.PcClose.GetData(), Equiv: k.PcClose.GetEquiv(),
		}, true
	}
	return nil, false
}

func (protoRunParser) Sub(m *pb.RunMessage) (*model.SubRun, bool) {
	if k, ok := m.GetKind().(*pb.RunMessage_Sub); ok {
		return &model.SubRun{ID: k.Sub.GetId(), Ref: k.Sub.GetRef(), Equiv: k.Sub.GetEquiv()}, true
	}
	return nil, false
}

func (protoRunParser) Plural(m *pb.RunMessage) (string, map[string]*pb.RunList, bool) {
	if k, ok := m.GetKind().(*pb.RunMessage_Plural); ok {
		return k.Plural.GetPivot(), k.Plural.GetForms(), true
	}
	return "", nil, false
}

func (protoRunParser) Select(m *pb.RunMessage) (string, map[string]*pb.RunList, bool) {
	if k, ok := m.GetKind().(*pb.RunMessage_Select); ok {
		return k.Select.GetPivot(), k.Select.GetCases(), true
	}
	return "", nil, false
}

func (protoRunParser) ListRuns(l *pb.RunList) []*pb.RunMessage { return l.GetRuns() }

// RunToProto converts a model.Run to a proto RunMessage, dispatching
// on the discriminator set on r.
func RunToProto(r model.Run) *pb.RunMessage {
	return model.BuildRun[*pb.RunMessage, *pb.RunList](r, protoRunBuilder{})
}

// ProtoToRun converts a proto RunMessage into its model.Run form.
func ProtoToRun(msg *pb.RunMessage) model.Run {
	if msg == nil {
		return model.Run{}
	}
	return model.ParseRun[*pb.RunMessage, *pb.RunList](msg, protoRunParser{})
}

// RunsToProto converts a slice of model.Run to proto RunMessages.
func RunsToProto(runs []model.Run) []*pb.RunMessage {
	return model.BuildRuns[*pb.RunMessage, *pb.RunList](runs, protoRunBuilder{})
}

// ProtoToRuns converts proto RunMessages to a slice of model.Run.
func ProtoToRuns(msgs []*pb.RunMessage) []model.Run {
	return model.ParseRuns[*pb.RunMessage, *pb.RunList](msgs, protoRunParser{})
}

func constraintsToProto(c *model.RunConstraints) *pb.RunConstraints {
	if c == nil {
		return nil
	}
	return &pb.RunConstraints{Deletable: c.Deletable, Cloneable: c.Cloneable, Reorderable: c.Reorderable}
}

func protoToConstraints(msg *pb.RunConstraints) *model.RunConstraints {
	if msg == nil {
		return nil
	}
	return &model.RunConstraints{Deletable: msg.GetDeletable(), Cloneable: msg.GetCloneable(), Reorderable: msg.GetReorderable()}
}

// ────────────────────────────────────────────────────────────────────────────
// Proto ↔ Model: Segment
// ────────────────────────────────────────────────────────────────────────────

// runsToSegmentProto wraps a run sequence in a single SegmentMessage for wire
// transfer. The model no longer has structural segments — a side's content is
// one run sequence, carried as one SegmentMessage.
func runsToSegmentProto(id string, runs []model.Run) *pb.SegmentMessage {
	return &pb.SegmentMessage{Id: id, Runs: RunsToProto(runs)}
}

func segSpanID(seg *model.Overlay, i int) string {
	if seg != nil && i < len(seg.Spans) && seg.Spans[i].ID != "" {
		return seg.Spans[i].ID
	}
	return fmt.Sprintf("s%d", i+1)
}

// sourceSegProtos emits one SegmentMessage per source segment span, carrying
// the span id so the reverse conversion can rebuild the segmentation overlay.
// An unsegmented block emits a single "s1" segment.
func sourceSegProtos(b *model.Block) []*pb.SegmentMessage {
	if len(b.Source) == 0 {
		return nil
	}
	seg := b.SourceSegmentation()
	n := b.SourceSegmentCount()
	out := make([]*pb.SegmentMessage, 0, n)
	for i := range n {
		out = append(out, runsToSegmentProto(segSpanID(seg, i), b.SourceSegmentRuns(i)))
	}
	return out
}

// targetSegProtos emits one SegmentMessage per target segment span for a
// locale (one "s1" segment when the target is unsegmented).
func targetSegProtos(b *model.Block, loc model.LocaleID) []*pb.SegmentMessage {
	runs := b.TargetRuns(loc)
	key := model.Variant(loc)
	seg := b.SegmentationFor(&key)
	if seg == nil || len(seg.Spans) == 0 {
		return []*pb.SegmentMessage{runsToSegmentProto("s1", runs)}
	}
	out := make([]*pb.SegmentMessage, 0, len(seg.Spans))
	for _, sp := range seg.Spans {
		out = append(out, runsToSegmentProto(sp.ID, sp.Range.ExtractRuns(runs)))
	}
	return out
}

// segProtosToRunsAndSpans concatenates SegmentMessages into a run sequence and
// the matching segmentation spans (run-index boundaries, preserving ids).
// Returns nil spans for the single-segment case (no overlay needed).
func segProtosToRunsAndSpans(msgs []*pb.SegmentMessage) ([]model.Run, []model.Span) {
	var runs []model.Run
	spans := make([]model.Span, 0, len(msgs))
	for _, m := range msgs {
		start := len(runs)
		runs = append(runs, ProtoToRuns(m.Runs)...)
		spans = append(spans, model.Span{ID: m.Id, Range: model.RunRange{StartRun: start, EndRun: len(runs)}})
	}
	if len(msgs) <= 1 {
		return runs, nil
	}
	return runs, spans
}

// applyTargetSegProtos sets a locale's target runs from SegmentMessages and a
// target segmentation overlay when the peer split it into multiple segments.
func applyTargetSegProtos(b *model.Block, loc model.LocaleID, msgs []*pb.SegmentMessage) {
	runs, spans := segProtosToRunsAndSpans(msgs)
	b.SetTargetRuns(loc, runs)
	if len(spans) > 0 {
		key := model.Variant(loc)
		b.SetSegmentation(&key, spans)
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Proto ↔ Model: Block
// ────────────────────────────────────────────────────────────────────────────

// BlockToProto converts a model.Block to a proto BlockMessage.
func BlockToProto(b *model.Block) *pb.BlockMessage {
	if b == nil {
		return nil
	}
	msg := &pb.BlockMessage{
		Id:                 b.ID,
		Name:               b.Name,
		Type:               b.Type,
		MimeType:           b.MimeType,
		Translatable:       b.Translatable,
		Properties:         b.Properties,
		Annotations:        AnnotationsToProto(b.AnnoMap()),
		DisplayHint:        DisplayHintToProto(b.DisplayHint),
		Skeleton:           SkeletonToProto(b.Skeleton),
		PreserveWhitespace: b.PreserveWhitespace,
		IsReferent:         b.IsReferent,
	}
	msg.Source = sourceSegProtos(b)
	for _, locale := range b.TargetLocales() {
		msg.Targets = append(msg.Targets, &pb.TargetEntry{
			Locale:   string(locale),
			Segments: targetSegProtos(b, locale),
		})
	}
	return msg
}

// ProtoToBlock converts a proto BlockMessage to a model.Block.
func ProtoToBlock(msg *pb.BlockMessage) *model.Block {
	if msg == nil {
		return nil
	}
	b := &model.Block{
		ID:                 msg.Id,
		Name:               msg.Name,
		Type:               msg.Type,
		MimeType:           msg.MimeType,
		Translatable:       msg.Translatable,
		Properties:         msg.Properties,
		Targets:            make(map[model.VariantKey]*model.Target),
		DisplayHint:        ProtoToDisplayHint(msg.DisplayHint),
		Skeleton:           ProtoToSkeleton(msg.Skeleton),
		PreserveWhitespace: msg.PreserveWhitespace,
		IsReferent:         msg.IsReferent,
	}
	for k, v := range ProtoToAnnotations(msg.Annotations) {
		b.SetAnno(k, v)
	}
	if b.Properties == nil {
		b.Properties = make(map[string]string)
	}
	srcRuns, srcSpans := segProtosToRunsAndSpans(msg.Source)
	b.Source = srcRuns
	if len(srcSpans) > 0 {
		b.SetSegmentation(nil, srcSpans)
	}
	for _, te := range msg.Targets {
		applyTargetSegProtos(b, model.LocaleID(te.Locale), te.Segments)
	}
	return b
}

// ────────────────────────────────────────────────────────────────────────────
// Proto ↔ Model: Layer
// ────────────────────────────────────────────────────────────────────────────

// LayerToProto converts a model.Layer to a proto LayerMessage.
func LayerToProto(l *model.Layer) *pb.LayerMessage {
	if l == nil {
		return nil
	}
	return &pb.LayerMessage{
		Id:             l.ID,
		Name:           l.Name,
		Format:         l.Format,
		Locale:         string(l.Locale),
		Encoding:       l.Encoding,
		MimeType:       l.MimeType,
		LineBreak:      l.LineBreak,
		IsMultilingual: l.IsMultilingual,
		ParentId:       l.ParentID,
		Properties:     l.Properties,
		HasBom:         l.HasBOM,
	}
}

// ProtoToLayer converts a proto LayerMessage to a model.Layer.
func ProtoToLayer(msg *pb.LayerMessage) *model.Layer {
	if msg == nil {
		return nil
	}
	return &model.Layer{
		ID:             msg.Id,
		Name:           msg.Name,
		Format:         msg.Format,
		Locale:         model.LocaleID(msg.Locale),
		Encoding:       msg.Encoding,
		MimeType:       msg.MimeType,
		LineBreak:      msg.LineBreak,
		IsMultilingual: msg.IsMultilingual,
		ParentID:       msg.ParentId,
		Properties:     msg.Properties,
		HasBOM:         msg.HasBom,
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Proto ↔ Model: Data
// ────────────────────────────────────────────────────────────────────────────

// DataToProto converts a model.Data to a proto DataMessage.
func DataToProto(d *model.Data) *pb.DataMessage {
	if d == nil {
		return nil
	}
	return &pb.DataMessage{
		Id:         d.ID,
		Name:       d.Name,
		Properties: d.Properties,
		Skeleton:   SkeletonToProto(d.Skeleton),
		IsReferent: d.IsReferent,
	}
}

// ProtoToData converts a proto DataMessage to a model.Data.
func ProtoToData(msg *pb.DataMessage) *model.Data {
	if msg == nil {
		return nil
	}
	return &model.Data{
		ID:         msg.Id,
		Name:       msg.Name,
		Properties: msg.Properties,
		Skeleton:   ProtoToSkeleton(msg.Skeleton),
		IsReferent: msg.IsReferent,
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Proto ↔ Model: Group
// ────────────────────────────────────────────────────────────────────────────

// GroupStartToProto converts a model.GroupStart to a proto GroupStartMessage.
func GroupStartToProto(g *model.GroupStart) *pb.GroupStartMessage {
	if g == nil {
		return nil
	}
	return &pb.GroupStartMessage{
		Id:         g.ID,
		Name:       g.Name,
		Type:       g.Type,
		Properties: g.Properties,
	}
}

// ProtoToGroupStart converts a proto GroupStartMessage to a model.GroupStart.
func ProtoToGroupStart(msg *pb.GroupStartMessage) *model.GroupStart {
	if msg == nil {
		return nil
	}
	return &model.GroupStart{
		ID:         msg.Id,
		Name:       msg.Name,
		Type:       msg.Type,
		Properties: msg.Properties,
	}
}

// GroupEndToProto converts a model.GroupEnd to a proto GroupEndMessage.
func GroupEndToProto(g *model.GroupEnd) *pb.GroupEndMessage {
	if g == nil {
		return nil
	}
	return &pb.GroupEndMessage{
		Id: g.ID,
	}
}

// ProtoToGroupEnd converts a proto GroupEndMessage to a model.GroupEnd.
func ProtoToGroupEnd(msg *pb.GroupEndMessage) *model.GroupEnd {
	if msg == nil {
		return nil
	}
	return &model.GroupEnd{
		ID: msg.Id,
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Proto ↔ Model: Media
// ────────────────────────────────────────────────────────────────────────────

// MediaToProto converts a model.Media to a proto MediaMessage.
func MediaToProto(m *model.Media) *pb.MediaMessage {
	if m == nil {
		return nil
	}
	return &pb.MediaMessage{
		Id:         m.ID,
		MimeType:   m.MimeType,
		Data:       m.Data,
		Uri:        m.URI,
		AltText:    m.AltText,
		Properties: m.Properties,
	}
}

// ProtoToMedia converts a proto MediaMessage to a model.Media.
func ProtoToMedia(msg *pb.MediaMessage) *model.Media {
	if msg == nil {
		return nil
	}
	return &model.Media{
		ID:         msg.Id,
		MimeType:   msg.MimeType,
		Data:       msg.Data,
		URI:        msg.Uri,
		AltText:    msg.AltText,
		Properties: msg.Properties,
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Proto ↔ Model: Part
// ────────────────────────────────────────────────────────────────────────────

// PartToProto converts a model.Part to a proto PartMessage.
func PartToProto(p *model.Part) *pb.PartMessage {
	msg := &pb.PartMessage{
		PartType: int32(p.Type),
	}
	switch p.Type {
	case model.PartBlock:
		if b, ok := p.Resource.(*model.Block); ok {
			msg.Block = BlockToProto(b)
		}
	case model.PartLayerStart, model.PartLayerEnd:
		if l, ok := p.Resource.(*model.Layer); ok {
			msg.Layer = LayerToProto(l)
		}
	case model.PartData:
		if d, ok := p.Resource.(*model.Data); ok {
			msg.Data = DataToProto(d)
		}
	case model.PartGroupStart:
		if g, ok := p.Resource.(*model.GroupStart); ok {
			msg.GroupStart = GroupStartToProto(g)
		}
	case model.PartGroupEnd:
		if g, ok := p.Resource.(*model.GroupEnd); ok {
			msg.GroupEnd = GroupEndToProto(g)
		}
	case model.PartMedia:
		if m, ok := p.Resource.(*model.Media); ok {
			msg.Media = MediaToProto(m)
		}
	}
	return msg
}

// ProtoToPart converts a proto PartMessage to a model.Part.
func ProtoToPart(msg *pb.PartMessage) *model.Part {
	p := &model.Part{
		Type: model.PartType(msg.PartType),
	}
	switch p.Type {
	case model.PartBlock:
		if msg.Block != nil {
			p.Resource = ProtoToBlock(msg.Block)
		}
	case model.PartLayerStart, model.PartLayerEnd:
		if msg.Layer != nil {
			p.Resource = ProtoToLayer(msg.Layer)
		}
	case model.PartData:
		if msg.Data != nil {
			p.Resource = ProtoToData(msg.Data)
		}
	case model.PartGroupStart:
		if msg.GroupStart != nil {
			p.Resource = ProtoToGroupStart(msg.GroupStart)
		}
	case model.PartGroupEnd:
		if msg.GroupEnd != nil {
			p.Resource = ProtoToGroupEnd(msg.GroupEnd)
		}
	case model.PartMedia:
		if msg.Media != nil {
			p.Resource = ProtoToMedia(msg.Media)
		}
	}
	return p
}

// PartsToProto converts a slice of model.Parts to proto PartMessages.
func PartsToProto(parts []*model.Part) []*pb.PartMessage {
	msgs := make([]*pb.PartMessage, 0, len(parts))
	for _, p := range parts {
		msgs = append(msgs, PartToProto(p))
	}
	return msgs
}

// ProtoToParts converts a slice of proto PartMessages to model.Parts.
func ProtoToParts(msgs []*pb.PartMessage) []*model.Part {
	parts := make([]*model.Part, 0, len(msgs))
	for _, msg := range msgs {
		parts = append(parts, ProtoToPart(msg))
	}
	return parts
}

// ────────────────────────────────────────────────────────────────────────────
// Proto ↔ Model: ContentBlock (lightweight block transfer)
// ────────────────────────────────────────────────────────────────────────────

// ContentBlockToPart converts a lightweight ContentBlock proto to a model.Part.
func ContentBlockToPart(cb *pb.ContentBlock) *model.Part {
	block := &model.Block{
		ID:                 cb.Id,
		Name:               cb.Name,
		Type:               cb.Type,
		MimeType:           cb.MimeType,
		Translatable:       cb.Translatable,
		PreserveWhitespace: cb.PreserveWhitespace,
	}

	// Source content
	srcRuns, srcSpans := segProtosToRunsAndSpans(cb.Source)
	block.Source = srcRuns
	if len(srcSpans) > 0 {
		block.SetSegmentation(nil, srcSpans)
	}

	// Target content
	if len(cb.Targets) > 0 {
		block.Targets = make(map[model.VariantKey]*model.Target)
		for _, te := range cb.Targets {
			applyTargetSegProtos(block, model.LocaleID(te.Locale), te.Segments)
		}
	}

	// Properties
	if len(cb.Properties) > 0 {
		block.Properties = cb.Properties
	}
	if block.Properties == nil {
		block.Properties = make(map[string]string)
	}

	// Annotations
	for k, v := range cb.Annotations {
		block.SetAnno(k, ProtoToAnnotation(v))
	}

	// Display hint
	if cb.DisplayHint != nil {
		block.DisplayHint = ProtoToDisplayHint(cb.DisplayHint)
	}

	return &model.Part{
		Type:     model.PartBlock,
		Resource: block,
	}
}

// PartToContentBlock converts a model.Part (Block) to a lightweight ContentBlock proto.
func PartToContentBlock(p *model.Part) *pb.ContentBlock {
	block, ok := p.Resource.(*model.Block)
	if !ok {
		return &pb.ContentBlock{}
	}

	cb := &pb.ContentBlock{
		Id:                 block.ID,
		Name:               block.Name,
		Type:               block.Type,
		MimeType:           block.MimeType,
		Translatable:       block.Translatable,
		PreserveWhitespace: block.PreserveWhitespace,
	}

	// Source content
	cb.Source = sourceSegProtos(block)

	// Target content
	for _, locale := range block.TargetLocales() {
		cb.Targets = append(cb.Targets, &pb.TargetEntry{
			Locale:   string(locale),
			Segments: targetSegProtos(block, locale),
		})
	}

	// Properties
	if len(block.Properties) > 0 {
		cb.Properties = block.Properties
	}

	// Annotations
	if am := block.AnnoMap(); len(am) > 0 {
		cb.Annotations = make(map[string]*pb.AnnotationEntry)
		for k, v := range am {
			cb.Annotations[k] = AnnotationToProto(v)
		}
	}

	// Display hint
	if block.DisplayHint != nil {
		cb.DisplayHint = DisplayHintToProto(block.DisplayHint)
	}

	return cb
}
