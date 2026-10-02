package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A run can name a target language for a tool only when the tool's locale
// contract says it works on one; `kapi exec` offers --target-lang from this
// answer.
func TestToolMeta_TakesTargetLanguage(t *testing.T) {
	cases := []struct {
		name        string
		meta        *ToolMeta
		takesTarget bool
	}{
		{"nil meta is treated as undeclared", nil, true},
		{"undeclared cardinality is assumed bilingual", &ToolMeta{}, true},
		{"bilingual", &ToolMeta{Cardinality: Bilingual}, true},
		{"multilingual", &ToolMeta{Cardinality: Multilingual}, true},
		{"monolingual, no languages declared", &ToolMeta{Cardinality: Monolingual}, false},
		{"monolingual writer, no languages declared",
			&ToolMeta{Cardinality: Monolingual, WritesOutput: true}, false},
		{"monolingual check that accepts a target",
			&ToolMeta{Cardinality: Monolingual, Accepts: []string{AcceptsTargetLanguage}}, true},
		{"monolingual writer that accepts a target",
			&ToolMeta{Cardinality: Monolingual, WritesOutput: true, Accepts: []string{AcceptsTargetLanguage}}, true},
		{"monolingual that requires a target",
			&ToolMeta{Cardinality: Monolingual, Requires: []string{RequiresTargetLanguage}}, true},
		{"monolingual with a default locale",
			&ToolMeta{Cardinality: Monolingual, DefaultLocale: "qps"}, true},
		{"monolingual that reads the source language",
			&ToolMeta{Cardinality: Monolingual, Accepts: []string{AcceptsSourceLanguage}}, false},
		{"monolingual that requires the source language",
			&ToolMeta{Cardinality: Monolingual, Requires: []string{RequiresSourceLanguage}}, false},
		{"an unrelated accept names no language",
			&ToolMeta{Cardinality: Monolingual, Accepts: []string{AcceptsMemory}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.takesTarget, tc.meta.TakesTargetLanguage())
		})
	}
}
