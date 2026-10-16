package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// enginesApp is an App with a second engine for ".json": a plugin format
// (source okapi-bridge) whose reader is the built-in JSON one, the way the
// registry looks with okapi-bridge installed.
func enginesApp(t *testing.T) *App {
	t.Helper()
	a := &App{Config: config.NewAppConfig()}
	a.InitRegistries()
	readJSON := a.FormatReg.ReaderFactory("json")
	require.NotNil(t, readJSON)
	sig := format.FormatSignature{Extensions: []string{".json"}}
	a.FormatReg.RegisterFormatInfo("okf_json", registry.FormatInfo{
		DisplayName: "Okapi JSON", Extensions: sig.Extensions, Source: "okapi-bridge", HasReader: true,
	})
	a.FormatReg.RegisterReader("okf_json", readJSON, sig, "Okapi JSON")
	a.FormatReg.SetFormatSource("okf_json", "okapi-bridge")
	return a
}

// runFormatsOn runs `kapi formats <args>` on a, applying --engine the way the
// root command does before a command runs.
func runFormatsOn(t *testing.T, a *App, args ...string) (string, error) {
	t.Helper()
	cmd := NewFormatsCmd(a)
	cmd.PersistentFlags().Bool("json", false, "")
	cmd.PersistentPreRunE = func(*cobra.Command, []string) error { return a.ApplyEngineSelection() }
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func decodeExplain(t *testing.T, out string) output.FormatExplainOutput {
	t.Helper()
	var ex output.FormatExplainOutput
	require.NoError(t, json.Unmarshal([]byte(out), &ex), out)
	return ex
}

func TestFormats_ExplainReportsTheDecision(t *testing.T) {
	t.Setenv(project.NoProjectEnvVar, "1")
	a := enginesApp(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "messages.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"a":"b"}`), 0o644))

	out, err := runFormatsOn(t, a, "explain", "--json", src)
	require.NoError(t, err, out)
	ex := decodeExplain(t, out)
	require.Len(t, ex.Files, 1)
	assert.Equal(t, "json", ex.Files[0].Format)
	assert.Equal(t, "native", ex.Files[0].Engine)
	assert.Equal(t, registry.RuleNativeFirst, ex.Files[0].Rule)
	assert.Equal(t, []string{"native", "okapi-bridge"}, ex.Files[0].Order)
	assert.Contains(t, ex.Files[0].Candidates, output.FormatCandidate{Format: "okf_json", Engine: "okapi-bridge"})

	out, err = runFormatsOn(t, a, "explain", "--json", "--engine", "okapi-bridge", src)
	require.NoError(t, err, out)
	ex = decodeExplain(t, out)
	assert.Equal(t, "okf_json", ex.Files[0].Format)
	assert.Equal(t, registry.RuleEngineOverride, ex.Files[0].Rule)

	text, err := runFormatsOn(t, a, "explain", "--engine", "native", src)
	require.NoError(t, err, text)
	assert.Contains(t, text, "json (native)")
	assert.Contains(t, text, "rule: "+registry.RuleEngineOverride)
	assert.Contains(t, text, "okf_json (okapi-bridge)")

	_, err = runFormatsOn(t, a, "explain", "--engine", "okapi", src)
	require.ErrorIs(t, err, registry.ErrNoEngine)

	// --format names the format; an engine in the reference selects that
	// engine's format for the same files.
	out, err = runFormatsOn(t, a, "explain", "--json", "-f", "json@okapi-bridge", src)
	require.NoError(t, err, out)
	ex = decodeExplain(t, out)
	assert.Equal(t, "okf_json", ex.Files[0].Format)
	assert.Equal(t, "okapi-bridge", ex.Files[0].Engine)
	assert.Equal(t, host.RuleFormatFlag, ex.Files[0].Rule)
	out, err = runFormatsOn(t, a, "explain", "--json", "-f", "markdown@okapi-bridge", src)
	require.NoError(t, err, out)
	assert.Contains(t, decodeExplain(t, out).Files[0].Error, "okapi-bridge")
	out, err = runFormatsOn(t, a, "explain", "--json", "-f", "no-such", src)
	require.NoError(t, err, out)
	assert.Contains(t, decodeExplain(t, out).Files[0].Error, "unknown format")
	_, err = runFormatsOn(t, a, "explain")
	require.Error(t, err, "a file is required")
}

// A path no format reads is reported on its line; the command still answers
// for the others.
func TestFormats_ExplainReportsAnUnreadPath(t *testing.T) {
	t.Setenv(project.NoProjectEnvVar, "1")
	a := enginesApp(t)
	out, err := runFormatsOn(t, a, "explain", "--json", "notes.nothing", "a.json")
	require.NoError(t, err, out)
	ex := decodeExplain(t, out)
	require.Len(t, ex.Files, 2)
	assert.Empty(t, ex.Files[0].Format)
	assert.Contains(t, ex.Files[0].Error, ".nothing")
	assert.Equal(t, "json", ex.Files[1].Format)
}

// The listing names each format's engine and marks the extensions more than
// one engine claims.
func TestFormats_ListNamesEnginesAndSharedExtensions(t *testing.T) {
	a := enginesApp(t)
	out, err := runFormatsOn(t, a, "list", "--json", "--ext", ".json")
	require.NoError(t, err, out)
	list := decodeFormats(t, out)
	byName := map[string]output.FormatInfo{}
	for _, f := range list.Formats {
		byName[f.Name] = f
	}
	require.Contains(t, byName, "json")
	require.Contains(t, byName, "okf_json")
	assert.Equal(t, "native", byName["json"].Engine)
	assert.Equal(t, "built-in", byName["json"].Source, "source stays for readers of the old field")
	assert.Equal(t, "okapi-bridge", byName["okf_json"].Engine)
	assert.Equal(t, []string{".json"}, byName["json"].SharedExtensions)
	assert.Equal(t, []string{".json"}, byName["okf_json"].SharedExtensions)

	out, err = runFormatsOn(t, a, "list", "--json", "--ext", ".md")
	require.NoError(t, err, out)
	for _, f := range decodeFormats(t, out).Formats {
		assert.Empty(t, f.SharedExtensions, "%s: .md has one engine", f.Name)
	}

	text, err := runFormatsOn(t, a, "list")
	require.NoError(t, err, text)
	assert.Contains(t, text, "ENGINE")
	assert.Contains(t, text, ".json*")
	assert.Contains(t, text, "kapi formats explain")

	// Without a second engine nothing is marked.
	plain := &App{Config: config.NewAppConfig()}
	plain.InitRegistries()
	text, err = runFormatsOn(t, plain, "list")
	require.NoError(t, err, text)
	assert.NotContains(t, text, "*")
}
