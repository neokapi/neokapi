package model

import "encoding/json"

// fragmentJSON is the JSON-serializable representation of a Fragment.
type fragmentJSON struct {
	CodedText string      `json:"coded_text"`
	Spans     []*spanJSON `json:"spans,omitempty"`
}

type spanJSON struct {
	SpanType    string `json:"span_type"`
	Type        string `json:"type,omitempty"`
	SubType     string `json:"sub_type,omitempty"`
	ID          string `json:"id,omitempty"`
	Data        string `json:"data,omitempty"`
	OuterData   string `json:"outer_data,omitempty"`
	DisplayText string `json:"display_text,omitempty"`
	EquivText   string `json:"equiv_text,omitempty"`
	Deletable   bool   `json:"deletable,omitempty"`
	Cloneable   bool   `json:"cloneable,omitempty"`
	CanReorder  bool   `json:"can_reorder,omitempty"`
}

// MarshalJSON serializes a Fragment to JSON, preserving coded text and span metadata.
func (f *Fragment) MarshalJSON() ([]byte, error) {
	fj := fragmentJSON{
		CodedText: f.CodedText,
	}
	for _, s := range f.Spans {
		fj.Spans = append(fj.Spans, &spanJSON{
			SpanType:    spanTypeToString(s.SpanType),
			Type:        s.Type,
			SubType:     s.SubType,
			ID:          s.ID,
			Data:        s.Data,
			OuterData:   s.OuterData,
			DisplayText: s.DisplayText,
			EquivText:   s.EquivText,
			Deletable:   s.Deletable,
			Cloneable:   s.Cloneable,
			CanReorder:  s.CanReorder,
		})
	}
	return json.Marshal(fj)
}

// UnmarshalJSON deserializes a Fragment from JSON.
func (f *Fragment) UnmarshalJSON(data []byte) error {
	var fj fragmentJSON
	if err := json.Unmarshal(data, &fj); err != nil {
		return err
	}
	f.CodedText = fj.CodedText
	f.Spans = nil
	for _, sj := range fj.Spans {
		f.Spans = append(f.Spans, &Span{
			SpanType:    stringToSpanType(sj.SpanType),
			Type:        sj.Type,
			SubType:     sj.SubType,
			ID:          sj.ID,
			Data:        sj.Data,
			OuterData:   sj.OuterData,
			DisplayText: sj.DisplayText,
			EquivText:   sj.EquivText,
			Deletable:   sj.Deletable,
			Cloneable:   sj.Cloneable,
			CanReorder:  sj.CanReorder,
		})
	}
	return nil
}

func spanTypeToString(st SpanType) string {
	switch st {
	case SpanOpening:
		return "opening"
	case SpanClosing:
		return "closing"
	case SpanPlaceholder:
		return "placeholder"
	default:
		return "unknown"
	}
}

func stringToSpanType(s string) SpanType {
	switch s {
	case "opening":
		return SpanOpening
	case "closing":
		return SpanClosing
	case "placeholder":
		return SpanPlaceholder
	default:
		return SpanPlaceholder
	}
}
