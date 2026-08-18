package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// A collection says where its strings can be read in place. Per collection
// because a repository publishes one host per surface it ships — this one has
// two — so the project level could only ever name one of them.
func TestPreviewSpec_DeclaresAHostAndHowToReadIt(t *testing.T) {
	var spec PreviewSpec
	require.NoError(t, yaml.Unmarshal([]byte(`
kind: storybook
url: https://neokapi.github.io/storybook/bowrain/
`), &spec))

	assert.Equal(t, PreviewKindStorybook, spec.Kind)
	assert.Equal(t, "https://neokapi.github.io/storybook/bowrain/", spec.URL)
	assert.NoError(t, spec.Validate())
}

// Declaring nothing is how a collection says it has no preview, and must not be
// an error: most collections are documents with no components behind them.
func TestPreviewSpec_AbsentIsNotAnError(t *testing.T) {
	var spec PreviewSpec
	require.NoError(t, yaml.Unmarshal([]byte(`{}`), &spec))
	assert.NoError(t, spec.Validate())

	var nilSpec *PreviewSpec
	assert.NoError(t, nilSpec.Validate())
}

// Half a declaration is refused at load rather than at review time, when the
// person waiting for a preview cannot tell a missing URL from an empty
// component.
func TestPreviewSpec_HalfADeclarationIsRefused(t *testing.T) {
	require.ErrorContains(t, (&PreviewSpec{Kind: PreviewKindStorybook}).Validate(),
		"preview.url is required")
	require.ErrorContains(t, (&PreviewSpec{URL: "https://example.dev/sb/"}).Validate(),
		"preview.kind is required")
}

// A kind this version cannot resolve a view within is refused by name, with the
// ones it can listed: the recipe author is the only person who can fix a typo,
// and they are not reading the reviewer's console.
func TestPreviewSpec_AnUnreadableKindIsRefusedByName(t *testing.T) {
	err := (&PreviewSpec{Kind: "ladle", URL: "https://example.dev/ladle/"}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"ladle"`)
	assert.Contains(t, err.Error(), "storybook")
}

// The URL is fetched by a browser that is not in this repository, so a relative
// path names nothing it can reach.
func TestPreviewSpec_TheURLIsAbsolute(t *testing.T) {
	for _, url := range []string{"storybook-static", "/storybook/bowrain/", "example.dev/sb"} {
		err := (&PreviewSpec{Kind: PreviewKindStorybook, URL: url}).Validate()
		require.Error(t, err, url)
		assert.Contains(t, err.Error(), "absolute URL")
	}

	require.ErrorContains(t,
		(&PreviewSpec{Kind: PreviewKindStorybook, URL: "file:///tmp/storybook/"}).Validate(),
		"http or https")
}
