//go:build unix

package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	bproject "github.com/neokapi/neokapi/host/venue/project"
)

// A push reads only files that resolve inside the project: a source file that
// is a link to a file outside it is neither listed nor read.
func TestPushReadsNoLinkOutOfTheProject(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "project")
	outside := filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "en.json"), []byte(`{"a":"Hello"}`), 0o644))
	secret := filepath.Join(outside, "secret.json")
	require.NoError(t, os.WriteFile(secret, []byte(`{"s":"outside the project"}`), 0o644))
	link := filepath.Join(root, "src", "fr.json")
	require.NoError(t, os.Symlink(secret, link))

	proj, err := bproject.InitProject(root, &bproject.Recipe{
		Defaults:    coreproj.Defaults{SourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"}},
		Collections: []coreproj.Collection{{Path: "src/*.json"}},
	})
	require.NoError(t, err)
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	conn := &BowrainSourceConnector{app: testApp(t), project: proj, formatReg: reg}

	assert.Equal(t, []string{"src/en.json"}, conn.contentPaths(), "the link is not listed")
	_, _, err = conn.readBlocksAndMedia(context.Background(), link, "json")
	require.ErrorContains(t, err, "leads outside the root")

	// A path named for the push that is the link stops the scan.
	_, err = conn.scanLocal(context.Background(), []string{link})
	require.ErrorContains(t, err, "leads outside the root")
}

// A push reads the translations the checkout holds through the project: a
// target file that is a link to a file outside the project holds no edition a
// push reads (status and the plan name it, check.WarningTargetOutside), and
// the file it points at is neither read nor written.
func TestPushReadsNoTargetLinkOutOfTheProject(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "project")
	outside := filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "en.json"), []byte(`{"a":"Hello"}`), 0o644))
	secret := filepath.Join(outside, "secret.json")
	const held = `{"a":"outside the project"}`
	require.NoError(t, os.WriteFile(secret, []byte(held), 0o644))
	require.NoError(t, os.Symlink(secret, filepath.Join(root, "src", "fr.json")))
	before, err := os.Stat(secret)
	require.NoError(t, err)

	proj, err := bproject.InitProject(root, &bproject.Recipe{
		Defaults: coreproj.Defaults{SourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"}},
		Collections: []coreproj.Collection{{Content: []coreproj.ContentItem{
			{Path: "src/en.json", Target: "src/{lang}.json"},
		}}},
	})
	require.NoError(t, err)
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	conn := &BowrainSourceConnector{app: testApp(t), project: proj, formatReg: reg}

	revs, err := conn.app.EditionRevisions(context.Background(), proj.RecipePath(), "src/en.json", []model.EditionKey{{Locale: "fr"}})
	require.NoError(t, err)
	for ref := range revs {
		assert.NotContains(t, ref.Edition, "fr", "no fr edition is read through the link")
	}

	got, err := os.ReadFile(secret)
	require.NoError(t, err)
	assert.Equal(t, held, string(got), "the file outside is left as it was")
	after, err := os.Stat(secret)
	require.NoError(t, err)
	assert.Equal(t, before.ModTime(), after.ModTime())
}
