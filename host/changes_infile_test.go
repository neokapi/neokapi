package host

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

const heldPO = `msgid ""
msgstr ""
"Language: nb\n"
"Content-Type: text/plain; charset=UTF-8\n"

msgctxt "button"
msgid "Book"
msgstr "Bok"

msgid "Cancel"
msgstr ""
`

// A PO catalog that keeps its translation in it lists that edition in a read
// without being told the language: the one its header declares, the
// project's only target language, or the one its path names. A block that
// holds no translation lists no remove_edition.
func TestChangeService_AnInFileReadListsTheEditionItHolds(t *testing.T) {
	for name, tc := range map[string]struct {
		path  string
		langs []model.LocaleID
	}{
		"the project's only target language": {"locales/messages.po", []model.LocaleID{"nb"}},
		"the language its directory names":   {"locales/nb/messages.po", []model.LocaleID{"de", "nb"}},
		"the language its name names":        {"po/nb.po", []model.LocaleID{"de", "nb"}},
		"the language its header declares":   {"po/messages.po", []model.LocaleID{"de", "nb"}},
	} {
		t.Run(name, func(t *testing.T) {
			item := project.ContentItem{Path: tc.path, Format: &project.FormatSpec{Name: "po"}}
			a, recipe := changeProject(t, item, map[string]string{tc.path: heldPO}, func(p *project.KapiProject) {
				p.Defaults.TargetLanguages = tc.langs
			})
			page, err := changeService(t, a, recipe).Read(context.Background(), change.ReadRequest{Doc: tc.path})
			require.NoError(t, err)
			byKey := map[string]change.BlockRead{}
			for _, b := range page.Blocks {
				byKey[b.Ref.Block] = b
			}
			book, ok := byKey["button/Book"]
			require.True(t, ok, "%+v", page.Blocks)
			require.Contains(t, book.Editions, "nb")
			assert.Equal(t, "Bok", book.Editions["nb"].Text)
			assert.Contains(t, book.Ops, change.KindRemoveEdition)
			cancel, ok := byKey["Cancel"]
			require.True(t, ok, "%+v", page.Blocks)
			assert.Empty(t, cancel.Editions)
			assert.NotContains(t, cancel.Ops, change.KindRemoveEdition, "a block that holds no translation has none to remove")
		})
	}
}

// A read that names the document's own edition gives each ref the language
// the recipe gives the document, and the service takes a ref so named, and
// an insert_block whose editions use it, as the document's own edition.
func TestChangeService_AReadNamesTheDocumentsOwnEdition(t *testing.T) {
	item := project.ContentItem{Path: "locales/en.json"}
	a, recipe := changeProject(t, item, map[string]string{"locales/en.json": `{"nav": {"home": "Home"}}` + "\n"})
	svc := changeService(t, a, recipe)
	ctx := context.Background()

	plain, err := svc.Read(ctx, change.ReadRequest{Doc: "locales/en.json"})
	require.NoError(t, err)
	require.Len(t, plain.Blocks, 1)
	assert.True(t, plain.Blocks[0].Ref.Edition.IsZero(), "a read that is not asked leaves the edition out")

	page, err := svc.Read(ctx, change.ReadRequest{Doc: "locales/en.json", OwnEdition: true})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1)
	b := page.Blocks[0]
	assert.Equal(t, change.Ref{Doc: "locales/en.json", Block: "nav.home", Edition: model.EditionKey{Locale: "en"}}, b.Ref)

	own := b.Ref.Edition.Locale
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{
		setTo(b.Ref, b.Rev, "Start"),
		{Kind: change.KindInsertBlock, At: change.Ref{Doc: "locales/en.json"}, Body: &change.InsertBlock{
			After: "nav.home", Name: "nav.help", Editions: map[string]change.Content{string(own): {Text: new("Help")}}}},
	}}, change.Actor{Kind: change.ActorAgent, Name: "claude", Session: "s1"})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.JSONEq(t, `{"nav": {"home": "Start", "help": "Help"}}`, readFile(t, recipe, "locales/en.json"))
}

func TestHeldLocale(t *testing.T) {
	assert.Equal(t, model.LocaleID("nb"), heldLocale("x/messages.po", "", []model.LocaleID{"nb"}))
	assert.Equal(t, model.LocaleID("de"), heldLocale("locales/de/messages.po", "", []model.LocaleID{"de", "nb"}))
	assert.Equal(t, model.LocaleID("nb"), heldLocale("po/nb.po", "", []model.LocaleID{"de", "nb"}))
	assert.Empty(t, heldLocale("po/messages.po", "", []model.LocaleID{"de", "nb"}), "nothing names one of several")
	assert.Empty(t, heldLocale("de/nb.po", "", []model.LocaleID{"de", "nb"}), "two names for two languages say nothing")
	assert.Equal(t, model.LocaleID("nb"), heldLocale("po/messages.po", "nb", []model.LocaleID{"de", "nb"}), "the header says")
	assert.Equal(t, model.LocaleID("de"), heldLocale("po/nb.po", "de", []model.LocaleID{"de", "nb"}), "the header wins over the name")
	assert.Equal(t, model.LocaleID("pt-BR"), heldLocale("po/messages.po", "pt_BR", []model.LocaleID{"pt-BR"}))
	assert.Equal(t, model.LocaleID("fr"), heldLocale("po/messages.po", "fr", []model.LocaleID{"de", "nb"}), "a language no target names")
}

// The header of a PO catalog is the entry with an empty msgid that opens it;
// a catalog whose first entry has a msgid continued on the next lines has
// none.
func TestPOHeaderField(t *testing.T) {
	for name, tc := range map[string]struct{ po, want string }{
		"one line per field":    {heldPO, "nb"},
		"after comments":        {"# Norwegian\n#, fuzzy\nmsgid \"\"\nmsgstr \"\"\n\"Language: nb_NO\\n\"\n", "nb_NO"},
		"the msgstr's own line": {"msgid \"\"\nmsgstr \"Language: de\\n\"\n\nmsgid \"a\"\nmsgstr \"b\"\n", "de"},
		"no Language field":     {"msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain\\n\"\n", ""},
		"no header":             {"msgid \"Book\"\nmsgstr \"Bok\"\n", ""},
		"a continued msgid":     {"msgid \"\"\n\"Language: nb\"\nmsgstr \"x\"\n", ""},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, poHeaderField(strings.NewReader(tc.po), "Language"))
		})
	}
}
