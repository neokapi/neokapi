package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeDirFlow puts flow files in a fresh flows directory and returns it.
func writeDirFlow(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "flows")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}
	return dir
}

func TestLoadDirFlow_ReadsTheStepsAndTheDescription(t *testing.T) {
	dir := writeDirFlow(t, map[string]string{
		"guard.yaml": "name: guard\ndescription: Check the terms\nsteps:\n  - tool: dnt-check\n    config:\n      terms: [Acme]\n  - tool: qa\n",
	})

	def, err := LoadDirFlow(dir, "guard")
	require.NoError(t, err)
	assert.Equal(t, "guard", def.Name, "the file name is the flow's name")
	assert.Equal(t, "Check the terms", def.Description)
	require.NotNil(t, def.Spec)
	require.Len(t, def.Spec.Steps, 2)
	assert.Equal(t, "dnt-check", def.Spec.Steps[0].Tool)
	assert.Equal(t, []any{"Acme"}, def.Spec.Steps[0].Config["terms"])
	assert.Equal(t, "qa", def.Spec.Steps[1].Tool)
}

func TestLoadDirFlow_MissingFileIsNotExist(t *testing.T) {
	dir := writeDirFlow(t, nil)

	_, err := LoadDirFlow(dir, "nowhere")
	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

// A flow name is a file name, never a path: a name carrying a separator
// resolves to no flow rather than reaching outside the flows directory.
func TestLoadDirFlow_RejectsANameThatIsAPath(t *testing.T) {
	dir := writeDirFlow(t, map[string]string{"guard.yaml": "steps:\n  - tool: qa\n"})

	for _, name := range []string{"../guard", "sub/guard", "", ".", ".."} {
		_, err := LoadDirFlow(dir, name)
		require.Error(t, err, "name %q", name)
		assert.ErrorIs(t, err, os.ErrNotExist, "name %q", name)
	}
}

func TestLoadDirFlow_RejectsAStepNamingAPath(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "input",
			body: "steps:\n  - tool: pseudo-translate\n    input: locales/en.json\n",
			want: "step 1 (pseudo-translate) names an input path",
		},
		{
			name: "output",
			body: "steps:\n  - tool: qa\n  - tool: pseudo-translate\n    output: locales/qps.json\n",
			want: "step 2 (pseudo-translate) names an output path",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeDirFlow(t, map[string]string{"pseudo.yaml": tt.body})

			_, err := LoadDirFlow(dir, "pseudo")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.Contains(t, err.Error(), "--input", "the message says where a run's files come from")
		})
	}
}

func TestLoadDirFlow_RejectsAFileWithNoSteps(t *testing.T) {
	dir := writeDirFlow(t, map[string]string{"empty.yaml": "name: empty\ndescription: nothing\n"})

	_, err := LoadDirFlow(dir, "empty")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "declares no steps")
	assert.NotErrorIs(t, err, os.ErrNotExist, "the file is there; it does not describe a flow")
}

// The listing is ordered by name and carries a file that will not run, so an
// author sees the flow they wrote along with what is wrong with it.
func TestListDirFlows_ListsEveryFileWithItsProblem(t *testing.T) {
	dir := writeDirFlow(t, map[string]string{
		"zulu.yaml":   "description: Last\nsteps:\n  - tool: qa\n",
		"alpha.yaml":  "description: First\nsteps:\n  - tool: qa\n",
		"broken.yaml": "steps:\n  - tool: qa\n    input: a.json\n",
		"notes.txt":   "not a flow",
	})

	flows := ListDirFlows(dir)
	require.Len(t, flows, 3)

	assert.Equal(t, "alpha", flows[0].Name)
	assert.Equal(t, "First", flows[0].Description)
	require.NoError(t, flows[0].Err)

	assert.Equal(t, "broken", flows[1].Name)
	require.Error(t, flows[1].Err)
	assert.Nil(t, flows[1].Spec)

	assert.Equal(t, "zulu", flows[2].Name)
	require.NoError(t, flows[2].Err)
}

func TestListDirFlows_NoDirectoryIsNoFlows(t *testing.T) {
	assert.Empty(t, ListDirFlows(filepath.Join(t.TempDir(), "absent")))
	assert.Empty(t, ListDirFlows(""), "a recipe with no flows_dir lists no flow files")
}

func TestLoadDirFlow_NoFlowsDirIsNotExist(t *testing.T) {
	_, err := LoadDirFlow("", "guard")
	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestFlowsDirIn_ResolvesAgainstTheRecipeDirectory(t *testing.T) {
	p := &KapiProject{FlowsDir: "config/flows"}
	assert.Equal(t, filepath.Join("proj", "config", "flows"), p.FlowsDirIn("proj"))

	assert.Empty(t, (&KapiProject{}).FlowsDirIn("proj"), "no default directory")
}

func TestValidateFlowsDir(t *testing.T) {
	tests := []struct {
		dir  string
		want string
	}{
		{dir: "flows"},
		{dir: "config/flows"},
		{dir: "../shared/flows"},
		{dir: ".kapi/flows", want: "disposable cache"},
		{dir: "./.kapi", want: "disposable cache"},
		{dir: filepath.Join(string(filepath.Separator), "abs", "flows"), want: "absolute path"},
	}
	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			err := (&KapiProject{FlowsDir: tt.dir}).validateFlowsDir()
			if tt.want == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// A recipe load says once that flow files left under .kapi/flows/ are not
// read, naming the files and the fix, and says nothing when there are none.
func TestRecipeLoad_WarnsAboutFlowFilesInTheCache(t *testing.T) {
	root := t.TempDir()
	recipe := filepath.Join(root, RecipeFileName)
	require.NoError(t, os.WriteFile(recipe, []byte("version: "+CurrentVersion+"\nname: t\n"), 0o644))

	var got []string
	saved := OnKeyWarnings
	OnKeyWarnings = func(_ string, w []string) { got = append(got, w...) }
	t.Cleanup(func() { OnKeyWarnings = saved })

	_, err := Load(recipe)
	require.NoError(t, err)
	assert.Empty(t, got, "no flow files, no note")

	cache := filepath.Join(root, StateDirName, "flows")
	require.NoError(t, os.MkdirAll(cache, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cache, "guard.yaml"), []byte("steps:\n  - tool: qa\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cache, "notes.txt"), []byte("x"), 0o644))

	_, err = Load(recipe)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Contains(t, got[0], ".kapi/flows/")
	assert.Contains(t, got[0], "guard.yaml")
	assert.NotContains(t, got[0], "notes.txt")
	assert.Contains(t, got[0], "flows_dir:")
}
