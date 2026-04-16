package model_test

import (
	"fmt"
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

// BenchmarkFragment_AppendText measures the cost of sequential text appends
// to a Fragment, exercising the internal strings.Builder path.
func BenchmarkFragment_AppendText(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		f := model.NewFragment("")
		for i := range 100 {
			f.AppendText(fmt.Sprintf("word%d ", i))
		}
	}
}

// BenchmarkFragment_Text measures text extraction (stripping span markers)
// from a Fragment with mixed content and inline spans.
func BenchmarkFragment_Text(b *testing.B) {
	f := model.NewFragment("")
	for i := range 50 {
		f.AppendText(fmt.Sprintf("Some text content %d ", i))
		f.AppendSpan(&model.Span{
			SpanType: model.SpanPlaceholder,
			ID:       fmt.Sprintf("ph%d", i),
			Data:     fmt.Sprintf("<br id=\"%d\"/>", i),
		})
	}
	f.AppendText("Final text segment.")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = f.Text()
	}
}

// BenchmarkFragment_Clone measures deep cloning of a Fragment with spans.
func BenchmarkFragment_Clone(b *testing.B) {
	f := model.NewFragment("")
	for i := range 20 {
		f.AppendText(fmt.Sprintf("Segment %d with content ", i))
		f.AppendSpan(&model.Span{
			SpanType: model.SpanOpening,
			ID:       fmt.Sprintf("s%d", i),
			Data:     "<b>",
			Type:     "fmt:bold",
		})
		f.AppendText("bold text")
		f.AppendSpan(&model.Span{
			SpanType: model.SpanClosing,
			ID:       fmt.Sprintf("s%d", i),
			Data:     "</b>",
			Type:     "fmt:bold",
		})
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = f.Clone()
	}
}

// BenchmarkBlock_Clone measures deep cloning of a Block with source and
// target segments across multiple locales.
func BenchmarkBlock_Clone(b *testing.B) {
	block := model.NewBlock("tu-bench", "The quick brown fox jumps over the lazy dog.")
	block.SetTargetText(model.LocaleFrench, "Le rapide renard brun saute par-dessus le chien paresseux.")
	block.Properties["context"] = "test sentence"
	block.Properties["domain"] = "general"

	// Add a second source segment to exercise multi-segment cloning.
	block.Source = append(block.Source, &model.Segment{
		ID:   "s2",
		Runs: []model.Run{{Text: &model.TextRun{Text: "A second segment with more content for realism."}}},
	})

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		clone := &model.Block{
			ID:           block.ID,
			Name:         block.Name,
			Type:         block.Type,
			MimeType:     block.MimeType,
			Translatable: block.Translatable,
			Source:       make([]*model.Segment, len(block.Source)),
			Targets:      make(map[model.LocaleID][]*model.Segment, len(block.Targets)),
			Properties:   make(map[string]string, len(block.Properties)),
		}
		for i, seg := range block.Source {
			clone.Source[i] = &model.Segment{
				ID:   seg.ID,
				Runs: append([]model.Run(nil), seg.Runs...),
			}
		}
		for locale, segs := range block.Targets {
			cloneSegs := make([]*model.Segment, len(segs))
			for i, seg := range segs {
				cloneSegs[i] = &model.Segment{
					ID:   seg.ID,
					Runs: append([]model.Run(nil), seg.Runs...),
				}
			}
			clone.Targets[locale] = cloneSegs
		}
		for k, v := range block.Properties {
			clone.Properties[k] = v
		}
		_ = clone
	}
}
