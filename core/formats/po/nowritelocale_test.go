package po_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/po"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A PO msgstr is a target slot, and the writer is not always pointed at the
// locale it belongs to. `kapi apply` and the MCP edit tool pass no write locale
// at all; `kapi ksed -i` passes one only with --target-lang. With none, the
// writer's msgstr resolved to the empty string and every translation in the
// catalog was rewritten away — with nothing edited, and exit 0 (#1482).
//
// The document has no target locale here either, which is exactly what
// host.EditDocument does: it sets SourceLocale and leaves TargetLocale unset. So
// the model carries no target at all, the captured msgstr is the ONLY statement
// of the slot's content, and re-emitting it verbatim is the writer's only
// faithful move. "The writer cannot say what belongs here" is not the same fact
// as "there is no translation".
func TestNoWriteLocale_KeepsTheCatalogsTranslations(t *testing.T) {
	t.Parallel()
	input := `msgid ""
msgstr ""
"Content-Type: text/plain; charset=UTF-8\n"
"Language: fr\n"

#: src/app.c:12
msgid "We utilize the finest ingredients"
msgstr "Nous employons les meilleurs ingredients"

msgid "Save now"
msgstr "Enregistrer maintenant"
`
	out := noLocaleRoundtrip(t, input, nil)
	assert.Equal(t, input, out,
		"a catalog written with no active locale must come back unchanged, not blanked")
}

// The same write, with a source edit applied — the `kapi ksed -i` case. The edit
// must reach the msgid, and the translations must still be there afterwards.
func TestNoWriteLocale_SourceEditLandsAndTranslationsSurvive(t *testing.T) {
	t.Parallel()
	input := `msgid ""
msgstr ""
"Content-Type: text/plain; charset=UTF-8\n"
"Language: fr\n"

msgid "We utilize the finest ingredients"
msgstr "Nous employons les meilleurs ingredients"
`
	out := noLocaleRoundtrip(t, input, func(b *model.Block) {
		b.SetSourceText("We use the finest ingredients")
	})

	assert.Contains(t, out, `msgid "We use the finest ingredients"`,
		"the source edit must reach the msgid")
	assert.Contains(t, out, `msgstr "Nous employons les meilleurs ingredients"`,
		"editing the source must not blank the translation")
}

// A plural entry has one slot per form, and every one of them was blanked too.
func TestNoWriteLocale_KeepsPluralTranslations(t *testing.T) {
	t.Parallel()
	input := `msgid ""
msgstr ""
"Content-Type: text/plain; charset=UTF-8\n"
"Plural-Forms: nplurals=2; plural=(n > 1);\n"

msgid "one file"
msgid_plural "%d files"
msgstr[0] "un fichier"
msgstr[1] "%d fichiers"
`
	out := noLocaleRoundtrip(t, input, nil)
	assert.Equal(t, input, out,
		"a plural entry written with no active locale must keep every form's translation")
}

// noLocaleRoundtrip reads input the way host.EditDocument does — source locale
// only, no document target locale — optionally applies edit to every block, and
// writes with NO writer locale.
func noLocaleRoundtrip(t *testing.T, input string, edit func(*model.Block)) string {
	t.Helper()
	ctx := t.Context()

	reader := po.NewReader()
	writer := po.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	require.NoError(t, reader.Open(ctx, doc))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NoError(t, reader.Close())

	if edit != nil {
		for _, p := range parts {
			if b, ok := p.Resource.(*model.Block); ok {
				edit(b)
			}
		}
	}

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	// Deliberately no SetLocale.
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	require.NoError(t, writer.Close())
	return buf.String()
}
