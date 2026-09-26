package kpz

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The context profile of the container: a transfer file holding one
// project's shared context in the layout a context backend keeps.

// contextPackage is a small transfer file: a segment, the blob it names, and
// a checkpoint.
func contextPackage() *Package {
	return &Package{
		Kind: KindContext,
		Layout: []LayoutDoc{
			{Path: "log/w1/0000000000000000000000ab.jsonl", Data: []byte(`{"id":"0000000000000000000000ab"}` + "\n")},
			{Path: "blobs/" + "aa11", Data: []byte("payload")},
			{Path: "checkpoints/0000000000000000000000ab.kpz", Data: []byte("checkpoint")},
		},
	}
}

// TestContextPackage_RoundTripsItsLayout: every file comes back at its path
// with its bytes.
func TestContextPackage_RoundTripsItsLayout(t *testing.T) {
	pkg := contextPackage()
	require.True(t, pkg.HasContent(), "the layout is content")

	data, err := pkg.Marshal()
	require.NoError(t, err)
	got, err := Unmarshal(data)
	require.NoError(t, err)

	assert.Equal(t, KindContext, got.Kind)
	require.Len(t, got.Layout, 3)
	held := map[string]string{}
	for _, l := range got.Layout {
		held[l.Path] = string(l.Data)
	}
	for _, l := range pkg.Layout {
		assert.Equal(t, string(l.Data), held[l.Path], l.Path)
	}
}

// TestContextPackage_IsDeterministic: two marshals of the same package are the
// same bytes, which is what lets a transfer file be compared rather than
// merely read.
func TestContextPackage_IsDeterministic(t *testing.T) {
	first, err := contextPackage().Marshal()
	require.NoError(t, err)
	second, err := contextPackage().Marshal()
	require.NoError(t, err)
	assert.Equal(t, first, second)

	hash, err := contextPackage().RootHash()
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

// TestContextPackage_RefusesAPathOutsideTheLayout: a member is written to a
// remote at its path, so only the layout's directories are accepted.
func TestContextPackage_RefusesAPathOutsideTheLayout(t *testing.T) {
	for _, path := range []string{"../log/x.jsonl", "notes/readme.md", "log/", "/etc/passwd"} {
		pkg := contextPackage()
		pkg.Layout[0].Path = path
		_, err := pkg.Marshal()
		assert.Error(t, err, path)
	}
}

// TestPackage_RefusesAnUnknownKind names the profiles it does read, so a
// package of another is refused with the list rather than a bare no.
func TestPackage_RefusesAnUnknownKind(t *testing.T) {
	pkg := contextPackage()
	pkg.Kind = "kapi-something-else"
	data, err := pkg.Marshal()
	require.NoError(t, err)

	_, err = Unmarshal(data)
	require.Error(t, err)
	for _, kind := range []string{KindProject, KindInterchange, KindContext, KindCheckpoint} {
		assert.Contains(t, err.Error(), kind)
	}
}

// mediaPackage carries a member that is a reference to a file, the way a
// package too large to hold in memory is built.
func mediaPackage(t *testing.T) *Package {
	t.Helper()
	path := filepath.Join(t.TempDir(), "logo.png")
	require.NoError(t, os.WriteFile(path, []byte("not really a png"), 0o644))
	return &Package{Media: []Media{{Path: "media/logo.png", Content: FileContent(path)}}}
}

func TestPackage_WriteToMatchesMarshal(t *testing.T) {
	for name, pkg := range map[string]*Package{
		"context": contextPackage(),
		"media":   mediaPackage(t),
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
	path := filepath.Join(dir, "media.kpz")
	data, err := mediaPackage(t).Marshal()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))

	got, closer, err := OpenFile(path)
	require.NoError(t, err)
	require.Len(t, got.Media, 1)
	member, err := ReadAll(got.Media[0].Content)
	require.NoError(t, err)
	assert.Equal(t, "not really a png", string(member))
	require.NoError(t, closer.Close())

	_, _, err = OpenFile(filepath.Join(dir, "absent.kpz"))
	require.Error(t, err)
}
