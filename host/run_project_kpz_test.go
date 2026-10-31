package host

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A project flow reads the project's files and writes its work to the project
// store, so given a .kpz package it runs nothing and names the command that
// writes the package's files first.
func TestProjectFlowRefusesAKpzPackage(t *testing.T) {
	a, recipe, dir := dirFlowProject(t, "pseudo", pseudoDirFlow)
	pkg := filepath.Join(dir, "work.kpz")
	require.NoError(t, os.WriteFile(pkg, []byte("not read"), 0o644))

	cmd := NewEnvCommand(context.Background(), "pseudo")
	fs := cmd.Flags()
	fs.String("target-lang", "", "")
	fs.String("source-lang", "", "")
	fs.String("output", "", "")
	fs.String("encoding", "", "")
	fs.String("trace", "", "")
	fs.String("format", "", "")
	fs.StringSlice("input", nil, "")
	fs.Int("concurrency", 0, "")
	fs.Bool("explain", false, "")
	require.NoError(t, fs.Set("target-lang", "fr"))
	require.NoError(t, fs.Set("input", pkg))
	a.TargetLang = "fr"
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := a.RunFromProject(cmd, "pseudo", recipe, RunCmdOptions{})
	require.Error(t, err)
	assert.Equal(t, pkg+" is a package, and a project flow runs over the project's files. "+
		"Run `kapi exec <tool> "+pkg+"` to run one tool on the package, or `kapi merge "+pkg+"` to bring it into the project first.", err.Error())
	var coded interface{ ExitCode() int }
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, ExitUsage, coded.ExitCode())
	_, statErr := os.Stat(filepath.Join(dir, "docs", "page.fr.md"))
	assert.True(t, os.IsNotExist(statErr), "the flow writes nothing")
}
