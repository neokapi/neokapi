package model_test

import (
	"fmt"
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

// BenchmarkRunBuilder_AppendText measures the cost of sequential text
// appends to a Run sequence using direct slice growth.
func BenchmarkRunBuilder_AppendText(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		runs := make([]model.Run, 0, 100)
		for i := range 100 {
			runs = append(runs, model.Run{Text: &model.TextRun{Text: fmt.Sprintf("word%d ", i)}})
		}
		_ = runs
	}
}

// BenchmarkRuns_FlattenText measures plain-text extraction from a Run
// sequence with mixed text and inline-code runs.
func BenchmarkRuns_FlattenText(b *testing.B) {
	runs := make([]model.Run, 0, 100)
	for i := range 50 {
		runs = append(runs, model.Run{Text: &model.TextRun{Text: fmt.Sprintf("Some text content %d ", i)}})
		runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
			ID:    fmt.Sprintf("ph%d", i),
			Equiv: fmt.Sprintf("br%d", i),
			Data:  fmt.Sprintf("<br id=\"%d\"/>", i),
		}})
	}
	runs = append(runs, model.Run{Text: &model.TextRun{Text: "Final text segment."}})

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = model.RunsText(runs)
	}
}

// BenchmarkRuns_Clone measures deep cloning of a Run sequence with
// inline codes via a manual element-wise copy.
func BenchmarkRuns_Clone(b *testing.B) {
	runs := make([]model.Run, 0, 80)
	for i := range 20 {
		runs = append(runs, model.Run{Text: &model.TextRun{Text: fmt.Sprintf("Segment %d with content ", i)}})
		runs = append(runs, model.Run{PcOpen: &model.PcOpenRun{
			ID: fmt.Sprintf("s%d", i), Type: "fmt:bold", Data: "<b>",
		}})
		runs = append(runs, model.Run{Text: &model.TextRun{Text: "bold text"}})
		runs = append(runs, model.Run{PcClose: &model.PcCloseRun{
			ID: fmt.Sprintf("s%d", i), Type: "fmt:bold", Data: "</b>",
		}})
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		clone := make([]model.Run, len(runs))
		copy(clone, runs)
		_ = clone
	}
}

// BenchmarkBlock_Clone measures deep cloning of a Block with source and
// target segments across multiple locales.
func BenchmarkBlock_Clone(b *testing.B) {
	block := model.NewBlock("tu-bench", "The quick brown fox jumps over the lazy dog.")
	block.SetTargetText(model.LocaleFrench, "Le rapide renard brun saute par-dessus le chien paresseux.")
	block.Properties["context"] = "test sentence"
	block.Properties["domain"] = "general"

	// Add more source content + a segmentation overlay to exercise cloning.
	block.Source = append(block.Source,
		model.Run{Text: &model.TextRun{Text: "A second segment with more content for realism."}})
	block.SetSegmentation(nil, []model.Span{
		{ID: "s1", Range: model.RunRange{StartRun: 0, EndRun: 1}},
		{ID: "s2", Range: model.RunRange{StartRun: 1, EndRun: 2}},
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
			Source:       append([]model.Run(nil), block.Source...),
			Targets:      make(map[model.VariantKey]*model.Target, len(block.Targets)),
			Overlays:     append([]model.Overlay(nil), block.Overlays...),
			Properties:   make(map[string]string, len(block.Properties)),
		}
		for key, t := range block.Targets {
			clone.Targets[key] = &model.Target{
				Runs:   append([]model.Run(nil), t.Runs...),
				Status: t.Status,
				Origin: t.Origin,
				Score:  t.Score,
			}
		}
		for k, v := range block.Properties {
			clone.Properties[k] = v
		}
		_ = clone
	}
}
