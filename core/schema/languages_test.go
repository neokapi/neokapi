package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The language a run names reaches a tool only when the tool's locale contract
// says it works on that language; a surface offers --source-lang and
// --target-lang from these two answers.
func TestToolMeta_TakesLanguages(t *testing.T) {
	cases := []struct {
		name        string
		meta        *ToolMeta
		takesTarget bool
		takesSource bool
	}{
		{"nil meta is treated as undeclared", nil, true, true},
		{"undeclared cardinality is assumed bilingual", &ToolMeta{}, true, true},
		{"bilingual", &ToolMeta{Cardinality: Bilingual}, true, true},
		{"multilingual", &ToolMeta{Cardinality: Multilingual}, true, true},
		{"monolingual, no languages declared", &ToolMeta{Cardinality: Monolingual}, false, false},
		{"monolingual writer, no languages declared",
			&ToolMeta{Cardinality: Monolingual, WritesOutput: true}, false, false},
		{"monolingual check that accepts a target",
			&ToolMeta{Cardinality: Monolingual, Accepts: []string{AcceptsTargetLanguage}}, true, false},
		{"monolingual writer that accepts a target swaps the source locale in its output path",
			&ToolMeta{Cardinality: Monolingual, WritesOutput: true, Accepts: []string{AcceptsTargetLanguage}}, true, true},
		{"monolingual that requires a target",
			&ToolMeta{Cardinality: Monolingual, Requires: []string{RequiresTargetLanguage}}, true, false},
		{"monolingual with a default locale",
			&ToolMeta{Cardinality: Monolingual, DefaultLocale: "qps"}, true, false},
		{"monolingual that reads the source language",
			&ToolMeta{Cardinality: Monolingual, Accepts: []string{AcceptsSourceLanguage}}, false, true},
		{"monolingual that requires the source language",
			&ToolMeta{Cardinality: Monolingual, Requires: []string{RequiresSourceLanguage}}, false, true},
		{"an unrelated accept names no language",
			&ToolMeta{Cardinality: Monolingual, Accepts: []string{AcceptsMemory}}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.takesTarget, tc.meta.TakesTargetLanguage(), "TakesTargetLanguage")
			assert.Equal(t, tc.takesSource, tc.meta.TakesSourceLanguage(), "TakesSourceLanguage")
		})
	}
}
