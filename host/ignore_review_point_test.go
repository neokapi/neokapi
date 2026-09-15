package host

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A source unit in a file the project's ignore rules match is reviewed at the
// project's default point, under the terms bound there, as `kapi context`
// answers for that file. A sibling the rules leave alone is reviewed at its
// item's point, under the site terms.
func TestReviewOfAnIgnoredFileSitsAtTheDefaultPoint(t *testing.T) {
	root := ignoredPointProject(t)
	recipe := filepath.Join(root, "kapi.yaml")
	resolver := (&App{}).NewReviewPointResolver(bindingsCmd(t, recipe), root)

	for file, want := range map[string]struct {
		point     string
		siteTerms bool
	}{
		"app.yaml":   {point: "/", siteTerms: false},
		"other.yaml": {point: "site/web", siteTerms: true},
	} {
		entry := resolver.entryAt(t.Context(), "config", filepath.Join(root, "config", file), "en")
		assert.Equal(t, want.point, entry.point.Profile+"/"+entry.point.Channel, "the point a source unit in %s is reviewed at", file)
		if assert.NotNil(t, entry.terms, file) {
			_, found, err := entry.terms.GetConcept(t.Context(), "site-name")
			require.NoError(t, err)
			assert.Equal(t, want.siteTerms, found, "the terms a unit in %s is reviewed under hold site-name", file)
		}
	}
}
