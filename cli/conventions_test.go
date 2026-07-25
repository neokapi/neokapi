package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host/output"
)

// The CLI's input/output conventions are contracts, not per-command choices.
// These tests hold the whole command surface to them, so a new command cannot
// quietly reintroduce shell-only globs, a silent stdin hang, or a listing with
// no machine-readable form.

// fileInputCommands are the commands that take file paths positionally. Each
// must accept a glob pattern and a directory argument identically.
var fileInputCommands = []string{"stats", "check", "inspect"}

// listCommands are the commands whose whole purpose is to emit a list or a
// record. Each must offer a machine-readable form.
var listCommands = [][]string{
	{"flows", "list"},
	{"tools", "list"},
	{"formats"},
	{"plugin", "list"},
	{"models", "list"},
	{"config", "list"},
	{"config", "get"},
	{"config", "set"},
	{"config", "unset"},
	{"config", "path"},
	{"credentials", "list"},
	{"version"},
	{"status"},
	{"stats"},
	{"tm", "stats"},
	{"tm", "sessions", "list"},
	{"tm", "sessions", "show"},
	{"tm", "sessions", "delete"},
	{"termbase", "stats"},
	{"inspect"},
	{"info"},
	{"ls"},
	{"add"},
	{"rm"},
}

func newTestRoot(t *testing.T) (*cobra.Command, *App) {
	t.Helper()
	app := &App{SourceLang: "en"}
	root := &cobra.Command{Use: "kapi"}
	AddPersistentFlags(app, root)
	AddCommandGroups(app, root)
	root.AddCommand(KapiCommandSet(app)...)
	return root, app
}

// effectiveFlag looks a flag up on the command's own set, then on the flags it
// inherits from its parents — the set a user actually gets to type.
func effectiveFlag(cmd *cobra.Command, name string) *pflag.Flag {
	if f := cmd.Flags().Lookup(name); f != nil {
		return f
	}
	return cmd.InheritedFlags().Lookup(name)
}

// find walks the command tree by path, e.g. ("plugin", "list").
func find(t *testing.T, root *cobra.Command, path ...string) *cobra.Command {
	t.Helper()
	cur := root
	for _, name := range path {
		var next *cobra.Command
		for _, c := range cur.Commands() {
			if c.Name() == name {
				next = c
				break
			}
		}
		require.NotNilf(t, next, "command %q not found under %q", name, cur.Name())
		cur = next
	}
	return cur
}

// TestConvention_ListCommandsSupportJSON holds every list/record command to the
// machine-readable contract: `-o json` (or the equivalent --json) must resolve
// to the JSON format. A command is allowed to reach it through the persistent
// root flags or its own; what matters is that the user can ask for JSON.
func TestConvention_ListCommandsSupportJSON(t *testing.T) {
	for _, path := range listCommands {
		t.Run(strings.Join(path, " "), func(t *testing.T) {
			root, _ := newTestRoot(t)
			cmd := find(t, root, path...)
			// Either spelling is fine — what matters is that the user can ask
			// for JSON, whether the flag is the command's own or inherited from
			// the root's persistent set.
			require.NotNil(t, effectiveFlag(cmd, "json"),
				"%s must accept --json", strings.Join(path, " "))
			require.NotNil(t, effectiveFlag(cmd, "output-format"),
				"%s must accept --output-format", strings.Join(path, " "))

			// Every value on the shared format axis must resolve, so a caller
			// can pick the shape it wants without knowing which command it is.
			for value, want := range map[string]output.Format{
				"json":  output.FormatJSON,
				"yaml":  output.FormatYAML,
				"text":  output.FormatText,
				"table": output.FormatText,
			} {
				f := effectiveFlag(cmd, "output-format")
				require.NoError(t, f.Value.Set(value))
				f.Changed = true
				assert.Equalf(t, want, output.ResolveFormat(cmd),
					"%s: --output-format %s must resolve to %s", strings.Join(path, " "), value, want)
			}
		})
	}
}

// TestConvention_FileInputCommandsAcceptGlobs asserts a quoted glob is expanded
// by kapi itself, not the shell: the pattern reaches the command verbatim (as
// it would under PowerShell, or from any exec without a shell) and still
// resolves to the matching files.
func TestConvention_FileInputCommandsAcceptGlobs(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, "a.json"), `{"one":"Hello"}`)
	writeJSON(t, filepath.Join(dir, "nested", "b.json"), `{"two":"World"}`)
	writeJSON(t, filepath.Join(dir, "nested", "deep", "c.json"), `{"three":"Again"}`)

	for _, name := range fileInputCommands {
		t.Run(name+" glob", func(t *testing.T) {
			out := runCommand(t, name, filepath.Join(dir, "**", "*.json"))
			assert.NotEmpty(t, out, "%s must produce output for a recursive glob", name)
		})
		t.Run(name+" directory", func(t *testing.T) {
			out := runCommand(t, name, dir)
			assert.NotEmpty(t, out, "%s must accept a directory argument", name)
		})
	}
}

// TestConvention_GlobMatchesEveryDepth pins `**` to recursive semantics —
// filepath.Glob silently matches a single level, which is the bug this
// convention exists to prevent.
func TestConvention_GlobMatchesEveryDepth(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, "a.json"), `{"k":"one"}`)
	writeJSON(t, filepath.Join(dir, "x", "b.json"), `{"k":"two"}`)
	writeJSON(t, filepath.Join(dir, "x", "y", "c.json"), `{"k":"three"}`)

	out := runCommand(t, "stats", "--output-format", "json", filepath.Join(dir, "**", "*.json"))
	for _, want := range []string{"a.json", "b.json", "c.json"} {
		assert.Contains(t, out, want, "`**` must match at every depth")
	}
}

func writeJSON(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

// runCommand executes one built-in command with args and returns stdout.
func runCommand(t *testing.T, args ...string) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "1")

	root, app := newTestRoot(t)
	require.NoError(t, app.Init())

	var out, errOut strings.Builder
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	err := root.Execute()
	require.NoError(t, err, "stderr: %s", errOut.String())
	return out.String()
}
