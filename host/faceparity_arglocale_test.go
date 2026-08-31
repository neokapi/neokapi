package host_test

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/facetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A locale supplied as an ARGUMENT is held to the same contract as one declared
// in a recipe: `nb_NO` typed at a flag, sent as an MCP tool argument, or passed
// to a desktop method names the same locale as `nb-NO`.
//
// The recipe half is covered by faceparity_locale_test.go. This is the half a
// person or an agent types, which is where a style other than BCP-47 most often
// arrives.

// The shared CLI argument helpers are the funnel every locale flag goes through.
func TestFaceParity_ArgumentLocalesCanonicalizeAtTheCLIHelpers(t *testing.T) {
	t.Run("source language flag", func(t *testing.T) {
		assert.Equal(t, "nb-NO", host.ResolveSourceLocale("nb_NO", ""),
			"--source-lang nb_NO names the same language as nb-NO")
		assert.Equal(t, "nb-NO", host.ResolveSourceLocale("NB-no", ""))
		assert.Equal(t, "en-US", host.ResolveSourceLocale("", "en_US"),
			"a recipe value reaching this resolution is canonical too")
	})

	t.Run("locale list flag", func(t *testing.T) {
		assert.Equal(t, []model.LocaleID{"nb-NO", "pt-BR"},
			host.ParseLocaleList("nb_NO, pt_BR"),
			"--locales takes a list in whatever style and yields canonical tags")
	})
}

// The MCP face refuses a locale that names no language, rather than searching
// for one.
func TestFaceParity_MCPArgumentLocaleIsCanonicalized(t *testing.T) {
	p := facetest.Write(t)

	a := &host.App{}
	a.InitRegistries()
	session := mcpSession(t, a)

	t.Run("posix locale narrows the same search", func(t *testing.T) {
		res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
			Name: "context_search",
			Arguments: map[string]any{
				"query":  p.SearchQuery,
				"locale": "en_US",
				"limit":  p.SearchLimit,
			},
		})
		require.NoError(t, err)
		require.False(t, res.IsError, "a POSIX locale is a locale: %+v", res.Content)

		var out host.ContextSearchResult
		require.NoError(t, json.Unmarshal(structuredJSON(t, res), &out))
		assert.Equal(t, p.SearchQuery, out.Query)
	})

	t.Run("a locale that is not one is refused", func(t *testing.T) {
		res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
			Name: "context_search",
			Arguments: map[string]any{
				"query":  p.SearchQuery,
				"locale": "xx_YY",
			},
		})
		// The SDK reports a handler error either as a transport error or as an
		// error result; both are the refusal this asserts.
		if err == nil {
			assert.True(t, res.IsError, "xx_YY names no language and must be refused")
		}
	})
}

// A recipe written by a surface rather than by hand is written canonically. This
// is the one ingress that persists, so it canonicalizes rather than leaving the
// caller's spelling in a file every later read has to undo.
func TestFaceParity_SetFieldPersistsCanonicalLocales(t *testing.T) {
	p := facetest.Write(t)
	proj, err := project.Load(p.Recipe)
	require.NoError(t, err)

	changed, err := project.SetField(proj, "defaults.source_language", json.RawMessage(`"nb_NO"`))
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, model.LocaleID("nb-NO"), proj.Defaults.SourceLanguage,
		"a locale set through a surface is stored canonically")

	changed, err = project.SetField(proj, "defaults.target_languages", json.RawMessage(`["pt_BR","zh_Hans"]`))
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, []model.LocaleID{"pt-BR", "zh-Hans"}, proj.Defaults.TargetLanguages)

	_, err = project.SetField(proj, "defaults.source_language", json.RawMessage(`"xx_YY"`))
	require.Error(t, err, "a locale that names no language is refused before it is written")
	assert.Contains(t, err.Error(), "invalid locale")
}
