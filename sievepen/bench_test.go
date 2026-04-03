package sievepen_test

import (
	"fmt"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
)

// BenchmarkTMMatch benchmarks fuzzy TM matching with a populated translation memory.
func BenchmarkTMMatch(b *testing.B) {
	tm := sievepen.NewInMemoryTM()

	// Populate with 100 entries of varying length.
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
	for i := 0; i < 100; i++ {
		base := sentences[i%len(sentences)]
		err := tm.Add(sievepen.TMEntry{
			ID:           fmt.Sprintf("entry-%d", i),
			Source:       model.NewFragment(fmt.Sprintf("%s (variant %d)", base, i)),
			Target:       model.NewFragment(fmt.Sprintf("Translated: %s %d", base, i)),
			SourceLocale: model.LocaleEnglish,
			TargetLocale: model.LocaleFrench,
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
		_, _ = tm.LookupText("The file was saved", model.LocaleEnglish, model.LocaleFrench, opts)
	}
}
