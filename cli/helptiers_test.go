package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/skills"
)

// newTieredRoot builds a root wired the way kapi/cmd/kapi wires its own: the
// global flags, the groups, the built-in command set, the context notice, the
// styled help and the help command with its topics.
func newTieredRoot(t *testing.T) *cobra.Command {
	t.Helper()
	a := processOnlyApp(t)
	root := &cobra.Command{
		Use:           "kapi",
		Short:         KapiRootShort,
		Long:          KapiRootLong,
		Version:       "test",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          func(cmd *cobra.Command, args []string) error { return RootRunE(a, cmd, args) },
	}
	AddPersistentFlags(a, root)
	AddCommandGroups(a, root)
	AttachContextNotice(a, root)
	root.AddCommand(KapiCommandSet(a)...)
	SetupStyledHelp(root)
	SetupHelpTopics(root)
	return root
}

// runRoot executes args on a fresh tiered root and returns what it printed to
// its own streams and to the process's standard output, where status prints.
func runRoot(t *testing.T, args ...string) string {
	t.Helper()
	root := newTieredRoot(t)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	stdout, err := captureStdout(t, root.Execute)
	require.NoError(t, err, buf.String())
	return stdout + buf.String()
}

// writeTierRecipe writes a one-collection project and returns its directory.
// With target set, the recipe declares a target language.
func writeTierRecipe(t *testing.T, target bool) string {
	t.Helper()
	dir := t.TempDir()
	recipe := "version: v1\nname: tiers\ndefaults:\n  source_language: en\n"
	if target {
		recipe += "  target_languages: [nb]\n"
	}
	recipe += "collections:\n  - path: en.json\n"
	if target {
		recipe += "    target: \"{lang}.json\"\n"
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "en.json"), []byte(`{"a":"Apple"}`), 0o644))
	return dir
}

// isolateHelp points every root kapi reads outside the project at throwaway
// directories (the dogfood isolation contract in CLAUDE.md).
func isolateHelp(t *testing.T) {
	t.Helper()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
}

// helpSection returns the lines under heading, up to the next blank line.
func helpSection(help, heading string) []string {
	_, rest, ok := strings.Cut(help, "\n"+heading+"\n")
	if !ok {
		return nil
	}
	block, _, _ := strings.Cut(rest, "\n\n")
	return strings.Split(block, "\n")
}

// flagNames returns the long name of each flag listed in a flags section.
func flagNames(lines []string) []string {
	long := regexp.MustCompile(`--([A-Za-z][\w-]*)`)
	var names []string
	for _, line := range lines {
		if m := long.FindStringSubmatch(line); m != nil {
			names = append(names, m[1])
		}
	}
	sort.Strings(names)
	return names
}

// commandRows returns the command names listed in a group section.
func commandRows(lines []string) []string {
	var names []string
	for _, line := range lines {
		if fields := strings.Fields(line); len(fields) > 0 {
			names = append(names, fields[0])
		}
	}
	return names
}

// TestRootHelp_RevealsLanguagesForATargetLanguage: the Languages group is on
// the root help exactly when the project kapi resolves declares a target
// language. KAPI_NO_PROJECT turns discovery off, and with it the group.
func TestRootHelp_RevealsLanguagesForATargetLanguage(t *testing.T) {
	isolateHelp(t)
	tests := []struct {
		name      string
		target    bool
		noProject bool
		want      bool
	}{
		{name: "recipe declares a target language", target: true, want: true},
		{name: "recipe declares no target language", target: false, want: false},
		{name: "KAPI_NO_PROJECT hides it", target: true, noProject: true, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeTierRecipe(t, tt.target)
			t.Setenv("KAPI_PROJECT", dir)
			if tt.noProject {
				t.Setenv("KAPI_NO_PROJECT", "1")
			} else {
				t.Setenv("KAPI_NO_PROJECT", "")
			}

			help := runRoot(t, "--help")
			if tt.want {
				assert.Contains(t, help, "\nLanguages:\n")
			} else {
				assert.NotContains(t, help, "\nLanguages:\n")
			}
			assert.NotContains(t, help, "\nAdvanced:\n", "the engine tier is only in kapi help --all")

			// The footer is cobra's line plus where the left-out commands are
			// listed; the languages line is for a page that leaves that group out.
			footer := []string{
				`Use "kapi [command] --help" for more information about a command.`,
				`Use "kapi help --all" to list every command.`,
			}
			if !tt.want {
				footer = append(footer, `Use "kapi help languages" to list the commands for other languages.`)
			}
			lines := strings.Split(strings.TrimRight(help, "\n"), "\n")
			require.Greater(t, len(lines), len(footer))
			assert.Equal(t, footer, lines[len(lines)-len(footer):])
			assert.Empty(t, lines[len(lines)-len(footer)-1], "a blank line sets the footer off")
			for _, line := range footer {
				assert.LessOrEqual(t, len(line), 80, "footer line %q is wider than 80 columns", line)
			}
		})
	}
}

// runRootSplit executes args on a fresh tiered root and returns what it wrote
// to standard output and to standard error, separately: the command's own
// streams and the process's.
func runRootSplit(t *testing.T, args ...string) (stdout, stderr string) {
	t.Helper()
	root := newTieredRoot(t)
	var out, errs bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errs)
	root.SetArgs(args)

	outR, outW, err := os.Pipe()
	require.NoError(t, err)
	errR, errW, err := os.Pipe()
	require.NoError(t, err)
	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	runErr := root.Execute()
	_ = outW.Close()
	_ = errW.Close()
	os.Stdout, os.Stderr = origOut, origErr

	var procOut, procErr bytes.Buffer
	_, _ = procOut.ReadFrom(outR)
	_, _ = procErr.ReadFrom(errR)
	require.NoError(t, runErr, errs.String())
	return out.String() + procOut.String(), errs.String() + procErr.String()
}

// TestRootHelp_PrintsNoRecipeWarning: help reads the project's recipe to choose
// its tiers and must never warn about it. project.Load reports near-miss keys
// through project.OnKeyWarnings, which host.App.Init points at standard error,
// so help decodes the recipe instead. A recorder stands in for the hook, and
// the recipe carries a near-miss key: every help view leaves the recorder and
// standard error empty, and project.Load on the same recipe fills the recorder.
func TestRootHelp_PrintsNoRecipeWarning(t *testing.T) {
	isolateHelp(t)
	dir := t.TempDir()
	recipe := filepath.Join(dir, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte(`version: v1
name: nearmiss
defaults:
  source_language: en
  source: en
  target_languages: [nb]
collections:
  - path: en.json
    target: "{lang}.json"
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "en.json"), []byte(`{"a":"Apple"}`), 0o644))
	t.Setenv("KAPI_NO_PROJECT", "")
	t.Setenv("KAPI_PROJECT", dir)

	var recorded []string
	previous := project.OnKeyWarnings
	project.OnKeyWarnings = func(_ string, warnings []string) { recorded = append(recorded, warnings...) }
	t.Cleanup(func() { project.OnKeyWarnings = previous })

	for _, args := range [][]string{{"--help"}, {"help"}, {"help", "--all"}, {"help", "languages"}} {
		stdout, stderr := runRootSplit(t, args...)
		assert.NotEmpty(t, stdout, "kapi %s prints help", strings.Join(args, " "))
		assert.Empty(t, stderr, "kapi %s writes nothing to standard error", strings.Join(args, " "))
		if args[0] == "--help" {
			assert.Contains(t, stdout, "\nLanguages:\n", "the decoded recipe declares a target language")
		}
	}
	assert.Empty(t, recorded, "no help view reports the recipe's near-miss key")

	_, err := project.Load(recipe)
	require.NoError(t, err)
	assert.NotEmpty(t, recorded, "project.Load reports the near-miss key, so the recipe exercises the hook")
}

// languageWork matches a line that names work in other languages, which a
// person in a one-language project should never read.
var languageWork = regexp.MustCompile(`(?i)translat|locale|bilingual|monolingual|target language|per-language|l10n|localiz`)

// TestOneLanguageProject_NamesNoLanguageWork: a person in a one-language
// project reads no word about other languages on the root help, on bare
// `kapi`, or in `kapi status`.
func TestOneLanguageProject_NamesNoLanguageWork(t *testing.T) {
	recipe, root := monolingualProject(t)
	t.Setenv("KAPI_NO_PROJECT", "")
	t.Setenv("KAPI_PROJECT", recipe)

	banned := languageWork
	for _, tt := range []struct {
		name string
		args []string
	}{
		{name: "kapi --help", args: []string{"--help"}},
		{name: "kapi help", args: []string{"help"}},
		{name: "kapi", args: nil},
		{name: "kapi status", args: []string{"status"}},
		{name: "kapi look", args: []string{"look"}},
		{name: "kapi look <path>", args: []string{"look", filepath.Join(root, "docs", "berths.md")}},
		{name: "kapi fix --help", args: []string{"fix", "--help"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out := runRoot(t, tt.args...)
			require.NotEmpty(t, strings.TrimSpace(out))
			for line := range strings.SplitSeq(out, "\n") {
				if banned.MatchString(line) {
					t.Errorf("%s names language work: %q", tt.name, line)
				}
			}
		})
	}

	help := runRoot(t, "--help")
	assert.NotContains(t, help, "\nLanguages:\n", "the Languages group stays off the default page")
	assert.NotContains(t, help, languagesLead, "the kapi help languages lead stays off the default page")
}

// TestFreshOneLanguageProject_StatusNamesTheNextStep: a project `kapi init`
// just scaffolded has no content and no target language. Its status is one
// source line naming the command that adds content, never an empty report.
// `kapi look` prints that line in place of the Map's counts, and bare `kapi`
// prints it above its hint. None names language work.
func TestFreshOneLanguageProject_StatusNamesTheNextStep(t *testing.T) {
	isolateHelp(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi.yaml"),
		[]byte("version: v1\nname: fresh\ndefaults:\n  source_language: en\ncollections: []\n"), 0o644))
	t.Setenv("KAPI_NO_PROJECT", "")
	t.Setenv("KAPI_PROJECT", dir)

	const line = "source      no content yet: add files with `kapi add <pattern>`"
	banned := languageWork
	for _, tt := range []struct {
		name string
		args []string
	}{
		{name: "kapi status", args: []string{"status"}},
		{name: "kapi look", args: []string{"look"}},
		{name: "kapi", args: nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out := runRoot(t, tt.args...)
			assert.True(t, strings.HasPrefix(out, line+"\n"), "%s opens with the source line:\n%s", tt.name, out)
			for l := range strings.SplitSeq(out, "\n") {
				if banned.MatchString(l) {
					t.Errorf("%s names language work: %q", tt.name, l)
				}
			}
		})
	}
}

// TestRootHelp_FlagTiers: the default root help lists the flags a writer uses;
// `kapi help --all` lists every root flag; a subcommand's --help lists every
// global flag under "Global Flags:". No flag is hidden for it, so completion
// still offers every one.
func TestRootHelp_FlagTiers(t *testing.T) {
	isolateHelp(t)
	t.Setenv("KAPI_NO_PROJECT", "1")

	var allowed []string
	for name := range rootDefaultFlags {
		allowed = append(allowed, name)
	}
	sort.Strings(allowed)

	root := newTieredRoot(t)
	root.InitDefaultHelpFlag()
	root.InitDefaultVersionFlag()
	var every []string
	root.LocalFlags().VisitAll(func(f *pflag.Flag) {
		assert.False(t, f.Hidden, "--%s is hidden", f.Name)
		every = append(every, f.Name)
	})
	sort.Strings(every)
	require.Greater(t, len(every), len(allowed), "the root has developer flags beyond the default set")

	defaultHelp := runRoot(t, "--help")
	assert.Equal(t, allowed, flagNames(helpSection(defaultHelp, "Flags:")),
		"the default root help lists exactly the writer's flags")

	allHelp := runRoot(t, "help", "--all")
	assert.Equal(t, every, flagNames(helpSection(allHelp, "Flags:")),
		"kapi help --all lists every root flag")

	inspect, _, err := root.Find([]string{"inspect"})
	require.NoError(t, err)
	own := map[string]bool{}
	inspect.LocalFlags().VisitAll(func(f *pflag.Flag) { own[f.Name] = true })
	var global []string
	root.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if !own[f.Name] {
			global = append(global, f.Name)
		}
	})
	sort.Strings(global)
	inspectHelp := runRoot(t, "inspect", "--help")
	assert.Equal(t, global, flagNames(helpSection(inspectHelp, "Global Flags:")),
		"kapi inspect --help lists every global flag")

	completion := runRoot(t, "__complete", "inspect", "--")
	for _, name := range []string{"plugin-dir", "explain-prompts", "output-format", "lang", "config", "jq", "text"} {
		assert.Contains(t, completion, "--"+name, "completion offers --%s", name)
	}
}

// TestHelpLanguages_ListsTheLanguagesGroup: `kapi help languages` prints the
// lead and every command of the Languages group, and none of the engine tier.
func TestHelpLanguages_ListsTheLanguagesGroup(t *testing.T) {
	isolateHelp(t)
	t.Setenv("KAPI_NO_PROJECT", "1")

	out := runRoot(t, "help", "languages")
	assert.True(t, strings.HasPrefix(out, languagesLead), "the lead comes first:\n%s", out)

	rows := commandRows(helpSection(out, "Languages:"))
	root := newTieredRoot(t)
	var want, advanced []string
	for _, c := range root.Commands() {
		if !c.IsAvailableCommand() {
			continue
		}
		switch c.GroupID {
		case languagesGroupID:
			want = append(want, c.Name())
		case "advanced":
			advanced = append(advanced, c.Name())
		}
	}
	require.NotEmpty(t, want)
	assert.ElementsMatch(t, want, rows, "every command in the Languages group, and only those")
	for _, name := range advanced {
		assert.NotContains(t, out, "\n  "+name+" ", "the engine tier's %s is not listed", name)
	}
}

// TestHelpTopics_FollowTheTiers: the topic list under `kapi help` follows the
// command groups. Outside a project it lists the person tier's topics and
// says where the rest are; `kapi help languages` lists the language topics;
// `kapi help --all` and a project with a target language list every topic.
// Nothing is lost between the two lists.
func TestHelpTopics_FollowTheTiers(t *testing.T) {
	isolateHelp(t)

	var all, language, person []string
	for _, topic := range skills.Topics() {
		all = append(all, topic.Name)
		if topicTier(topic.Name) == tierLanguages {
			language = append(language, topic.Name)
		} else {
			person = append(person, topic.Name)
		}
	}
	require.Contains(t, language, "translate")
	require.Contains(t, language, "i18n")
	require.Contains(t, person, "check")
	for _, name := range language {
		assert.True(t, name == "translate" || name == "i18n" || strings.HasPrefix(name, "i18n-"), name)
	}

	for _, tt := range []struct {
		name      string
		noProject string
		target    bool
		args      []string
		want      []string
		hint      bool
	}{
		{name: "kapi help outside a project", noProject: "1", args: []string{"help"}, want: person, hint: true},
		{name: "kapi help in a one-language project", noProject: "", target: false, args: []string{"help"}, want: person, hint: true},
		{name: "kapi help with a target language", noProject: "", target: true, args: []string{"help"}, want: all},
		{name: "kapi help --all", noProject: "1", args: []string{"help", "--all"}, want: all},
		{name: "kapi help languages", noProject: "1", args: []string{"help", "languages"}, want: language},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("KAPI_NO_PROJECT", tt.noProject)
			t.Setenv("KAPI_PROJECT", writeTierRecipe(t, tt.target))
			out := runRoot(t, tt.args...)
			rows := commandRows(helpSection(out, "Topics:"))
			assert.ElementsMatch(t, tt.want, rows)
			assert.Contains(t, out, `Use "kapi help <topic>" to read a topic.`)
			assert.Equal(t, tt.hint, strings.Contains(out, `Use "kapi help languages" to list the topics for other languages.`),
				"the hint appears exactly when the language topics are left out:\n%s", out)
		})
	}
}

// TestHelpTiers_EveryCommandStillResolves: a tier filters the help page and
// nothing else. Every command in the set resolves whether or not its tier is
// shown, and rendering the tiered help marks no command hidden, so completion
// and the reference keep every one.
func TestHelpTiers_EveryCommandStillResolves(t *testing.T) {
	isolateHelp(t)
	for _, noProject := range []string{"1", ""} {
		t.Run("KAPI_NO_PROJECT="+noProject, func(t *testing.T) {
			t.Setenv("KAPI_NO_PROJECT", noProject)
			t.Setenv("KAPI_PROJECT", writeTierRecipe(t, noProject == ""))

			root := newTieredRoot(t)
			var buf bytes.Buffer
			root.SetOut(&buf)
			root.SetErr(&buf)
			root.SetArgs([]string{"--help"})
			require.NoError(t, root.Execute())

			for _, c := range KapiCommandSet(processOnlyApp(t)) {
				found, _, err := root.Find([]string{c.Name()})
				require.NoError(t, err, c.Name())
				assert.Equal(t, c.Name(), found.Name(), "%s resolves to its own command", c.Name())
				assert.Equal(t, c.Hidden, found.Hidden, "rendering the help changed whether %s is hidden", c.Name())
			}

			completion := "\n" + runRoot(t, "__complete", "")
			for _, name := range []string{"translate", "memory", "extract", "resolve", "telemetry", "completion"} {
				assert.Contains(t, completion, "\n"+name+"\t", "completion offers %s", name)
			}
		})
	}
}
