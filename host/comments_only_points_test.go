package host

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
)

// Every block the project reads from a file an item declares for its comments
// alone is a comment. A surface holding those blocks, rather than a file it was
// named, answers at the point the comments sit at: the review of a source unit
// in the file, and the places context_search reports a term in use. The
// must-fail case declares the same items with `comments: {channel: ...}`, whose
// file's blocks sit at site/web.
func TestDeclaredBlocksOfACommentsOnlyFileSitAtTheCommentsPoint(t *testing.T) {
	for name, tc := range map[string]struct {
		placement placement
		want      string
		voice     string
		// concept is the one the terms store bound at the point holds.
		concept string
	}{
		"comments only":                     {placement: onlyOnItem, want: "source/comments", voice: "Source", concept: "source-name"},
		"must fail: comments beside values": {placement: onItem, want: "site/web", voice: "Site", concept: "site-name"},
	} {
		t.Run(name, func(t *testing.T) {
			root := commentPointProject(t, tc.placement)
			recipe := filepath.Join(root, "kapi.yaml")
			proj, err := project.Load(recipe)
			require.NoError(t, err)

			entry := (&App{}).NewReviewPointResolver(bindingsCmd(t, recipe), root).entryAt(t.Context(), "config", filepath.Join(root, "config", "app.yaml"), "en")
			assert.Equal(t, tc.want, entry.point.Profile+"/"+entry.point.Channel, "the point a source unit is reviewed at")
			if assert.NotNil(t, entry.voice) {
				assert.Equal(t, tc.voice, entry.voice.Name)
			}
			if assert.NotNil(t, entry.terms) {
				_, found, err := entry.terms.GetConcept(t.Context(), tc.concept)
				require.NoError(t, err)
				assert.True(t, found, "the terms bound at %s hold %s", tc.want, tc.concept)
			}

			places := &pointResolver{proj: proj, at: time.Now(), cache: map[string]string{}}
			assert.Equal(t, tc.want, places.pointOf("config/app.yaml"), "the point a term use sits at")
		})
	}
}
