package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeDirFlow puts one flow file in a project's flows directory and returns
// the project's layout.
func writeDirFlow(t *testing.T, files map[string]string) Layout {
	t.Helper()
	root := t.TempDir()
	l := LayoutAt(root)
	require.NoError(t, os.MkdirAll(l.FlowsDir(), 0o755))
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(l.FlowsDir(), name), []byte(body), 0o644))
	}
	return l
}

func TestLoadDirFlow_ReadsTheStepsAndTheDescription(t *testing.T) {
	l := writeDirFlow(t, map[string]string{
		"guard.yaml": "name: guard\ndescription: Check the terms\nsteps:\n  - tool: dnt-check\n    config:\n      terms: [Acme]\n  - tool: qa\n",
	})

	def, err := LoadDirFlow(l, "guard")
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
	l := writeDirFlow(t, nil)

	_, err := LoadDirFlow(l, "nowhere")
	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

// A flow name is a file name, never a path: a name carrying a separator
// resolves to no flow rather than reaching outside the flows directory.
func TestLoadDirFlow_RejectsANameThatIsAPath(t *testing.T) {
	l := writeDirFlow(t, map[string]string{"guard.yaml": "steps:\n  - tool: qa\n"})

	for _, name := range []string{"../guard", "sub/guard", "", ".", ".."} {
		_, err := LoadDirFlow(l, name)
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
			l := writeDirFlow(t, map[string]string{"pseudo.yaml": tt.body})

			_, err := LoadDirFlow(l, "pseudo")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.Contains(t, err.Error(), "--input", "the message says where a run's files come from")
		})
	}
}

func TestLoadDirFlow_RejectsAFileWithNoSteps(t *testing.T) {
	l := writeDirFlow(t, map[string]string{"empty.yaml": "name: empty\ndescription: nothing\n"})

	_, err := LoadDirFlow(l, "empty")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "declares no steps")
	assert.NotErrorIs(t, err, os.ErrNotExist, "the file is there; it does not describe a flow")
}

// The listing is ordered by name and carries a file that will not run, so an
// author sees the flow they wrote along with what is wrong with it.
func TestListDirFlows_ListsEveryFileWithItsProblem(t *testing.T) {
	l := writeDirFlow(t, map[string]string{
		"zulu.yaml":   "description: Last\nsteps:\n  - tool: qa\n",
		"alpha.yaml":  "description: First\nsteps:\n  - tool: qa\n",
		"broken.yaml": "steps:\n  - tool: qa\n    input: a.json\n",
		"notes.txt":   "not a flow",
	})

	flows := ListDirFlows(l)
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
	assert.Empty(t, ListDirFlows(LayoutAt(t.TempDir())))
}

func TestFlowsDir_SitsInTheStateDirectory(t *testing.T) {
	l := LayoutAt("proj")
	assert.Equal(t, filepath.Join("proj", StateDirName, FlowsDirName), l.FlowsDir())
}
