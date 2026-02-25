package shared

import (
	"encoding/json"

	"github.com/gokapi/gokapi/core/model"
)

// ────────────────────────────────────────────────────────────────────────────
// Annotation conversions
// ────────────────────────────────────────────────────────────────────────────

// AnnotationToDTO converts a model.Annotation to an AnnotationDTO.
func AnnotationToDTO(a model.Annotation) AnnotationDTO {
	data, _ := json.Marshal(a)
	return AnnotationDTO{
		Type: a.AnnotationType(),
		Data: data,
	}
}

// DTOToAnnotation converts an AnnotationDTO to a model.Annotation.
func DTOToAnnotation(d AnnotationDTO) model.Annotation {
	a, ok := model.NewAnnotation(d.Type)
	if !ok {
		// Fall back to generic annotation.
		return &model.GenericAnnotation{
			Type_:  d.Type,
			Fields: jsonToMap(d.Data),
		}
	}
	_ = json.Unmarshal(d.Data, a)
	return a
}

// AnnotationsToDTO converts a map of model.Annotations to AnnotationDTOs.
func AnnotationsToDTO(anns map[string]model.Annotation) map[string]AnnotationDTO {
	if len(anns) == 0 {
		return nil
	}
	result := make(map[string]AnnotationDTO, len(anns))
	for key, a := range anns {
		result[key] = AnnotationToDTO(a)
	}
	return result
}

// DTOToAnnotations converts a map of AnnotationDTOs to model.Annotations.
func DTOToAnnotations(dtos map[string]AnnotationDTO) map[string]model.Annotation {
	if len(dtos) == 0 {
		return nil
	}
	result := make(map[string]model.Annotation, len(dtos))
	for key, d := range dtos {
		result[key] = DTOToAnnotation(d)
	}
	return result
}

func jsonToMap(data []byte) map[string]any {
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	return m
}

// ────────────────────────────────────────────────────────────────────────────
// Skeleton conversions
// ────────────────────────────────────────────────────────────────────────────

// SkeletonToDTO converts a model.Skeleton to a SkeletonDTO.
func SkeletonToDTO(s *model.Skeleton) *SkeletonDTO {
	if s == nil {
		return nil
	}
	dto := &SkeletonDTO{
		Strategy:  int(s.Strategy),
		SourceURI: s.SourceURI,
	}
	for _, p := range s.Parts {
		switch v := p.(type) {
		case *model.SkeletonText:
			dto.Parts = append(dto.Parts, SkeletonPartDTO{Text: v.Text})
		case *model.SkeletonRef:
			dto.Parts = append(dto.Parts, SkeletonPartDTO{
				ResourceID: v.ResourceID,
				Property:   v.Property,
				Locale:     v.Locale,
			})
		}
	}
	return dto
}

// DTOToSkeleton converts a SkeletonDTO to a model.Skeleton.
func DTOToSkeleton(d *SkeletonDTO) *model.Skeleton {
	if d == nil {
		return nil
	}
	s := &model.Skeleton{
		Strategy:  model.SkeletonStrategy(d.Strategy),
		SourceURI: d.SourceURI,
	}
	for _, p := range d.Parts {
		if p.ResourceID != "" {
			s.Parts = append(s.Parts, &model.SkeletonRef{
				ResourceID: p.ResourceID,
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
// DisplayHint conversions
// ────────────────────────────────────────────────────────────────────────────

// DisplayHintToDTO converts a model.DisplayHint to a DisplayHintDTO.
func DisplayHintToDTO(h *model.DisplayHint) *DisplayHintDTO {
	if h == nil {
		return nil
	}
	return &DisplayHintDTO{
		MaxLength:   h.MaxLength,
		ContentType: h.ContentType,
		Context:     h.Context,
		Preview:     h.Preview,
	}
}

// DTOToDisplayHint converts a DisplayHintDTO to a model.DisplayHint.
func DTOToDisplayHint(d *DisplayHintDTO) *model.DisplayHint {
	if d == nil {
		return nil
	}
	return &model.DisplayHint{
		MaxLength:   d.MaxLength,
		ContentType: d.ContentType,
		Context:     d.Context,
		Preview:     d.Preview,
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Span conversions
// ────────────────────────────────────────────────────────────────────────────

// SpanToDTO converts a model.Span to a SpanDTO.
func SpanToDTO(s *model.Span) SpanDTO {
	return SpanDTO{
		SpanType:    int(s.SpanType),
		Type:        s.Type,
		ID:          s.ID,
		Data:        s.Data,
		OuterData:   s.OuterData,
		Deletable:   s.Deletable,
		Cloneable:   s.Cloneable,
		OriginalID:  s.OriginalID,
		DisplayText: s.DisplayText,
		Flags:       s.Flags,
		EquivText:   s.EquivText,
		CanReorder:  s.CanReorder,
		Annotations: AnnotationsToDTO(s.Annotations),
	}
}

// DTOToSpan converts a SpanDTO to a model.Span.
func DTOToSpan(d SpanDTO) *model.Span {
	return &model.Span{
		SpanType:    model.SpanType(d.SpanType),
		Type:        d.Type,
		ID:          d.ID,
		Data:        d.Data,
		OuterData:   d.OuterData,
		Deletable:   d.Deletable,
		Cloneable:   d.Cloneable,
		OriginalID:  d.OriginalID,
		DisplayText: d.DisplayText,
		Flags:       d.Flags,
		EquivText:   d.EquivText,
		CanReorder:  d.CanReorder,
		Annotations: DTOToAnnotations(d.Annotations),
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Fragment conversions
// ────────────────────────────────────────────────────────────────────────────

// FragmentToDTO converts a model.Fragment to a FragmentDTO.
func FragmentToDTO(f *model.Fragment) FragmentDTO {
	if f == nil {
		return FragmentDTO{}
	}
	dto := FragmentDTO{
		CodedText: f.CodedText,
	}
	for _, s := range f.Spans {
		dto.Spans = append(dto.Spans, SpanToDTO(s))
	}
	return dto
}

// DTOToFragment converts a FragmentDTO to a model.Fragment.
func DTOToFragment(d FragmentDTO) *model.Fragment {
	f := &model.Fragment{
		CodedText: d.CodedText,
	}
	for _, s := range d.Spans {
		f.Spans = append(f.Spans, DTOToSpan(s))
	}
	return f
}

// ────────────────────────────────────────────────────────────────────────────
// Segment conversions
// ────────────────────────────────────────────────────────────────────────────

// SegmentToDTO converts a model.Segment to a SegmentDTO.
func SegmentToDTO(s *model.Segment) SegmentDTO {
	return SegmentDTO{
		ID:         s.ID,
		Content:    FragmentToDTO(s.Content),
		Properties: s.Properties,
	}
}

// DTOToSegment converts a SegmentDTO to a model.Segment.
func DTOToSegment(d SegmentDTO) *model.Segment {
	return &model.Segment{
		ID:         d.ID,
		Content:    DTOToFragment(d.Content),
		Properties: d.Properties,
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Block conversions
// ────────────────────────────────────────────────────────────────────────────

// BlockToDTO converts a model.Block to a BlockDTO.
func BlockToDTO(b *model.Block) *BlockDTO {
	if b == nil {
		return nil
	}
	dto := &BlockDTO{
		ID:                 b.ID,
		Name:               b.Name,
		Type:               b.Type,
		MimeType:           b.MimeType,
		Translatable:       b.Translatable,
		Properties:         b.Properties,
		Annotations:        AnnotationsToDTO(b.Annotations),
		DisplayHint:        DisplayHintToDTO(b.DisplayHint),
		Skeleton:           SkeletonToDTO(b.Skeleton),
		PreserveWhitespace: b.PreserveWhitespace,
		IsReferent:         b.IsReferent,
	}
	for _, seg := range b.Source {
		dto.Source = append(dto.Source, SegmentToDTO(seg))
	}
	for locale, segs := range b.Targets {
		t := TargetDTO{Locale: string(locale)}
		for _, seg := range segs {
			t.Segments = append(t.Segments, SegmentToDTO(seg))
		}
		dto.Targets = append(dto.Targets, t)
	}
	return dto
}

// DTOToBlock converts a BlockDTO to a model.Block.
func DTOToBlock(d *BlockDTO) *model.Block {
	if d == nil {
		return nil
	}
	b := &model.Block{
		ID:                 d.ID,
		Name:               d.Name,
		Type:               d.Type,
		MimeType:           d.MimeType,
		Translatable:       d.Translatable,
		Properties:         d.Properties,
		Targets:            make(map[model.LocaleID][]*model.Segment),
		Annotations:        DTOToAnnotations(d.Annotations),
		DisplayHint:        DTOToDisplayHint(d.DisplayHint),
		Skeleton:           DTOToSkeleton(d.Skeleton),
		PreserveWhitespace: d.PreserveWhitespace,
		IsReferent:         d.IsReferent,
	}
	if b.Properties == nil {
		b.Properties = make(map[string]string)
	}
	if b.Annotations == nil {
		b.Annotations = make(map[string]model.Annotation)
	}
	for _, seg := range d.Source {
		b.Source = append(b.Source, DTOToSegment(seg))
	}
	for _, t := range d.Targets {
		locale := model.LocaleID(t.Locale)
		for _, seg := range t.Segments {
			b.Targets[locale] = append(b.Targets[locale], DTOToSegment(seg))
		}
	}
	return b
}

// ────────────────────────────────────────────────────────────────────────────
// Layer conversions
// ────────────────────────────────────────────────────────────────────────────

// LayerToDTO converts a model.Layer to a LayerDTO.
func LayerToDTO(l *model.Layer) *LayerDTO {
	if l == nil {
		return nil
	}
	return &LayerDTO{
		ID:             l.ID,
		Name:           l.Name,
		Format:         l.Format,
		Locale:         string(l.Locale),
		Encoding:       l.Encoding,
		MimeType:       l.MimeType,
		LineBreak:      l.LineBreak,
		IsMultilingual: l.IsMultilingual,
		ParentID:       l.ParentID,
		Properties:     l.Properties,
		HasBOM:         l.HasBOM,
	}
}

// DTOToLayer converts a LayerDTO to a model.Layer.
func DTOToLayer(d *LayerDTO) *model.Layer {
	if d == nil {
		return nil
	}
	return &model.Layer{
		ID:             d.ID,
		Name:           d.Name,
		Format:         d.Format,
		Locale:         model.LocaleID(d.Locale),
		Encoding:       d.Encoding,
		MimeType:       d.MimeType,
		LineBreak:      d.LineBreak,
		IsMultilingual: d.IsMultilingual,
		ParentID:       d.ParentID,
		Properties:     d.Properties,
		HasBOM:         d.HasBOM,
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Data conversions
// ────────────────────────────────────────────────────────────────────────────

// DataToDTO converts a model.Data to a DataDTO.
func DataToDTO(d *model.Data) *DataDTO {
	if d == nil {
		return nil
	}
	return &DataDTO{
		ID:         d.ID,
		Name:       d.Name,
		Properties: d.Properties,
		Skeleton:   SkeletonToDTO(d.Skeleton),
		IsReferent: d.IsReferent,
	}
}

// DTOToData converts a DataDTO to a model.Data.
func DTOToData(d *DataDTO) *model.Data {
	if d == nil {
		return nil
	}
	return &model.Data{
		ID:         d.ID,
		Name:       d.Name,
		Properties: d.Properties,
		Skeleton:   DTOToSkeleton(d.Skeleton),
		IsReferent: d.IsReferent,
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Group conversions
// ────────────────────────────────────────────────────────────────────────────

// GroupStartToDTO converts a model.GroupStart to a GroupStartDTO.
func GroupStartToDTO(g *model.GroupStart) *GroupStartDTO {
	if g == nil {
		return nil
	}
	return &GroupStartDTO{
		ID:         g.ID,
		Name:       g.Name,
		Type:       g.Type,
		Properties: g.Properties,
	}
}

// DTOToGroupStart converts a GroupStartDTO to a model.GroupStart.
func DTOToGroupStart(d *GroupStartDTO) *model.GroupStart {
	if d == nil {
		return nil
	}
	return &model.GroupStart{
		ID:         d.ID,
		Name:       d.Name,
		Type:       d.Type,
		Properties: d.Properties,
	}
}

// GroupEndToDTO converts a model.GroupEnd to a GroupEndDTO.
func GroupEndToDTO(g *model.GroupEnd) *GroupEndDTO {
	if g == nil {
		return nil
	}
	return &GroupEndDTO{
		ID: g.ID,
	}
}

// DTOToGroupEnd converts a GroupEndDTO to a model.GroupEnd.
func DTOToGroupEnd(d *GroupEndDTO) *model.GroupEnd {
	if d == nil {
		return nil
	}
	return &model.GroupEnd{
		ID: d.ID,
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Media conversions
// ────────────────────────────────────────────────────────────────────────────

// MediaToDTO converts a model.Media to a MediaDTO.
func MediaToDTO(m *model.Media) *MediaDTO {
	if m == nil {
		return nil
	}
	return &MediaDTO{
		ID:         m.ID,
		MimeType:   m.MimeType,
		Data:       m.Data,
		URI:        m.URI,
		AltText:    m.AltText,
		Properties: m.Properties,
	}
}

// DTOToMedia converts a MediaDTO to a model.Media.
func DTOToMedia(d *MediaDTO) *model.Media {
	if d == nil {
		return nil
	}
	return &model.Media{
		ID:         d.ID,
		MimeType:   d.MimeType,
		Data:       d.Data,
		URI:        d.URI,
		AltText:    d.AltText,
		Properties: d.Properties,
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Part conversions
// ────────────────────────────────────────────────────────────────────────────

// PartToDTO converts a model.Part to a PartDTO.
func PartToDTO(p *model.Part) PartDTO {
	dto := PartDTO{
		PartType: int(p.Type),
	}
	switch p.Type {
	case model.PartBlock:
		if b, ok := p.Resource.(*model.Block); ok {
			dto.Block = BlockToDTO(b)
		}
	case model.PartLayerStart, model.PartLayerEnd:
		if l, ok := p.Resource.(*model.Layer); ok {
			dto.Layer = LayerToDTO(l)
		}
	case model.PartData:
		if d, ok := p.Resource.(*model.Data); ok {
			dto.Data = DataToDTO(d)
		}
	case model.PartGroupStart:
		if g, ok := p.Resource.(*model.GroupStart); ok {
			dto.GroupStart = GroupStartToDTO(g)
		}
	case model.PartGroupEnd:
		if g, ok := p.Resource.(*model.GroupEnd); ok {
			dto.GroupEnd = GroupEndToDTO(g)
		}
	case model.PartMedia:
		if m, ok := p.Resource.(*model.Media); ok {
			dto.Media = MediaToDTO(m)
		}
	}
	return dto
}

// DTOToPart converts a PartDTO to a model.Part.
func DTOToPart(d PartDTO) *model.Part {
	p := &model.Part{
		Type: model.PartType(d.PartType),
	}
	switch p.Type {
	case model.PartBlock:
		if d.Block != nil {
			p.Resource = DTOToBlock(d.Block)
		}
	case model.PartLayerStart, model.PartLayerEnd:
		if d.Layer != nil {
			p.Resource = DTOToLayer(d.Layer)
		}
	case model.PartData:
		if d.Data != nil {
			p.Resource = DTOToData(d.Data)
		}
	case model.PartGroupStart:
		if d.GroupStart != nil {
			p.Resource = DTOToGroupStart(d.GroupStart)
		}
	case model.PartGroupEnd:
		if d.GroupEnd != nil {
			p.Resource = DTOToGroupEnd(d.GroupEnd)
		}
	case model.PartMedia:
		if d.Media != nil {
			p.Resource = DTOToMedia(d.Media)
		}
	}
	return p
}

// PartsToDTO converts a slice of model.Part to a slice of PartDTO.
func PartsToDTO(parts []*model.Part) []PartDTO {
	dtos := make([]PartDTO, 0, len(parts))
	for _, p := range parts {
		dtos = append(dtos, PartToDTO(p))
	}
	return dtos
}

// DTOToParts converts a slice of PartDTO to a slice of model.Part.
func DTOToParts(dtos []PartDTO) []*model.Part {
	parts := make([]*model.Part, 0, len(dtos))
	for _, d := range dtos {
		parts = append(parts, DTOToPart(d))
	}
	return parts
}
