package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreprofile "github.com/neokapi/neokapi/core/profile"
)

func writeTermRulesFile(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

// A rules file is read in the shape the MCP term-check tool takes, in YAML or
// JSON, and a bare list reads as the same thing.
func TestLoadTermRulesFile_ReadsTheMCPShape(t *testing.T) {
	want := []coreprofile.TermRule{{Term: "content memory", Replacement: "innholdsminnet", Advisory: true}}

	rules, err := LoadTermRulesFile(writeTermRulesFile(t, "rules.yaml", "term_rules:\n  - term: content memory\n    replacement: innholdsminnet\n    advisory: true\n"))
	require.NoError(t, err)
	assert.Equal(t, want, rules)

	rules, err = LoadTermRulesFile(writeTermRulesFile(t, "rules.json", `{"term_rules":[{"term":"content memory","replacement":"innholdsminnet","advisory":true}]}`))
	require.NoError(t, err)
	assert.Equal(t, want, rules)

	rules, err = LoadTermRulesFile(writeTermRulesFile(t, "list.yaml", "- term: content memory\n  replacement: innholdsminnet\n  advisory: true\n"))
	require.NoError(t, err)
	assert.Equal(t, want, rules)
}

// A file that names no rule, or a rule with no term, is refused with the file
// named, since a run over it would check against nothing.
func TestLoadTermRulesFile_RefusesAnEmptyOrShapelessFile(t *testing.T) {
	_, err := LoadTermRulesFile(writeTermRulesFile(t, "empty.yaml", "term_rules: []\n"))
	require.ErrorContains(t, err, "names no rule")

	_, err = LoadTermRulesFile(writeTermRulesFile(t, "noterm.yaml", "term_rules:\n  - replacement: x\n"))
	require.ErrorContains(t, err, "rule 1 has no term")

	_, err = LoadTermRulesFile(writeTermRulesFile(t, "scalar.yaml", "just a string\n"))
	require.ErrorContains(t, err, "term_rules:")

	_, err = LoadTermRulesFile(filepath.Join(t.TempDir(), "missing.yaml"))
	require.ErrorContains(t, err, "read term rules")
}
