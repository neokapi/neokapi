package shared

import (
	"github.com/gokapi/gokapi/core/model"
)

// SpanToDTO converts a model.Span to a SpanDTO.
func SpanToDTO(s *model.Span) SpanDTO {
	return SpanDTO{
		SpanType:  int(s.SpanType),
		Type:      s.Type,
		ID:        s.ID,
		Data:      s.Data,
		OuterData: s.OuterData,
		Deletable: s.Deletable,
		Cloneable: s.Cloneable,
	}
}

// DTOToSpan converts a SpanDTO to a model.Span.
func DTOToSpan(d SpanDTO) *model.Span {
	return &model.Span{
		SpanType:  model.SpanType(d.SpanType),
		Type:      d.Type,
		ID:        d.ID,
		Data:      d.Data,
		OuterData: d.OuterData,
		Deletable: d.Deletable,
		Cloneable: d.Cloneable,
	}
}

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

// SegmentToDTO converts a model.Segment to a SegmentDTO.
func SegmentToDTO(s *model.Segment) SegmentDTO {
	return SegmentDTO{
		ID:      s.ID,
		Content: FragmentToDTO(s.Content),
	}
}

// DTOToSegment converts a SegmentDTO to a model.Segment.
func DTOToSegment(d SegmentDTO) *model.Segment {
	return &model.Segment{
		ID:      d.ID,
		Content: DTOToFragment(d.Content),
	}
}

// BlockToDTO converts a model.Block to a BlockDTO.
func BlockToDTO(b *model.Block) *BlockDTO {
	if b == nil {
		return nil
	}
	dto := &BlockDTO{
		ID:           b.ID,
		Name:         b.Name,
		Type:         b.Type,
		MimeType:     b.MimeType,
		Translatable: b.Translatable,
		Properties:   b.Properties,
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
		ID:           d.ID,
		Name:         d.Name,
		Type:         d.Type,
		MimeType:     d.MimeType,
		Translatable: d.Translatable,
		Properties:   d.Properties,
		Targets:      make(map[model.LocaleID][]*model.Segment),
	}
	if b.Properties == nil {
		b.Properties = make(map[string]string)
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
	}
}

// DataToDTO converts a model.Data to a DataDTO.
func DataToDTO(d *model.Data) *DataDTO {
	if d == nil {
		return nil
	}
	return &DataDTO{
		ID:         d.ID,
		Name:       d.Name,
		Properties: d.Properties,
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
	}
}

// GroupStartToDTO converts a model.GroupStart to a GroupStartDTO.
func GroupStartToDTO(g *model.GroupStart) *GroupStartDTO {
	if g == nil {
		return nil
	}
	return &GroupStartDTO{
		ID:   g.ID,
		Name: g.Name,
		Type: g.Type,
	}
}

// DTOToGroupStart converts a GroupStartDTO to a model.GroupStart.
func DTOToGroupStart(d *GroupStartDTO) *model.GroupStart {
	if d == nil {
		return nil
	}
	return &model.GroupStart{
		ID:   d.ID,
		Name: d.Name,
		Type: d.Type,
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
