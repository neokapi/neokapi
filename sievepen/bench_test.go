package sievepen_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
)

// BenchmarkSQLiteTM_LookupExact benchmarks exact match retrieval on a SQLite TM
// populated with 100 entries, exercising the indexed query path.
func BenchmarkSQLiteTM_LookupExact(b *testing.B) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
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
		err := tm.Add(context.Background(), sievepen.TMEntry{
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

	opts := sievepen.LookupOptions{
		MinScore:   1.0,
		MaxResults: 5,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = tm.LookupText(context.Background(), "The file was saved successfully (variant 0)", model.LocaleEnglish, model.LocaleFrench, opts)
	}
}

func BenchmarkTMMatch(b *testing.B) {
	tm := sievepen.NewInMemoryTM()

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
		err := tm.Add(context.Background(), sievepen.TMEntry{
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

	opts := sievepen.LookupOptions{
		MinScore:   0.6,
		MaxResults: 5,
	}

	b.ResetTimer()
	for b.Loop() {
		_, _ = tm.LookupText(context.Background(), "The file was saved", model.LocaleEnglish, model.LocaleFrench, opts)
	}
}
