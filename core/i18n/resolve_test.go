package i18n

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
