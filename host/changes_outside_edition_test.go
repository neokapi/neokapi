//go:build !js

package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// Outside a project a catalog whose name is a language holds that language:
// locales/nb.json is Norwegian, so Norwegian sent as its English edition is
// refused, and sent as its own edition (or with no edition) lands.
func TestOutsideAProject_ACatalogHoldsTheLanguageItsNameSays(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "locales"), 0o755))
	nb := filepath.Join(dir, "locales", "nb.json")
	require.NoError(t, os.WriteFile(nb, []byte(`{"a":"Hei"}`+"\n"), 0o644))
	app := &App{}
	app.InitRegistries()
	svc, err := app.ChangeService(t.Context(), ChangeServiceOptions{Root: dir, Origin: "apply"})
	require.NoError(t, err)

	page, err := svc.Read(t.Context(), change.ReadRequest{Doc: "locales/nb.json", OwnEdition: true})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1)
	assert.Equal(t, model.LocaleID("nb"), page.Blocks[0].Ref.Edition.Locale, "the read names the file's own edition")
	rev := page.Blocks[0].Rev

	text := func(s string) *string { return &s }
	set := func(edition model.LocaleID) change.Set {
		return change.Set{Ops: []change.Op{{Kind: change.KindSetContent, IfMatch: rev,
			At:   change.Ref{Doc: "locales/nb.json", Block: "a", Edition: model.EditionKey{Locale: edition}},
			Body: &change.SetContent{Text: text("Hei du")}}}}
	}
	res, err := svc.Apply(t.Context(), set("en"), change.Actor{Kind: change.ActorAgent, Name: "test"})
	require.NoError(t, err)
	assert.Equal(t, change.SetRefused, res.Status)
	require.NotNil(t, res.Ops[0].Error)
	assert.Contains(t, res.Ops[0].Error.Message, "outside a project locales/nb.json holds one edition, its own (nb)")
	assert.Contains(t, res.Ops[0].Error.Message, "kapi apply --out")

	res, err = svc.Apply(t.Context(), set("nb"), change.Actor{Kind: change.ActorAgent, Name: "test"})
	require.NoError(t, err)
	assert.Equal(t, change.SetApplied, res.Status, res.Ops[0].Error)
	assert.JSONEq(t, `{"a":"Hei du"}`, readFileIn(t, dir, "locales/nb.json"))
}

func TestPathLanguage(t *testing.T) {
	for in, want := range map[string]model.LocaleID{
		"locales/nb.json":      "nb",
		"de/guide.md":          "de",
		"i18n/pt_BR/app.json":  "pt-BR",
		"app/nb-NO.json":       "nb-NO",
		"docs/guide.md":        "",
		"config/app.yaml":      "",
		"src/strings.json":     "",
		"locales/messages.pot": "",
	} {
		assert.Equal(t, want, pathLanguage(in), in)
	}
}

// --out names the file the one edition a change set adds to a document
// outside a project is written to, built from the document; a second edition
// in the same change set has no file and is refused.
func TestOutsideAProject_OutWritesTheEditionAChangeSetAdds(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "locales"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "locales", "en.json"), []byte(`{"a":"Hello","b":"Bye"}`+"\n"), 0o644))
	app := &App{}
	app.InitRegistries()
	svc, err := app.ChangeService(t.Context(), ChangeServiceOptions{Root: dir, Origin: "apply", EditionOut: filepath.Join(dir, "locales", "nb.json")})
	require.NoError(t, err)

	text := func(s string) *string { return &s }
	op := func(block, edition, s string) change.Op {
		return change.Op{Kind: change.KindSetContent, IfMatch: model.AbsentRevision,
			At:   change.Ref{Doc: "locales/en.json", Block: block, Edition: model.EditionKey{Locale: model.LocaleID(edition)}},
			Body: &change.SetContent{Text: text(s)}}
	}
	res, err := svc.Apply(t.Context(), change.Set{Ops: []change.Op{op("a", "nb", "Hei"), op("b", "nb", "Ha det")}},
		change.Actor{Kind: change.ActorAgent, Name: "test"})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.JSONEq(t, `{"a":"Hei","b":"Ha det"}`, readFileIn(t, dir, "locales/nb.json"))
	assert.JSONEq(t, `{"a":"Hello","b":"Bye"}`, readFileIn(t, dir, "locales/en.json"), "the source is as it was")

	res, err = svc.Apply(t.Context(), change.Set{Ops: []change.Op{op("a", "de", "Hallo")}}, change.Actor{Kind: change.ActorAgent, Name: "test"})
	require.NoError(t, err)
	assert.Equal(t, change.SetRefused, res.Status, "--out holds the one edition it took")
}
