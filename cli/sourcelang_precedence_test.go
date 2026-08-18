package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/host"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The precedence a run's source language follows — explicit flag, then the
// recipe, then the built-in default — is only as good as the registrations it
// rests on. pflag writes a flag's default into the bound field at registration,
// so ONE command in the tree declaring `--source-lang` with a literal default
// puts that language in App.SourceLang for every command in the process, and no
// recipe is reachable from anywhere (#2074).
//
// Which makes the whole command tree the unit under test: build it, then look.

// walkCommands visits cmd and every subcommand beneath it.
func walkCommands(cmd *cobra.Command, visit func(*cobra.Command)) {
	visit(cmd)
	for _, sub := range cmd.Commands() {
		walkCommands(sub, visit)
	}
}

// TestProcessingFlags_NoDefaultOutranksTheRecipe builds kapi's full built-in
// command set and holds every registration of the two flags a recipe key
// governs to an empty default — and the App they all bind to, after the whole
// tree is assembled, to an empty field. The second assertion is the one that
// cannot be worked around: a literal default anywhere lands there.
func TestProcessingFlags_NoDefaultOutranksTheRecipe(t *testing.T) {
	governed := map[string]string{
		"source-lang": "defaults.source_language",
		"encoding":    "defaults.encoding",
	}

	a := &App{}
	a.InitRegistries()

	for _, cmd := range KapiCommandSet(a) {
		walkCommands(cmd, func(c *cobra.Command) {
			c.Flags().VisitAll(func(f *pflag.Flag) {
				key, ok := governed[f.Name]
				if !ok {
					return
				}
				assert.Empty(t, f.DefValue,
					"`kapi %s --%s` registers a default of %q, which pflag writes into the "+
						"bound field at registration — the recipe's %s can then never be reached",
					c.Name(), f.Name, f.DefValue, key)
			})
		})
	}

	assert.Empty(t, a.SourceLang,
		"assembling the command tree must leave the source language unnamed, so a recipe can name it")
	assert.Empty(t, a.Encoding,
		"and the encoding, for the same reason")
	assert.Equal(t, host.DefaultSourceLang, a.SourceLocale(),
		"while every read of it still answers, with the built-in default")
	assert.Equal(t, host.DefaultEncoding, a.InputEncoding())
}

// TestAdHocRun_OutsideAProjectKeepsTheBuiltInDefault is the path the empty
// default had to not break: files named on the command line, no recipe anywhere
// above them, and therefore nothing to adopt a source language from. The run
// reads them in the built-in default and produces output, exactly as before.
func TestAdHocRun_OutsideAProjectKeepsTheBuiltInDefault(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	in := filepath.Join(real, "messages.json")
	require.NoError(t, os.WriteFile(in, []byte(`{"greeting":"Every berth is allocated on arrival."}`), 0o644))
	out := filepath.Join(real, "messages.qps.json")

	a := processOnlyApp(t)
	// pseudo-translate is the keyless built-in flow, so this asserts the ad-hoc
	// path without reaching a provider.
	cmd := NewPseudoTranslateCmd(a)
	stdout, err := runCLI(t, cmd, in, "--target-lang", "qps", "-o", out)
	require.NoError(t, err, stdout)

	assert.Empty(t, a.SourceLang,
		"nothing named a source language: no flag, and no project to ask")
	assert.Equal(t, host.DefaultSourceLang, a.SourceLocale(),
		"so the run read its input in the built-in default")

	produced, rerr := os.ReadFile(out)
	require.NoError(t, rerr, "the ad-hoc run writes its output: %s", stdout)
	assert.NotEmpty(t, produced)
	assert.NotContains(t, string(produced), "Every berth is allocated on arrival.",
		"and the flow actually ran over the content: %s", produced)
}

// And the same run with the flag typed: what the user named is what the run
// works in, with no project involved either way.
func TestAdHocRun_HonoursAnExplicitSourceLang(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	in := filepath.Join(real, "messages.json")
	require.NoError(t, os.WriteFile(in, []byte(`{"greeting":"Every berth is allocated on arrival."}`), 0o644))
	out := filepath.Join(real, "messages.qps.json")

	a := processOnlyApp(t)
	stdout, err := runCLI(t, NewPseudoTranslateCmd(a), in,
		"--source-lang", "en-GB", "--target-lang", "qps", "-o", out)
	require.NoError(t, err, stdout)

	assert.Equal(t, "en-GB", a.SourceLocale(), "the typed language is the run's")
	assert.FileExists(t, out)
}
