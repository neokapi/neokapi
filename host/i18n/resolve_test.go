package i18n

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corei18n "github.com/neokapi/neokapi/core/i18n"
	"github.com/neokapi/neokapi/core/model"
)

func TestCatalog_ExactTag(t *testing.T) {
	t.Parallel()
	assert.NotNil(t, Catalog("nb"), "the committed nb CLI catalog must load")
	assert.Nil(t, Catalog("xx"), "an untranslated language has no catalog")
	assert.Nil(t, Catalog(""), "an empty locale has no catalog")
}

// TestCatalog_RegionalTagFallsBackToMinimal is the CLI-side half of the
// compatibility guarantee: a config that still says "nb-NO" must load the "nb"
// catalog rather than silently degrading to English.
func TestCatalog_RegionalTagFallsBackToMinimal(t *testing.T) {
	t.Parallel()
	for _, tag := range []string{"nb-NO", "NB-no"} {
		assert.NotNil(t, Catalog(model.LocaleID(tag)),
			"%q must resolve the nb CLI catalog", tag)
	}
}

// TestCatalog_FallbackStaysWithinTheLanguage guards the blast radius: the
// fallback chain walks a tag's own subtags and never reaches a sibling
// language, so Nynorsk does not silently get Bokmål strings.
func TestCatalog_FallbackStaysWithinTheLanguage(t *testing.T) {
	t.Parallel()
	assert.Nil(t, Catalog("nn-NO"), "nn-NO must not pick up the nb catalog")
	assert.Nil(t, Catalog("da-DK"), "da-DK must not pick up the nb catalog")
}

// TestResolve_RegionalTagTranslatesCLIStrings exercises the whole CLI
// translator path with a region-bearing tag, asserting a genuinely changed
// string so the test cannot pass on a catalog miss.
func TestResolve_RegionalTagTranslatesCLIStrings(t *testing.T) {
	const (
		scope  = corei18n.Scope("cli.commands.kapi.version.short")
		source = "Show version information"
	)

	t.Setenv("KAPI_LANG", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "")

	minimal := Resolve(corei18n.ResolveOptions{Flag: "nb"})
	want := minimal.T(scope, source)
	require.NotEqual(t, source, want,
		"the nb CLI catalog must actually translate %q — otherwise this test is vacuous", source)

	regional := Resolve(corei18n.ResolveOptions{Flag: "nb-NO"})
	assert.Equal(t, "nb-NO", string(regional.Locale()),
		"the tag the user asked for is reported back verbatim")
	assert.Equal(t, want, regional.T(scope, source),
		"nb-NO must translate identically to nb")
}
