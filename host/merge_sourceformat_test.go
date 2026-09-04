package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The format a merge reads a source with is the one the claiming item
// declares, or detection when it declares none. A later item's explicit format
// used to be read into a file an earlier item had already claimed.
func TestDetectSourceFormat_ClaimingItemDecides(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	for _, name := range []string{"api.md", "guide.md"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, "docs", name), []byte("# x\n"), 0o644))
	}

	app := &App{}
	app.InitRegistries()
	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Collections: []project.Collection{{Name: "Docs", Content: []project.ContentItem{
			{Path: "docs/api.md"},
			{Path: "docs/**/*.md", Format: &project.FormatSpec{Name: "html"}},
		}}},
	}
	pctx := project.NewProjectContext(proj, filepath.Join(root, project.RecipeFileName))

	detected := pctx.DetectFormat(app.FormatReg, filepath.Join(root, "docs/api.md"))
	require.NotEmpty(t, detected)
	require.NotEqual(t, "html", detected)

	assert.Equal(t, detected, detectSourceFormat(app.FormatReg, pctx, "docs/api.md", filepath.Join(root, "docs/api.md")),
		"the claiming item declares no format, so the file is detected")
	assert.Equal(t, "html", detectSourceFormat(app.FormatReg, pctx, "docs/guide.md", filepath.Join(root, "docs/guide.md")),
		"a file the glob claims reads with the glob's declared format")
}
