package loader

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/formats/mo"
	"github.com/neokapi/neokapi/core/model"
)

// buildFixtureMO compiles a tiny MO catalog for the given (ctx, src, tgt)
// triples using the real MO writer. Returns the file bytes so tests can
// drop them at exactly the path the loader expects.
func buildFixtureMO(t *testing.T, locale string, entries ...[3]string) []byte {
	t.Helper()
	w := mo.NewWriter()
	var buf bytes.Buffer
	require.NoError(t, w.SetOutputWriter(&buf))
	w.SetLocale(model.LocaleID(locale))

	parts := make(chan *model.Part, len(entries))
	for i, e := range entries {
		b := model.NewBlock(("tu" + strFromInt(i+1)), e[1])
		b.Name = e[0]
		b.SetTargetText(model.LocaleID(locale), e[2])
		parts <- &model.Part{Type: model.PartBlock, Resource: b}
	}
	close(parts)
	require.NoError(t, w.Write(context.Background(), parts))
	return buf.Bytes()
}

func strFromInt(i int) string {
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

func TestI18nCatalogs_LoadsPluginMOByConvention(t *testing.T) {
	dir := t.TempDir()

	// Convention: <pluginRoot>/<name>/<version>/i18n/<locale>.mo
	pluginDir := filepath.Join(dir, "okapi-bridge", "2.20.0", "i18n")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))

	catalogBytes := buildFixtureMO(t, "fr-FR",
		[3]string{"plugins.okf_html.displayName", "HTML Filter", "Filtre HTML"},
	)
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "fr-FR.mo"), catalogBytes, 0o644))

	l := NewPluginLoader(dir, nil)
	// Inject a plugin entry (skipping the full manifest scan to keep the
	// test narrow to the I18nCatalogs method).
	l.plugins = []PluginInfo{{Name: "okapi-bridge", Version: "2.20.0"}}

	catalogs, err := l.I18nCatalogs("fr-FR")
	require.NoError(t, err)
	require.Len(t, catalogs, 1)
	assert.Equal(t, "Filtre HTML",
		catalogs[0].GetC("HTML Filter", "plugins.okf_html.displayName"))
}

func TestI18nCatalogs_SilentSkipWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "okapi-bridge", "2.20.0"), 0o755))

	l := NewPluginLoader(dir, nil)
	l.plugins = []PluginInfo{{Name: "okapi-bridge", Version: "2.20.0"}}

	catalogs, err := l.I18nCatalogs("fr-FR")
	require.NoError(t, err, "missing catalog is not an error")
	assert.Empty(t, catalogs)
}

func TestI18nCatalogs_EmptyOrEnglishLocaleReturnsNil(t *testing.T) {
	l := NewPluginLoader("", nil)
	c, err := l.I18nCatalogs("")
	require.NoError(t, err)
	assert.Nil(t, c)
	c, err = l.I18nCatalogs("en")
	require.NoError(t, err)
	assert.Nil(t, c)
}

func TestI18nCatalogs_MultiplePlugins(t *testing.T) {
	dir := t.TempDir()

	// Plugin A
	aDir := filepath.Join(dir, "plugin-a", "1.0.0", "i18n")
	require.NoError(t, os.MkdirAll(aDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(aDir, "fr-FR.mo"),
		buildFixtureMO(t, "fr-FR", [3]string{"plugins.a.displayName", "A", "A-FR"}),
		0o644))

	// Plugin B
	bDir := filepath.Join(dir, "plugin-b", "2.0.0", "i18n")
	require.NoError(t, os.MkdirAll(bDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bDir, "fr-FR.mo"),
		buildFixtureMO(t, "fr-FR", [3]string{"plugins.b.displayName", "B", "B-FR"}),
		0o644))

	l := NewPluginLoader(dir, nil)
	l.plugins = []PluginInfo{
		{Name: "plugin-a", Version: "1.0.0"},
		{Name: "plugin-b", Version: "2.0.0"},
	}

	catalogs, err := l.I18nCatalogs("fr-FR")
	require.NoError(t, err)
	require.Len(t, catalogs, 2)
	// Exact catalog-to-index ordering mirrors plugin enumeration order,
	// which is stable (the loader preserves insertion order from scan).
	assert.Equal(t, "A-FR", catalogs[0].GetC("A", "plugins.a.displayName"))
	assert.Equal(t, "B-FR", catalogs[1].GetC("B", "plugins.b.displayName"))
}
