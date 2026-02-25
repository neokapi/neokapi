package shared

import (
	"encoding/json"

	"github.com/gokapi/gokapi/core/model"
	pb "github.com/gokapi/gokapi/core/plugin/proto/v2"
)

// ────────────────────────────────────────────────────────────────────────────
// Proto ↔ Model: Annotations
// ────────────────────────────────────────────────────────────────────────────

// AnnotationToProto converts a model.Annotation to a proto AnnotationEntry.
func AnnotationToProto(a model.Annotation) *pb.AnnotationEntry {
	data, _ := json.Marshal(a)
	return &pb.AnnotationEntry{
		Type: a.AnnotationType(),
		Data: data,
	}
}

// ProtoToAnnotation converts a proto AnnotationEntry to a model.Annotation.
func ProtoToAnnotation(e *pb.AnnotationEntry) model.Annotation {
	a, ok := model.NewAnnotation(e.Type)
	if !ok {
		return &model.GenericAnnotation{
			Type_:  e.Type,
			Fields: jsonToMap(e.Data),
		}
	}
	if err := json.Unmarshal(e.Data, a); err != nil {
		// Structured unmarshal can fail when the wire format uses simpler
		// types than the Go model (e.g., the bridge sends Source/Target as
		// plain strings but the Go AltTranslation expects *Fragment).
		// Fall back to map-based population.
		m := jsonToMap(e.Data)
		if m != nil {
			return populateAnnotation(e.Type, a, m)
		}
	}
	return a
}

// AnnotationsToProto converts a map of model.Annotations to proto entries.
func AnnotationsToProto(anns map[string]model.Annotation) map[string]*pb.AnnotationEntry {
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
func ProtoToAnnotations(entries map[string]*pb.AnnotationEntry) map[string]model.Annotation {
	if len(entries) == 0 {
		return nil
	}
	result := make(map[string]model.Annotation, len(entries))
	for key, e := range entries {
		result[key] = ProtoToAnnotation(e)
	}
	return result
}

// populateAnnotation fills a typed annotation from a raw map.
// This is used as a fallback when json.Unmarshal fails due to type mismatches
// (e.g., the bridge sends Source/Target as strings but Go expects *Fragment).
func populateAnnotation(typeName string, a model.Annotation, m map[string]any) model.Annotation {
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
			v.Source = model.NewFragment(s)
		}
		if s, ok := m["target"].(string); ok && s != "" {
			v.Target = model.NewFragment(s)
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
		v.MatchType, _ = m["match_type"].(string)
		v.ToolID, _ = m["tool_id"].(string)
		v.AltTransType, _ = m["alt_trans_type"].(string)
		v.FromOriginal, _ = m["from_original"].(bool)
		return v

	default:
		// Unknown type — wrap as GenericAnnotation.
		return &model.GenericAnnotation{
			Type_:  typeName,
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
// Proto ↔ Model: Span
// ────────────────────────────────────────────────────────────────────────────

// SpanToProto converts a model.Span to a proto SpanMessage.
func SpanToProto(s *model.Span) *pb.SpanMessage {
	return &pb.SpanMessage{
		SpanType:    int32(s.SpanType),
		Type:        s.Type,
		Id:          s.ID,
		Data:        s.Data,
		OuterData:   s.OuterData,
		Deletable:   s.Deletable,
		Cloneable:   s.Cloneable,
		OriginalId:  s.OriginalID,
		DisplayText: s.DisplayText,
		Flags:       int32(s.Flags),
		EquivText:   s.EquivText,
		CanReorder:  s.CanReorder,
		Annotations: AnnotationsToProto(s.Annotations),
	}
}

// ProtoToSpan converts a proto SpanMessage to a model.Span.
func ProtoToSpan(msg *pb.SpanMessage) *model.Span {
	return &model.Span{
		SpanType:    model.SpanType(msg.SpanType),
		Type:        msg.Type,
		ID:          msg.Id,
		Data:        msg.Data,
		OuterData:   msg.OuterData,
		Deletable:   msg.Deletable,
		Cloneable:   msg.Cloneable,
		OriginalID:  msg.OriginalId,
		DisplayText: msg.DisplayText,
		Flags:       int(msg.Flags),
		EquivText:   msg.EquivText,
		CanReorder:  msg.CanReorder,
		Annotations: ProtoToAnnotations(msg.Annotations),
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Proto ↔ Model: Fragment
// ────────────────────────────────────────────────────────────────────────────

// FragmentToProto converts a model.Fragment to a proto FragmentMessage.
func FragmentToProto(f *model.Fragment) *pb.FragmentMessage {
	if f == nil {
		return nil
	}
	msg := &pb.FragmentMessage{
		CodedText: f.CodedText,
	}
	for _, s := range f.Spans {
		msg.Spans = append(msg.Spans, SpanToProto(s))
	}
	return msg
}

// ProtoToFragment converts a proto FragmentMessage to a model.Fragment.
func ProtoToFragment(msg *pb.FragmentMessage) *model.Fragment {
	if msg == nil {
		return nil
	}
	f := &model.Fragment{
		CodedText: msg.CodedText,
	}
	for _, s := range msg.Spans {
		f.Spans = append(f.Spans, ProtoToSpan(s))
	}
	return f
}

// ────────────────────────────────────────────────────────────────────────────
// Proto ↔ Model: Segment
// ────────────────────────────────────────────────────────────────────────────

// SegmentToProto converts a model.Segment to a proto SegmentMessage.
func SegmentToProto(s *model.Segment) *pb.SegmentMessage {
	return &pb.SegmentMessage{
		Id:         s.ID,
		Content:    FragmentToProto(s.Content),
		Properties: s.Properties,
	}
}

// ProtoToSegment converts a proto SegmentMessage to a model.Segment.
func ProtoToSegment(msg *pb.SegmentMessage) *model.Segment {
	return &model.Segment{
		ID:         msg.Id,
		Content:    ProtoToFragment(msg.Content),
		Properties: msg.Properties,
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
		Annotations:        AnnotationsToProto(b.Annotations),
		DisplayHint:        DisplayHintToProto(b.DisplayHint),
		Skeleton:           SkeletonToProto(b.Skeleton),
		PreserveWhitespace: b.PreserveWhitespace,
		IsReferent:         b.IsReferent,
	}
	for _, seg := range b.Source {
		msg.Source = append(msg.Source, SegmentToProto(seg))
	}
	for locale, segs := range b.Targets {
		te := &pb.TargetEntry{Locale: string(locale)}
		for _, seg := range segs {
			te.Segments = append(te.Segments, SegmentToProto(seg))
		}
		msg.Targets = append(msg.Targets, te)
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
		Targets:            make(map[model.LocaleID][]*model.Segment),
		Annotations:        ProtoToAnnotations(msg.Annotations),
		DisplayHint:        ProtoToDisplayHint(msg.DisplayHint),
		Skeleton:           ProtoToSkeleton(msg.Skeleton),
		PreserveWhitespace: msg.PreserveWhitespace,
		IsReferent:         msg.IsReferent,
	}
	if b.Properties == nil {
		b.Properties = make(map[string]string)
	}
	if b.Annotations == nil {
		b.Annotations = make(map[string]model.Annotation)
	}
	for _, seg := range msg.Source {
		b.Source = append(b.Source, ProtoToSegment(seg))
	}
	for _, te := range msg.Targets {
		locale := model.LocaleID(te.Locale)
		for _, seg := range te.Segments {
			b.Targets[locale] = append(b.Targets[locale], ProtoToSegment(seg))
		}
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
