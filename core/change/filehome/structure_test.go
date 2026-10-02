package filehome_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/model"
)

// presentLayout keeps each translation at <locale>/<name>, and names as
// derived the editions whose files exist, as a project's layout does.
type presentLayout struct{ targetLayout }

func (l presentLayout) Locate(ctx context.Context, doc string) (filehome.Doc, error) {
	d, err := l.targetLayout.Locate(ctx, doc)
	if err != nil {
		return d, err
	}
	entries, _ := os.ReadDir(l.Root)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, serr := os.Stat(filepath.Join(l.Root, e.Name(), filepath.Base(d.Ref))); serr == nil {
			d.Derived = append(d.Derived, model.EditionKey{Locale: model.LocaleID(e.Name())})
		}
	}
	return d, nil
}

func newPresentFixture(t *testing.T, files map[string]string, opts ...filehome.Options) *fixture {
	t.Helper()
	f := newFixture(t, files)
	o := filehome.Options{LockDir: t.TempDir()}
	if len(opts) > 0 {
		o = opts[0]
		o.LockDir = t.TempDir()
	}
	home := filehome.New(presentLayout{targetLayout{filehome.DirLayout{Root: f.dir, Formats: f.reg, SourceLocale: "en"}}}, o)
	f.svc = change.NewService(filehome.Formats{Registry: f.reg}, change.OneHome(home))
	return f
}

func insertAt(doc, after, name string, editions map[string]string) change.Op {
	ed := map[string]change.Content{}
	for k, text := range editions {
		ed[k] = change.Content{Text: &text}
	}
	return change.Op{Kind: change.KindInsertBlock, At: change.Ref{Doc: doc}, Body: &change.InsertBlock{After: after, Name: name, Editions: ed}}
}

func deleteAt(doc, key string, revs map[string]string) change.Op {
	return change.Op{Kind: change.KindDeleteBlock, At: change.Ref{Doc: doc, Block: key}, Body: &change.DeleteBlock{IfMatch: revs}}
}

func (f *fixture) apply(t *testing.T, ops ...change.Op) *change.Result {
	t.Helper()
	res, err := f.svc.Apply(context.Background(), change.Set{Ops: ops}, person)
	require.NoError(t, err)
	return res
}

func (f *fixture) block(t *testing.T, doc, key string, editions ...string) change.BlockRead {
	t.Helper()
	var ks []model.EditionKey
	for _, e := range editions {
		ks = append(ks, mustEdition(t, e))
	}
	page, err := f.svc.Read(context.Background(), change.ReadRequest{Doc: doc, Blocks: []string{key}, Editions: ks})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1, "block %s of %s", key, doc)
	return page.Blocks[0]
}

func (f *fixture) keys(t *testing.T, doc string) []string {
	t.Helper()
	page, err := f.svc.Read(context.Background(), change.ReadRequest{Doc: doc})
	require.NoError(t, err)
	var out []string
	for _, b := range page.Blocks {
		out = append(out, b.Ref.Block)
	}
	return out
}

func requireApplied(t *testing.T, res *change.Result) {
	t.Helper()
	for _, op := range res.Ops {
		require.Equal(t, change.OpApplied, op.Status, "op %d %s: %+v", op.I, op.Op, op.Error)
	}
	require.Equal(t, change.SetApplied, res.Status)
}

func TestFileHome_AddsAndRemovesKeysOfACatalog(t *testing.T) {
	tests := []struct {
		name   string
		file   string
		before string
		ops    func(f *fixture) []change.Op
		after  string
		order  []string
	}{
		{
			name: "a JSON key beside another, and one removed", file: "en.json",
			before: "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"cart\": \"Cart\",\n    \"legacy\": \"Old\"\n  }\n}\n",
			ops: func(f *fixture) []change.Op {
				return []change.Op{
					insertAt("en.json", "nav.cart", "nav.checkout", map[string]string{"en": "Checkout"}),
					deleteAt("en.json", "nav.legacy", map[string]string{"en": f.block(t, "en.json", "nav.legacy").Rev}),
				}
			},
			after: "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"cart\": \"Cart\",\n    \"checkout\": \"Checkout\"\n  }\n}\n",
			order: []string{"nav.home", "nav.cart", "nav.checkout"},
		},
		{
			name: "a YAML key added and one removed keep every comment", file: "en.yaml",
			before: "# Navigation\nnav:\n  # The home link\n  home: Home\n  # Remove after 2.0\n  legacy: Old # unused\n  cart: Cart\n",
			ops: func(f *fixture) []change.Op {
				return []change.Op{
					insertAt("en.yaml", "nav.home", "nav.checkout", map[string]string{"en": "Checkout: now"}),
					deleteAt("en.yaml", "nav.legacy", map[string]string{"en": f.block(t, "en.yaml", "nav.legacy").Rev}),
				}
			},
			after: "# Navigation\nnav:\n  # The home link\n  home: Home\n  checkout: \"Checkout: now\"\n  # Remove after 2.0\n  cart: Cart\n",
			order: []string{"nav.home", "nav.checkout", "nav.cart"},
		},
		{
			name: "an ARB message added after another's metadata, and one removed with its metadata", file: "app_en.arb",
			before: "{\n  \"@@locale\": \"en\",\n  \"greeting\": \"Hello {name}\",\n  \"@greeting\": {\n    \"placeholders\": {\"name\": {}}\n  },\n  \"legacy\": \"Old\",\n  \"@legacy\": {\n    \"description\": \"Remove after 2.0\"\n  }\n}\n",
			ops: func(f *fixture) []change.Op {
				return []change.Op{
					insertAt("app_en.arb", "greeting", "farewell", map[string]string{"en": "Goodbye {name}"}),
					deleteAt("app_en.arb", "legacy", map[string]string{"en": f.block(t, "app_en.arb", "legacy").Rev}),
				}
			},
			after: "{\n  \"@@locale\": \"en\",\n  \"greeting\": \"Hello {name}\",\n  \"@greeting\": {\n    \"placeholders\": {\"name\": {}}\n  },\n  \"farewell\": \"Goodbye {name}\"\n}\n",
			order: []string{"greeting", "farewell"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, map[string]string{tt.file: tt.before})
			res := f.apply(t, tt.ops(f)...)
			requireApplied(t, res)
			assert.Equal(t, tt.after, f.read(t, tt.file))
			assert.Equal(t, tt.order, f.keys(t, tt.file))
			added := res.Ops[0]
			assert.Equal(t, f.block(t, tt.file, added.At.Block).Rev, added.After,
				"the result carries the new block's revision as the format reads it back")
		})
	}
}

func TestFileHome_AddsAKeyToAnObjectInAnArray(t *testing.T) {
	f := newFixture(t, map[string]string{"en.json": "{\"list\": [{\"t\": \"T\"}]}"})
	res := f.apply(t, insertAt("en.json", "list[0].t", "list[0].u", map[string]string{"en": "U"}))
	requireApplied(t, res)
	assert.Equal(t, "{\"list\": [{\"t\": \"T\", \"u\": \"U\"}]}", f.read(t, "en.json"))
	assert.Equal(t, []string{"list[0].t", "list[0].u"}, f.keys(t, "en.json"))
}

func TestFileHome_ANewARBMessageReadsWithItsPlaceholders(t *testing.T) {
	f := newFixture(t, map[string]string{"app_en.arb": "{\n  \"@@locale\": \"en\",\n  \"a\": \"A\"\n}\n"})
	res := f.apply(t, insertAt("app_en.arb", "a", "items", map[string]string{"en": "{count, plural, one{# item} other{# items}}"}))
	requireApplied(t, res)
	b := f.block(t, "app_en.arb", "items")
	assert.NotEmpty(t, b.Codes, "the ICU message reads as the protected construct it is")
	assert.Equal(t, b.Rev, res.Ops[0].After)
}

func TestFileHome_StructureAndContentLandTogether(t *testing.T) {
	f := newFixture(t, map[string]string{"en.json": "{\n  \"a\": \"A\",\n  \"b\": \"B\"\n}\n"})
	a := f.block(t, "en.json", "a")
	res := f.apply(t,
		insertAt("en.json", "a", "n", map[string]string{"en": "N"}),
		setOp(a.Ref, a.Rev, "A2"))
	requireApplied(t, res)
	assert.Equal(t, "{\n  \"a\": \"A2\",\n  \"n\": \"N\",\n  \"b\": \"B\"\n}\n", f.read(t, "en.json"))
}

func TestFileHome_RefusesStructureItCannotWrite(t *testing.T) {
	t.Run("a format whose writer adds no blocks", func(t *testing.T) {
		f := newFixture(t, map[string]string{"guide.md": "# Title\n\nBody text.\n"})
		d, err := f.svc.Describe(context.Background(), change.DescribeRequest{Doc: "guide.md"})
		require.NoError(t, err)
		assert.Nil(t, d.Ops[change.KindInsertBlock])
		assert.Nil(t, d.Ops[change.KindDeleteBlock])
		res := f.apply(t, insertAt("guide.md", "", "x", map[string]string{"en": "X"}))
		assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code)
		assert.Equal(t, "# Title\n\nBody text.\n", f.read(t, "guide.md"))
	})

	t.Run("a key the document holds without a block", func(t *testing.T) {
		f := newFixture(t, map[string]string{"en.json": "{\"a\": \"A\", \"count\": 4}"})
		res := f.apply(t, insertAt("en.json", "a", "count", map[string]string{"en": "Four"}))
		assert.Equal(t, change.CodeInvalid, res.Ops[0].Error.Code)
		assert.Equal(t, "name", res.Ops[0].Error.Field)
		assert.Equal(t, "{\"a\": \"A\", \"count\": 4}", f.read(t, "en.json"))
	})

	t.Run("a new block the format reads as something else", func(t *testing.T) {
		f := newFixture(t, map[string]string{"en.yaml": "a: A\n"})
		res := f.apply(t, insertAt("en.yaml", "a", "b", map[string]string{"en": "   "}))
		assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code, "a blank value reads as no block: %+v", res.Ops[0].Error)
		assert.Equal(t, "a: A\n", f.read(t, "en.yaml"))
	})
}

func TestFileHome_APreviewShowsTheStructureAndWritesNothing(t *testing.T) {
	f := newFixture(t, map[string]string{"en.json": "{\n  \"a\": \"A\"\n}\n"})
	res, err := f.svc.Apply(context.Background(), change.Set{Mode: change.ModePreview,
		Ops: []change.Op{insertAt("en.json", "a", "b", map[string]string{"en": "B"})}}, person)
	require.NoError(t, err)
	assert.Equal(t, change.SetPreviewed, res.Status)
	require.NotEmpty(t, res.Docs)
	assert.Contains(t, res.Docs[0].Diff, "+  \"b\": \"B\"")
	assert.Equal(t, "{\n  \"a\": \"A\"\n}\n", f.read(t, "en.json"))
}

func TestFileHome_AStructuralEditIsMadeAgainWhenTheFileMoves(t *testing.T) {
	var f *fixture
	saved := false
	f = newFixture(t, map[string]string{"en.json": "{\n  \"a\": \"A\",\n  \"b\": \"B\"\n}\n"}, filehome.Options{BeforeSettle: func(string) {
		if saved {
			return
		}
		saved = true
		// Another writer changes b between this change's stage and commit.
		require.NoError(t, os.WriteFile(filepath.Join(f.dir, "en.json"), []byte("{\n  \"a\": \"A\",\n  \"b\": \"B2\"\n}\n"), 0o640))
	}})
	res := f.apply(t, insertAt("en.json", "a", "n", map[string]string{"en": "N"}))
	requireApplied(t, res)
	assert.Equal(t, "{\n  \"a\": \"A\",\n  \"n\": \"N\",\n  \"b\": \"B2\"\n}\n", f.read(t, "en.json"), "both changes land")
}

func TestFileHome_ARemovalIsStaleWhenTheBlockMovesBeforeTheCommit(t *testing.T) {
	var f *fixture
	saved := false
	f = newFixture(t, map[string]string{"en.json": "{\n  \"a\": \"A\",\n  \"b\": \"B\"\n}\n"}, filehome.Options{BeforeSettle: func(string) {
		if saved {
			return
		}
		saved = true
		require.NoError(t, os.WriteFile(filepath.Join(f.dir, "en.json"), []byte("{\n  \"a\": \"A\",\n  \"b\": \"B2\"\n}\n"), 0o640))
	}})
	res := f.apply(t, deleteAt("en.json", "b", map[string]string{"en": f.block(t, "en.json", "b").Rev}))
	require.Equal(t, change.SetRefused, res.Status)
	assert.Equal(t, change.CodeStale, res.Ops[0].Error.Code, "the block another writer changed is not removed")
	assert.Equal(t, "B2", res.Ops[0].Current.Text)
	assert.Equal(t, "{\n  \"a\": \"A\",\n  \"b\": \"B2\"\n}\n", f.read(t, "en.json"))
}

func TestFileHome_StructureReachesTheFilesOfEditions(t *testing.T) {
	const en = "{\n  \"a\": \"A\",\n  \"b\": \"B\"\n}\n"
	const de = "{\n  \"a\": \"Ah\",\n  \"b\": \"Be\"\n}\n"

	t.Run("a block removed leaves every edition's file", func(t *testing.T) {
		f := newPresentFixture(t, map[string]string{"en.json": en, "de/en.json": de})
		b := f.block(t, "en.json", "b", "de")
		res := f.apply(t, deleteAt("en.json", "b", map[string]string{"en": b.Rev}))
		assert.Equal(t, change.CodeStale, res.Ops[0].Error.Code, "the German edition is not named")
		assert.Equal(t, "if_match/de", res.Ops[0].Error.Field)
		assert.Equal(t, "Be", res.Ops[0].Current.Text)
		assert.Equal(t, de, f.read(t, "de/en.json"))

		res = f.apply(t, deleteAt("en.json", "b", map[string]string{"en": b.Rev, "de": b.Editions["de"].Rev}))
		requireApplied(t, res)
		assert.Equal(t, "{\n  \"a\": \"A\"\n}\n", f.read(t, "en.json"))
		assert.Equal(t, "{\n  \"a\": \"Ah\"\n}\n", f.read(t, "de/en.json"))
	})

	t.Run("a new block's edition goes beside the anchor's partner in its file", func(t *testing.T) {
		f := newPresentFixture(t, map[string]string{"en.json": en, "de/en.json": de})
		res := f.apply(t, insertAt("en.json", "a", "n", map[string]string{"en": "New", "de": "Neu"}))
		requireApplied(t, res)
		assert.Equal(t, "{\n  \"a\": \"A\",\n  \"n\": \"New\",\n  \"b\": \"B\"\n}\n", f.read(t, "en.json"))
		assert.Equal(t, "{\n  \"a\": \"Ah\",\n  \"n\": \"Neu\",\n  \"b\": \"Be\"\n}\n", f.read(t, "de/en.json"))
		assert.Equal(t, "Neu", f.block(t, "en.json", "n", "de").Editions["de"].Text)
	})

	t.Run("a new block's edition in a file written for the first time", func(t *testing.T) {
		f := newPresentFixture(t, map[string]string{"en.json": en})
		res := f.apply(t, insertAt("en.json", "a", "n", map[string]string{"en": "New", "fr": "Nouveau"}))
		requireApplied(t, res)
		assert.Equal(t, "{\n  \"a\": \"A\",\n  \"n\": \"Nouveau\",\n  \"b\": \"B\"\n}\n", f.read(t, "fr/en.json"),
			"the file is written from the document, the blocks with no translation falling back to the document's own text")
	})

	t.Run("an anchor with no partner in the edition's file", func(t *testing.T) {
		f := newPresentFixture(t, map[string]string{"en.json": en, "de/en.json": "{\n  \"b\": \"Be\"\n}\n"})
		res := f.apply(t, insertAt("en.json", "a", "n", map[string]string{"en": "New", "de": "Neu"}))
		require.Equal(t, change.SetRefused, res.Status)
		assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code)
		assert.Equal(t, "editions/de", res.Ops[0].Error.Field)
		assert.Equal(t, en, f.read(t, "en.json"))

		res = f.apply(t, insertAt("en.json", "a", "n", map[string]string{"en": "New"}))
		requireApplied(t, res)
		assert.Equal(t, "{\n  \"b\": \"Be\"\n}\n", f.read(t, "de/en.json"), "an edition the insert does not name stays to translate")
	})

	t.Run("a file that gives its keys the language's prefix", func(t *testing.T) {
		f := newPresentFixture(t, map[string]string{
			"app.yaml":    "en:\n  a: A\n  b: B\n",
			"de/app.yaml": "de:\n  a: Ah\n  b: Be\n",
		})
		res := f.apply(t, insertAt("app.yaml", "en.a", "en.n", map[string]string{"en": "New", "de": "Neu"}))
		requireApplied(t, res)
		assert.Equal(t, "en:\n  a: A\n  n: New\n  b: B\n", f.read(t, "app.yaml"))
		assert.Equal(t, "de:\n  a: Ah\n  n: Neu\n  b: Be\n", f.read(t, "de/app.yaml"))
	})
}
