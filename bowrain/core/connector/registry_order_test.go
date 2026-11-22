package connector

import (
	"testing"

	venueconn "github.com/neokapi/neokapi/core/venue/connector"
	"github.com/stretchr/testify/assert"
)

// List answers in name order, the same on every call: the server's /info
// response carries it, and a list that reorders itself defeats caching and
// makes two identical responses look different.
func TestRegistryListIsSortedByName(t *testing.T) {
	r := NewRegistry()
	for _, name := range []string{"wordpress", "figma", "hubspot", "forge", "contentful", "strapi", "github", "sanity"} {
		r.Register(name, venueconn.CategoryFile, nil)
	}

	want := []string{"contentful", "figma", "forge", "github", "hubspot", "sanity", "strapi", "wordpress"}
	for range 20 {
		var got []string
		for _, info := range r.List() {
			got = append(got, info.Name)
		}
		assert.Equal(t, want, got)
	}
}
