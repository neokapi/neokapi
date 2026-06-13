package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/brand/packs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runBrandValidate executes `brand validate <path>` (always with --json) on a
// fresh command, capturing the structured output and the RunE error so callers
// can assert the verdict, the problems, and the exit code.
func runBrandValidate(t *testing.T, path string) (output.BrandValidateOutput, error) {
	t.Helper()
	a := &App{}
	cmd := a.newBrandValidateCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{path, "--json"})

	runErr := cmd.Execute()

	var parsed output.BrandValidateOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed),
		"brand validate must emit valid JSON: %s", buf.String())
	return parsed, runErr
}

// writeTempProfile writes content to a temp .yaml file and returns its path.
func writeTempProfile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "profile.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// errorFields collapses validation problems into a field→message map.
func errorFields(o output.BrandValidateOutput) map[string]string {
	m := make(map[string]string, len(o.Errors))
	for _, e := range o.Errors {
		m[e.Field] = e.Message
	}
	return m
}

func TestBrandValidate_Valid(t *testing.T) {
	path := writeTempProfile(t, `name: Acme
tone:
  formality: neutral
  emotion: warm
style:
  sentence_length: varied
  prohibited_patterns:
    - regex: '\b(synergy|leverage)\b'
      description: jargon
      severity: minor
vocabulary:
  forbidden_terms:
    - term: utilize
      replacement: use
      severity: minor
`)
	out, runErr := runBrandValidate(t, path)

	require.NoError(t, runErr, "a valid profile must exit 0")
	assert.Equal(t, ExitOK, ExitCode(nil, runErr))
	assert.True(t, out.Valid, "profile must be valid: %+v", out.Errors)
	assert.Empty(t, out.Errors)
	assert.Equal(t, "Acme", out.Profile)
}

func TestBrandValidate_TemplateIsValid(t *testing.T) {
	// The scaffold emitted by `kapi brand new` must validate cleanly.
	out, runErr := runBrandValidate(t, writeTempProfile(t, brandProfileTemplate))
	require.NoError(t, runErr)
	assert.True(t, out.Valid, "brand new template must validate: %+v", out.Errors)
}

func TestBrandValidate_StarterPacksAreValid(t *testing.T) {
	names, _ := packs.List()
	require.NotEmpty(t, names, "expected built-in starter packs")
	for _, n := range names {
		p, err := packs.Load(n)
		require.NoError(t, err)
		assert.Empty(t, brand.ValidateProfile(p), "starter pack %q must validate", n)
	}
}

func TestBrandValidate_MissingName(t *testing.T) {
	out, runErr := runBrandValidate(t, writeTempProfile(t, "description: a profile with no name\n"))

	require.ErrorIs(t, runErr, ErrSilentExit, "an invalid profile must return a non-zero exit")
	assert.Equal(t, ExitError, ExitCode(nil, runErr), "invalid profile maps to exit 1")
	assert.False(t, out.Valid)
	assert.Equal(t, "name is required", errorFields(out)["name"])
}

func TestBrandValidate_ManyProblems(t *testing.T) {
	// One fixture covering: unknown field, missing name, invalid enum, bad regex,
	// invalid severity, and an empty vocabulary term.
	path := writeTempProfile(t, `tonee:
  formality: neutral
tone:
  formality: snooty
style:
  prohibited_patterns:
    - regex: "(unclosed"
      severity: minor
    - regex: '\bok\b'
      severity: showstopper
vocabulary:
  forbidden_terms:
    - term: ""
      replacement: use
`)
	out, runErr := runBrandValidate(t, path)

	require.ErrorIs(t, runErr, ErrSilentExit)
	assert.Equal(t, ExitError, ExitCode(nil, runErr))
	assert.False(t, out.Valid)

	fields := errorFields(out)
	// Unknown field (strict decode).
	require.Contains(t, fields, "tonee")
	assert.Contains(t, fields["tonee"], "unknown field")
	// Missing required name.
	assert.Equal(t, "name is required", fields["name"])
	// Invalid enum value.
	assert.Contains(t, fields, "tone.formality")
	assert.Contains(t, fields["tone.formality"], "snooty")
	// Uncompilable regex.
	assert.Contains(t, fields, "style.prohibited_patterns[0].regex")
	assert.Contains(t, fields["style.prohibited_patterns[0].regex"], "invalid regex")
	// Unknown severity.
	assert.Contains(t, fields, "style.prohibited_patterns[1].severity")
	// Empty term.
	assert.Equal(t, "term is empty", fields["vocabulary.forbidden_terms[0].term"])
}

func TestBrandValidate_SyntaxError(t *testing.T) {
	// A YAML type error (sequence where a string is expected) stops parsing; the
	// profile is reported invalid with a single parse problem.
	path := writeTempProfile(t, "name: [not, a, string]\n")
	out, runErr := runBrandValidate(t, path)

	require.ErrorIs(t, runErr, ErrSilentExit)
	assert.False(t, out.Valid)
	require.NotEmpty(t, out.Errors)
}

func TestBrandValidate_JSONShape(t *testing.T) {
	// The --json result is exactly {valid, errors:[...]} (errors present even when
	// valid), so CI can rely on the shape.
	a := &App{}
	cmd := a.newBrandValidateCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{writeTempProfile(t, "name: Shape\n"), "--json"})
	require.NoError(t, cmd.Execute())

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(buf.Bytes(), &raw))
	assert.Contains(t, raw, "valid")
	assert.Contains(t, raw, "errors")
}

func TestBrandValidate_Stdin(t *testing.T) {
	withStdin(t, "name: Piped\n", func() {
		out, runErr := runBrandValidate(t, "-")
		require.NoError(t, runErr)
		assert.True(t, out.Valid, "%+v", out.Errors)
		assert.Equal(t, "stdin", out.Source)
	})
}

// withStdin runs fn with os.Stdin backed by a pipe carrying content.
func withStdin(t *testing.T, content string, fn func()) {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, werr := w.WriteString(content)
	require.NoError(t, werr)
	require.NoError(t, w.Close())
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })
	fn()
}
