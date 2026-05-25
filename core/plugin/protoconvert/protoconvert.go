// Package protoconvert provides conversions between core/model types and
// the v2 plugin gRPC proto types defined in core/plugin/proto/v2. The
// in-process Java bridge runner and Mode-C daemon clients use these
// helpers to ferry parts across the wire.
package protoconvert

import (
	"encoding/json"

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
// (e.g., the bridge sends Source/Target as strings but Go expects []Run).
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

// RunToProto converts a model.Run to a proto RunMessage, dispatching
// on the discriminator set on r.
func RunToProto(r model.Run) *pb.RunMessage {
	switch {
	case r.Text != nil:
		return &pb.RunMessage{Kind: &pb.RunMessage_Text{Text: &pb.TextRunMessage{Text: r.Text.Text}}}
	case r.Ph != nil:
		return &pb.RunMessage{Kind: &pb.RunMessage_Ph{Ph: &pb.PlaceholderRunMessage{
			Id: r.Ph.ID, Type: r.Ph.Type, SubType: r.Ph.SubType,
			Data: r.Ph.Data, Equiv: r.Ph.Equiv, Disp: r.Ph.Disp,
			Constraints: constraintsToProto(r.Ph.Constraints),
		}}}
	case r.PcOpen != nil:
		return &pb.RunMessage{Kind: &pb.RunMessage_PcOpen{PcOpen: &pb.PcOpenRunMessage{
			Id: r.PcOpen.ID, Type: r.PcOpen.Type, SubType: r.PcOpen.SubType,
			Data: r.PcOpen.Data, Equiv: r.PcOpen.Equiv, Disp: r.PcOpen.Disp,
			Constraints: constraintsToProto(r.PcOpen.Constraints),
		}}}
	case r.PcClose != nil:
		return &pb.RunMessage{Kind: &pb.RunMessage_PcClose{PcClose: &pb.PcCloseRunMessage{
			Id: r.PcClose.ID, Type: r.PcClose.Type, SubType: r.PcClose.SubType,
			Data: r.PcClose.Data, Equiv: r.PcClose.Equiv,
		}}}
	case r.Sub != nil:
		return &pb.RunMessage{Kind: &pb.RunMessage_Sub{Sub: &pb.SubRunMessage{
			Id: r.Sub.ID, Ref: r.Sub.Ref, Equiv: r.Sub.Equiv,
		}}}
	case r.Plural != nil:
		forms := make(map[string]*pb.RunList, len(r.Plural.Forms))
		for form, runs := range r.Plural.Forms {
			forms[string(form)] = &pb.RunList{Runs: RunsToProto(runs)}
		}
		return &pb.RunMessage{Kind: &pb.RunMessage_Plural{Plural: &pb.PluralRunMessage{
			Pivot: r.Plural.Pivot, Forms: forms,
		}}}
	case r.Select != nil:
		cases := make(map[string]*pb.RunList, len(r.Select.Cases))
		for key, runs := range r.Select.Cases {
			cases[key] = &pb.RunList{Runs: RunsToProto(runs)}
		}
		return &pb.RunMessage{Kind: &pb.RunMessage_Select{Select: &pb.SelectRunMessage{
			Pivot: r.Select.Pivot, Cases: cases,
		}}}
	}
	return nil
}

// ProtoToRun converts a proto RunMessage into its model.Run form.
func ProtoToRun(msg *pb.RunMessage) model.Run {
	if msg == nil {
		return model.Run{}
	}
	switch k := msg.Kind.(type) {
	case *pb.RunMessage_Text:
		return model.Run{Text: &model.TextRun{Text: k.Text.GetText()}}
	case *pb.RunMessage_Ph:
		return model.Run{Ph: &model.PlaceholderRun{
			ID: k.Ph.GetId(), Type: k.Ph.GetType(), SubType: k.Ph.GetSubType(),
			Data: k.Ph.GetData(), Equiv: k.Ph.GetEquiv(), Disp: k.Ph.GetDisp(),
			Constraints: protoToConstraints(k.Ph.GetConstraints()),
		}}
	case *pb.RunMessage_PcOpen:
		return model.Run{PcOpen: &model.PcOpenRun{
			ID: k.PcOpen.GetId(), Type: k.PcOpen.GetType(), SubType: k.PcOpen.GetSubType(),
			Data: k.PcOpen.GetData(), Equiv: k.PcOpen.GetEquiv(), Disp: k.PcOpen.GetDisp(),
			Constraints: protoToConstraints(k.PcOpen.GetConstraints()),
		}}
	case *pb.RunMessage_PcClose:
		return model.Run{PcClose: &model.PcCloseRun{
			ID: k.PcClose.GetId(), Type: k.PcClose.GetType(), SubType: k.PcClose.GetSubType(),
			Data: k.PcClose.GetData(), Equiv: k.PcClose.GetEquiv(),
		}}
	case *pb.RunMessage_Sub:
		return model.Run{Sub: &model.SubRun{
			ID: k.Sub.GetId(), Ref: k.Sub.GetRef(), Equiv: k.Sub.GetEquiv(),
		}}
	case *pb.RunMessage_Plural:
		forms := make(map[model.PluralForm][]model.Run, len(k.Plural.GetForms()))
		for form, runList := range k.Plural.GetForms() {
			forms[model.PluralForm(form)] = ProtoToRuns(runList.GetRuns())
		}
		return model.Run{Plural: &model.PluralRun{Pivot: k.Plural.GetPivot(), Forms: forms}}
	case *pb.RunMessage_Select:
		cases := make(map[string][]model.Run, len(k.Select.GetCases()))
		for key, runList := range k.Select.GetCases() {
			cases[key] = ProtoToRuns(runList.GetRuns())
		}
		return model.Run{Select: &model.SelectRun{Pivot: k.Select.GetPivot(), Cases: cases}}
	}
	return model.Run{}
}

// RunsToProto converts a slice of model.Run to proto RunMessages.
func RunsToProto(runs []model.Run) []*pb.RunMessage {
	if len(runs) == 0 {
		return nil
	}
	out := make([]*pb.RunMessage, len(runs))
	for i, r := range runs {
		out[i] = RunToProto(r)
	}
	return out
}

// ProtoToRuns converts proto RunMessages to a slice of model.Run.
func ProtoToRuns(msgs []*pb.RunMessage) []model.Run {
	if len(msgs) == 0 {
		return nil
	}
	out := make([]model.Run, len(msgs))
	for i, m := range msgs {
		out[i] = ProtoToRun(m)
	}
	return out
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

// SegmentToProto converts a model.Segment to a proto SegmentMessage.
func SegmentToProto(s *model.Segment) *pb.SegmentMessage {
	return &pb.SegmentMessage{
		Id:         s.ID,
		Runs:       RunsToProto(s.Runs),
		Properties: s.Properties,
	}
}

// ProtoToSegment converts a proto SegmentMessage to a model.Segment.
func ProtoToSegment(msg *pb.SegmentMessage) *model.Segment {
	seg := &model.Segment{
		ID:         msg.Id,
		Properties: msg.Properties,
	}
	seg.SetRuns(ProtoToRuns(msg.Runs))
	return seg
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

	// Source segments
	for _, seg := range cb.Source {
		block.Source = append(block.Source, ProtoToSegment(seg))
	}

	// Target segments
	if len(cb.Targets) > 0 {
		block.Targets = make(map[model.LocaleID][]*model.Segment)
		for _, te := range cb.Targets {
			locale := model.LocaleID(te.Locale)
			for _, seg := range te.Segments {
				block.Targets[locale] = append(block.Targets[locale], ProtoToSegment(seg))
			}
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
	if len(cb.Annotations) > 0 {
		block.Annotations = make(map[string]model.Annotation)
		for k, v := range cb.Annotations {
			block.Annotations[k] = ProtoToAnnotation(v)
		}
	}
	if block.Annotations == nil {
		block.Annotations = make(map[string]model.Annotation)
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

	// Source segments
	for _, seg := range block.Source {
		cb.Source = append(cb.Source, SegmentToProto(seg))
	}

	// Target segments
	for locale, segs := range block.Targets {
		te := &pb.TargetEntry{Locale: string(locale)}
		for _, seg := range segs {
			te.Segments = append(te.Segments, SegmentToProto(seg))
		}
		cb.Targets = append(cb.Targets, te)
	}

	// Properties
	if len(block.Properties) > 0 {
		cb.Properties = block.Properties
	}

	// Annotations
	if len(block.Annotations) > 0 {
		cb.Annotations = make(map[string]*pb.AnnotationEntry)
		for k, v := range block.Annotations {
			cb.Annotations[k] = AnnotationToProto(v)
		}
	}

	// Display hint
	if block.DisplayHint != nil {
		cb.DisplayHint = DisplayHintToProto(block.DisplayHint)
	}

	return cb
}
