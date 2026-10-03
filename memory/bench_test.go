package memory_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
)

// BenchmarkSQLiteMemory_LookupExact benchmarks exact match retrieval on a SQLite content memory
// populated with 100 entries, exercising the indexed query path.
func BenchmarkSQLiteMemory_LookupExact(b *testing.B) {
	tm, err := memory.NewSQLiteStore(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer tm.Close()

	sentences := []string{
		"The file was saved successfully",
		"An error occurred while processing your request",
		"Please enter your username and password",
		"The document has been translated",
		"Click here to continue",
		"Your changes have been saved",
		"Unable to connect to the server",
		"The operation completed successfully",
		"Please wait while the file is being uploaded",
		"The session has expired, please log in again",
	}
	for i := range 100 {
		base := sentences[i%len(sentences)]
		err := tm.Add(context.Background(), memory.Entry{
			ID: fmt.Sprintf("entry-%d", i),
			Variants: map[model.LocaleID][]model.Run{
				model.LocaleEnglish: {{Text: &model.TextRun{Text: fmt.Sprintf("%s (variant %d)", base, i)}}},
				model.LocaleFrench:  {{Text: &model.TextRun{Text: fmt.Sprintf("Translated: %s %d", base, i)}}},
			},
			HintSrcLang: model.LocaleEnglish,
		})
		if err != nil {
			b.Fatal(err)
		}
	}

	opts := memory.LookupOptions{
		MinScore:   1.0,
		MaxResults: 5,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = tm.LookupText(context.Background(), "The file was saved successfully (variant 0)", model.LocaleEnglish, model.LocaleFrench, opts)
	}
}

// BenchmarkSQLiteMemory_LookupExactMiss benchmarks the exact-only question
// `kapi up` puts to the content memory for every unit it plans, over a
// memory of 3,000 entries that holds no exact answer: the case of a unit whose
// source was rewritten since its translation was recorded.
func BenchmarkSQLiteMemory_LookupExactMiss(b *testing.B) {
	tm, err := memory.NewSQLiteStore(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer tm.Close()
	for i := range 3000 {
		err := tm.Add(context.Background(), memory.Entry{
			ID: fmt.Sprintf("entry-%d", i),
			Variants: map[model.LocaleID][]model.Run{
				model.LocaleEnglish: {{Text: &model.TextRun{Text: fmt.Sprintf("Paragraph %d explains how the shop opens every day.", i)}}},
				model.LocaleFrench:  {{Text: &model.TextRun{Text: fmt.Sprintf("Le paragraphe %d explique comment la boutique ouvre.", i)}}},
			},
			HintSrcLang: model.LocaleEnglish,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
	opts := memory.LookupOptions{MinScore: 1.0, MaxResults: 1}

	b.ReportAllocs()
	for b.Loop() {
		m, err := tm.LookupText(context.Background(), "Paragraph 7 explains how the shop opens each day.", model.LocaleEnglish, model.LocaleFrench, opts)
		if err != nil || len(m) != 0 {
			b.Fatalf("an exact-only lookup answered a rewritten source: %v %v", m, err)
		}
	}
}

// BenchmarkSQLiteMemory_LookupBlockExactHit benchmarks the exact-only question
// for a block the memory answers, through every match mode: the entry answers
// under its generalized, structural and plain keys alike.
func BenchmarkSQLiteMemory_LookupBlockExactHit(b *testing.B) {
	tm, err := memory.NewSQLiteStore(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer tm.Close()
	for i := range 3000 {
		err := tm.Add(context.Background(), memory.Entry{
			ID: fmt.Sprintf("entry-%d", i),
			Variants: map[model.LocaleID][]model.Run{
				model.LocaleEnglish: {{Text: &model.TextRun{Text: fmt.Sprintf("Paragraph %d explains how the shop opens every day.", i)}}},
				model.LocaleFrench:  {{Text: &model.TextRun{Text: fmt.Sprintf("Le paragraphe %d explique comment la boutique ouvre.", i)}}},
			},
			HintSrcLang: model.LocaleEnglish,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
	block := &model.Block{ID: "p7"}
	block.SetSourceRuns([]model.Run{{Text: &model.TextRun{Text: "Paragraph 7 explains how the shop opens every day."}}})
	opts := memory.LookupOptions{MinScore: 1.0, MaxResults: 1}

	b.ReportAllocs()
	for b.Loop() {
		m, err := tm.Lookup(context.Background(), block, model.LocaleEnglish, model.LocaleFrench, opts)
		if err != nil || len(m) != 1 {
			b.Fatalf("an exact-only lookup missed its entry: %v %v", m, err)
		}
	}
}

func BenchmarkMemoryMatch(b *testing.B) {
	tm := memory.NewInMemoryStore()

	sentences := []string{
		"The file was saved successfully",
		"An error occurred while processing your request",
		"Please enter your username and password",
		"The document has been translated",
		"Click here to continue",
		"Your changes have been saved",
		"Unable to connect to the server",
		"The operation completed successfully",
		"Please wait while the file is being uploaded",
		"The session has expired, please log in again",
	}
	for i := range 100 {
		base := sentences[i%len(sentences)]
		err := tm.Add(context.Background(), memory.Entry{
			ID: fmt.Sprintf("entry-%d", i),
			Variants: map[model.LocaleID][]model.Run{
				model.LocaleEnglish: {{Text: &model.TextRun{Text: fmt.Sprintf("%s (variant %d)", base, i)}}},
				model.LocaleFrench:  {{Text: &model.TextRun{Text: fmt.Sprintf("Translated: %s %d", base, i)}}},
			},
			HintSrcLang: model.LocaleEnglish,
		})
		if err != nil {
			b.Fatal(err)
		}
	}

	opts := memory.LookupOptions{
		MinScore:   0.6,
		MaxResults: 5,
	}

	b.ResetTimer()
	for b.Loop() {
		_, _ = tm.LookupText(context.Background(), "The file was saved", model.LocaleEnglish, model.LocaleFrench, opts)
	}
}
