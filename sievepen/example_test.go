package sievepen_test

import (
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
)

func ExampleNewInMemoryTM() {
	tm := sievepen.NewInMemoryTM()

	// Add a translation entry.
	err := tm.Add(sievepen.TMEntry{
		ID:           "e1",
		Source:       model.NewFragment("Save"),
		Target:       model.NewFragment("Enregistrer"),
		SourceLocale: "en",
		TargetLocale: "fr",
	})
	if err != nil {
		panic(err)
	}

	fmt.Println(tm.Count())

	// Look up a match by text.
	matches, err := tm.LookupText("Save", "en", "fr", sievepen.DefaultLookupOptions())
	if err != nil {
		panic(err)
	}

	fmt.Println(len(matches))
	fmt.Println(matches[0].Entry.TargetText())
	fmt.Println(matches[0].Score)
	// Output:
	// 1
	// 1
	// Enregistrer
	// 1
}

func ExampleNewInMemoryTM_fuzzyMatch() {
	tm := sievepen.NewInMemoryTM()

	_ = tm.Add(sievepen.TMEntry{
		ID:           "e1",
		Source:       model.NewFragment("Save the document"),
		Target:       model.NewFragment("Enregistrer le document"),
		SourceLocale: "en",
		TargetLocale: "fr",
	})

	// A similar but not identical source text returns a fuzzy match.
	// Use a lower MinScore to include partial matches.
	opts := sievepen.LookupOptions{MinScore: 0.5, MaxResults: 10}
	matches, _ := tm.LookupText("Save the file", "en", "fr", opts)
	if len(matches) > 0 {
		fmt.Printf("score=%.2f type=%s\n", matches[0].Score, matches[0].MatchType)
	}
	// Output:
	// score=0.59 type=fuzzy
}
