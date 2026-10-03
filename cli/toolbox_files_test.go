package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The project-free alias exposes the read and write legs of the change
// contract, the format list and the toolbox, and nothing else.
func TestBusyboxRootFilesAlias(t *testing.T) {
	for _, name := range []string{FilesAliasName, "/usr/local/bin/kapi-files", "kapi-files.exe"} {
		root := BusyboxRoot(newToolboxApp(t), name)
		require.NotNil(t, root, "prog %q should map to the project-free root", name)
		var names []string
		for _, c := range root.Commands() {
			if c.Name() == "help" {
				continue
			}
			names = append(names, c.Name())
			assert.False(t, c.Hidden, "%s is part of the alias's surface", c.Name())
			if f := c.Flags().Lookup(filesProjectFlag); f != nil {
				assert.True(t, f.Hidden, "%s must not offer -p", c.Name())
			}
		}
		assert.ElementsMatch(t,
			[]string{"inspect", "apply", "formats", "kgrep", "ksed", "kcat", "kconv", "kdiff"}, names)
	}
}

// kapi's own command tree never carries the alias, so neither its help nor the
// command reference generated from KapiCommandSet lists it.
func TestFilesAliasIsNotAKapiCommand(t *testing.T) {
	for _, c := range KapiCommandSet(newAppForTest(t)) {
		assert.NotEqual(t, FilesAliasName, c.Name())
		assert.NotContains(t, c.Aliases, FilesAliasName)
	}
}

// Under the alias a file inside a project is read as a plain file: neither the
// upward walk nor KAPI_PROJECT binds the recipe, and -p is refused.
func TestFilesAliasResolvesNoProject(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	recipe := "version: v1\nname: alias-probe\ndefaults:\n  source_language: en\ncollections:\n" +
		"  - name: docs\n    content:\n      - path: sub/*.md\n        format:\n          name: markdown\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "guide.md"), []byte("# Title\n\nHello.\n"), 0o644))
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("KAPI_PROJECT", filepath.Join(dir, "kapi.yaml"))
	t.Setenv("KAPI_NO_PROJECT", "")
	t.Chdir(filepath.Join(dir, "sub"))

	// The same read through kapi's own inspect names the project's document.
	inProject := NewInspectCmd(newAppForTest(t))
	var projectOut bytes.Buffer
	inProject.SetOut(&projectOut)
	inProject.SetErr(&bytes.Buffer{})
	inProject.SetArgs([]string{"--jsonl", "guide.md"})
	require.NoError(t, inProject.Execute())
	require.Contains(t, projectOut.String(), `"doc":"sub/guide.md"`)

	root := BusyboxRoot(newAppForTest(t), FilesAliasName)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"inspect", "--jsonl", "guide.md"})
	require.NoError(t, root.Execute())
	assert.Contains(t, out.String(), `"doc":"guide.md"`)
	assert.NotContains(t, out.String(), "sub/guide.md")

	refused := BusyboxRoot(newAppForTest(t), FilesAliasName)
	refused.SetOut(&bytes.Buffer{})
	refused.SetErr(&bytes.Buffer{})
	refused.SetArgs([]string{"inspect", "-p", dir, "guide.md"})
	cmd, err := refused.ExecuteC()
	require.Error(t, err)
	assert.Equal(t, ExitUsage, ExitCode(cmd, err))
}
