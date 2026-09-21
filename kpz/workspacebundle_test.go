package kpz

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/terms/ktb"
)

// The workspace profile of the container: a package whose members are whole
// context packages, one per project, plus the registry that names them.

// projectBundle writes a small context package to a file and returns the path.
func projectBundle(t *testing.T, dir, name string) string {
	t.Helper()
	pkg := contextPackage()
	pkg.Voice[0].ID = name
	pkg.Voice[0].Path = VoiceDir + name + ".yaml"
	pkg.Voice[0].Profile.ID = name
	data, err := pkg.Marshal()
	require.NoError(t, err)
	path := filepath.Join(dir, name+".kpz")
	require.NoError(t, os.WriteFile(path, data, 0o644))
	return path
}

// workspacePackage is a workspace holding two projects, each carried as a file
// reference the way an export builds one.
func workspacePackage(t *testing.T, dir string) *Package {
	t.Helper()
	return &Package{
		Kind: KindWorkspace,
		Projects: []ProjectDoc{
			{
				Path: ProjectsDir + "second.kpz", Key: "prj_second", Name: "Second",
				Content: FileContent(projectBundle(t, dir, "second")),
			},
			{
				Path: ProjectsDir + "first.kpz", Key: "prj_first", Name: "First",
				Content: FileContent(projectBundle(t, dir, "first")),
			},
		},
	}
}

// TestWorkspacePackage_RoundTripsItsProjects: each project comes back with the
// identity the registry carried and a reader over its own package.
func TestWorkspacePackage_RoundTripsItsProjects(t *testing.T) {
	pkg := workspacePackage(t, t.TempDir())
	require.True(t, pkg.HasContent(), "projects are content")

	data, err := pkg.Marshal()
	require.NoError(t, err)
	got, err := Unmarshal(data)
	require.NoError(t, err)

	assert.Equal(t, KindWorkspace, got.Kind)
	require.Len(t, got.Projects, 2)
	assert.Equal(t, "prj_first", got.Projects[0].Key, "projects come back ordered by key")
	assert.Equal(t, "First", got.Projects[0].Name)
	assert.Equal(t, ProjectsDir+"first.kpz", got.Projects[0].Path)
	assert.Equal(t, "prj_second", got.Projects[1].Key)

	// A member is a whole context package, readable on its own.
	inner, err := ReadAll(got.Projects[0].Content)
	require.NoError(t, err)
	innerPkg, err := Unmarshal(inner)
	require.NoError(t, err)
	assert.Equal(t, KindContext, innerPkg.Kind)
	require.Len(t, innerPkg.Voice, 1)
	assert.Equal(t, "first", innerPkg.Voice[0].ID)
}

// TestWorkspacePackage_IsDeterministic: the same workspace packs to the same
// bytes whatever order its projects were listed in, which is what lets two
// exports be compared rather than merely read.
func TestWorkspacePackage_IsDeterministic(t *testing.T) {
	dir := t.TempDir()
	pkg := workspacePackage(t, dir)

	first, err := pkg.Marshal()
	require.NoError(t, err)

	reversed := &Package{Kind: KindWorkspace, Projects: []ProjectDoc{pkg.Projects[1], pkg.Projects[0]}}
	second, err := reversed.Marshal()
	require.NoError(t, err)
	assert.Equal(t, first, second, "a workspace packs the same however its projects were listed")

	hash, err := pkg.RootHash()
	require.NoError(t, err)
	other, err := reversed.RootHash()
	require.NoError(t, err)
	assert.Equal(t, hash, other)
}

// TestPackage_WriteToMatchesMarshal: streaming a package to a writer produces
// exactly the bytes Marshal returns, so a caller writing to a file and one
// building in memory cannot disagree.
func TestPackage_WriteToMatchesMarshal(t *testing.T) {
	for name, pkg := range map[string]*Package{
		"context":   contextPackage(),
		"workspace": workspacePackage(t, t.TempDir()),
	} {
		t.Run(name, func(t *testing.T) {
			want, err := pkg.Marshal()
			require.NoError(t, err)

			path := filepath.Join(t.TempDir(), "out.kpz")
			f, err := os.Create(path)
			require.NoError(t, err)
			n, err := pkg.WriteTo(f)
			require.NoError(t, err)
			require.NoError(t, f.Close())

			got, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, want, got)
			assert.Equal(t, int64(len(want)), n, "WriteTo reports the bytes it wrote")
		})
	}
}

// TestOpenFile_ReadsWithoutHoldingTheArchive: a package read from a file
// validates exactly as one read from bytes, and its members stay readable
// until the handle is closed.
func TestOpenFile_ReadsWithoutHoldingTheArchive(t *testing.T) {
	dir := t.TempDir()
	pkg := workspacePackage(t, dir)
	path := filepath.Join(dir, "workspace.kpz")
	data, err := pkg.Marshal()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))

	got, closer, err := OpenFile(path)
	require.NoError(t, err)
	require.Len(t, got.Projects, 2)
	member, err := ReadAll(got.Projects[0].Content)
	require.NoError(t, err)
	assert.NotEmpty(t, member)
	require.NoError(t, closer.Close())

	_, _, err = OpenFile(filepath.Join(dir, "absent.kpz"))
	require.Error(t, err)
}

// TestWorkspacePackage_RefusesARegistryThatDoesNotMatchItsMembers: the registry
// is what a restore rebuilds identities from, so a package whose registry and
// members disagree is refused rather than restored short.
func TestWorkspacePackage_RefusesARegistryThatDoesNotMatchItsMembers(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*projectRegistry)
		want   string
	}{
		{
			name:   "names a member the package does not carry",
			mutate: func(r *projectRegistry) { r.Projects[0].Bundle = ProjectsDir + "absent.kpz" },
			want:   "does not carry",
		},
		{
			name:   "leaves a member unclaimed",
			mutate: func(r *projectRegistry) { r.Projects = r.Projects[:1] },
			want:   "says nothing about",
		},
		{
			name:   "claims one member twice",
			mutate: func(r *projectRegistry) { r.Projects[1].Bundle = r.Projects[0].Bundle },
			want:   "twice",
		},
		{
			name:   "carries an entry with no project",
			mutate: func(r *projectRegistry) { r.Projects[0].Key = "" },
			want:   "no project key",
		},
		{
			name:   "names a path outside the package",
			mutate: func(r *projectRegistry) { r.Projects[0].Bundle = "../elsewhere.kpz" },
			want:   "not a path inside the package",
		},
		{
			name:   "speaks another major version",
			mutate: func(r *projectRegistry) { r.SchemaVersion = "9.0" },
			want:   "schemaVersion",
		},
		{
			name:   "is not a registry",
			mutate: func(r *projectRegistry) { r.Kind = "something-else" },
			want:   "want",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			reg := projectRegistry{
				SchemaVersion: SchemaVersion, Kind: registryKind,
				Projects: []registryProject{
					{Key: "prj_first", Name: "First", Bundle: ProjectsDir + "first.kpz"},
					{Key: "prj_second", Name: "Second", Bundle: ProjectsDir + "second.kpz"},
				},
			}
			tc.mutate(&reg)
			body, err := json.MarshalIndent(reg, "", "  ")
			require.NoError(t, err)

			members := map[string][]byte{RegistryPath: append(body, '\n')}
			manifest := Manifest{Kind: KindWorkspace, Members: []Member{
				{Path: RegistryPath, ContentType: ContentTypeRegistry},
			}}
			for _, name := range []string{"first", "second"} {
				path := ProjectsDir + name + ".kpz"
				inner, rerr := os.ReadFile(projectBundle(t, dir, name))
				require.NoError(t, rerr)
				members[path] = inner
				manifest.Members = append(manifest.Members, Member{Path: path, ContentType: ContentTypeProject})
			}

			_, err = Unmarshal(hostileArchive(t, manifest, members))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// TestWorkspacePackage_RefusesAProjectWithNoIdentity: a member with no key
// cannot be registered on the way back in, so it is refused on the way out.
func TestWorkspacePackage_RefusesAProjectWithNoIdentity(t *testing.T) {
	dir := t.TempDir()
	pkg := &Package{Kind: KindWorkspace, Projects: []ProjectDoc{{
		Path: ProjectsDir + "first.kpz", Content: FileContent(projectBundle(t, dir, "first")),
	}}}
	_, err := pkg.Marshal()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "names no project")

	pkg.Projects[0].Key = "prj_first"
	pkg.Projects = append(pkg.Projects, pkg.Projects[0])
	_, err = pkg.Marshal()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "appears twice")
}

// TestWorkspacePackage_HasNoRegistryWithoutProjects: a package of another
// profile carries no registry member at all.
func TestWorkspacePackage_HasNoRegistryWithoutProjects(t *testing.T) {
	pkg := contextPackage()
	pkg.Terms = ktb.FromConcepts(nil)
	members, err := pkg.serializeMembers()
	require.NoError(t, err)
	for _, m := range members {
		assert.NotEqual(t, RegistryPath, m.Path)
		assert.NotEqual(t, ContentTypeRegistry, m.ContentType)
	}

	data, err := pkg.Marshal()
	require.NoError(t, err)
	got, err := Unmarshal(data)
	require.NoError(t, err)
	assert.Empty(t, got.Projects)
}
