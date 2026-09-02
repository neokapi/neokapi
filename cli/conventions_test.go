package cli

import (
	"os"
	"path/filepath"
	"regexp"
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
	{"memory", "stats"},
	{"memory", "sessions", "list"},
	{"memory", "sessions", "show"},
	{"memory", "sessions", "delete"},
	{"terms", "stats"},
	{"inspect"},
	{"info"},
	{"ls"},
	{"add"},
	{"rm"},
}

func newTestRoot(t *testing.T) (*cobra.Command, *App) {
	t.Helper()
	app := &App{SourceLang: "en"}
	// Carry the real root's Short/Long so the two most visible strings in the
	// product are held to the vocabulary rule, not exempted by a bare root.
	root := &cobra.Command{Use: "kapi", Short: KapiRootShort, Long: KapiRootLong}
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

// retiredVocabulary matches the wording this project has retired. Inflections
// are the point: a noun-only search misses "localized", which is how most of
// these survive.
//
// TMX and TBX are deliberately absent — they are external file-format standards
// we do not own, and naming them is correct.
//
// "voice" is here because CLI help names the mechanism, and the mechanism
// is the voice profile. Positioning prose that sells the use case ("keep it on
// brand") lives on the docs and landing surfaces, not in help text.
var retiredVocabulary = regexp.MustCompile(`(?i)termbases?|glossar(?:y|ies)|locali[sz](?:e|es|ed|ing|ation|ations)|l10n|translation memor(?:y|ies)|brand[- ]voice`)

// retiredCasedVocabulary matches the retired wording that case alone separates
// from a name a rename may not touch. The checks family retired the uppercase
// initialism as a word for itself; lowercase `qa` is the tool `kapi exec qa`
// runs, the flow id in `kapi run translate-qa`, the overlay type and the gate
// id, and help text spells all of those. The word boundaries keep an identifier
// out too: a camel-cased QAFoo carries none around the initialism.
//
// scripts/check-vocabulary.sh holds the file-backed surfaces to the same rule;
// keep the two in step.
var retiredCasedVocabulary = regexp.MustCompile(`\bQA\b|[Qq]uality [Aa]ssurance`)

// TestConvention_HelpTextUsesCurrentVocabulary holds the CLI's own help to the
// vocabulary rule in CLAUDE.md.
//
// The docs site, the landing pages and the web app are swept by
// scripts/check-vocabulary.sh, which reads files. Help text is not reachable
// that way: it is assembled from Go string literals spread across the command
// constructors, so a grep either misses it or drowns in identifiers and
// comments. Walking the built tree is exact — it sees the words a user sees,
// and only those.
//
// This is a user-facing surface like any other. `kapi terms --help` documented a
// "~/.config/kapi/termbases/<n>.db" path for years after named stores moved to
// <ConfigDir>/terms/ — retired wording and a wrong answer in the same line.
func TestConvention_HelpTextUsesCurrentVocabulary(t *testing.T) {
	root, _ := newTestRoot(t)

	// The root's own Short and Long are the two most visible strings in the
	// product and the easiest to exempt by accident: a bare `&cobra.Command{Use:
	// "kapi"}` carries neither, and the walk below then passes over the words a
	// user reads first. Assert the fixture is the real root before trusting it.
	require.Equal(t, KapiRootShort, root.Short, "the test root must carry the real root's Short")
	require.Equal(t, KapiRootLong, root.Long, "the test root must carry the real root's Long")

	var walk func(cmd *cobra.Command, path string)
	walk = func(cmd *cobra.Command, path string) {
		check := func(field, text string) {
			for line := range strings.SplitSeq(text, "\n") {
				found := retiredVocabulary.FindAllString(line, -1)
				found = append(found, retiredCasedVocabulary.FindAllString(line, -1)...)
				if found != nil {
					t.Errorf("%s (%s) uses retired vocabulary %v: %s",
						path, field, found, strings.TrimSpace(line))
				}
			}
		}
		check("Short", cmd.Short)
		check("Long", cmd.Long)
		check("Example", cmd.Example)
		// Group titles are section headings in `kapi --help` — as visible as any
		// Short line, and registered far from the commands they gather, which is
		// how "Localization:" outlived the sweep that renamed everything under it.
		for _, g := range cmd.Groups() {
			check("group "+g.ID, g.Title)
		}
		// Flag usage is help text too, and it is the easiest place to forget:
		// it is written inline at the registration call, far from the Long
		// paragraph a reviewer reads.
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			check("--"+f.Name, f.Usage)
		})

		for _, sub := range cmd.Commands() {
			walk(sub, path+" "+sub.Name())
		}
	}
	walk(root, "kapi")
}

// TestConvention_ToolExamplesResolveInCommandTree holds every tool's Example to
// the real command tree: registry tools mount under `kapi exec <tool>` (there
// is no `kapi qa`), so an example that names the bare verb resolves to no
// subcommand and would print a command the user cannot run. `translate` and
// `pseudo-translate` are the exception — they own top-level commands.
func TestConvention_ToolExamplesResolveInCommandTree(t *testing.T) {
	app := &App{SourceLang: "en"}
	app.InitRegistries()
	root := &cobra.Command{Use: "kapi"}
	AddCommandGroups(app, root)
	root.AddCommand(KapiCommandSet(app)...)

	for tool, examples := range ToolExamples {
		for line := range strings.SplitSeq(examples, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			require.Truef(t, strings.HasPrefix(line, "kapi "), "example must start with 'kapi ': %q", line)
			args := strings.Fields(strings.TrimPrefix(line, "kapi "))
			cmd, _, err := root.Find(args)
			require.NoErrorf(t, err, "tool %q example %q", tool, line)
			assert.NotSamef(t, root, cmd,
				"tool %q example resolves to no subcommand — a registry tool needs the `kapi exec` prefix: %q", tool, line)
		}
	}
}
