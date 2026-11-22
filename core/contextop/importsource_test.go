package contextop

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRecordImportOf(t *testing.T) {
	note := func(text string) Subject { return Subject{Kind: SubjectNote, Text: text} }
	tests := []struct {
		name string
		rec  Record
		want ImportSource
		ok   bool
	}{
		{"a marked import", Record{Kind: KindImport, Subject: note("anything"), Import: &ImportSource{Source: ImportTerms, Path: "a.json", Count: 3}},
			ImportSource{Source: ImportTerms, Path: "a.json", Count: 3}, true},
		{"a terms bundle", Record{Kind: KindImport, Subject: note("terms bundle .kapi/terms.json, 61 concepts")},
			ImportSource{Source: ImportTerms, Path: ".kapi/terms.json", Count: 61}, true},
		{"one concept", Record{Kind: KindImport, Subject: note("terms bundle t.json, 1 concept")},
			ImportSource{Source: ImportTerms, Path: "t.json", Count: 1}, true},
		{"a content-memory bundle", Record{Kind: KindImport, Subject: note("content-memory bundle .kapi/memory/builtins-nb.memory.json, 688 entries")},
			ImportSource{Source: ImportMemory, Path: ".kapi/memory/builtins-nb.memory.json", Count: 688}, true},
		{"a decision record", Record{Kind: KindImport, Subject: note("decision record .kapi/state, 13205 decisions")},
			ImportSource{Source: ImportDecisions, Path: ".kapi/state", Count: 13205}, true},
		{"a voice profile", Record{Kind: KindImport, Subject: note("voice profile .kapi/voice.yaml")},
			ImportSource{Source: ImportVoice, Path: ".kapi/voice.yaml", Count: 1}, true},
		{"unknown text falls back to the evidence", Record{Kind: KindImport, Subject: note("x"), Evidence: []Evidence{{Path: "f.yaml"}}},
			ImportSource{Path: "f.yaml"}, true},
		{"not an import", Record{Kind: KindEdit, Subject: note("terms bundle a, 2 concepts")}, ImportSource{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.rec.ImportOf()
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRecordBulk(t *testing.T) {
	tests := []struct {
		name string
		rec  Record
		want bool
	}{
		{"an import", Record{Kind: KindImport}, true},
		{"a voice edit with its change", Record{Kind: KindEdit, Voice: &VoiceEdit{Profile: "docs"}}, true},
		{"a voice edit recorded without it", Record{Kind: KindEdit, Subject: Subject{Kind: SubjectNote, Text: "voice profile docs edited whole"}}, true},
		{"a note", Record{Kind: KindObserve, Subject: Subject{Kind: SubjectNote, Text: "Use sentence case."}}, false},
		{"a term rule", Record{Kind: KindObserve, Subject: Subject{Kind: SubjectTerm}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.rec.Bulk())
		})
	}
}
