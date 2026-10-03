package filehome_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/model"
)

const frenchPO = "msgid \"\"\nmsgstr \"\"\n\"Language: fr\\n\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\nmsgid \"Hello\"\nmsgstr \"Bonjour\"\n"

const frenchXLIFF = `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
<file original="a.txt" source-language="en" target-language="fr" datatype="plaintext">
<body>
<trans-unit id="1"><source>Hello</source><target>Bonjour</target></trans-unit>
</body>
</file>
</xliff>
`

// A translation a bilingual file holds is removed where its writer leaves a
// unit with no translation, and refused where the writer would write the
// source in its place, as the XLIFF and TMX writers fill a missing
// translation from the source: removed, not replaced, or nothing is written.
func TestFileHome_RemovingATranslationABilingualFileHolds(t *testing.T) {
	const xliff2 = `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1"><segment><source>Hello</source><target>Bonjour</target></segment></unit>
  </file>
</xliff>
`
	const tmx = `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4"><header srclang="en" datatype="plaintext" segtype="sentence" adminlang="en" creationtool="t" creationtoolversion="1" o-tmf="t"/><body>
<tu tuid="1"><tuv xml:lang="en"><seg>Hello</seg></tuv><tuv xml:lang="fr"><seg>Bonjour</seg></tuv></tu>
</body></tmx>
`
	tests := []struct {
		doc, body string
		target    model.LocaleID
		want      string // empty: refused, the file as it was
	}{
		{doc: "fr.po", body: frenchPO, target: "fr", want: strings.Replace(frenchPO, `msgstr "Bonjour"`, `msgstr ""`, 1)},
		{doc: "fr.xlf", body: frenchXLIFF},
		{doc: "fr.xliff", body: xliff2},
		{doc: "fr.tmx", body: tmx},
	}
	for _, tc := range tests {
		t.Run(tc.doc, func(t *testing.T) {
			f := newFixture(t, map[string]string{tc.doc: tc.body})
			f.svc = change.NewService(filehome.Formats{Registry: f.reg}, change.OneHome(filehome.New(
				filehome.DirLayout{Root: f.dir, Formats: f.reg, SourceLocale: "en", TargetLocale: tc.target},
				filehome.Options{LockDir: t.TempDir()})))
			ctx := context.Background()
			page, err := f.svc.Read(ctx, change.ReadRequest{Doc: tc.doc})
			require.NoError(t, err)
			require.Len(t, page.Blocks, 1)
			b := page.Blocks[0]
			fr, ok := b.Editions["fr"]
			require.True(t, ok, "the file holds the French translation")
			at := b.Ref
			at.Edition = mustEdition(t, "fr")
			res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{
				{Kind: change.KindRemoveEdition, At: at, IfMatch: fr.Rev, Body: &change.RemoveEdition{}},
			}}, person)
			require.NoError(t, err)
			if tc.want == "" {
				require.Equal(t, change.SetRefused, res.Status, "%+v", res.Ops)
				require.NotNil(t, res.Ops[0].Error)
				assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code)
				assert.Equal(t, string(change.KindRemoveEdition), res.Ops[0].Error.Capability)
				assert.Contains(t, res.Ops[0].Error.Message, "would write its source in its place")
				assert.Equal(t, tc.body, f.read(t, tc.doc), "nothing is written")
				return
			}
			require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
			assert.Equal(t, tc.want, f.read(t, tc.doc))
			page, err = f.svc.Read(ctx, change.ReadRequest{Doc: tc.doc})
			require.NoError(t, err)
			_, held := page.Blocks[0].Editions["fr"]
			assert.False(t, held, "the translation reads back absent")
		})
	}
}

// A translation kept in a bilingual file of its own (fr/messages.po beside
// messages.po, as a recipe's target template keeps it) is removed the same
// way: the PO catalog's entry loses its msgstr, and the XLIFF writers, which
// would fill the unit from its source, refuse the removal with the file left
// as it was. The document's own file is never written.
func TestFileHome_RemovingATranslationFromItsOwnBilingualFile(t *testing.T) {
	xliff2 := func(lang, target string) string {
		tgt := ""
		if target != "" {
			tgt = "<target>" + target + "</target>"
		}
		return `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="` + lang + `">
  <file id="f1">
    <unit id="1"><segment><source>Hello</source>` + tgt + `</segment></unit>
  </file>
</xliff>
`
	}
	frenchXLIFF2 := xliff2("fr", "Bonjour")
	tests := []struct {
		name, doc, source, translation string
		want                           string // empty: refused, the file as it was
	}{
		{name: "a PO catalog", doc: "messages.po", source: poCatalog("en", [2]string{"Hello", ""}), translation: frenchPO,
			want: strings.Replace(frenchPO, `msgstr "Bonjour"`, `msgstr ""`, 1)},
		{name: "an XLIFF 1.2 file", doc: "a.xlf", source: strings.Replace(frenchXLIFF, "<target>Bonjour</target>", "", 1), translation: frenchXLIFF},
		{name: "an XLIFF 2 file", doc: "a.xliff", source: xliff2("en", ""), translation: frenchXLIFF2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newCatalogFixture(t, map[string]string{tc.doc: tc.source, "fr/" + tc.doc: tc.translation}, filehome.Options{})
			ctx := context.Background()
			french := mustEdition(t, "fr")
			page, err := f.svc.Read(ctx, change.ReadRequest{Doc: tc.doc, Editions: []model.EditionKey{french}})
			require.NoError(t, err)
			require.Len(t, page.Blocks, 1)
			fr, ok := page.Blocks[0].Editions["fr"]
			require.True(t, ok, "the French file holds the translation")
			at := page.Blocks[0].Ref
			at.Edition = french
			res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{
				{Kind: change.KindRemoveEdition, At: at, IfMatch: fr.Rev, Body: &change.RemoveEdition{}},
			}}, person)
			require.NoError(t, err)
			assert.Equal(t, tc.source, f.read(t, tc.doc), "the document's own file is never written")
			if tc.want == "" {
				require.Equal(t, change.SetRefused, res.Status, "%+v", res.Ops)
				require.NotNil(t, res.Ops[0].Error)
				assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code)
				assert.Equal(t, string(change.KindRemoveEdition), res.Ops[0].Error.Capability)
				assert.Contains(t, res.Ops[0].Error.Message, "would write its source in its place")
				assert.Equal(t, tc.translation, f.read(t, "fr/"+tc.doc), "nothing is written")
				return
			}
			require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
			assert.Equal(t, tc.want, f.read(t, "fr/"+tc.doc))
			page, err = f.svc.Read(ctx, change.ReadRequest{Doc: tc.doc, Editions: []model.EditionKey{french}})
			require.NoError(t, err)
			_, held := page.Blocks[0].Editions["fr"]
			assert.False(t, held, "the translation reads back absent")
		})
	}
}

// TestFileHome_ABilingualFileTakesOnlyTheEditionsItHolds pins rule 7 of the
// contract for a file that holds its translations itself: an edit of the
// translation the file holds reaches its bytes, and an edit of an edition the
// file has no place for is refused instead of reported applied and dropped.
func TestFileHome_ABilingualFileTakesOnlyTheEditionsItHolds(t *testing.T) {
	tests := []struct {
		name    string
		doc     string
		body    string
		target  model.LocaleID
		edition string
		code    change.Code // empty: the edit lands
		want    string
	}{
		{name: "the translation an XLIFF file declares", doc: "fr.xlf", body: frenchXLIFF, edition: "fr",
			want: strings.Replace(frenchXLIFF, "<target>Bonjour</target>", "<target>Salut</target>", 1)},
		{name: "the translation of a PO catalog whose language the layout gives", doc: "fr.po", body: frenchPO, target: "fr", edition: "fr",
			want: strings.Replace(frenchPO, `msgstr "Bonjour"`, `msgstr "Salut"`, 1)},
		{name: "a PO catalog read with no language for its translation", doc: "fr.po", body: frenchPO, edition: "fr", code: change.CodeUnsupported},
		{name: "a language the file holds no translation in", doc: "fr.po", body: frenchPO, target: "fr", edition: "de", code: change.CodeUnsupported},
		{name: "an edition with a channel", doc: "fr.po", body: frenchPO, target: "fr", edition: "fr;channel=short", code: change.CodeUnsupported},
		{name: "an edition with a channel, in XLIFF", doc: "fr.xlf", body: frenchXLIFF, edition: "fr;channel=short", code: change.CodeUnsupported},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, map[string]string{tc.doc: tc.body})
			f.svc = change.NewService(filehome.Formats{Registry: f.reg}, change.OneHome(filehome.New(
				filehome.DirLayout{Root: f.dir, Formats: f.reg, SourceLocale: "en", TargetLocale: tc.target},
				filehome.Options{LockDir: t.TempDir()})))
			ctx := context.Background()
			page, err := f.svc.Read(ctx, change.ReadRequest{Doc: tc.doc})
			require.NoError(t, err)
			require.NotEmpty(t, page.Blocks)
			b := page.Blocks[len(page.Blocks)-1]
			require.Equal(t, "Hello", b.Text)
			rev := model.AbsentRevision
			if ed, ok := b.Editions[tc.edition]; ok {
				rev = ed.Rev
			}
			at := b.Ref
			at.Edition = mustEdition(t, tc.edition)

			res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{setOp(at, rev, "Salut")}}, person)
			require.NoError(t, err)
			if tc.code == "" {
				require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
				assert.Equal(t, change.OpApplied, res.Ops[0].Status)
				assert.Equal(t, tc.want, f.read(t, tc.doc))
				return
			}
			require.Equal(t, change.SetRefused, res.Status, "%+v", res.Ops)
			require.NotNil(t, res.Ops[0].Error)
			assert.Equal(t, tc.code, res.Ops[0].Error.Code, res.Ops[0].Error.Message)
			assert.Equal(t, "edition", res.Ops[0].Error.Capability)
			assert.Equal(t, tc.body, f.read(t, tc.doc), "nothing is written")
		})
	}
}

// catalogLayout is a directory of PO catalogs whose translation into each
// language is a catalog of its own, <locale>/<name>, as a recipe whose target
// template names one file per language keeps them.
type catalogLayout struct {
	filehome.DirLayout
}

func (l catalogLayout) Locate(ctx context.Context, doc string) (filehome.Doc, error) {
	d, err := l.DirLayout.Locate(ctx, doc)
	if err != nil {
		return d, err
	}
	ref := d.Ref
	d.Editions, d.TargetLocale = change.EditionsPerFile, ""
	d.EditionFile = func(k model.EditionKey) (filehome.EditionFile, bool) {
		rel := string(k.Locale) + "/" + filepath.Base(ref)
		return filehome.EditionFile{Ref: rel, Path: filepath.Join(l.Root, filepath.FromSlash(rel)), Bilingual: true}, true
	}
	return d, nil
}

func newCatalogFixture(t *testing.T, files map[string]string, opts filehome.Options) *fixture {
	t.Helper()
	f := newFixture(t, files)
	opts.LockDir = t.TempDir()
	home := filehome.New(catalogLayout{filehome.DirLayout{Root: f.dir, Formats: f.reg, SourceLocale: "en"}}, opts)
	f.svc = change.NewService(filehome.Formats{Registry: f.reg}, change.OneHome(home))
	return f
}

// poCatalog is a PO catalog in lang holding each msgid and msgstr pair.
func poCatalog(lang string, entries ...[2]string) string {
	var b strings.Builder
	b.WriteString("msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\"Language: " + lang + "\\n\"\n")
	for _, e := range entries {
		b.WriteString("\nmsgid \"" + e[0] + "\"\nmsgstr \"" + e[1] + "\"\n")
	}
	return b.String()
}

// TestFileHome_ABilingualTranslationFileHoldsTheEditionAsItsTranslation pins
// EditionFile.Bilingual: the French catalog beside an English one holds the
// French edition as each entry's msgstr, never as its msgid. A read joins the
// msgstr, an entry with no msgstr holds no edition, and a write keeps every
// translation the change does not name. While the catalog holds the source's
// entries it is written through its own skeleton, so its header stays.
func TestFileHome_ABilingualTranslationFileHoldsTheEditionAsItsTranslation(t *testing.T) {
	en := poCatalog("en", [2]string{"Hello", ""}, [2]string{"Goodbye", ""}, [2]string{"Thanks", ""})
	fr := poCatalog("fr", [2]string{"Hello", "Bonjour"}, [2]string{"Goodbye", "Au revoir"}, [2]string{"Thanks", ""})
	grown := poCatalog("en", [2]string{"Hello", ""}, [2]string{"Goodbye", ""}, [2]string{"Thanks", ""}, [2]string{"New", ""})
	ctx := context.Background()
	french := mustEdition(t, "fr")

	for _, tc := range []struct {
		name        string
		materialize bool
		source      string
		want        string
	}{
		{name: "edited in place", source: en,
			want: poCatalog("fr", [2]string{"Hello", "Bonjour"}, [2]string{"Goodbye", "Au revoir"}, [2]string{"Thanks", "Merci"})},
		{name: "materialized while it holds every entry of the source", materialize: true, source: en,
			want: poCatalog("fr", [2]string{"Hello", "Bonjour"}, [2]string{"Goodbye", "Au revoir"}, [2]string{"Thanks", "Merci"})},
		// The catalog follows the source again, written from its skeleton.
		{name: "materialized once the source gained an entry", materialize: true, source: grown,
			want: poCatalog("en", [2]string{"Hello", "Bonjour"}, [2]string{"Goodbye", "Au revoir"}, [2]string{"Thanks", "Merci"}, [2]string{"New", ""})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCatalogFixture(t, map[string]string{"messages.po": tc.source, "fr/messages.po": fr}, filehome.Options{Materialize: tc.materialize})
			page, err := f.svc.Read(ctx, change.ReadRequest{Doc: "messages.po", Editions: []model.EditionKey{french}})
			require.NoError(t, err)
			held := map[string]string{}
			var thanks change.BlockRead
			for _, b := range page.Blocks {
				if ed, ok := b.Editions["fr"]; ok {
					held[b.Text] = ed.Text
				}
				if b.Text == "Thanks" {
					thanks = b
				}
			}
			assert.Equal(t, map[string]string{"Hello": "Bonjour", "Goodbye": "Au revoir"}, held,
				"the read joins each msgstr, and an untranslated entry holds no French")

			at := thanks.Ref
			at.Edition = french
			res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{setOp(at, model.AbsentRevision, "Merci")}}, person)
			require.NoError(t, err)
			require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
			assert.Equal(t, tc.want, f.read(t, "fr/messages.po"))
			assert.Equal(t, tc.source, f.read(t, "messages.po"), "the source catalog is untouched")
		})
	}
}
