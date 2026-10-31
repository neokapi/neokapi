package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A recipe that carries a retired or incomplete shape does not load. The
// error names the recipe file and the key path, and says what to write
// instead. Each corrected shape loads.
func TestLoad_RefusesRetiredAndIncompleteShapes(t *testing.T) {
	const head = "version: v1\nname: shapes\ndefaults:\n  source_language: en\n  target_languages: [fr]\n"
	const coll = "collections:\n  - path: \"docs/*.json\"\n"
	const modes = "Write mode: rules for the rule checks, or mode: ai for the AI review"

	load := func(t *testing.T, body string) (string, error) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "kapi.yaml")
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
		_, err := Load(path)
		return path, err
	}

	for _, tc := range []struct {
		name, body, want, fixed string
	}{
		{
			name:  "checkTerminology in a flow step",
			body:  head + coll + "flows:\n  review:\n    steps:\n      - tool: translate\n      - tool: qa\n        config:\n          checkTerminology: true\n",
			want:  "flows.review.steps[1].config.checkTerminology is retired and nothing reads it. Remove the key",
			fixed: head + coll + "flows:\n  review:\n    steps:\n      - tool: translate\n      - tool: qa\n",
		},
		{
			name:  "check_terminology in a parallel branch",
			body:  head + coll + "flows:\n  review:\n    steps:\n      - parallel:\n          - tool: word-count\n          - tool: qa\n            config:\n              check_terminology: false\n",
			want:  "flows.review.steps[0].parallel[1].config.check_terminology is retired and nothing reads it. Remove the key",
			fixed: head + coll + "flows:\n  review:\n    steps:\n      - parallel:\n          - tool: word-count\n          - tool: qa\n",
		},
		{
			name:  "checkTerminology in a qa preset",
			body:  head + "  tools:\n    qa:\n      checkTerminology: true\n" + coll,
			want:  "defaults.tools.qa.checkTerminology is retired and nothing reads it. Remove the key",
			fixed: head + "  tools:\n    qa:\n      checkDoubleSpaces: false\n" + coll,
		},
		{
			name:  "defaults.redaction with rules but no enabled",
			body:  head + "  redaction:\n    rules: redact.yaml\n" + coll,
			want:  "defaults.redaction sets rules without enabled: true, so it redacts nothing. Write enabled: true to apply the policy, or enabled: false with no other key to switch redaction off",
			fixed: head + "  redaction:\n    enabled: true\n    rules: redact.yaml\n" + coll,
		},
		{
			name:  "a qa step with a provider and no mode",
			body:  head + coll + "flows:\n  review:\n    steps:\n      - tool: qa\n        config:\n          provider: anthropic\n",
			want:  "flows.review.steps[0] (qa) sets provider: anthropic without mode, and a qa step that names a provider requires mode. " + modes,
			fixed: head + coll + "flows:\n  review:\n    steps:\n      - tool: qa\n        config:\n          mode: ai\n          provider: anthropic\n",
		},
		{
			name:  "a per-locale qa preset with a provider and no mode",
			body:  head + "  locales:\n    fr:\n      tools:\n        qa:\n          provider: openai\n" + coll,
			want:  "defaults.locales.fr.tools.qa sets provider: openai without mode, and a qa step that names a provider requires mode. " + modes,
			fixed: head + "  locales:\n    fr:\n      tools:\n        qa:\n          mode: ai\n          provider: openai\n" + coll,
		},
		{
			name:  "a qa mode the tool does not take",
			body:  head + coll + "flows:\n  review:\n    steps:\n      - tool: qa\n        config:\n          mode: llm\n",
			want:  "flows.review.steps[0].config.mode is llm, a value qa does not take. " + modes,
			fixed: head + coll + "flows:\n  review:\n    steps:\n      - tool: qa\n        config:\n          mode: rules\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, err := load(t, tc.body)
			require.EqualError(t, err, "invalid project file "+path+": "+tc.want)

			_, err = load(t, tc.fixed)
			assert.NoError(t, err, "the corrected shape loads")
		})
	}
}
