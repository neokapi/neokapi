package po_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/po"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
)

// gettext identifies a message by the pair (msgctxt, msgid) — that pair is why
// the same msgid can appear twice under different contexts, and it is what the
// reader names blocks by. These tests hold the name to the properties the rest
// of the system depends on: it survives a deletion, a reordering and a second
// read of the same bytes, and it separates entries a counter would conflate.

func identityNames(t *testing.T, input string) []string {
	t.Helper()
	blocks := translatableBlocks(readDefault(t, input))
	names := make([]string, 0, len(blocks))
	for _, b := range blocks {
		names = append(names, b.Name)
	}
	return names
}

// identityNamesSkeleton reads the same input through the skeleton path, which
// is a separate parse in this reader and must agree with the plain one.
func identityNamesSkeleton(t *testing.T, input string) []string {
	t.Helper()
	ctx := t.Context()
	reader := po.NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	reader.SetSkeletonStore(store)
	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	require.NoError(t, reader.Open(ctx, doc))
	t.Cleanup(func() { reader.Close() })
	blocks := translatableBlocks(testutil.CollectParts(t, reader.Read(ctx)))
	names := make([]string, 0, len(blocks))
	for _, b := range blocks {
		names = append(names, b.Name)
	}
	return names
}

const identityCatalog = `msgid "Alpha"
msgstr "Alfa"

msgid "Bravo"
msgstr "Beta"

msgid "Charlie"
msgstr "Gamma"
`

func TestIdentity_NameIsTheMsgid(t *testing.T) {
	assert.Equal(t, []string{"Alpha", "Bravo", "Charlie"}, identityNames(t, identityCatalog))
}

func TestIdentity_DeletingAnEarlierEntryLeavesLaterNames(t *testing.T) {
	after := identityNames(t, `msgid "Alpha"
msgstr "Alfa"

msgid "Charlie"
msgstr "Gamma"
`)
	assert.Equal(t, []string{"Alpha", "Charlie"}, after,
		"deleting Bravo must not rename Charlie")
}

func TestIdentity_ReorderingLeavesNames(t *testing.T) {
	after := identityNames(t, `msgid "Charlie"
msgstr "Gamma"

msgid "Alpha"
msgstr "Alfa"

msgid "Bravo"
msgstr "Beta"
`)
	assert.ElementsMatch(t, identityNames(t, identityCatalog), after)
	assert.Equal(t, []string{"Charlie", "Alpha", "Bravo"}, after)
}

// msgctxt is the other half of the gettext key. Two entries with the same
// source text under different contexts are different messages and must not
// share a name.
func TestIdentity_MsgctxtSeparatesIdenticalMsgids(t *testing.T) {
	names := identityNames(t, `msgctxt "menu"
msgid "Open"
msgstr "Ouvrir"

msgctxt "button"
msgid "Open"
msgstr "Ouvrir"

msgid "Open"
msgstr "Ouvrir"
`)
	require.Len(t, names, 3)
	assert.Equal(t, []string{"menu/Open", "button/Open", "Open"}, names)
	assert.Len(t, uniqueNames(names), 3)
}

func TestIdentity_StableAcrossTwoReads(t *testing.T) {
	assert.Equal(t, identityNames(t, identityCatalog), identityNames(t, identityCatalog))
}

// The skeleton path is a second, independent parse. A file read for a
// byte-exact round-trip must name its blocks exactly as a plain read does, or
// the same entry has two identities depending on how it was opened.
func TestIdentity_SkeletonPathAgreesWithPlainRead(t *testing.T) {
	const src = `msgctxt "menu"
msgid "Open"
msgstr "Ouvrir"

msgid "Alpha"
msgstr "Alfa"

msgid "One file"
msgid_plural "%d files"
msgstr[0] "Un fichier"
msgstr[1] "%d fichiers"
`
	assert.Equal(t, identityNames(t, src), identityNamesSkeleton(t, src))
}

// A plural entry becomes one block per declared plural form. The forms share a
// msgid (and, from form 1 on, a msgid_plural), so the form index separates
// them — the file's own msgstr[N] index, not a position in the document.
func TestIdentity_PluralFormsScopedByIndex(t *testing.T) {
	names := identityNames(t, `msgid "One file"
msgid_plural "%d files"
msgstr[0] "Un fichier"
msgstr[1] "%d fichiers"

msgid "One folder"
msgid_plural "%d folders"
msgstr[0] "Un dossier"
msgstr[1] "%d dossiers"
`)
	assert.Equal(t, []string{
		"One file[0]", "One file[1]",
		"One folder[0]", "One folder[1]",
	}, names)
}

// Two plural entries can share a msgid_plural (a common shape when the plural
// string is generic). Naming a form after its source text would collapse them;
// naming it after the entry key does not.
func TestIdentity_PluralFormsWithSharedPluralText(t *testing.T) {
	names := identityNames(t, `msgid "One file"
msgid_plural "%d items"
msgstr[0] ""
msgstr[1] ""

msgid "One folder"
msgid_plural "%d items"
msgstr[0] ""
msgstr[1] ""
`)
	assert.Len(t, uniqueNames(names), 4, "every plural form needs its own name: %v", names)
}

// A catalog that repeats a (msgctxt, msgid) pair is malformed — gettext rejects
// it — but the reader must survive it without handing two entries one identity.
func TestIdentity_RepeatedKeyIsDisambiguated(t *testing.T) {
	names := identityNames(t, `msgid "Alpha"
msgstr "Alfa"

msgid "Bravo"
msgstr "Beta"

msgid "Alpha"
msgstr "Alpha again"
`)
	assert.Equal(t, []string{"Alpha", "Bravo", "Alpha#2"}, names)
}

// A PO block is named by its msgid, and in bilingual mode the msgid IS the
// source text — the one shape core/model/structural.go warns about, since
// reconcile grades the name (context) and the text (content) as independent
// signals and a name derived from the text moves both at once.
//
// It is the right answer for gettext anyway, and these tests pin what it costs
// and what it buys, so neither is a surprise later. See entryKey for the
// reasoning.
func gradePO(t *testing.T, before, after string, cfg map[string]any) map[string]reconcile.Kind {
	t.Helper()
	read := func(src string) []*model.Block {
		if cfg == nil {
			return translatableBlocks(readDefault(t, src))
		}
		return translatableBlocks(readWithConfig(t, src, cfg))
	}

	const scope = "messages.po"
	var prior []reconcile.Unit
	for _, b := range read(before) {
		u := reconcile.Identify(scope, b)
		u.Key = b.Name
		prior = append(prior, u)
	}

	grades := make(map[string]reconcile.Kind)
	for _, res := range reconcile.Blocks(scope, read(after), prior) {
		grades[res.Block.Name] = res.Kind
	}
	return grades
}

const editedBefore = `msgid "Alpha"
msgstr "Alfa"

msgid "Bravo"
msgstr "Beta"
`

// Editing a msgid retires one message and declares another — gettext's own
// semantics, and what the grading says.
func TestIdentity_EditedMsgidGradesNew(t *testing.T) {
	grades := gradePO(t, editedBefore, `msgid "Alpha"
msgstr "Alfa"

msgid "Bravo edited"
msgstr "Beta"
`, nil)

	assert.Equal(t, reconcile.Unchanged, grades["Alpha"],
		"the untouched entry keeps everything")
	assert.Equal(t, reconcile.New, grades["Bravo edited"],
		"a changed msgid is a new message, not an amended one — see entryKey")
}

// The cost is bounded to source edits. Editing only the translation leaves both
// the msgid and the block's source text alone, so the entry keeps its identity
// and its history.
func TestIdentity_EditedMsgstrKeepsIdentity(t *testing.T) {
	grades := gradePO(t, editedBefore, `msgid "Alpha"
msgstr "Alfa"

msgid "Bravo"
msgstr "Beta korrigert"
`, nil)

	assert.Equal(t, reconcile.Unchanged, grades["Bravo"],
		"only the target moved; the entry is the same entry")
}

// And deleting an entry does not disturb the one after it — the property the
// key was chosen for.
func TestIdentity_DeletionDoesNotDisturbSurvivors(t *testing.T) {
	grades := gradePO(t, `msgid "Alpha"
msgstr "Alfa"

msgid "Bravo"
msgstr "Beta"

msgid "Charlie"
msgstr "Gamma"
`, `msgid "Alpha"
msgstr "Alfa"

msgid "Charlie"
msgstr "Gamma"
`, nil)

	assert.Equal(t, reconcile.Unchanged, grades["Charlie"])
}

// msgctxt does not rescue a msgid edit, and is not meant to: it is a shared
// bucket ("menu", "button"), not a per-message key, so naming by context alone
// would collide across unrelated entries and fall back to an ordinal.
func TestIdentity_EditedMsgidUnderContextStillGradesNew(t *testing.T) {
	grades := gradePO(t, `msgctxt "menu"
msgid "Open"
msgstr "Ouvrir"
`, `msgctxt "menu"
msgid "Open file"
msgstr "Ouvrir"
`, nil)

	assert.Equal(t, reconcile.New, grades["menu/Open file"])
}

// Monolingual mode is the case where the trade-off does not arise: the msgid is
// a pure identifier and the msgstr carries the source text, so name and content
// are independent and an edited string keeps its history.
func TestIdentity_MonolingualEditedTextGradesEdited(t *testing.T) {
	cfg := map[string]any{"bilingualMode": false}
	grades := gradePO(t, `msgid "greeting"
msgstr "Hello"
`, `msgid "greeting"
msgstr "Hello there"
`, cfg)

	assert.Equal(t, reconcile.Edited, grades["greeting"],
		"the identifier held still while the words changed")
}

func uniqueNames(names []string) map[string]bool {
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		seen[n] = true
	}
	return seen
}
