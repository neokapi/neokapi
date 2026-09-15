package host

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
)

// A file one item claims for its values and a comments-only item claims for its
// comments holds values, so a surface holding its blocks answers at the values'
// point whichever item the recipe lists first: the review of a source unit in
// the file, and the places context_search reports a term in use.
func TestBlocksOfAFileClaimedApartSitAtTheValuesPoint(t *testing.T) {
	for name, commentsFirst := range map[string]bool{"the comments-only item first": true, "the value item first": false} {
		t.Run(name, func(t *testing.T) {
			root := claimOrderProject(t, commentsFirst)
			recipe := filepath.Join(root, "kapi.yaml")
			proj, err := project.Load(recipe)
			require.NoError(t, err)

			entry := (&App{}).NewReviewPointResolver(bindingsCmd(t, recipe), root).entryAt(t.Context(), "config", filepath.Join(root, "config", "app.yaml"), "en")
			assert.Equal(t, "site/web", entry.point.Profile+"/"+entry.point.Channel, "the point a source unit is reviewed at")
			if assert.NotNil(t, entry.voice) {
				assert.Equal(t, "Site", entry.voice.Name)
			}

			places := &pointResolver{proj: proj, at: time.Now(), cache: map[string]string{}}
			assert.Equal(t, "site/web", places.pointOf("config/app.yaml"), "the point a term use sits at")
		})
	}
}
