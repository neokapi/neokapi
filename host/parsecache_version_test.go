package host

import (
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A cached parse is only replayable by a reader that would produce the same
// parse. The key hashes the source locale, the merged config and the recipe,
// none of which move when a reader's classification changes — so an upgraded
// kapi replayed the old binary's parse, and content that had just become
// translatable came back untranslated with nothing failing to say so.
func TestParseCacheKeyChangesWithTheBuild(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	a, recipe, _ := mdxBoundProject(t)
	_ = dir

	// runnerPartCache declines without a project context and an open doc
	// cache, which would make this test vacuous. A run sets both; here they are
	// set directly.
	loaded, err := project.Load(recipe)
	require.NoError(t, err)
	a.ProjectContext = project.NewProjectContext(loaded, recipe)
	defer func() { a.ProjectContext = nil }()
	defer a.openParseCacheDefer(a.ProjectContext.ProjectDir)()

	proj := a.ProjectContext
	require.NotNil(t, proj)

	cfg := map[string]any{"translateFrontMatter": true}

	before := version.Version
	defer func() { version.Version = before }()

	version.Version = "v1.0.0"
	_, keyA := a.runnerPartCache(proj.ProjectDir, cfg)

	version.Version = "v1.0.1"
	_, keyB := a.runnerPartCache(proj.ProjectDir, cfg)

	require.NotEmpty(t, keyA)
	assert.NotEqual(t, keyA, keyB,
		"two kapi builds must not share a cached parse: the second may classify content differently")
}
