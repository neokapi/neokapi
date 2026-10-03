package host

import (
	"context"
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
// without being told the language: the project's only target language, or
// the one its path names. A block that holds no translation lists no
// remove_edition.
func TestChangeService_AnInFileReadListsTheEditionItHolds(t *testing.T) {
	for name, tc := range map[string]struct {
		path  string
		langs []model.LocaleID
	}{
		"the project's only target language": {"locales/messages.po", []model.LocaleID{"nb"}},
		"the language its directory names":   {"locales/nb/messages.po", []model.LocaleID{"de", "nb"}},
		"the language its name names":        {"po/nb.po", []model.LocaleID{"de", "nb"}},
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

func TestHeldLocale(t *testing.T) {
	assert.Equal(t, model.LocaleID("nb"), heldLocale("x/messages.po", []model.LocaleID{"nb"}))
	assert.Equal(t, model.LocaleID("de"), heldLocale("locales/de/messages.po", []model.LocaleID{"de", "nb"}))
	assert.Equal(t, model.LocaleID("nb"), heldLocale("po/nb.po", []model.LocaleID{"de", "nb"}))
	assert.Empty(t, heldLocale("po/messages.po", []model.LocaleID{"de", "nb"}), "nothing names one of several")
	assert.Empty(t, heldLocale("de/nb.po", []model.LocaleID{"de", "nb"}), "two names for two languages say nothing")
}
