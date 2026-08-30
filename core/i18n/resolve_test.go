package i18n

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve_FlagBeatsEverything(t *testing.T) {
	t.Setenv("KAPI_LANG", "de-DE")
	t.Setenv("LC_ALL", "nb_NO.UTF-8")
	t.Setenv("LANG", "sv_SE.UTF-8")
	tr := Resolve(ResolveOptions{Flag: "fr-FR", ConfigLanguage: "ja-JP"})
	assert.Equal(t, "fr-FR", string(tr.Locale()))
}

func TestResolve_KapiLangBeatsConfigAndEnv(t *testing.T) {
	t.Setenv("KAPI_LANG", "de-DE")
	t.Setenv("LC_ALL", "nb_NO.UTF-8")
	t.Setenv("LANG", "sv_SE.UTF-8")
	tr := Resolve(ResolveOptions{ConfigLanguage: "ja-JP"})
	assert.Equal(t, "de-DE", string(tr.Locale()))
}

func TestResolve_ConfigBeatsPOSIX(t *testing.T) {
	t.Setenv("KAPI_LANG", "")
	t.Setenv("LC_ALL", "nb_NO.UTF-8")
	tr := Resolve(ResolveOptions{ConfigLanguage: "ja-JP"})
	assert.Equal(t, "ja-JP", string(tr.Locale()))
}

func TestResolve_LCALLBeatsLANG(t *testing.T) {
	t.Setenv("KAPI_LANG", "")
	t.Setenv("LC_ALL", "nb_NO.UTF-8")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "sv_SE.UTF-8")
	tr := Resolve(ResolveOptions{})
	assert.Equal(t, "nb-NO", string(tr.Locale()),
		"LC_ALL normalizes POSIX form to BCP-47")
}

func TestResolve_LANGFallback(t *testing.T) {
	t.Setenv("KAPI_LANG", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "fr_CA@euro")
	tr := Resolve(ResolveOptions{})
	assert.Equal(t, "fr-CA", string(tr.Locale()),
		"LANG strips @modifier and normalizes _")
}

func TestResolve_DefaultsToEnglish(t *testing.T) {
	t.Setenv("KAPI_LANG", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "")
	tr := Resolve(ResolveOptions{})
	assert.Equal(t, "en", string(tr.Locale()))
	_, isNoop := tr.(NoopTranslator)
	assert.True(t, isNoop, "English defaults to NoopTranslator — no lookup needed")
}

// nbScope / nbSource name a string that is genuinely translated in the
// committed nb catalog. Asserting a *changed* value keeps the fallback tests
// from passing vacuously against a catalog miss.
const (
	nbScope  = Scope("formats.androidxml.displayName")
	nbSource = "Android String Resources"
)

// TestResolve_RegionalTagFindsMinimalCatalog is the compatibility guarantee: a
// project or user config that still writes the region-bearing "nb-NO" must
// resolve the same catalog as the CLDR-minimal "nb".
func TestResolve_RegionalTagFindsMinimalCatalog(t *testing.T) {
	t.Setenv("KAPI_LANG", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "")

	minimal := Resolve(ResolveOptions{Flag: "nb"})
	translated := minimal.T(nbScope, nbSource)
	assert.NotEqual(t, nbSource, translated,
		"the nb catalog must actually translate %q — otherwise the fallback assertions below are vacuous", nbSource)

	for _, tag := range []string{"nb-NO", "NB-no", "nb_NO"} {
		t.Run(tag, func(t *testing.T) {
			tr := Resolve(ResolveOptions{Flag: tag})
			assert.Equal(t, "nb-NO", string(tr.Locale()),
				"a resolved translator reports the canonical tag, whatever style was asked for")
			assert.Equal(t, translated, tr.T(nbScope, nbSource),
				"%q must resolve the nb catalog", tag)
		})
	}
}

// Every source of a UI locale is canonicalized, not just the LANG family. The
// same locale asked for three ways used to resolve three ways, because only the
// environment branch converted POSIX.
func TestResolve_CanonicalizesEverySource(t *testing.T) {
	t.Setenv("KAPI_LANG", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "")

	t.Run("flag", func(t *testing.T) {
		tr := Resolve(ResolveOptions{Flag: "nb_NO"})
		assert.Equal(t, "nb-NO", string(tr.Locale()))
	})

	t.Run("config", func(t *testing.T) {
		tr := Resolve(ResolveOptions{ConfigLanguage: "nb_NO"})
		assert.Equal(t, "nb-NO", string(tr.Locale()))
	})

	t.Run("KAPI_LANG", func(t *testing.T) {
		t.Setenv("KAPI_LANG", "nb_NO")
		tr := Resolve(ResolveOptions{})
		assert.Equal(t, "nb-NO", string(tr.Locale()))
	})

	t.Run("LANG with codeset", func(t *testing.T) {
		t.Setenv("LANG", "nb_NO.UTF-8")
		tr := Resolve(ResolveOptions{})
		assert.Equal(t, "nb-NO", string(tr.Locale()))
	})
}

// A locale that names no language still resolves to something: asking for a
// catalog is not where a bad locale is refused, and the caller falls back to
// the source text.
func TestResolve_UnknownLocaleDegrades(t *testing.T) {
	t.Setenv("KAPI_LANG", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "")

	for _, tag := range []string{"C", "POSIX", "xx_YY"} {
		t.Run(tag, func(t *testing.T) {
			tr := Resolve(ResolveOptions{Flag: tag})
			assert.Equal(t, nbSource, tr.T(nbScope, nbSource),
				"an unknown locale falls back to the source text")
		})
	}
}

// TestResolve_POSIXRegionalEnvFindsMinimalCatalog covers the same guarantee
// coming in through the POSIX environment rather than a flag.
func TestResolve_POSIXRegionalEnvFindsMinimalCatalog(t *testing.T) {
	t.Setenv("KAPI_LANG", "")
	t.Setenv("LC_ALL", "nb_NO.UTF-8")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "")

	tr := Resolve(ResolveOptions{})
	want := Resolve(ResolveOptions{Flag: "nb"}).T(nbScope, nbSource)
	require.NotEqual(t, nbSource, want)
	assert.Equal(t, want, tr.T(nbScope, nbSource))
}

// TestResolve_FallbackDoesNotCrossLanguages guards the blast radius: the
// fallback chain walks a tag's own subtags, it never reaches sideways to a
// different language or to a differently-scripted sibling.
func TestResolve_FallbackDoesNotCrossLanguages(t *testing.T) {
	t.Setenv("KAPI_LANG", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "")

	// nn (Nynorsk) shares the NO region with nb but is a different language:
	// it must not pick up the nb catalog.
	tr := Resolve(ResolveOptions{Flag: "nn-NO"})
	assert.Equal(t, nbSource, tr.T(nbScope, nbSource),
		"nn-NO must not fall back to the nb catalog")
}

// TestResolve_EnglishVariants pins which English tags short-circuit to the
// no-op translator: only those whose minimal form is exactly "en".
func TestResolve_EnglishVariants(t *testing.T) {
	t.Setenv("KAPI_LANG", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "")

	for _, tag := range []string{"en", "en-US", "EN-us"} {
		tr := Resolve(ResolveOptions{Flag: tag})
		_, isNoop := tr.(NoopTranslator)
		assert.True(t, isNoop, "%q is source English — no catalog lookup needed", tag)
	}

	tr := Resolve(ResolveOptions{Flag: "en-GB"})
	_, isNoop := tr.(NoopTranslator)
	assert.False(t, isNoop,
		"en-GB is a distinct written variant — it must stay eligible for a catalog")
	assert.Equal(t, "en-GB", string(tr.Locale()))
}

func TestNormalizePOSIXLocale(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"en_US", "en-US"},
		{"en_US.UTF-8", "en-US"},
		{"fr_CA@euro", "fr-CA"},
		{"ja_JP.UTF-8@some", "ja-JP"},
		{"C", "C"},
		{"POSIX", "POSIX"},
		{"nb", "nb"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, normalizePOSIXLocale(tc.in), tc.in)
	}
}
