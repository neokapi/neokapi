package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeOneUnitXLIFF writes a one-unit XLIFF 2.0 file pairing source with
// target, the fixture the MCP conformance suite drives term-check with.
func writeOneUnitXLIFF(t *testing.T, dir, name, source, target string) string {
	t.Helper()
	file := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(file, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="nb">
  <file id="f1">
    <unit id="u1">
      <segment>
        <source>`+source+`</source>
        <target>`+target+`</target>
      </segment>
    </unit>
  </file>
</xliff>
`), 0o644))
	return file
}

// kapi exec term-check --term-rules takes the rules an MCP call would send
// under term_rules, so the two surfaces can be driven with the same input: a
// translation that violates a rule is reported, and one that follows it is
// not.
func TestExecTermCheck_TermRulesFile(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")
	dir := t.TempDir()
	rules := filepath.Join(dir, "rules.yaml")
	require.NoError(t, os.WriteFile(rules, []byte("term_rules:\n  - term: content memory\n    replacement: innholdsminnet\n"), 0o644))
	const source = "Reuse comes from the content memory."
	mustFail := writeOneUnitXLIFF(t, dir, "must-fail.xlf", source, "Gjenbruk kommer fra oversettelsesminnet.")
	mustPass := writeOneUnitXLIFF(t, dir, "must-pass.xlf", source, "Gjenbruk kommer fra innholdsminnet.")

	run := func(file string) map[string]any {
		t.Helper()
		app := &App{Quiet: true}
		app.InitRegistries()
		defer app.Shutdown()
		execCmd := NewToolCommands(app)[0]
		execCmd.SetArgs([]string{"term-check", file, "--term-rules", rules, "--target-lang", "nb", "--json"})
		out, _ := captureStdout(t, func() error { return execCmd.Execute() })
		var report map[string]any
		require.NoError(t, json.Unmarshal([]byte(out), &report), out)
		return report
	}

	failing := run(mustFail)
	require.NotEmpty(t, failing["findings"], "must fail: the rule's term is rendered with something else")
	finding := failing["findings"].([]any)[0].(map[string]any)
	assert.Contains(t, finding["message"], "innholdsminnet")

	assert.Empty(t, run(mustPass)["findings"], "must pass: the rule's replacement is used")
}

// A rules file that cannot be read fails the run with the file named, rather
// than checking against nothing.
func TestExecTermCheck_TermRulesFileMustExist(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")
	dir := t.TempDir()
	file := writeOneUnitXLIFF(t, dir, "one.xlf", "Hello", "Hei")

	app := &App{Quiet: true}
	app.InitRegistries()
	defer app.Shutdown()
	execCmd := NewToolCommands(app)[0]
	execCmd.SetArgs([]string{"term-check", file, "--term-rules", filepath.Join(dir, "missing.yaml"), "--target-lang", "nb", "--json", "--strict"})
	_, err := captureStdout(t, func() error { return execCmd.Execute() })
	require.ErrorContains(t, err, "read term rules")
}
