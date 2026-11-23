//go:build !js && !windows

package host

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/flowdef"
)

// namedFileProject writes a project whose recipe runs a step naming a file in
// the project, with the file, and changes into the project as a run from it
// would. It returns the recipe path.
func namedFileProject(t *testing.T, id, step, file, body string) string {
	t.Helper()
	isolateTrust(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	recipe := "version: v1\nid: " + id + "\nflows:\n  default:\n    steps:\n" + step
	recipePath := filepath.Join(root, project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipePath, []byte(recipe), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, filepath.Dir(file)), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, file), []byte(body), 0o755))
	t.Chdir(root)
	return recipePath
}

// loadRecipe reads the recipe at path.
func loadRecipe(t *testing.T, path string) *project.KapiProject {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var p project.KapiProject
	require.NoError(t, yaml.Unmarshal(data, &p))
	return &p
}

// answer runs the gate with a person at a terminal answering yes, and
// returns what the prompt said.
func answer(t *testing.T, recipePath string) string {
	t.Helper()
	var out bytes.Buffer
	err := (&App{}).ensureExecTrust(recipePath, loadRecipe(t, recipePath), LoadProjectInteractiveOptions{
		In: strings.NewReader("y\n"), Out: &out, IsTTYFn: func() bool { return true },
	})
	require.NoError(t, err)
	return out.String()
}

// gateWithoutTerminal runs the gate with nobody to ask.
func gateWithoutTerminal(t *testing.T, recipePath string) error {
	t.Helper()
	return (&App{}).ensureExecTrust(recipePath, loadRecipe(t, recipePath), LoadProjectInteractiveOptions{
		IsTTYFn: func() bool { return false },
	})
}

const (
	idA            = "prj_aaaaaaaaaaaaaaaaaaaaaaaa"
	idB            = "prj_bbbbbbbbbbbbbbbbbbbbbbbb"
	scriptFileStep = "      - tool: script\n        config:\n          source: file\n          script_file: tools/rewrite.js\n"
	relativeExec   = "      - tool: external-command\n        config:\n          command: ./tools/run.sh\n"
	interpreterArg = "      - tool: external-command\n        config:\n          command: node\n          args: [\"tools/x.js\"]\n"
)

// A script_file edited after the approval asks again and names the file; with
// no terminal the gate refuses with the command that approves.
func TestExecTrust_AnEditedScriptFileAsksAgain(t *testing.T) {
	recipePath := namedFileProject(t, idA, scriptFileStep, "tools/rewrite.js", "// one\n")
	prompt := answer(t, recipePath)
	assert.Contains(t, prompt, "Checked, by content: tools/rewrite.js")
	assert.Contains(t, prompt, execCheckLimit)
	require.NoError(t, gateWithoutTerminal(t, recipePath), "the approval holds while the file is unchanged")

	require.NoError(t, os.WriteFile("tools/rewrite.js", []byte("// two\n"), 0o644))
	err := gateWithoutTerminal(t, recipePath)
	require.ErrorIs(t, err, ErrExecNotTrusted)
	assert.Contains(t, err.Error(), "Changed since it was answered: tools/rewrite.js")
	assert.Contains(t, err.Error(), "kapi trust allow -p "+recipePath)
}

// An executable named by a relative path is covered the same way.
func TestExecTrust_AnEditedExecutableAsksAgain(t *testing.T) {
	recipePath := namedFileProject(t, idA, relativeExec, "tools/run.sh", "#!/bin/sh\necho one\n")
	answer(t, recipePath)
	require.NoError(t, os.WriteFile("tools/run.sh", []byte("#!/bin/sh\necho two\n"), 0o755))
	err := gateWithoutTerminal(t, recipePath)
	require.ErrorIs(t, err, ErrExecNotTrusted)
	assert.Contains(t, err.Error(), "tools/run.sh")
}

// An interpreter's script argument is a file the command runs.
func TestExecTrust_AnInterpretersScriptArgumentIsCovered(t *testing.T) {
	recipePath := namedFileProject(t, idA, interpreterArg, "tools/x.js", "console.log(1)\n")
	prompt := answer(t, recipePath)
	assert.Contains(t, prompt, "tools/x.js")
	require.NoError(t, os.WriteFile("tools/x.js", []byte("console.log(2)\n"), 0o644))
	err := gateWithoutTerminal(t, recipePath)
	require.ErrorIs(t, err, ErrExecNotTrusted)
	assert.Contains(t, err.Error(), "Changed since it was answered: tools/x.js")
}

// Another repository at the same path, with the same command line and the
// same files, is another project and asks again.
func TestExecTrust_AnotherRepositoryAtTheSamePathAsksAgain(t *testing.T) {
	recipePath := namedFileProject(t, idA, relativeExec, "tools/run.sh", "#!/bin/sh\necho one\n")
	answer(t, recipePath)

	data, err := os.ReadFile(recipePath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(recipePath, bytes.Replace(data, []byte("id: "+idA+""), []byte("id: "+idB), 1), 0o644))
	require.ErrorIs(t, gateWithoutTerminal(t, recipePath), ErrExecNotTrusted)
}

// A named file replaced with a link out of the project is refused, not hashed.
func TestExecTrust_AScriptSwappedForALinkOutOfTheProjectIsRefused(t *testing.T) {
	recipePath := namedFileProject(t, idA, scriptFileStep, "tools/rewrite.js", "// one\n")
	answer(t, recipePath)

	outside := filepath.Join(t.TempDir(), "rewrite.js")
	require.NoError(t, os.WriteFile(outside, []byte("// one\n"), 0o644))
	require.NoError(t, os.Remove("tools/rewrite.js"))
	require.NoError(t, os.Symlink(outside, "tools/rewrite.js"))

	err := gateWithoutTerminal(t, recipePath)
	require.ErrorIs(t, err, ErrExecNotTrusted)
	assert.Contains(t, err.Error(), "a link that leads out of the project")
	err = (&App{}).ensureExecTrust(recipePath, loadRecipe(t, recipePath), LoadProjectInteractiveOptions{
		In: strings.NewReader("y\n"), Out: &bytes.Buffer{}, IsTTYFn: func() bool { return true },
	})
	require.Error(t, err, "a person cannot approve it either")
}

// A command that names a system program, outside the project, keeps the
// command-line digest it had: there is no project file to read.
func TestExecTrust_ASystemProgramNamesNoProjectFile(t *testing.T) {
	isolateTrust(t)
	surface, err := execSurfaceOf(filepath.Join(t.TempDir(), project.RecipeFileName), execProject(t))
	require.NoError(t, err)
	assert.Empty(t, surface.Files)
}

// One approval covers the recipe from anywhere under the project: the named
// file is read against the project root, not the directory kapi starts in.
func TestExecTrust_OneApprovalFromEveryDirectory(t *testing.T) {
	recipePath := namedFileProject(t, idA, relativeExec, "tools/run.sh", "#!/bin/sh\necho one\n")
	root := filepath.Dir(recipePath)
	answer(t, recipePath)
	for _, sub := range []string{"docs", filepath.Join("src", "deep")} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, sub), 0o755))
		t.Chdir(filepath.Join(root, sub))
		require.NoError(t, gateWithoutTerminal(t, recipePath), "from %s", sub)
	}
}

// A recipe's exec-class step runs from the project root: the working
// directory is the root, a relative script_file is read from the root, a
// recipe that sets either key kapi owns is refused, and with no project bound
// the configuration is the person's own and passes through.
func TestRecipeCommandConfig_FixesTheProjectRoot(t *testing.T) {
	root := t.TempDir()
	a := &App{ProjectContext: &project.ProjectContext{ProjectDir: root}}

	cmd, err := a.recipeCommandConfig("external-command", map[string]any{"command": "./run.sh"})
	require.NoError(t, err)
	assert.Equal(t, root, cmd["working_dir"])
	assert.NotEmpty(t, cmd["invoke_dir"])
	assert.Equal(t, "./run.sh", cmd["command"])

	script, err := a.recipeCommandConfig("script", map[string]any{"script_file": "tools/x.js"})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "tools", "x.js"), script["script_file"])

	for _, key := range []string{"working_dir", "invoke_dir"} {
		_, err := a.recipeCommandConfig("external-command", map[string]any{"command": "./run.sh", key: "/elsewhere"})
		require.ErrorContains(t, err, "sets "+key+", which kapi sets itself")
	}

	other := map[string]any{"locale": "fr"}
	got, err := a.recipeCommandConfig("pseudo-translate", other)
	require.NoError(t, err)
	assert.Equal(t, other, got)
	none := map[string]any{"command": "./run.sh"}
	got, err = (&App{}).recipeCommandConfig("external-command", none)
	require.NoError(t, err)
	assert.Equal(t, none, got)
}

// A flow file in flows_dir is part of what the project can execute: one a
// pull adds asks again and the question names it, and a build in a process
// that answered for the old project refuses its step.
func TestExecTrust_AFlowFileAddedAfterTheAnswerAsksAgain(t *testing.T) {
	recipePath := namedFileProject(t, idA, relativeExec, "tools/run.sh", "#!/bin/sh\necho one\n")
	data, err := os.ReadFile(recipePath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(recipePath, append(data, []byte("flows_dir: flows\n")...), 0o644))
	a := &App{}
	require.NoError(t, a.ensureExecTrust(recipePath, loadRecipe(t, recipePath), LoadProjectInteractiveOptions{
		In: strings.NewReader("y\n"), Out: &bytes.Buffer{}, IsTTYFn: func() bool { return true },
	}))
	a.ProjectContext = project.NewProjectContext(loadRecipe(t, recipePath), recipePath)
	require.NoError(t, a.checkExecToolAllowed("external-command"))

	require.NoError(t, os.MkdirAll("flows", 0o755))
	require.NoError(t, os.WriteFile("flows/sneaky.yaml",
		[]byte("steps:\n  - tool: external-command\n    config:\n      command: ./tools/other.sh\n"), 0o644))

	err = gateWithoutTerminal(t, recipePath)
	require.ErrorIs(t, err, ErrExecNotTrusted)
	assert.Contains(t, err.Error(), "Added since it was answered: flows/sneaky.yaml steps[0] external-command")
	err = a.checkExecToolAllowed("external-command")
	require.ErrorIs(t, err, ErrExecNotTrusted, "the running process refuses the new step too")
	assert.Contains(t, err.Error(), "flows/sneaky.yaml")
}

// The answer claims to check only regular files that exist. A directory is
// named as not checked, a shell string is called out, and the limit is said
// at the moment of answering, in the refusal and in kapi trust list.
func TestExecTrust_TheAnswerClaimsOnlyWhatItChecks(t *testing.T) {
	steps := "      - tool: external-command\n        config:\n          command: sh\n          args: [\"-c\", \"cd scripts && ./hidden.sh\"]\n" +
		"      - tool: external-command\n        config:\n          command: node\n          args: [\"lib\"]\n"
	recipePath := namedFileProject(t, idA, steps, "scripts/hidden.sh", "#!/bin/sh\necho hidden\n")
	require.NoError(t, os.MkdirAll("lib", 0o755))
	require.NoError(t, os.WriteFile("lib/index.js", []byte("console.log(1)\n"), 0o644))

	prompt := answer(t, recipePath)
	assert.Contains(t, prompt, "Checked, by content: no file; the commands name none in the project")
	assert.NotContains(t, prompt, "Checked, by content: -c")
	assert.Contains(t, prompt, "Named and not checked (directories): lib")
	assert.Contains(t, prompt, "Runs through a shell, so kapi cannot see which files it uses: flows.default.steps[0] external-command")
	assert.Contains(t, prompt, execCheckLimit)

	list := (&App{}).ExecTrustList()
	require.Len(t, list.Entries, 1)
	assert.Contains(t, list.Entries[0].Detail, "checks 0 files; not checked: lib; through a shell: flows.default.steps[0] external-command; not what they run or read")

	data, err := os.ReadFile(recipePath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(recipePath, bytes.Replace(data, []byte("lib"), []byte("lib2"), 1), 0o644))
	err = gateWithoutTerminal(t, recipePath)
	require.ErrorIs(t, err, ErrExecNotTrusted)
	assert.Contains(t, err.Error(), execCheckLimit)
}

// A long-running process checks at every build: a script changed after the
// answer, in the same process, is refused and named.
func TestExecTrust_AScriptChangedMidProcessIsRefused(t *testing.T) {
	recipePath := namedFileProject(t, idA, relativeExec, "tools/run.sh", "#!/bin/sh\necho one\n")
	a := &App{}
	require.NoError(t, a.ensureExecTrust(recipePath, loadRecipe(t, recipePath), LoadProjectInteractiveOptions{
		In: strings.NewReader("y\n"), Out: &bytes.Buffer{}, IsTTYFn: func() bool { return true },
	}))
	a.ProjectContext = project.NewProjectContext(loadRecipe(t, recipePath), recipePath)
	require.NoError(t, a.checkExecToolAllowed("external-command"), "the first run")

	require.NoError(t, os.WriteFile("tools/run.sh", []byte("#!/bin/sh\necho changed and longer\n"), 0o755))
	err := a.checkExecToolAllowed("external-command")
	require.ErrorIs(t, err, ErrExecNotTrusted)
	assert.Contains(t, err.Error(), "Changed since it was answered: tools/run.sh")
	assert.Contains(t, err.Error(), "kapi trust allow -p "+recipePath)
}

// A recipe that sets a key kapi owns on a command is refused at the gate.
func TestExecTrust_ARecipeSettingKapisKeysIsRefused(t *testing.T) {
	step := relativeExec + "          invoke_dir: /tmp/elsewhere\n"
	recipePath := namedFileProject(t, idA, step, "tools/run.sh", "#!/bin/sh\necho one\n")
	err := gateWithoutTerminal(t, recipePath)
	require.ErrorIs(t, err, ErrExecNotTrusted)
	assert.Contains(t, err.Error(), "sets invoke_dir, which kapi sets itself")
}

// kapi's own flows run no command a project chooses.
func TestBuiltInFlowsRunNoProjectCommand(t *testing.T) {
	for _, def := range flowdef.BuiltInFlows() {
		for _, n := range def.Nodes {
			assert.False(t, project.IsExecClassTool(n.Name), "built-in flow %s runs %s", def.ID, n.Name)
		}
	}
}

func TestFormatterTrust_CoversTheFilesItRuns(t *testing.T) {
	setup := func(t *testing.T) (config, exe string, trust *formatterTrust) {
		t.Helper()
		isolateTrust(t)
		root, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)
		config = filepath.Join(root, ".oxfmtrc.json")
		exe = filepath.Join(root, "node_modules", ".bin", "oxfmt")
		require.NoError(t, os.WriteFile(config, []byte("{}\n"), 0o644))
		require.NoError(t, os.MkdirAll(filepath.Dir(exe), 0o755))
		require.NoError(t, os.WriteFile(exe, []byte("#!/bin/sh\necho one\n"), 0o755))
		trust = newFormatterTrust(false)
		trust.ask = func(project.ExecSite) (bool, error) { return true, nil }
		require.NoError(t, trust.allow(config, "oxfmt", []string{exe}, nil))
		return config, exe, trust
	}
	// unattended is a fresh run with nobody to ask.
	unattended := func(config, exe string) error {
		trust := newFormatterTrust(false)
		trust.unanswered = "approve it at a terminal"
		return trust.allow(config, "oxfmt", []string{exe}, nil)
	}

	t.Run("the approval holds for the same files", func(t *testing.T) {
		config, exe, _ := setup(t)
		require.NoError(t, unattended(config, exe))
	})
	t.Run("an edited executable asks again", func(t *testing.T) {
		config, exe, _ := setup(t)
		require.NoError(t, os.WriteFile(exe, []byte("#!/bin/sh\necho two\n"), 0o755))
		require.ErrorContains(t, unattended(config, exe), "approve it at a terminal")
	})
	t.Run("an edited configuration asks again", func(t *testing.T) {
		config, exe, _ := setup(t)
		require.NoError(t, os.WriteFile(config, []byte(`{"semi": false}`+"\n"), 0o644))
		require.Error(t, unattended(config, exe))
	})
	t.Run("an executable swapped for a link to another program asks again", func(t *testing.T) {
		config, exe, _ := setup(t)
		other := filepath.Join(t.TempDir(), "oxfmt")
		require.NoError(t, os.WriteFile(other, []byte("#!/bin/sh\necho other\n"), 0o755))
		require.NoError(t, os.Remove(exe))
		require.NoError(t, os.Symlink(other, exe))
		require.Error(t, unattended(config, exe))
	})
	t.Run("an interpreter named by the executable is covered", func(t *testing.T) {
		config, exe, _ := setup(t)
		interp := filepath.Join(filepath.Dir(exe), "sh")
		require.NoError(t, os.WriteFile(interp, []byte("#!/bin/sh\nexec /bin/sh \"$@\"\n"), 0o755))
		require.NoError(t, os.WriteFile(exe, []byte("#!"+interp+"\necho one\n"), 0o755))
		require.NoError(t, (&formatterTrust{decided: map[string]error{}, hashed: map[string]string{},
			ask: func(project.ExecSite) (bool, error) { return true, nil }}).allow(config, "oxfmt", []string{exe}, nil))
		require.NoError(t, unattended(config, exe))
		require.NoError(t, os.WriteFile(interp, []byte("#!/bin/sh\nexec /bin/bash \"$@\"\n"), 0o755))
		require.Error(t, unattended(config, exe))
	})
}
