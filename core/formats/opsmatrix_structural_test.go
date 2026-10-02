package formats_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// The structural cells of the operations matrix prove insert_block and
// delete_block for every format whose writer declares them
// (format.StructuralWriter), through the path every surface takes: the change
// service over the file home. Each cell applies a change set to a catalog and
// asserts
//
//   - the document written, byte for byte: the shell of each block added or
//     removed, and every other byte as it was;
//   - the blocks read back, in order: each new block where the change set put
//     it, holding the content given, and every other block at the revision it
//     was read with;
//   - the refusals: the code, and a document left as it was.
//
// TestStructuralMatrixCoversEveryDeclaration holds every declaration to its
// cells, and every other format to refusing both operations.

// structuralRow is one format's catalog and its cells.
type structuralRow struct {
	format registry.FormatID
	// file is the catalog's name as the file home reads it, and template
	// its content under testdata/opsmatrix/structural.
	file     string
	template string
	cells    []structuralCell
}

// structuralCell is one change set over a row's catalog.
type structuralCell struct {
	name string
	// ops builds the change set; rev gives the revisions of a block's
	// editions as a read reports them.
	ops func(rev func(key string) map[string]string) []change.Op
	// want is the document written; order the blocks read back.
	want  string
	order []string
	// added maps each new block to the content it was given.
	added map[string]string
	// refused is the code the first operation is refused with; the
	// document then stays as it was.
	refused change.Code
}

// op names the structural operation a cell proves.
func (c structuralCell) op() string {
	for _, k := range []change.Kind{change.KindInsertBlock, change.KindDeleteBlock} {
		if len(c.name) >= len(k) && c.name[:len(k)] == string(k) {
			return string(k)
		}
	}
	return ""
}

func ins(doc, after, name, text string) change.Op {
	return change.Op{Kind: change.KindInsertBlock, At: change.Ref{Doc: doc},
		Body: &change.InsertBlock{After: after, Name: name, Editions: map[string]change.Content{"en": {Text: &text}}}}
}

func insBefore(doc, before, name, text string) change.Op {
	op := ins(doc, "", name, text)
	op.Body.(*change.InsertBlock).Before = before
	return op
}

func del(doc, key string, revs map[string]string) change.Op {
	return change.Op{Kind: change.KindDeleteBlock, At: change.Ref{Doc: doc, Block: key}, Body: &change.DeleteBlock{IfMatch: revs}}
}

var staleRev = map[string]string{"en": "r:0000000000000000"}

func structuralMatrix() []structuralRow {
	const json = "catalog.json"
	const yml = "catalog.yaml"
	const arb = "catalog.arb"
	return []structuralRow{
		{format: "json", file: json, template: "catalog.json.tmpl", cells: []structuralCell{
			{
				name: "insert_block after a key, in its object",
				ops: func(func(string) map[string]string) []change.Op {
					return []change.Op{ins(json, "nav.home", "nav.checkout", "Checkout")}
				},
				want:  "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"checkout\": \"Checkout\",\n    \"cart\": \"Cart\",\n    \"legacy\": \"Old link\"\n  },\n  \"title\": \"Welcome to the {shop}\",\n  \"list\": [\"kept\", \"as written\"],\n  \"count\": 42\n}\n",
				order: []string{"nav.home", "nav.checkout", "nav.cart", "nav.legacy", "title", "list[0]", "list[1]"},
				added: map[string]string{"nav.checkout": "Checkout"},
			},
			{
				name: "insert_block before the first key of an object",
				ops: func(func(string) map[string]string) []change.Op {
					return []change.Op{insBefore(json, "nav.home", "nav.top", "Top")}
				},
				want:  "{\n  \"nav\": {\n    \"top\": \"Top\",\n    \"home\": \"Home\",\n    \"cart\": \"Cart\",\n    \"legacy\": \"Old link\"\n  },\n  \"title\": \"Welcome to the {shop}\",\n  \"list\": [\"kept\", \"as written\"],\n  \"count\": 42\n}\n",
				order: []string{"nav.top", "nav.home", "nav.cart", "nav.legacy", "title", "list[0]", "list[1]"},
				added: map[string]string{"nav.top": "Top"},
			},
			{
				name: "insert_block after the last key of an object",
				ops: func(func(string) map[string]string) []change.Op {
					return []change.Op{ins(json, "nav.legacy", "nav.help", "Help")}
				},
				want:  "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"cart\": \"Cart\",\n    \"legacy\": \"Old link\",\n    \"help\": \"Help\"\n  },\n  \"title\": \"Welcome to the {shop}\",\n  \"list\": [\"kept\", \"as written\"],\n  \"count\": 42\n}\n",
				order: []string{"nav.home", "nav.cart", "nav.legacy", "nav.help", "title", "list[0]", "list[1]"},
				added: map[string]string{"nav.help": "Help"},
			},
			{
				name: "insert_block with no anchor goes last, escaped as the writer escapes a value",
				ops: func(func(string) map[string]string) []change.Op {
					return []change.Op{ins(json, "", "footer", "Terms/Privacy \"2026\"")}
				},
				want:  "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"cart\": \"Cart\",\n    \"legacy\": \"Old link\"\n  },\n  \"title\": \"Welcome to the {shop}\",\n  \"list\": [\"kept\", \"as written\"],\n  \"count\": 42,\n  \"footer\": \"Terms\\/Privacy \\\"2026\\\"\"\n}\n",
				order: []string{"nav.home", "nav.cart", "nav.legacy", "title", "list[0]", "list[1]", "footer"},
				added: map[string]string{"footer": "Terms/Privacy \"2026\""},
			},
			{
				name: "insert_block in order, each beside the one before",
				ops: func(func(string) map[string]string) []change.Op {
					return []change.Op{ins(json, "title", "a", "A"), ins(json, "a", "b", "B")}
				},
				want:  "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"cart\": \"Cart\",\n    \"legacy\": \"Old link\"\n  },\n  \"title\": \"Welcome to the {shop}\",\n  \"a\": \"A\",\n  \"b\": \"B\",\n  \"list\": [\"kept\", \"as written\"],\n  \"count\": 42\n}\n",
				order: []string{"nav.home", "nav.cart", "nav.legacy", "title", "a", "b", "list[0]", "list[1]"},
				added: map[string]string{"a": "A", "b": "B"},
			},
			{
				name: "delete_block of a key between two others",
				ops: func(rev func(string) map[string]string) []change.Op {
					return []change.Op{del(json, "nav.cart", rev("nav.cart"))}
				},
				want:  "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"legacy\": \"Old link\"\n  },\n  \"title\": \"Welcome to the {shop}\",\n  \"list\": [\"kept\", \"as written\"],\n  \"count\": 42\n}\n",
				order: []string{"nav.home", "nav.legacy", "title", "list[0]", "list[1]"},
			},
			{
				name: "delete_block of the last key of an object",
				ops: func(rev func(string) map[string]string) []change.Op {
					return []change.Op{del(json, "nav.legacy", rev("nav.legacy"))}
				},
				want:  "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"cart\": \"Cart\"\n  },\n  \"title\": \"Welcome to the {shop}\",\n  \"list\": [\"kept\", \"as written\"],\n  \"count\": 42\n}\n",
				order: []string{"nav.home", "nav.cart", "title", "list[0]", "list[1]"},
			},
			{
				name: "delete_block of a top-level key",
				ops: func(rev func(string) map[string]string) []change.Op {
					return []change.Op{del(json, "title", rev("title"))}
				},
				want:  "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"cart\": \"Cart\",\n    \"legacy\": \"Old link\"\n  },\n  \"list\": [\"kept\", \"as written\"],\n  \"count\": 42\n}\n",
				order: []string{"nav.home", "nav.cart", "nav.legacy", "list[0]", "list[1]"},
			},
			{
				name: "insert_block refuses a key a block holds",
				ops: func(func(string) map[string]string) []change.Op {
					return []change.Op{ins(json, "nav.home", "nav.cart", "Again")}
				},
				refused: change.CodeStale,
			},
			{
				name: "insert_block refuses a key the document holds a number at",
				ops: func(func(string) map[string]string) []change.Op {
					return []change.Op{ins(json, "title", "count", "Count")}
				},
				refused: change.CodeInvalid,
			},
			{
				name: "insert_block refuses an anchor no block answers to",
				ops: func(func(string) map[string]string) []change.Op {
					return []change.Op{ins(json, "nav.nope", "nav.x", "X")}
				},
				refused: change.CodeNotFound,
			},
			{
				name:    "delete_block refuses a revision that moved",
				ops:     func(func(string) map[string]string) []change.Op { return []change.Op{del(json, "nav.cart", staleRev)} },
				refused: change.CodeStale,
			},
		}},
		{format: "yaml", file: yml, template: "catalog.yaml.tmpl", cells: []structuralCell{
			{
				name: "insert_block after a key with an inline comment, the next key's comment staying with it",
				ops: func(func(string) map[string]string) []change.Op {
					return []change.Op{ins(yml, "nav.cart", "nav.checkout", "Checkout")}
				},
				want:  "# Store strings\nnav:\n  # The home link\n  home: Home\n  cart: Cart # shown in the header\n  checkout: Checkout\n  # Remove after 2.0\n  legacy: Old link\nhelp: |\n  Read the guide\n  before you order.\nfooter: \"Footer\"\n",
				order: []string{"nav.home", "nav.cart", "nav.checkout", "nav.legacy", "help", "footer"},
				added: map[string]string{"nav.checkout": "Checkout"},
			},
			{
				name: "insert_block before a key, comment lines staying where they are",
				ops: func(func(string) map[string]string) []change.Op {
					return []change.Op{insBefore(yml, "nav.home", "nav.top", "Top")}
				},
				want:  "# Store strings\nnav:\n  # The home link\n  top: Top\n  home: Home\n  cart: Cart # shown in the header\n  # Remove after 2.0\n  legacy: Old link\nhelp: |\n  Read the guide\n  before you order.\nfooter: \"Footer\"\n",
				order: []string{"nav.top", "nav.home", "nav.cart", "nav.legacy", "help", "footer"},
				added: map[string]string{"nav.top": "Top"},
			},
			{
				name:  "insert_block after a value of several lines",
				ops:   func(func(string) map[string]string) []change.Op { return []change.Op{ins(yml, "help", "faq", "FAQ")} },
				want:  "# Store strings\nnav:\n  # The home link\n  home: Home\n  cart: Cart # shown in the header\n  # Remove after 2.0\n  legacy: Old link\nhelp: |\n  Read the guide\n  before you order.\nfaq: FAQ\nfooter: \"Footer\"\n",
				order: []string{"nav.home", "nav.cart", "nav.legacy", "help", "faq", "footer"},
				added: map[string]string{"faq": "FAQ"},
			},
			{
				name: "insert_block with no anchor goes last, quoted where a plain scalar would not read back",
				ops: func(func(string) map[string]string) []change.Op {
					return []change.Op{ins(yml, "", "extra", "Yes: really")}
				},
				want:  "# Store strings\nnav:\n  # The home link\n  home: Home\n  cart: Cart # shown in the header\n  # Remove after 2.0\n  legacy: Old link\nhelp: |\n  Read the guide\n  before you order.\nfooter: \"Footer\"\nextra: \"Yes: really\"\n",
				order: []string{"nav.home", "nav.cart", "nav.legacy", "help", "footer", "extra"},
				added: map[string]string{"extra": "Yes: really"},
			},
			{
				name: "delete_block keeps the comment above the key",
				ops: func(rev func(string) map[string]string) []change.Op {
					return []change.Op{del(yml, "nav.legacy", rev("nav.legacy"))}
				},
				want:  "# Store strings\nnav:\n  # The home link\n  home: Home\n  cart: Cart # shown in the header\n  # Remove after 2.0\nhelp: |\n  Read the guide\n  before you order.\nfooter: \"Footer\"\n",
				order: []string{"nav.home", "nav.cart", "help", "footer"},
			},
			{
				name: "delete_block takes the comment on the key's own line",
				ops: func(rev func(string) map[string]string) []change.Op {
					return []change.Op{del(yml, "nav.cart", rev("nav.cart"))}
				},
				want:  "# Store strings\nnav:\n  # The home link\n  home: Home\n  # Remove after 2.0\n  legacy: Old link\nhelp: |\n  Read the guide\n  before you order.\nfooter: \"Footer\"\n",
				order: []string{"nav.home", "nav.legacy", "help", "footer"},
			},
			{
				name: "delete_block of a value of several lines",
				ops: func(rev func(string) map[string]string) []change.Op {
					return []change.Op{del(yml, "help", rev("help"))}
				},
				want:  "# Store strings\nnav:\n  # The home link\n  home: Home\n  cart: Cart # shown in the header\n  # Remove after 2.0\n  legacy: Old link\nfooter: \"Footer\"\n",
				order: []string{"nav.home", "nav.cart", "nav.legacy", "footer"},
			},
			{
				name: "insert_block refuses a key a block holds",
				ops: func(func(string) map[string]string) []change.Op {
					return []change.Op{ins(yml, "help", "footer", "Again")}
				},
				refused: change.CodeStale,
			},
			{
				name:    "insert_block refuses a key a mapping holds",
				ops:     func(func(string) map[string]string) []change.Op { return []change.Op{ins(yml, "help", "nav", "Nav")} },
				refused: change.CodeInvalid,
			},
			{
				name:    "delete_block refuses a revision that moved",
				ops:     func(func(string) map[string]string) []change.Op { return []change.Op{del(yml, "help", staleRev)} },
				refused: change.CodeStale,
			},
		}},
		{format: "arb", file: arb, template: "catalog.arb.tmpl", cells: []structuralCell{
			{
				name: "insert_block after a message goes after its metadata",
				ops: func(func(string) map[string]string) []change.Op {
					return []change.Op{ins(arb, "greeting", "farewell", "Goodbye {name}")}
				},
				want:  "{\n  \"@@locale\": \"en\",\n  \"greeting\": \"Hello {name}\",\n  \"@greeting\": {\n    \"description\": \"Shown on the home page\",\n    \"placeholders\": {\n      \"name\": {}\n    }\n  },\n  \"farewell\": \"Goodbye {name}\",\n  \"cart\": \"Cart\",\n  \"items\": \"{count, plural, one{# item} other{# items}}\",\n  \"@items\": {\n    \"placeholders\": {\n      \"count\": {}\n    }\n  }\n}\n",
				order: []string{"greeting", "farewell", "cart", "items"},
				added: map[string]string{"farewell": "Goodbye {name}"},
			},
			{
				name: "insert_block before a message",
				ops: func(func(string) map[string]string) []change.Op {
					return []change.Op{insBefore(arb, "cart", "checkout", "Checkout")}
				},
				want:  "{\n  \"@@locale\": \"en\",\n  \"greeting\": \"Hello {name}\",\n  \"@greeting\": {\n    \"description\": \"Shown on the home page\",\n    \"placeholders\": {\n      \"name\": {}\n    }\n  },\n  \"checkout\": \"Checkout\",\n  \"cart\": \"Cart\",\n  \"items\": \"{count, plural, one{# item} other{# items}}\",\n  \"@items\": {\n    \"placeholders\": {\n      \"count\": {}\n    }\n  }\n}\n",
				order: []string{"greeting", "checkout", "cart", "items"},
				added: map[string]string{"checkout": "Checkout"},
			},
			{
				name: "insert_block after the last message goes after its metadata, last",
				ops: func(func(string) map[string]string) []change.Op {
					return []change.Op{ins(arb, "items", "total", "Total: {amount}")}
				},
				want:  "{\n  \"@@locale\": \"en\",\n  \"greeting\": \"Hello {name}\",\n  \"@greeting\": {\n    \"description\": \"Shown on the home page\",\n    \"placeholders\": {\n      \"name\": {}\n    }\n  },\n  \"cart\": \"Cart\",\n  \"items\": \"{count, plural, one{# item} other{# items}}\",\n  \"@items\": {\n    \"placeholders\": {\n      \"count\": {}\n    }\n  },\n  \"total\": \"Total: {amount}\"\n}\n",
				order: []string{"greeting", "cart", "items", "total"},
				added: map[string]string{"total": "Total: {amount}"},
			},
			{
				name: "delete_block removes a message and its metadata",
				ops: func(rev func(string) map[string]string) []change.Op {
					return []change.Op{del(arb, "greeting", rev("greeting"))}
				},
				want:  "{\n  \"@@locale\": \"en\",\n  \"cart\": \"Cart\",\n  \"items\": \"{count, plural, one{# item} other{# items}}\",\n  \"@items\": {\n    \"placeholders\": {\n      \"count\": {}\n    }\n  }\n}\n",
				order: []string{"cart", "items"},
			},
			{
				name: "delete_block removes the last message and its metadata",
				ops: func(rev func(string) map[string]string) []change.Op {
					return []change.Op{del(arb, "items", rev("items"))}
				},
				want:  "{\n  \"@@locale\": \"en\",\n  \"greeting\": \"Hello {name}\",\n  \"@greeting\": {\n    \"description\": \"Shown on the home page\",\n    \"placeholders\": {\n      \"name\": {}\n    }\n  },\n  \"cart\": \"Cart\"\n}\n",
				order: []string{"greeting", "cart"},
			},
			{
				name:    "insert_block refuses an id that marks metadata",
				ops:     func(func(string) map[string]string) []change.Op { return []change.Op{ins(arb, "cart", "@cart", "x")} },
				refused: change.CodeUnsupported,
			},
			{
				name:    "insert_block refuses a message id a block holds",
				ops:     func(func(string) map[string]string) []change.Op { return []change.Op{ins(arb, "cart", "items", "x")} },
				refused: change.CodeStale,
			},
			{
				name:    "delete_block refuses a revision that moved",
				ops:     func(func(string) map[string]string) []change.Op { return []change.Op{del(arb, "cart", staleRev)} },
				refused: change.CodeStale,
			},
		}},
	}
}

// structuralService serves the documents of dir through the file home, every
// one read as format.
func structuralService(t *testing.T, dir string, reg *registry.FormatRegistry, name registry.FormatID) *change.Service {
	t.Helper()
	home := filehome.New(filehome.DirLayout{Root: dir, Formats: reg, SourceLocale: "en", Format: string(name)},
		filehome.Options{LockDir: t.TempDir()})
	return change.NewService(filehome.Formats{Registry: reg}, change.OneHome(home))
}

// readAll reads every block of doc, in order.
func readAllBlocks(t *testing.T, svc *change.Service, doc string) []change.BlockRead {
	t.Helper()
	page, err := svc.Read(context.Background(), change.ReadRequest{Doc: doc, Limit: 1000})
	require.NoError(t, err)
	return page.Blocks
}

func TestStructuralMatrix(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	for _, row := range structuralMatrix() {
		input, err := os.ReadFile(filepath.Join(opsMatrixDir, "structural", row.template))
		require.NoError(t, err)
		for _, cell := range row.cells {
			t.Run(string(row.format)+"/"+cell.name, func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, row.file)
				require.NoError(t, os.WriteFile(path, input, 0o644))
				svc := structuralService(t, dir, reg, row.format)

				before := readAllBlocks(t, svc, row.file)
				revs := map[string]string{}
				for _, b := range before {
					revs[b.Ref.Block] = b.Rev
				}
				rev := func(key string) map[string]string {
					require.Contains(t, revs, key, "the catalog has a block %s", key)
					return map[string]string{"en": revs[key]}
				}
				res, err := svc.Apply(context.Background(), change.Set{Ops: cell.ops(rev)}, change.Actor{Kind: change.ActorPerson, Name: "matrix"})
				require.NoError(t, err)
				out, err := os.ReadFile(path)
				require.NoError(t, err)

				if cell.refused != "" {
					require.Equal(t, change.SetRefused, res.Status)
					require.NotNil(t, res.Ops[0].Error, "%+v", res.Ops[0])
					assert.Equal(t, cell.refused, res.Ops[0].Error.Code, res.Ops[0].Error.Message)
					assert.Equal(t, string(input), string(out), "a refused change set writes nothing")
					return
				}
				require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
				assert.Equal(t, cell.want, string(out), "the written bytes")

				// The new blocks as the format's reader reads the bytes, codes
				// with their native data, so each compares by the value written.
				native := map[string]string{}
				for _, b := range (opsFixture{format: row.format}).readEditable(t, out, "") {
					native[b.Name] = model.RenderRunsWithData(b.Source)
				}
				after := readAllBlocks(t, svc, row.file)
				var order []string
				for _, b := range after {
					order = append(order, b.Ref.Block)
					if text, ok := cell.added[b.Ref.Block]; ok {
						assert.Equal(t, text, native[b.Ref.Block], "new block %s reads back with its content", b.Ref.Block)
						continue
					}
					if was, ok := revs[b.Ref.Block]; ok {
						assert.Equal(t, was, b.Rev, "block %s reads back as it was read", b.Ref.Block)
					}
				}
				assert.Equal(t, cell.order, order, "the blocks read back")
				for i, op := range res.Ops {
					if op.Op != change.KindInsertBlock {
						continue
					}
					assert.Equal(t, after[slices.IndexFunc(after, func(b change.BlockRead) bool { return b.Ref.Block == op.At.Block })].Rev, op.After,
						"operation %d reports the new block's revision as a read reports it", i)
				}
			})
		}
	}
}

// TestStructuralMatrixCoversEveryDeclaration: every built-in writer that
// declares a structural operation has a row with a cell proving each
// operation it declares and a cell refusing it, every row is such a format,
// and every other format with a skeleton pair refuses both operations
// unsupported and keeps its document.
func TestStructuralMatrixCoversEveryDeclaration(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	declared := map[registry.FormatID][]string{}
	for _, name := range reg.WriterNames() {
		w, err := reg.NewWriter(name)
		require.NoError(t, err)
		if ops := format.StructuralOps(w); len(ops) > 0 {
			declared[name] = ops
		}
	}
	rows := map[registry.FormatID]bool{}
	for _, row := range structuralMatrix() {
		rows[row.format] = true
		require.Contains(t, declared, row.format, "%s has structural cells and declares no structural operation", row.format)
		proved, refused := map[string]bool{}, map[string]bool{}
		for _, c := range row.cells {
			require.NotEmpty(t, c.op(), "%s: cell %q names no structural operation", row.format, c.name)
			if c.refused != "" {
				refused[c.op()] = true
			} else {
				proved[c.op()] = true
			}
		}
		for _, op := range declared[row.format] {
			assert.True(t, proved[op], "%s declares %s and no cell proves it", row.format, op)
			assert.True(t, refused[op], "%s declares %s and no cell holds it to a refusal", row.format, op)
		}
	}
	for name := range declared {
		assert.True(t, rows[name], "%s declares structural operations and has no structural cells", name)
	}

	for _, fx := range opsMatrix() {
		if rows[fx.format] {
			continue
		}
		t.Run("refused/"+fx.id(), func(t *testing.T) {
			doc := fx.render(t, nil)
			dir := t.TempDir()
			name := "doc" + filepath.Ext(fx.template[:len(fx.template)-len(".tmpl")])
			path := filepath.Join(dir, name)
			require.NoError(t, os.WriteFile(path, doc.input, 0o644))
			svc := structuralService(t, dir, reg, fx.format)
			ctx := context.Background()

			d, err := svc.Describe(ctx, change.DescribeRequest{Format: string(fx.format)})
			require.NoError(t, err)
			assert.Nil(t, d.Ops[change.KindInsertBlock])
			assert.Nil(t, d.Ops[change.KindDeleteBlock])

			blocks := readAllBlocks(t, svc, name)
			require.NotEmpty(t, blocks)
			ops := []change.Op{
				ins(name, blocks[0].Ref.Block, "matrix.new", "New"),
				del(name, blocks[0].Ref.Block, map[string]string{"en": blocks[0].Rev}),
			}
			for _, op := range ops {
				res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{op}}, change.Actor{Kind: change.ActorPerson, Name: "matrix"})
				require.NoError(t, err)
				require.NotNil(t, res.Ops[0].Error, "%s: %s applied", fx.id(), op.Kind)
				assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code, res.Ops[0].Error.Message)
				assert.Equal(t, string(op.Kind), res.Ops[0].Error.Capability)
			}
			out, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, doc.input, out, "the document stays as it was")
		})
	}
}
