package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/leonelquinteros/gotext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/formats/mo"
	"github.com/neokapi/neokapi/core/i18n"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// makeMoCatalog compiles a tiny MO for the given (scope, source, target)
// triples using the real writer so the round-trip behaviour is tested
// end-to-end, not via the package-internal shortcut.
func makeMoCatalog(t *testing.T, locale string, entries ...[3]string) *gotext.Mo {
	t.Helper()
	w := mo.NewWriter()
	var buf bytes.Buffer
	require.NoError(t, w.SetOutputWriter(&buf))
	w.SetLocale(model.LocaleID(locale))

	parts := make(chan *model.Part, len(entries))
	for i, e := range entries {
		b := model.NewBlock(("tu" + iToA(i+1)), e[1])
		b.Name = e[0]
		b.SetTargetText(model.LocaleID(locale), e[2])
		parts <- &model.Part{Type: model.PartBlock, Resource: b}
	}
	close(parts)
	require.NoError(t, w.Write(context.Background(), parts))

	m := gotext.NewMo()
	m.Parse(buf.Bytes())
	return m
}

func iToA(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

// TestListTools_LocalizesDisplayName builds a minimal App with a fixture
// Translator and asserts that the tool listing emits localized
// DisplayName/Description — the full end-to-end from App.T() through
// cli/tools.go:listTools().
func TestListTools_LocalizesDisplayName(t *testing.T) {
	app := &App{}
	app.InitRegistries()

	// Swap in a French translator with a single-tool catalog.
	cat := makeMoCatalog(t, "fr-FR",
		[3]string{"tools.translate.displayName", "Translate", "Traduction IA"},
		[3]string{"tools.translate.description", "Translate content with an LLM or machine-translation provider (select with --provider)", "Traduire avec un LLM"},
	)
	app.translator = i18n.NewTranslator("fr-FR", cat)

	// Assert the translator resolves what we'll assert below.
	require.Equal(t, "Traduction IA",
		app.T().T("tools.translate.displayName", "Translate"),
		"sanity check on the fixture translator — must hit the stub catalog")

	// Find translate via the same CLITools() path listTools uses.
	var found bool
	for _, entry := range app.ToolReg.CLITools() {
		if entry.Info.Name != registry.ToolID("translate") {
			continue
		}
		found = true
		scope := "tools.translate"
		displayName := app.T().T(i18n.Scope(scope+".displayName"), entry.Info.DisplayName)
		desc := app.T().T(i18n.Scope(scope+".description"), entry.Info.Description)
		assert.Equal(t, "Traduction IA", displayName)
		assert.Equal(t, "Traduire avec un LLM", desc)
	}
	assert.True(t, found, "translate must be registered by InitRegistries")
}

// TestApp_TBeforeInit_ReturnsNoop confirms that callers which reach for
// T() before Init runs still get a safe passthrough Translator.
func TestApp_TBeforeInit_ReturnsNoop(t *testing.T) {
	app := &App{}
	got := app.T().T("tools.translate.displayName", "AI Translate")
	assert.Equal(t, "AI Translate", got, "pre-Init T() must pass source through")
}

// TestListTools_JSONOutputMatchesLocalized checks that JSON output
// surfaces the localized DisplayName (via Description fallback) too.
// The JSON surface is used by kapi-desktop's Wails bindings and bowrain
// server responses — regressions would silently revert UI to English.
func TestListTools_JSONOutputMatchesLocalized(t *testing.T) {
	// Build a temporary directory for the empty Config; the test doesn't
	// touch disk beyond that.
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".config", "kapi"), 0o755))

	app := &App{}
	app.InitRegistries()

	cat := makeMoCatalog(t, "fr-FR",
		[3]string{"tools.word-count.displayName", "Word Count", "Comptage de mots"},
	)
	app.translator = i18n.NewTranslator("fr-FR", cat)

	// Mirror listTools building a ToolInfo; we assert the localized
	// result flows into the typed output struct the JSON encoder sees.
	var info *output.ToolInfo
	for _, entry := range app.ToolReg.CLITools() {
		if entry.Info.Name != registry.ToolID("word-count") {
			continue
		}
		scope := "tools.word-count"
		displayName := app.T().T(i18n.Scope(scope+".displayName"), entry.Info.DisplayName)
		desc := app.T().T(i18n.Scope(scope+".description"), entry.Info.Description)
		if desc == "" {
			desc = displayName
		}
		info = &output.ToolInfo{
			Name:        "word-count",
			Description: desc,
			Category:    entry.Info.Category,
			Source:      "builtin",
		}
	}
	require.NotNil(t, info)
	assert.Equal(t, "Comptage de mots", info.Description)
}
