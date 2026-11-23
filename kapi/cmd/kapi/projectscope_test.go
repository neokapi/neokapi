package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/dataroot"
)

// These tests run the kapi command tree as the binary builds it, inside a
// directory that holds a recipe and state an earlier kapi left in the tree, so
// every one-time notice about the checkout is owed. The data root starts
// empty, and a command that concerns no project must leave it so.

// stateNoticeFixture makes the working directory a checkout owed both
// one-time notices, with a fresh data root and fresh per-user locations, and
// returns the checkout and the data root.
func stateNoticeFixture(t *testing.T) (checkout, data string) {
	t.Helper()
	checkout, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(checkout, "kapi.yaml"), []byte("version: v1\nname: owed\ndefaults:\n  source_language: en\n"), 0o644))
	for _, dir := range []string{".kapi/work", ".kapi/vault"} {
		require.NoError(t, os.MkdirAll(filepath.Join(checkout, dir), 0o755))
	}
	data = t.TempDir()
	t.Setenv(dataroot.EnvDir, data)
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PARSE_CACHE_DIR", "")
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", "")
	t.Setenv("KAPI_TELEMETRY", "0")
	// Discovery is the point: the checkout is found from the working
	// directory, as it is for a person who runs kapi in a repository.
	t.Setenv("KAPI_NO_PROJECT", "")
	t.Setenv("KAPI_PROJECT", "")
	t.Chdir(checkout)
	return checkout, data
}

// dataRootFiles lists every entry under the data root, relative to it.
func dataRootFiles(t *testing.T, data string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(data, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p != data {
			rel, _ := filepath.Rel(data, p)
			out = append(out, rel)
		}
		return nil
	})
	require.NoError(t, err)
	return out
}

// runKapi runs the kapi root with args and returns what it wrote to its error
// stream. Flags a run set are put back to their defaults afterwards, because
// the tree is one value shared by every run in the process.
func runKapi(t *testing.T, args ...string) (stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errOut)
	rootCmd.SetArgs(args)
	defer func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
		resetFlags(rootCmd)
	}()
	err = rootCmd.Execute()
	return errOut.String(), err
}

// resetFlags puts every flag in the tree that a run changed back to its
// default.
func resetFlags(root *cobra.Command) {
	reset := func(f *pflag.Flag) {
		if !f.Changed {
			return
		}
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			_ = sv.Replace(nil)
		} else {
			_ = f.Value.Set(f.DefValue)
		}
		f.Changed = false
	}
	walkCommands(root, func(c *cobra.Command) {
		c.Flags().VisitAll(reset)
		c.PersistentFlags().VisitAll(reset)
	})
}

// walkCommands calls fn for root and every command beneath it.
func walkCommands(root *cobra.Command, fn func(*cobra.Command)) {
	fn(root)
	for _, c := range root.Commands() {
		walkCommands(c, fn)
	}
}

// projectDeclarations sorts the runnable commands under root by what they
// declare. It returns those that resolve no project, those that declare
// nothing, and those that concern a project while parsing no flags. cobra
// leaves --help to a command that parses no flags, so the prerun runs before
// such a command prints its help, and it must resolve no project there.
func projectDeclarations(root *cobra.Command) (none []*cobra.Command, undeclared, helpResolves []string) {
	walkCommands(root, func(c *cobra.Command) {
		if !c.Runnable() {
			return
		}
		uses, declared := cli.ProjectDeclaration(c)
		switch {
		case !declared:
			undeclared = append(undeclared, c.CommandPath())
		case !uses:
			none = append(none, c)
		case c.DisableFlagParsing:
			helpResolves = append(helpResolves, c.CommandPath())
		}
	})
	return none, undeclared, helpResolves
}

// noProjectCommands lists the commands in the tree that resolve no project, and
// fails the test for a runnable command that declares neither way.
func noProjectCommands(t *testing.T) []*cobra.Command {
	t.Helper()
	rootCmd.InitDefaultHelpCmd()
	none, undeclared, helpResolves := projectDeclarations(rootCmd)
	assert.Emptyf(t, undeclared, "each command declares whether it concerns a project: AddProjectFlag for one that takes -p, "+
		"cli.UsesProject for one that finds its project without -p, cli.NoProject for one that resolves none")
	assert.Emptyf(t, helpResolves, "a command that parses no flags concerns a project, so its --help would resolve one")
	names := make([]string, len(none))
	for i, c := range none {
		names[i] = c.CommandPath()
	}
	sort.Strings(names)
	t.Logf("commands that resolve no project: %s", strings.Join(names, ", "))
	return none
}

// A command added without a declaration fails the tree, rather than passing
// as one that resolves no project.
func TestACommandWithoutADeclarationFailsTheTree(t *testing.T) {
	stray := &cobra.Command{Use: "stray", Run: func(*cobra.Command, []string) {}}
	rootCmd.AddCommand(stray)
	defer rootCmd.RemoveCommand(stray)
	_, undeclared, _ := projectDeclarations(rootCmd)
	assert.Contains(t, undeclared, "kapi stray")

	cli.NoProject(stray)
	_, undeclared, _ = projectDeclarations(rootCmd)
	assert.NotContains(t, undeclared, "kapi stray")
}

// A command that concerns no project resolves none and writes nothing, run by
// a person at a terminal in a checkout that is owed every notice. Each such
// command's own run is replaced by one that does nothing, so this measures what
// kapi does around a command: its prerun, its notices and its postrun.
func TestCommandsThatConcernNoProjectLeaveTheDataRootEmpty(t *testing.T) {
	_, data := stateNoticeFixture(t)
	t.Cleanup(cli.AssumeTerminal())

	for _, c := range noProjectCommands(t) {
		run, runE, argsFn := c.Run, c.RunE, c.Args
		c.Run, c.RunE, c.Args = func(*cobra.Command, []string) {}, nil, cobra.ArbitraryArgs
		path := strings.Fields(c.CommandPath())[1:]
		stderr, err := runKapi(t, path...)
		c.Run, c.RunE, c.Args = run, runE, argsFn
		if err != nil {
			t.Logf("%s: %v", c.CommandPath(), err)
		}
		assert.NotContainsf(t, stderr, "in this checkout", "%s gave a notice about the checkout", c.CommandPath())
		require.Emptyf(t, dataRootFiles(t, data), "%s wrote into the data root", c.CommandPath())
	}
}

// The commands that answer about kapi itself, run whole: help at every level,
// shell completion, the version, the plugin commands and `kapi state path`
// with discovery turned off. None of them resolves the checkout or writes into
// the data root.
func TestCommandsAboutKapiRunWholeWriteNothing(t *testing.T) {
	_, data := stateNoticeFixture(t)
	t.Cleanup(cli.AssumeTerminal())

	runs := [][]string{
		{"version"},
		{"version", "--json"},
		{"--help"},
		{"help"},
		{"help", "--all"},
		{"help", "status"},
		{"completion", "bash"},
		{"__complete", "st"},
		{"__complete", "status", "--"},
		{"plugin", "list"},
		{"plugin", "install", "no-such-plugin"},
		{"plugin", "update", "no-such-plugin"},
	}
	rootCmd.InitDefaultHelpCmd()
	walkCommands(rootCmd, func(c *cobra.Command) {
		if c.HasParent() {
			runs = append(runs, append(strings.Fields(c.CommandPath())[1:], "--help"))
		}
	})
	for _, args := range runs {
		stderr, err := runKapi(t, args...)
		if err != nil {
			t.Logf("kapi %s: %v", strings.Join(args, " "), err)
		}
		assert.NotContainsf(t, stderr, "in this checkout", "kapi %s gave a notice about the checkout", strings.Join(args, " "))
		require.Emptyf(t, dataRootFiles(t, data), "kapi %s wrote into the data root", strings.Join(args, " "))
	}

	t.Setenv("KAPI_NO_PROJECT", "1")
	_, err := runKapi(t, "state", "path")
	require.ErrorContains(t, err, "no kapi project")
	assert.Empty(t, dataRootFiles(t, data), "kapi state path without a project wrote into the data root")
}

// A one-time notice is recorded as given only when a person was shown it: not
// by `kapi version`, not by a run whose error stream is not a terminal, not by
// a `--json` run. The next interactive project command shows it, once.
func TestAStateNoticeIsGivenOnlyToAPerson(t *testing.T) {
	checkout, data := stateNoticeFixture(t)

	told := func() map[string]bool {
		rec, err := dataroot.ReadRecord(dataroot.CheckoutDir(checkout))
		if err != nil {
			return nil
		}
		return rec.Told
	}

	// Not a terminal: the error stream is a buffer.
	stderr, err := runKapi(t, "ls")
	require.NoError(t, err)
	assert.NotContains(t, stderr, "in this checkout")
	assert.Empty(t, told(), "a run nobody watched consumed a notice")

	restore := cli.AssumeTerminal()
	t.Cleanup(restore)

	_, err = runKapi(t, "version")
	require.NoError(t, err)
	assert.Empty(t, dataRootFiles(t, data), "kapi version wrote into the data root")

	stderr, err = runKapi(t, "ls", "--json")
	require.NoError(t, err)
	assert.NotContains(t, stderr, "in this checkout")
	assert.Empty(t, told(), "a --json run consumed a notice")

	stderr, err = runKapi(t, "ls")
	require.NoError(t, err)
	assert.Contains(t, stderr, ".kapi/work in this checkout is no longer used")
	assert.Contains(t, stderr, "kapi state import -p "+checkout)
	assert.Equal(t, map[string]bool{"work-in-tree": true, "state-in-tree": true}, told())

	stderr, err = runKapi(t, "ls")
	require.NoError(t, err)
	assert.NotContains(t, stderr, "in this checkout", "each notice is shown once")
}
