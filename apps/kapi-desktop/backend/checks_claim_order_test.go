package backend

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// The Checks panel reads both layers of a file that one item claims for its
// values and a comments-only item claims for its comments, whichever the recipe
// lists first: the value's "utilize" and the comment's are each a finding.
func TestRunChecksReadsBothLayersOfAFileClaimedApart(t *testing.T) {
	for name, commentsFirst := range map[string]bool{"the comments-only item first": true, "the value item first": false} {
		t.Run(name, func(t *testing.T) {
			isolateCheckPlugins(t)
			root := t.TempDir()
			path := filepath.Join(root, "config", "app.yaml")
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte("# Please utilize the comment.\ngreeting: Please utilize the reader.\n"), 0o644))
			writeVoice(t, filepath.Join(root, project.RelStatePath(project.ProfilesDirName, "site", "voice.yaml")), `name: Site
vocabulary:
  forbidden_terms:
    - term: utilize
      replacement: use
      severity: major
`)
			collections := []project.Collection{
				{Name: "config", Channel: "site/web", Content: []project.ContentItem{{Path: "config/*.yaml"}}},
				{Name: "config-comments", Channel: "site/web", Content: []project.ContentItem{
					{Path: "config/**/*.yaml", Comments: project.ContentComments{Declared: true, Only: true}},
				}},
			}
			if commentsFirst {
				slices.Reverse(collections)
			}
			recipe := filepath.Join(root, "project.kapi")
			require.NoError(t, project.Save(recipe, &project.KapiProject{
				Version:     project.CurrentVersion,
				Name:        "ClaimOrder",
				Defaults:    project.Defaults{SourceLanguage: "en-US", TargetLanguages: []model.LocaleID{"fr-FR"}},
				Profiles:    map[string]project.Profile{"site": {Channels: []project.Channel{{ID: "web"}}}},
				Collections: collections,
			}))

			app := NewApp()
			tab, err := app.OpenProject(recipe)
			require.NoError(t, err)
			t.Cleanup(func() { app.CloseProject(tab.ID) })
			res, err := app.RunChecks(tab.ID, ProjectFilter{})
			require.NoError(t, err)

			var comments, values int
			for _, f := range panelFindings(t, res, "app.yaml") {
				if strings.HasPrefix(f.block, "comment/") {
					comments++
				} else {
					values++
				}
			}
			assert.Equal(t, 1, values, "the value is checked")
			assert.Equal(t, 1, comments, "the comment is checked")
		})
	}
}
