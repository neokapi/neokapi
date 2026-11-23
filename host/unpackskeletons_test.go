package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/kpz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A skeleton's content hash comes from the package, and names the file it is
// written to. A hash spelled as a path is refused before anything is written,
// so no package writes outside the batch directory.
func TestUnpackSkeletonsRefusesAHashSpelledAsAPath(t *testing.T) {
	root := t.TempDir()
	batchDir := filepath.Join(root, "project", ".kapi", "work", "cache", "extractions", "unpacked")
	require.NoError(t, os.MkdirAll(batchDir, 0o755))
	escape := "x/../../../../../../../planted"

	err := unpackSkeletons(batchDir, []kpz.SkeletonDoc{
		{SourcePath: "a.json", ContentHash: "sha256:abc", Content: kpz.BytesContent([]byte("ok"))},
		{SourcePath: "b.json", ContentHash: "sha256:" + escape, Content: kpz.BytesContent([]byte("planted"))},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "content hash")
	assert.NoFileExists(t, filepath.Join(batchDir, project.SkeletonFilename("abc")), "nothing is written when one hash is refused")
	planted := filepath.Join(batchDir, project.SkeletonFilename(escape))
	assert.NoFileExists(t, planted)

	require.NoError(t, unpackSkeletons(batchDir, []kpz.SkeletonDoc{
		{SourcePath: "a.json", ContentHash: "sha256:abc", Content: kpz.BytesContent([]byte("ok"))},
	}))
	assert.FileExists(t, filepath.Join(batchDir, project.SkeletonFilename("abc")))
}
