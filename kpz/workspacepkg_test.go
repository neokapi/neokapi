package kpz

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The workspace profile: the files of a file system and one context package
// per project among them.

func workspacePackage(t *testing.T) *Package {
	t.Helper()
	inner, err := contextPackage().Marshal()
	require.NoError(t, err)
	return &Package{
		Kind: KindWorkspace,
		Files: []FileDoc{
			{Path: FilePath("project/kapi.yaml"), Content: BytesContent([]byte("version: v1\n"))},
			{Path: FilePath("project/app.json"), Content: BytesContent([]byte(`{"a":"Hello"}`))},
		},
		Contexts: []ContextDoc{{Path: ContextsDir + "1.kpz", Project: "project", Data: inner}},
	}
}

// TestWorkspacePackage_RoundTripsFilesAndContexts: every file comes back at
// its path with its bytes, and each context with the project it belongs to.
func TestWorkspacePackage_RoundTripsFilesAndContexts(t *testing.T) {
	pkg := workspacePackage(t)
	require.True(t, pkg.HasContent())

	data, err := pkg.Marshal()
	require.NoError(t, err)
	got, err := Unmarshal(data)
	require.NoError(t, err)

	assert.Equal(t, KindWorkspace, got.Kind)
	files := map[string]string{}
	for _, f := range got.Files {
		b, err := ReadAll(f.Content)
		require.NoError(t, err)
		files[FileRel(f.Path)] = string(b)
	}
	assert.Equal(t, map[string]string{
		"project/kapi.yaml": "version: v1\n",
		"project/app.json":  `{"a":"Hello"}`,
	}, files)

	require.Len(t, got.Contexts, 1)
	assert.Equal(t, "project", got.Contexts[0].Project)
	inner, err := Unmarshal(got.Contexts[0].Data)
	require.NoError(t, err)
	assert.Equal(t, KindContext, inner.Kind)
	assert.Len(t, inner.Layout, 3)
}

// TestWorkspacePackage_RefusesPathsOutsideIt: a file or a project root that
// climbs out of the package is refused on the way out and on the way in.
func TestWorkspacePackage_RefusesPathsOutsideIt(t *testing.T) {
	for _, bad := range []string{"files/../etc/passwd", "files/", "notes/readme.md"} {
		pkg := workspacePackage(t)
		pkg.Files[0].Path = bad
		_, err := pkg.Marshal()
		assert.Error(t, err, bad)
	}
	for _, bad := range []string{"../elsewhere", "/abs"} {
		pkg := workspacePackage(t)
		pkg.Contexts[0].Project = bad
		_, err := pkg.Marshal()
		assert.Error(t, err, bad)
	}
}

// TestWorkspacePackage_IsDeterministic: the same workspace packs to the same
// bytes.
func TestWorkspacePackage_IsDeterministic(t *testing.T) {
	first, err := workspacePackage(t).Marshal()
	require.NoError(t, err)
	second, err := workspacePackage(t).Marshal()
	require.NoError(t, err)
	assert.Equal(t, first, second)
}
