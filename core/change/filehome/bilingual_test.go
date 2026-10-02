package filehome_test

import (
	"context"
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
