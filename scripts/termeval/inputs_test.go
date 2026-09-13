package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/terms"
)

// TestWithForms_KeepsStoreForms pins that the reviewed forms add to what the
// store declares: a form the store gives a term survives an overlay that names
// the same term.
func TestWithForms_KeepsStoreForms(t *testing.T) {
	tests := []struct {
		name  string
		store []string
		entry *FormsEntry
		want  []string
	}{
		{
			name:  "overlay naming the term keeps the store forms first",
			store: []string{"trakk ut", "trukket ut"},
			entry: &FormsEntry{ConceptID: "term:extract", Locale: "nb", Term: "Trekk ut", Forms: []string{"trekke ut", " Trakk ut ", "trekk ut", "trekker ut"}},
			want:  []string{"trakk ut", "trukket ut", "trekke ut", "trekker ut"},
		},
		{
			name:  "overlay adds forms to a term the store gives none",
			entry: &FormsEntry{ConceptID: "term:extract", Locale: "nb", Term: "trekk ut", Forms: []string{"trekke ut"}},
			want:  []string{"trekke ut"},
		},
		{
			name:  "term the overlay does not name keeps its store forms",
			store: []string{"trakk ut"},
			want:  []string{"trakk ut"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			concepts := []terms.Concept{{
				ID: "term:extract",
				Terms: []terms.Term{
					{Text: "extract", Locale: "en"},
					{Text: "trekk ut", Locale: "nb", Forms: tt.store},
				},
			}}
			var entries []FormsEntry
			if tt.entry != nil {
				entries = append(entries, *tt.entry)
			}

			got := withForms(concepts, entries)

			assert.Equal(t, tt.want, got[0].Terms[1].Forms)
			assert.Nil(t, got[0].Terms[0].Forms, "a term no entry names gains no forms")
			assert.Equal(t, tt.store, concepts[0].Terms[1].Forms, "the concepts passed in are not modified")
		})
	}
}
