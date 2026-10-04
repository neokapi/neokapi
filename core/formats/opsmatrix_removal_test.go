package formats_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// The removal cells of the operations matrix prove remove_edition for every
// format that keeps each translation in a file of its own and lists the
// operation: the translation is a block of that file, so it leaves the file
// with its block, which the format's writer removes (format.StructureEditor).
// Each cell removes one translation through the change service over the file
// home, with the translation in fr/<catalog> as a project's target template
// puts it, and asserts
//
//   - the translation's file, byte for byte: the block's shell gone, with the
//     notes and metadata the format keeps beside it, and every other byte as
//     it was;
//   - the document's own file, unchanged;
//   - the read back: the translation absent, and every other translation at
//     the revision it was read with;
//   - the refusals: the code, and both files as they were.
//
// A format lists the operation for every block it reads, so a block its
// writer cannot take out (a YAML sequence item, named by its position) has a
// cell holding it to a refusal.
//
// TestRemovalMatrixCoversEveryDeclaration holds every format that lists
// remove_edition with one edition per file to its cells, and every other
// such format with a skeleton pair to refusing the operation as unsupported.

// removalRow is one format's catalog, the French translation of it, and the
// cells over them.
type removalRow struct {
	format registry.FormatID
	// file is the catalog's name, template its content under
	// testdata/opsmatrix/structural, and french the content of fr/<file>.
	file     string
	template string
	french   string
	cells    []removalCell
}

// removalCell removes the French translation of one block.
type removalCell struct {
	name  string
	block string
	// stale sends a revision the translation does not hold.
	stale bool
	// want is fr/<file> as the removal leaves it; refused is the code the
	// removal is refused with, and both files then stay as they were.
	want    string
	refused change.Code
	// source and french, when set, are the catalog and its French
	// translation the cell runs on in place of the row's.
	source, french string
	// again, when set, is the text a set_content with if_match absent then
	// creates the translation with, and recreated fr/<file> as that leaves
	// it: the writer adds the block back beside the one it sat by.
	again, recreated string
}

func removalMatrix() []removalRow {
	return []removalRow{
		{format: "json", file: "catalog.json", template: "catalog.json.tmpl",
			french: "{\n  \"nav\": {\n    \"home\": \"Accueil\",\n    \"cart\": \"Panier\",\n    \"legacy\": \"Ancien lien\"\n  },\n  \"title\": \"Bienvenue dans la {shop}\",\n  \"list\": [\"gardé\", \"tel quel\"],\n  \"count\": 42\n}\n",
			cells: []removalCell{
				{name: "remove_edition of a key between two others, and set_content creates it again there", block: "nav.cart", again: "Panier",
					want:      "{\n  \"nav\": {\n    \"home\": \"Accueil\",\n    \"legacy\": \"Ancien lien\"\n  },\n  \"title\": \"Bienvenue dans la {shop}\",\n  \"list\": [\"gardé\", \"tel quel\"],\n  \"count\": 42\n}\n",
					recreated: "{\n  \"nav\": {\n    \"home\": \"Accueil\",\n    \"cart\": \"Panier\",\n    \"legacy\": \"Ancien lien\"\n  },\n  \"title\": \"Bienvenue dans la {shop}\",\n  \"list\": [\"gardé\", \"tel quel\"],\n  \"count\": 42\n}\n"},
				{name: "remove_edition of the last key of an object", block: "nav.legacy",
					want: "{\n  \"nav\": {\n    \"home\": \"Accueil\",\n    \"cart\": \"Panier\"\n  },\n  \"title\": \"Bienvenue dans la {shop}\",\n  \"list\": [\"gardé\", \"tel quel\"],\n  \"count\": 42\n}\n"},
				{name: "remove_edition of a top-level key holding a placeholder", block: "title",
					want: "{\n  \"nav\": {\n    \"home\": \"Accueil\",\n    \"cart\": \"Panier\",\n    \"legacy\": \"Ancien lien\"\n  },\n  \"list\": [\"gardé\", \"tel quel\"],\n  \"count\": 42\n}\n"},
				{name: "remove_edition refuses a revision that moved", block: "nav.home", stale: true, refused: change.CodeStale},
			}},
		{format: "yaml", file: "catalog.yaml", template: "catalog.yaml.tmpl",
			french: "# Chaînes de la boutique\nnav:\n  # Le lien d'accueil\n  home: Accueil\n  cart: Panier # dans l'en-tête\n  # Retirer après 2.0\n  legacy: Ancien lien\nhelp: |\n  Lisez le guide\n  avant de commander.\nfooter: \"Pied de page\"\n",
			cells: []removalCell{
				{name: "remove_edition removes the comment above the key, which is the block's note, and set_content creates the key again", block: "nav.legacy", again: "Ancien lien",
					want:      "# Chaînes de la boutique\nnav:\n  # Le lien d'accueil\n  home: Accueil\n  cart: Panier # dans l'en-tête\nhelp: |\n  Lisez le guide\n  avant de commander.\nfooter: \"Pied de page\"\n",
					recreated: "# Chaînes de la boutique\nnav:\n  # Le lien d'accueil\n  home: Accueil\n  cart: Panier # dans l'en-tête\n  legacy: Ancien lien\nhelp: |\n  Lisez le guide\n  avant de commander.\nfooter: \"Pied de page\"\n"},
				{name: "remove_edition of a value of several lines", block: "help",
					want: "# Chaînes de la boutique\nnav:\n  # Le lien d'accueil\n  home: Accueil\n  cart: Panier # dans l'en-tête\n  # Retirer après 2.0\n  legacy: Ancien lien\nfooter: \"Pied de page\"\n"},
				{name: "remove_edition refuses a revision that moved", block: "footer", stale: true, refused: change.CodeStale},
				{name: "remove_edition refuses an item of a sequence, named by its position", block: "steps.[0]", refused: change.CodeUnsupported,
					source: "steps:\n  - First\n  - Second\ntitle: Title\n", french: "steps:\n  - Premier\n  - Deuxième\ntitle: Titre\n"},
			}},
		{format: "arb", file: "catalog.arb", template: "catalog.arb.tmpl",
			french: "{\n  \"@@locale\": \"fr\",\n  \"greeting\": \"Bonjour {name}\",\n  \"@greeting\": {\n    \"description\": \"Shown on the home page\",\n    \"placeholders\": {\n      \"name\": {}\n    }\n  },\n  \"cart\": \"Panier\",\n  \"items\": \"{count, plural, one{# article} other{# articles}}\",\n  \"@items\": {\n    \"placeholders\": {\n      \"count\": {}\n    }\n  }\n}\n",
			cells: []removalCell{
				{name: "remove_edition removes a message and its metadata, and set_content creates the message again", block: "greeting", again: `Bonjour <x id="p1/"/>`,
					want:      "{\n  \"@@locale\": \"fr\",\n  \"cart\": \"Panier\",\n  \"items\": \"{count, plural, one{# article} other{# articles}}\",\n  \"@items\": {\n    \"placeholders\": {\n      \"count\": {}\n    }\n  }\n}\n",
					recreated: "{\n  \"@@locale\": \"fr\",\n  \"greeting\": \"Bonjour {name}\",\n  \"cart\": \"Panier\",\n  \"items\": \"{count, plural, one{# article} other{# articles}}\",\n  \"@items\": {\n    \"placeholders\": {\n      \"count\": {}\n    }\n  }\n}\n"},
				{name: "remove_edition of a plural message", block: "items",
					want: "{\n  \"@@locale\": \"fr\",\n  \"greeting\": \"Bonjour {name}\",\n  \"@greeting\": {\n    \"description\": \"Shown on the home page\",\n    \"placeholders\": {\n      \"name\": {}\n    }\n  },\n  \"cart\": \"Panier\"\n}\n"},
				{name: "remove_edition refuses a revision that moved", block: "cart", stale: true, refused: change.CodeStale},
			}},
	}
}

// removalLayout serves the documents of a directory, every one read as one
// format, each translation in a file of its own at <locale>/<name>, as a
// project's target template puts it.
type removalLayout struct{ filehome.DirLayout }

func (l removalLayout) Locate(ctx context.Context, doc string) (filehome.Doc, error) {
	d, err := l.DirLayout.Locate(ctx, doc)
	if err != nil {
		return d, err
	}
	ref := d.Ref
	d.EditionFile = func(k model.EditionKey) (filehome.EditionFile, bool) {
		rel := string(k.Locale) + "/" + ref
		return filehome.EditionFile{Ref: rel, Path: filepath.Join(l.Root, filepath.FromSlash(rel))}, true
	}
	return d, nil
}

// removalService serves the documents of dir, read as format name, with
// their translations in files of their own.
func removalService(t *testing.T, dir string, reg *registry.FormatRegistry, name registry.FormatID) *change.Service {
	t.Helper()
	home := filehome.New(removalLayout{filehome.DirLayout{Root: dir, Formats: reg, SourceLocale: "en", Format: string(name)}},
		filehome.Options{LockDir: t.TempDir()})
	return change.NewService(filehome.Formats{Registry: reg}, change.OneHome(home))
}

var french = model.EditionKey{Locale: "fr"}

// frenchRevs reads the revision of every French translation doc's blocks
// hold, by block key.
func frenchRevs(t *testing.T, svc *change.Service, doc string) map[string]string {
	t.Helper()
	page, err := svc.Read(context.Background(), change.ReadRequest{Doc: doc, Limit: 1000, Editions: []model.EditionKey{french}})
	require.NoError(t, err)
	out := map[string]string{}
	for _, b := range page.Blocks {
		if ed, ok := b.Editions["fr"]; ok {
			out[b.Ref.Block] = ed.Rev
		}
	}
	return out
}

func removeFrench(doc, block, rev string) change.Op {
	return change.Op{Kind: change.KindRemoveEdition, At: change.Ref{Doc: doc, Block: block, Edition: french}, IfMatch: rev, Body: &change.RemoveEdition{}}
}

func TestRemovalMatrix(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	for _, row := range removalMatrix() {
		input, err := os.ReadFile(filepath.Join(opsMatrixDir, "structural", row.template))
		require.NoError(t, err)
		for _, cell := range row.cells {
			t.Run(string(row.format)+"/"+cell.name, func(t *testing.T) {
				input, translation := input, row.french
				if cell.source != "" {
					input, translation = []byte(cell.source), cell.french
				}
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, row.file), input, 0o644))
				require.NoError(t, os.MkdirAll(filepath.Join(dir, "fr"), 0o755))
				frPath := filepath.Join(dir, "fr", row.file)
				require.NoError(t, os.WriteFile(frPath, []byte(translation), 0o644))
				svc := removalService(t, dir, reg, row.format)

				before := frenchRevs(t, svc, row.file)
				require.Contains(t, before, cell.block, "the French file holds a translation of %s", cell.block)
				rev := before[cell.block]
				if cell.stale {
					rev = "r:0000000000000000"
				}
				res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{removeFrench(row.file, cell.block, rev)}},
					change.Actor{Kind: change.ActorPerson, Name: "matrix"})
				require.NoError(t, err)
				out, err := os.ReadFile(frPath)
				require.NoError(t, err)
				own, err := os.ReadFile(filepath.Join(dir, row.file))
				require.NoError(t, err)
				assert.Equal(t, string(input), string(own), "the document's own file keeps its bytes")

				if cell.refused != "" {
					require.Equal(t, change.SetRefused, res.Status)
					require.NotNil(t, res.Ops[0].Error, "%+v", res.Ops[0])
					assert.Equal(t, cell.refused, res.Ops[0].Error.Code, res.Ops[0].Error.Message)
					assert.Equal(t, translation, string(out), "a refused removal writes nothing")
					return
				}
				require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
				assert.Equal(t, model.AbsentRevision, res.Ops[0].After)
				assert.Equal(t, cell.want, string(out), "the translation's file")

				after := frenchRevs(t, svc, row.file)
				assert.NotContains(t, after, cell.block, "the removed translation reads back absent")
				for block, was := range before {
					if block != cell.block {
						assert.Equal(t, was, after[block], "the translation of %s reads back as it was read", block)
					}
				}
				if cell.again == "" {
					return
				}

				text := cell.again
				res, err = svc.Apply(context.Background(), change.Set{Ops: []change.Op{{Kind: change.KindSetContent,
					At: change.Ref{Doc: row.file, Block: cell.block, Edition: french}, IfMatch: model.AbsentRevision, Body: &change.SetContent{Text: &text}}}},
					change.Actor{Kind: change.ActorPerson, Name: "matrix"})
				require.NoError(t, err)
				require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
				out, err = os.ReadFile(frPath)
				require.NoError(t, err)
				assert.Equal(t, cell.recreated, string(out), "the translation's file with the translation created again")
				again := frenchRevs(t, svc, row.file)
				assert.Equal(t, res.Ops[0].After, again[cell.block], "the translation created again reads back")
				for block, was := range before {
					if block != cell.block {
						assert.Equal(t, was, again[block], "the translation of %s reads back as it was read", block)
					}
				}
			})
		}
	}
}

// TestRemovalMatrixCoversEveryDeclaration: every built-in format that lists
// remove_edition and keeps one edition per file has a row with a cell
// proving the removal and a cell holding it to a refusal, every row is such a
// format, and every other such format with a skeleton pair refuses the
// removal unsupported and keeps both files.
func TestRemovalMatrixCoversEveryDeclaration(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	facts := filehome.Formats{Registry: reg}
	perFile := func(name registry.FormatID) (change.Description, bool) {
		f, ok := facts.Facts(string(name))
		if !ok {
			return change.Description{}, false
		}
		d := change.DescribeFormat(f)
		return d, d.Editions == change.EditionsPerFile
	}

	rows := map[registry.FormatID]bool{}
	for _, row := range removalMatrix() {
		rows[row.format] = true
		d, ok := perFile(row.format)
		require.True(t, ok, "%s has removal cells and keeps editions in one file", row.format)
		assert.NotNil(t, d.Ops.RemoveEdition, "%s has removal cells and does not list remove_edition", row.format)
		var proved, refused bool
		for _, c := range row.cells {
			if c.refused != "" {
				refused = true
			} else {
				proved = true
			}
		}
		assert.True(t, proved, "%s: no cell proves remove_edition", row.format)
		assert.True(t, refused, "%s: no cell holds remove_edition to a refusal", row.format)
	}
	for _, name := range reg.WriterNames() {
		if d, ok := perFile(name); ok && d.Ops.RemoveEdition != nil {
			assert.True(t, rows[name], "%s lists remove_edition and has no removal cells", name)
		}
	}

	for _, fx := range opsMatrix() {
		if rows[fx.format] {
			continue
		}
		if _, ok := perFile(fx.format); !ok {
			continue
		}
		t.Run("refused/"+fx.id(), func(t *testing.T) {
			doc := fx.render(t, nil)
			dir := t.TempDir()
			name := "doc" + filepath.Ext(fx.template[:len(fx.template)-len(".tmpl")])
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), doc.input, 0o644))
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "fr"), 0o755))
			// The translation's file is a copy of the document: every block
			// pairs with one that holds a translation.
			require.NoError(t, os.WriteFile(filepath.Join(dir, "fr", name), doc.input, 0o644))
			svc := removalService(t, dir, reg, fx.format)
			ctx := context.Background()

			d, err := svc.Describe(ctx, change.DescribeRequest{Format: string(fx.format)})
			require.NoError(t, err)
			assert.Nil(t, d.Ops.RemoveEdition)

			revs := frenchRevs(t, svc, name)
			require.NotEmpty(t, revs, "%s: the French file pairs with the document", fx.id())
			var block string
			for b := range revs {
				if block == "" || b < block {
					block = b
				}
			}
			res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{removeFrench(name, block, revs[block])}},
				change.Actor{Kind: change.ActorPerson, Name: "matrix"})
			require.NoError(t, err)
			require.NotNil(t, res.Ops[0].Error, "%s: remove_edition applied", fx.id())
			assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code, res.Ops[0].Error.Message)
			assert.Equal(t, string(change.KindRemoveEdition), res.Ops[0].Error.Capability)
			for _, f := range []string{name, filepath.Join("fr", name)} {
				out, err := os.ReadFile(filepath.Join(dir, f))
				require.NoError(t, err)
				assert.Equal(t, doc.input, out, "%s stays as it was", f)
			}
		})
	}
}
