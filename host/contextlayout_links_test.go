//go:build unix

package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/rootpath"
	"github.com/neokapi/neokapi/memory/kmb"
	"github.com/neokapi/neokapi/terms/ktb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCommittedContextSourcesRefuseLinksOut plants committed context files
// and directories that are links out of the project and asserts none of them
// is listed or read as a source.
func TestCommittedContextSourcesRefuseLinksOut(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	layout := project.LayoutAt(root)
	require.NoError(t, os.MkdirAll(layout.StateDir, 0o755))

	// A profile directory that leads out is no profile directory.
	outProfile := filepath.Join(outside, "web")
	require.NoError(t, os.MkdirAll(outProfile, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(outProfile, VoiceConventionalName), []byte("name: x\n"), 0o644))
	require.NoError(t, os.MkdirAll(layout.Export().ProfilesDir(), 0o755))
	require.NoError(t, os.Symlink(outProfile, layout.Export().ProfileDir("web")))
	sources, err := committedContextSources(layout)
	require.NoError(t, err)
	assert.Empty(t, sources, "a profile directory link out of the project holds no source")

	// A memory directory that leads out is refused.
	outMemory := filepath.Join(outside, "memory")
	require.NoError(t, os.MkdirAll(outMemory, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(outMemory, kmb.ConventionalName), []byte("{}"), 0o644))
	require.NoError(t, os.Symlink(outMemory, layout.Export().MemoryDir()))
	_, err = committedContextSources(layout)
	var esc *rootpath.EscapeError
	require.ErrorAs(t, err, &esc)
	require.NoError(t, os.Remove(layout.Export().MemoryDir()))

	// A terms file that is itself a link out is refused, not read.
	secret := filepath.Join(outside, "terms.json")
	require.NoError(t, os.WriteFile(secret, []byte("{}"), 0o644))
	require.NoError(t, os.Symlink(secret, filepath.Join(layout.StateDir, ktb.ConventionalName)))
	_, err = committedContextSources(layout)
	require.ErrorAs(t, err, &esc)

	_, err = fileDigest(root, filepath.Join(layout.StateDir, ktb.ConventionalName))
	require.ErrorAs(t, err, &esc, "a link out is not hashed")
	assert.False(t, committedRecordExists(root, layout.Export().UnitStateDir()))
}

// TestContextFilesRefuseLinksOut asserts the single-file reads of the context
// commands refuse a file that is a link out of the project.
func TestContextFilesRefuseLinksOut(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	// An evidence file that holds the form, reached through a link.
	secret := filepath.Join(outside, "notes.md")
	require.NoError(t, os.WriteFile(secret, []byte("use colour here"), 0o644))
	linked := filepath.Join(root, "notes.md")
	require.NoError(t, os.Symlink(secret, linked))
	_, err := fileHoldsAnyForm(root, linked, []string{"colour"})
	require.Error(t, err, "an evidence file that is a link out is not read")
	plain := filepath.Join(root, "plain.md")
	require.NoError(t, os.WriteFile(plain, []byte("use colour here"), 0o644))
	holds, err := fileHoldsAnyForm(root, plain, []string{"colour"})
	require.NoError(t, err)
	assert.True(t, holds)

	// A location under a directory link out is refused, not answered.
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "docs")))
	require.Error(t, contextLocationErr(root, "docs/notes.md", "docs/notes.md"))
	require.Error(t, contextLocationErr(root, "docs/new.md", "docs/new.md"))
	require.NoError(t, contextLocationErr(root, "plain.md", "plain.md"))

	// A project's Claude Code settings file that is a link out is not read.
	settings := filepath.Join(outside, "settings.json")
	require.NoError(t, os.WriteFile(settings, []byte(`{"enabledPlugins":{"kapi@x":true}}`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude"), 0o755))
	require.NoError(t, os.Symlink(settings, filepath.Join(root, ".claude", "settings.json")))
	assert.Empty(t, claudePluginEnabledIn(root))
	assert.True(t, enablesKapiPlugin("", settings), "the person's own settings file is read as named")

	// A recipe that is a link out of its folder is not read.
	recipe := filepath.Join(outside, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte("context:\n  backend: git\n"), 0o644))
	inner := filepath.Join(root, "p")
	require.NoError(t, os.MkdirAll(inner, 0o755))
	require.NoError(t, os.Symlink(recipe, filepath.Join(inner, project.RecipeFileName)))
	_, err = recipeContextBackend(filepath.Join(inner, project.RecipeFileName))
	require.Error(t, err)
}

// TestCountFileBlocksRefusesLinkOut asserts kapi ls --stats counts no blocks
// of a project file that is a link out of the project.
func TestCountFileBlocksRefusesLinkOut(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, project.RecipeFileName), []byte("version: v1\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, project.StateDirName), 0o755))
	secret := filepath.Join(outside, "fr.json")
	require.NoError(t, os.WriteFile(secret, []byte(`{"a":"Bonjour"}`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales"), 0o755))
	linked := filepath.Join(root, "locales", "fr.json")
	require.NoError(t, os.Symlink(secret, linked))

	a := appWithFormats()
	blocks, _, err := a.CountFileBlocks(t.Context(), linked, "json", "en", nil)
	var esc *rootpath.EscapeError
	require.ErrorAs(t, err, &esc)
	assert.Zero(t, blocks)

	plain := filepath.Join(root, "locales", "en.json")
	require.NoError(t, os.WriteFile(plain, []byte(`{"a":"Hello"}`), 0o644))
	blocks, _, err = a.CountFileBlocks(t.Context(), plain, "json", "en", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, blocks)
}
