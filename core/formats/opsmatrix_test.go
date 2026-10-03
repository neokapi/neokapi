package formats_test

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The operations matrix proves, for every built-in format whose reader and
// writer share a skeleton, which operations of the edit contract reach the
// format's bytes (docs/internals/edit-model.md, section 2.7). Each cell is one
// (fixture, operation) pair: the fixture is read, the operation is applied to
// each block through change.ApplyBlock, as the change service applies it, the
// document is written through the format's writer, and then
//
//   - the edited blocks read back with exactly the edited content, codes
//     included, and every other block reads back as it was read;
//   - every byte the operation did not touch is in the output unchanged;
//   - the blocks the operation must refuse are refused, and their bytes stay.
//
// Fixtures carry inline codes, attributes, plurals and selects where the format
// has them, and a target edition in every format that holds one. A fixture is a
// template under testdata/opsmatrix: a word written «like this» is content of
// an edition that the operation must change, and the same word written plainly
// (in a key, an attribute, a comment, code data, or a block every operation
// is refused on) must come through byte for byte. A word written
// «like this!op@edition» is changed by every operation but the one named,
// which the fixture declares refused on its block.
//
// An operation is added as one more matrixOp with a driver in opsFixture.run
// and a rule for the document it produces in matrixOp.want. A cell a format
// cannot carry is declared in the fixture's refused map with the reason, and
// the matrix then asserts the refusal.

// editionRole names which edition of a block an operation addresses.
type editionRole string

const (
	sourceEdition editionRole = "source"
	targetEdition editionRole = "target"
)

// substitution is the word replace_text rewrites in an edition.
type substitution struct{ from, to string }

var (
	// sourceWord and targetWord are the words the operations rewrite, one per
	// edition so a source edit and a target edit are told apart in the bytes.
	// Neither contains a character any format escapes.
	sourceWord = substitution{from: "utilize", to: "use"}
	targetWord = substitution{from: "employons", to: "utilisons"}
)

// respelling is a byte sequence a writer spells its own way on every write,
// edited or not.
type respelling struct{ from, to, reason string }

// opsFixture is one row of the matrix: a format and its template.
type opsFixture struct {
	format registry.FormatID
	// name tells two rows of one format apart; empty for the format's main row.
	name string
	// template is the path under testdata/opsmatrix: a file for a text
	// document, or a directory of members for a ZIP container. Every template
	// path ends in .tmpl so no fixture walk mistakes it for a document.
	template string
	// target is the locale of the target edition the document holds, for the
	// formats that keep one beside the source. Empty means one edition per file.
	target model.LocaleID
	// config configures reader and writer.
	config map[string]any
	// normalized names the ZIP members whose bytes the writer regenerates on
	// every write, with the reason; the comparison for them is the read back
	// alone.
	normalized map[string]string
	// respelled lists what the writer spells its own way on every write, with
	// the reason. The document a cell must produce is the input with each one
	// respelled, so a declaration the writer stops needing fails the cell.
	respelled []respelling
	// refuse names, per operation key, the blocks (by id) the operation must
	// be refused on, with the reason.
	refuse map[string]map[string]string
	// refused names, per operation key, the cells this fixture refuses whole.
	refused map[string]refusal
}

// refusal is a cell a fixture refuses whole, with the reason. err is the error
// the writer refuses the document with, matched with errors.Is, and the writer
// writes nothing; a nil err means the operation applies to no block and the
// document is written unchanged.
type refusal struct {
	reason string
	err    error
}

// id names the row in subtest names and messages.
func (fx opsFixture) id() string {
	if fx.name == "" {
		return string(fx.format)
	}
	return string(fx.format) + "/" + fx.name
}

// matrixOp is one operation of the edit contract, as the matrix drives it.
type matrixOp struct {
	// name is the contract operation (docs/internals/edit-model.md, section 2.2).
	name    string
	edition editionRole
	sub     substitution
}

// key names the operation in a fixture's refuse and refused maps.
func (op matrixOp) key() string { return op.name + "@" + string(op.edition) }

// want renders the document op must produce from a template.
func (op matrixOp) want(tmpl string) string {
	return renderTemplate(tmpl, &op.sub, op.key())
}

// matrixOps is every operation the matrix drives, each a word substitution:
// set_content with the block's edit text and the word replaced, as `kapi
// apply` sends a block `kapi inspect` showed, and replace_text with the
// offsets of each match, as ksed sends a substitution (replaceEdits).
var matrixOps = []matrixOp{
	{name: "set_content", edition: sourceEdition, sub: sourceWord},
	{name: "replace_text", edition: sourceEdition, sub: sourceWord},
	{name: "replace_text", edition: targetEdition, sub: targetWord},
}

// opsMatrix is the table. Keep it exhaustive: TestOperationsMatrixCoversEverySkeletonPair
// fails when a format gains a skeleton pair without a row here.
func opsMatrix() []opsFixture {
	return []opsFixture{
		{format: "androidxml", template: "androidxml.xml.tmpl"},
		{format: "applestrings", template: "applestrings.strings.tmpl"},
		// The plural message reads as one opaque placeholder, so the words in
		// its branches are out of the edit's reach and stay as written.
		{format: "arb", template: "arb.arb.tmpl"},
		{format: "asciidoc", template: "asciidoc.adoc.tmpl"},
		{format: "csv", template: "csv.csv.tmpl"},
		{format: "designtokens", template: "designtokens.tokens.json.tmpl"},
		{format: "epub", template: "epub.tmpl"},
		{format: "html", template: "html.html.tmpl"},
		{format: "i18next", template: "i18next.json.tmpl"},
		// Strings in an array are not extracted by default. An edited value
		// is encoded under escapeForwardSlashes (on by default), so the edited
		// value here already spells its slash as \/.
		{format: "json", template: "json.json.tmpl"},
		{format: "markdown", template: "markdown.md.tmpl"},
		{format: "mdx", template: "mdx.mdx.tmpl"},
		// Each plural and select branch is a block of its own; the prose
		// framing a select is read as non-translatable, so its word stays.
		{format: "messageformat", template: "messageformat.mf.tmpl"},
		{format: "odf", template: "odf.tmpl"},
		{
			format: "openxml", template: "openxml.tmpl",
			normalized: map[string]string{
				"word/document.xml": "the writer re-serializes the document part from the skeleton on every write " +
					"(see containerMemberNormalised in sourceedit_test.go)",
			},
		},
		{
			format: "plaintext", template: "plaintext.txt.tmpl",
			refuse: map[string]map[string]string{
				"set_content@source": {
					// The line holds the characters <x id="1"/> as text. Edit text
					// spells a code the same way, so the edit parses them as a
					// code the block does not have, and the inline-code guard
					// refuses it. replace_text changes the line's text and
					// leaves the characters as they are.
					"tu5": "edit text cannot tell literal <x id=\"1\"/> characters from a code",
				},
			},
		},
		{format: "po", template: "po.po.tmpl", target: "fr"},
		{format: "properties", template: "properties.properties.tmpl"},
		{format: "resx", template: "resx.resx.tmpl"},
		{format: "srt", template: "srt.srt.tmpl"},
		{
			format: "tmx", template: "tmx.tmx.tmpl", target: "fr",
			respelled: []respelling{{
				from: `<bpt i="1">`, to: `<bpt i="1" x="1">`,
				reason: "the writer renders every <seg> from its runs and writes an x on each <bpt>, taken from its i when the source gave none",
			}},
		},
		{format: "ts", template: "ts.ts.tmpl", target: "fr", respelled: tsPrologue},
		// The word is in both forms; replace_text edits each form by its path.
		{format: "ts", name: "numerus", template: "ts-numerus.ts.tmpl", target: "fr", respelled: tsPrologue},
		{format: "tsv", template: "tsv.tsv.tmpl"},
		{format: "vtt", template: "vtt.vtt.tmpl"},
		{format: "xcstrings", template: "xcstrings.xcstrings.tmpl", target: "fr"},
		{
			format: "xliff", template: "xliff.xlf.tmpl", target: "fr",
			respelled: []respelling{
				{
					from: "&lt;b&gt;</bpt>", to: "&lt;b></bpt>",
					reason: "the writer renders a code's content from the parsed tree, which keeps no record of a > written as &gt;",
				},
				{
					from: "&lt;/b&gt;</ept>", to: "&lt;/b></ept>",
					reason: "the writer renders a code's content from the parsed tree, which keeps no record of a > written as &gt;",
				},
			},
		},
		{format: "xliff2", template: "xliff2.xlf.tmpl", target: "fr"},
		// The applier rebases the segment overlays over an edit, so the unit
		// still divides into its segments.
		{format: "xliff2", name: "segments", template: "xliff2-segments.xlf.tmpl", target: "fr"},
		{format: "xml", template: "xml.xml.tmpl"},
		{format: "yaml", template: "yaml.yaml.tmpl"},
	}
}

// tsPrologue is how the Qt writer spells the document prologue: the XML
// declaration and DOCTYPE as Okapi's TsFilter writes them (normalizeTSPrologue).
var tsPrologue = []respelling{
	{from: `encoding="utf-8"`, to: `encoding="UTF-8"`, reason: "the writer writes the encoding name in upper case"},
	{from: "<!DOCTYPE TS>", to: "<!DOCTYPE TS []>", reason: "the writer writes the DOCTYPE with an empty internal subset"},
}

const opsMatrixDir = "testdata/opsmatrix"

// opsDocument is a fixture rendered for one operation: the input the format
// reads and the document the operation must produce.
type opsDocument struct {
	input []byte
	want  []byte
	// members holds the rendered want per ZIP member, nil for a text format.
	members map[string][]byte
}

// renderTemplate replaces every «word» marker: the substitution's own word
// with its replacement when sub is set, every other marked word with itself.
// A «word!key» marker is replaced unless key is the operation's key.
func renderTemplate(tmpl string, sub *substitution, key string) string {
	var b strings.Builder
	for {
		before, marked, ok := strings.Cut(tmpl, "«")
		if !ok {
			b.WriteString(tmpl)
			return b.String()
		}
		word, after, ok := strings.Cut(marked, "»")
		if !ok {
			b.WriteString(tmpl)
			return b.String()
		}
		b.WriteString(before)
		word, except, _ := strings.Cut(word, "!")
		if sub != nil && word == sub.from && except != key {
			b.WriteString(sub.to)
		} else {
			b.WriteString(word)
		}
		tmpl = after
	}
}

// render builds the fixture's input and the document op must produce, with the
// fixture's respellings applied. A nil op renders the untouched document.
func (fx opsFixture) render(t *testing.T, op *matrixOp) opsDocument {
	t.Helper()
	want := func(tmpl string) string {
		out := renderTemplate(tmpl, nil, "")
		if op != nil {
			out = op.want(tmpl)
		}
		for _, r := range fx.respelled {
			out = strings.ReplaceAll(out, r.from, r.to)
		}
		return out
	}
	root := filepath.Join(opsMatrixDir, fx.template)
	info, err := os.Stat(root)
	require.NoError(t, err, "fixture %s", root)
	if !info.IsDir() {
		raw, err := os.ReadFile(root)
		require.NoError(t, err)
		return opsDocument{input: []byte(renderTemplate(string(raw), nil, "")), want: []byte(want(string(raw)))}
	}
	in, members := map[string][]byte{}, map[string][]byte{}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return rerr
		}
		name := strings.TrimSuffix(filepath.ToSlash(rel), ".tmpl")
		raw, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		in[name] = []byte(renderTemplate(string(raw), nil, ""))
		members[name] = []byte(want(string(raw)))
		return nil
	})
	require.NoError(t, err)
	return opsDocument{input: buildZip(t, in), members: members}
}

// buildZip packs members into a ZIP container in a fixed order: an EPUB or ODF
// mimetype member first and stored, as both specifications require, then the
// rest by name.
func buildZip(t *testing.T, members map[string][]byte) []byte {
	t.Helper()
	names := make([]string, 0, len(members))
	for name := range members {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if (names[i] == "mimetype") != (names[j] == "mimetype") {
			return names[i] == "mimetype"
		}
		return names[i] < names[j]
	})
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range names {
		method := zip.Deflate
		if name == "mimetype" {
			method = zip.Store
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method})
		require.NoError(t, err)
		_, err = w.Write(members[name])
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// newPair builds the fixture's reader and writer, configured.
func (fx opsFixture) newPair(t *testing.T) (format.DataFormatReader, format.DataFormatWriter) {
	t.Helper()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	reader, err := reg.NewReader(fx.format)
	require.NoError(t, err, "no reader for %s", fx.format)
	writer, err := reg.NewWriter(fx.format)
	require.NoError(t, err, "no writer for %s", fx.format)
	if len(fx.config) > 0 {
		require.NoError(t, schema.ApplyConfig(fx.config, reader.Config()), "configure %s reader", fx.format)
		if wc, ok := writer.(format.WriterConfigurable); ok {
			require.NoError(t, schema.ApplyConfig(fx.config, wc.WriterConfig()), "configure %s writer", fx.format)
		}
	}
	return reader, writer
}

// readEditable reads data as `kapi inspect` does: the reader wired to the
// skeleton store its writer would replay, so the blocks are the ones an edit
// addresses. locale is the document's target locale, empty for none.
func (fx opsFixture) readEditable(t *testing.T, data []byte, locale model.LocaleID) []*model.Block {
	t.Helper()
	ctx := context.Background()
	reader, writer := fx.newPair(t)
	store, err := format.NewWiredSkeleton(reader, writer)
	require.NoError(t, err)
	require.NotNil(t, store, "%s has no skeleton pair", fx.format)
	defer store.Close()

	require.NoError(t, reader.Open(ctx, &model.RawDocument{
		URI:          "opsmatrix." + string(fx.format),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: locale,
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}))
	var blocks []*model.Block
	for res := range reader.Read(ctx) {
		require.NoError(t, res.Error, "read %s", fx.format)
		if res.Part == nil {
			continue
		}
		if b, ok := res.Part.Resource.(*model.Block); ok {
			blocks = append(blocks, b)
		}
	}
	require.NoError(t, reader.Close())
	return blocks
}

// editDocument reads input with the reader wired to the writer's skeleton
// store, hands every block to edit with the environment the change service
// applies operations in (the writer's declared capabilities, a person's
// edit), and writes the parts with the original bytes bound and writeLocale
// active, as the file home writes a document (core/change/filehome). It
// returns what the writer wrote and the error it refused the write with, if
// it did.
func (fx opsFixture) editDocument(t *testing.T, input []byte, readLocale, writeLocale model.LocaleID, edit func(b *model.Block, env change.BlockEnv)) ([]byte, error) {
	t.Helper()
	ctx := context.Background()
	reader, writer := fx.newPair(t)
	store, err := format.NewWiredSkeleton(reader, writer)
	require.NoError(t, err)
	require.NotNil(t, store, "%s has no skeleton pair", fx.format)
	defer store.Close()
	env := change.BlockEnv{Actor: change.Actor{Kind: change.ActorPerson}, Format: change.WriterCapabilities(string(fx.format), writer)}

	require.NoError(t, reader.Open(ctx, &model.RawDocument{
		URI:          "opsmatrix." + string(fx.format),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: readLocale,
		Reader:       io.NopCloser(bytes.NewReader(input)),
	}))
	var parts []*model.Part
	for res := range reader.Read(ctx) {
		require.NoError(t, res.Error, "read %s", fx.format)
		if res.Part == nil {
			continue
		}
		if b, ok := res.Part.Resource.(*model.Block); ok && b != nil {
			edit(b, env)
		}
		parts = append(parts, res.Part)
	}
	require.NoError(t, reader.Close())

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	if ocs, ok := writer.(format.OriginalContentSetter); ok {
		ocs.SetOriginalContent(input)
	}
	writer.SetLocale(writeLocale)
	ch := make(chan *model.Part, len(parts))
	for _, p := range parts {
		ch <- p
	}
	close(ch)
	werr := writer.Write(ctx, ch)
	require.NoError(t, writer.Close())
	return buf.Bytes(), werr
}

// opOutcome is what driving one operation over a document produced.
type opOutcome struct {
	out []byte
	// writeErr is the error the writer refused the document with.
	writeErr error
	// applied and refused list block ids by outcome.
	applied, refused []string
	// expect is every block's edition, as edit text, after the operation: the
	// edited text for an applied block, the text as read for any other.
	expect []string
}

// locales returns the target locale a document is read with and the locale its
// writer is given for op. A source edit reads and writes with no target
// locale. A target edit addresses the target edition on both.
func (fx opsFixture) locales(op matrixOp) (read, write model.LocaleID) {
	if op.edition == targetEdition {
		return fx.target, fx.target
	}
	return "", ""
}

// editionText returns a block's edition as edit text, and whether it has one.
func editionText(b *model.Block, edition editionRole, loc model.LocaleID) (string, bool) {
	if edition == sourceEdition {
		return model.RunsEditText(b.Source), true
	}
	if !b.HasTarget(loc) {
		return "", false
	}
	return model.RunsEditText(b.TargetRuns(loc)), true
}

// run drives op over input: each block whose edition holds op's word gets
// the operation, applied as the change service applies it.
func (fx opsFixture) run(t *testing.T, op matrixOp, input []byte) opOutcome {
	t.Helper()
	switch op.name {
	case "set_content", "replace_text":
		return fx.substitute(t, op, input)
	}
	t.Fatalf("the matrix has no driver for %s", op.name)
	return opOutcome{}
}

// substitute replaces op's word in op's edition of every block. Each block
// whose edition holds the word gets one operation of op's kind: set_content
// with the edition's edit text and the word replaced, or replace_text with an
// edit at each match. A block the operation is refused on keeps the text it
// was read with.
func (fx opsFixture) substitute(t *testing.T, op matrixOp, input []byte) opOutcome {
	t.Helper()
	readLoc, writeLoc := fx.locales(op)
	key := model.EditionKey{}
	if op.edition == targetEdition {
		key = model.EditionKey{Locale: writeLoc}.Canonical()
	}

	var res opOutcome
	res.out, res.writeErr = fx.editDocument(t, input, readLoc, writeLoc, func(b *model.Block, env change.BlockEnv) {
		text, ok := editionText(b, op.edition, readLoc)
		res.expect = append(res.expect, text)
		if !ok || !b.Translatable {
			return
		}
		edited := strings.ReplaceAll(text, op.sub.from, op.sub.to)
		if edited == text {
			return
		}
		at := change.Ref{Doc: "opsmatrix", Block: b.ID, Edition: key}
		o := change.Op{Kind: change.KindSetContent, At: at, IfMatch: change.AnyRevision, Body: &change.SetContent{Text: &edited}}
		if op.name == "replace_text" {
			runs := b.Source
			if op.edition == targetEdition {
				runs = b.TargetRuns(readLoc)
			}
			o = change.Op{Kind: change.KindReplaceText, At: at, IfMatch: change.AnyRevision, Body: &change.ReplaceText{Edits: replaceEdits(runs, nil, op.sub)}}
		}
		results := change.ApplyBlock(b, []change.Op{o}, env)
		switch results[0].Status {
		case change.OpApplied:
			res.applied = append(res.applied, b.ID)
			res.expect[len(res.expect)-1] = edited
		case change.OpRefused:
			res.refused = append(res.refused, b.ID)
		}
	})
	return res
}

// replaceEdits are the replace_text edits that substitute sub in seq, the run
// sequence path reaches, and in the branches of every plural and select it
// holds, as ksed computes them for s/from/to/g (host/toolbox_sed.go,
// sedCmd.edits): a match is found in the sequence's own text, in which inline
// codes, plurals and selects have no width, and a match that would swallow a
// plural or a select is left alone. A match is found at a byte offset and a
// text edit counts code points, so each match is converted before it becomes
// an edit. A framework test cannot import host, so this is a copy of ksed's
// computation; a change there has to be made here too.
func replaceEdits(seq []model.Run, path model.RunPath, sub substitution) []change.TextEdit {
	text := model.SequenceText(seq)
	var structures []int
	off := 0
	for _, r := range seq {
		switch {
		case r.Text != nil:
			off += utf8.RuneCountInString(r.Text.Text)
		case r.Plural != nil, r.Select != nil:
			structures = append(structures, off)
		}
	}
	byteAt, runeAt := 0, 0
	toRunes := func(b int) int {
		runeAt += utf8.RuneCountInString(text[byteAt:b])
		byteAt = b
		return runeAt
	}
	var out []change.TextEdit
	for at := 0; sub.from != sub.to; {
		i := strings.Index(text[at:], sub.from)
		if i < 0 {
			break
		}
		start := toRunes(at + i)
		at += i + len(sub.from)
		end := toRunes(at)
		if slices.ContainsFunc(structures, func(p int) bool { return start < p && p < end }) {
			continue
		}
		out = append(out, change.TextEdit{Path: slices.Clone(path), Start: &start, End: &end, Text: sub.to})
	}
	for i, r := range seq {
		step := model.RunPathStep{Kind: model.StepIndex, Index: i}
		switch {
		case r.Plural != nil:
			for _, form := range slices.Sorted(maps.Keys(r.Plural.Forms)) {
				branch := append(slices.Clone(path), step, model.RunPathStep{Kind: model.StepPlural, PluralForm: form})
				out = append(out, replaceEdits(r.Plural.Forms[form], branch, sub)...)
			}
		case r.Select != nil:
			for _, value := range slices.Sorted(maps.Keys(r.Select.Cases)) {
				branch := append(slices.Clone(path), step, model.RunPathStep{Kind: model.StepSelect, SelectValue: value})
				out = append(out, replaceEdits(r.Select.Cases[value], branch, sub)...)
			}
		}
	}
	return out
}

// describeBlocks lists blocks for a failure message.
func describeBlocks(blocks []*model.Block, edition editionRole, loc model.LocaleID) string {
	var b strings.Builder
	for i, blk := range blocks {
		text, ok := editionText(blk, edition, loc)
		if !ok {
			text = "(no edition)"
		}
		fmt.Fprintf(&b, "  [%d] id=%q translatable=%v: %q\n", i, blk.ID, blk.Translatable, text)
	}
	return b.String()
}

// cellsOf returns the operations that apply to a fixture: target operations
// only where the document holds a target edition.
func cellsOf(fx opsFixture) []matrixOp {
	var ops []matrixOp
	for _, op := range matrixOps {
		if op.edition == targetEdition && fx.target.IsEmpty() {
			continue
		}
		ops = append(ops, op)
	}
	return ops
}

// TestOperationsMatrix drives every operation over every fixture.
func TestOperationsMatrix(t *testing.T) {
	for _, fx := range opsMatrix() {
		for _, op := range cellsOf(fx) {
			cell := fx.id() + "/" + op.key()
			t.Run(cell, func(t *testing.T) {
				doc := fx.render(t, &op)
				res := fx.run(t, op, doc.input)

				if r, refused := fx.refused[op.key()]; refused {
					if r.err != nil {
						require.ErrorIs(t, res.writeErr, r.err,
							"%s is declared refused by the writer (%s); remove the declaration once the cell passes", cell, r.reason)
						assert.Empty(t, res.out, "%s: a refused write writes nothing", cell)
						return
					}
					require.NoError(t, res.writeErr, "%s: the writer refused the document", cell)
					assert.Empty(t, res.applied,
						"%s is declared refused (%s) but the operation applied and was written; "+
							"remove the declaration once the cell passes", cell, r.reason)
					assert.Equal(t, string(doc.input), string(res.out),
						"%s is declared refused (%s): the document must be written unchanged", cell, r.reason)
					return
				}
				require.NoError(t, res.writeErr, "%s: the writer refused the document", cell)

				readLoc, _ := fx.locales(op)
				require.NotEmpty(t, res.applied,
					"%s: no block took the edit, so the cell asserts nothing. Blocks read:\n%s",
					cell, describeBlocks(fx.readEditable(t, doc.input, readLoc), op.edition, readLoc))

				// Refusals: exactly the declared blocks are refused.
				var wantRefused []string
				for id := range fx.refuse[op.key()] {
					wantRefused = append(wantRefused, id)
				}
				assert.ElementsMatch(t, wantRefused, res.refused,
					"%s: the blocks refused differ from the declared refusals", cell)

				// Read back: every block holds the edition the operation left.
				back := fx.readEditable(t, res.out, readLoc)
				var got []string
				for _, b := range back {
					text, _ := editionText(b, op.edition, readLoc)
					got = append(got, text)
				}
				assert.Equal(t, res.expect, got,
					"%s: the written document does not read back as edited. Blocks read back:\n%s",
					cell, describeBlocks(back, op.edition, readLoc))

				// Bytes: everything the operation did not touch is unchanged.
				fx.assertBytes(t, cell, doc, res.out)
			})
		}
	}
}

// assertBytes compares the written document with the document the operation
// must produce: byte for byte for a text format, member by member for a ZIP
// container, skipping the members the fixture declares normalized.
func (fx opsFixture) assertBytes(t *testing.T, cell string, doc opsDocument, out []byte) {
	t.Helper()
	if doc.members == nil {
		assert.Equal(t, string(doc.want), string(out),
			"%s: the written bytes differ from the input outside the edit", cell)
		return
	}
	got := zipMembers(t, out)
	require.Len(t, got, len(doc.members), "%s: the member set changed", cell)
	for name, want := range doc.members {
		member, ok := got[name]
		require.True(t, ok, "%s: member %s is missing from the output", cell, name)
		if _, ok := fx.normalized[name]; ok {
			continue
		}
		assert.Equal(t, string(want), string(member),
			"%s: member %s differs from the input outside the edit", cell, name)
	}
}

// TestOperationsMatrixUntouchedDocument is the control: the same path with
// nothing to edit writes the document it read, under both the source and the
// target configuration.
func TestOperationsMatrixUntouchedDocument(t *testing.T) {
	for _, fx := range opsMatrix() {
		for _, op := range cellsOf(fx) {
			t.Run(fx.id()+"/"+string(op.edition), func(t *testing.T) {
				doc := fx.render(t, nil)
				noop := op
				noop.sub = substitution{from: op.sub.from, to: op.sub.from}
				res := fx.run(t, noop, doc.input)
				require.NoError(t, res.writeErr)
				assert.Empty(t, res.applied)
				fx.assertBytes(t, fx.id()+" untouched", doc, res.out)
			})
		}
	}
}

// TestOperationsMatrixCoversEverySkeletonPair keeps the matrix exhaustive and
// its declarations honest: every registered format whose reader and writer
// share a skeleton has a row, every row is such a format, and every
// declaration names something the fixture holds.
func TestOperationsMatrixCoversEverySkeletonPair(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	pairs := map[registry.FormatID]bool{}
	for _, name := range reg.WriterNames() {
		writer, err := reg.NewWriter(name)
		if err != nil {
			continue
		}
		reader, err := reg.NewReader(name)
		if err != nil {
			continue
		}
		if format.SkeletonPairEligible(reader, writer) {
			pairs[name] = true
		}
	}

	rows := map[registry.FormatID]bool{}
	ids := map[string]bool{}
	for _, fx := range opsMatrix() {
		assert.False(t, ids[fx.id()], "%s has two rows", fx.id())
		ids[fx.id()] = true
		rows[fx.format] = true
		assert.True(t, pairs[fx.format], "%s has a row but no skeleton pair", fx.id())
		assert.True(t, strings.HasSuffix(fx.template, ".tmpl"), "%s: a template path ends in .tmpl", fx.id())
		_, err := os.Stat(path.Join(opsMatrixDir, fx.template))
		require.NoError(t, err, "%s: template %s", fx.id(), fx.template)

		doc := fx.render(t, nil)
		input := string(doc.input)
		if doc.members != nil {
			var all strings.Builder
			for _, m := range zipMembers(t, doc.input) {
				all.Write(m)
			}
			input = all.String()
		}
		for _, r := range fx.respelled {
			assert.Contains(t, input, r.from, "%s: respelling %q names bytes the input does not hold", fx.id(), r.from)
			assert.NotEmpty(t, r.reason, "%s: respelling %q gives no reason", fx.id(), r.from)
		}
		for member, reason := range fx.normalized {
			if assert.NotNil(t, doc.members, "%s: normalized members name a text document", fx.id()) {
				assert.Contains(t, doc.members, member, "%s: normalized member %q is not in the container", fx.id(), member)
			}
			assert.NotEmpty(t, reason, "%s: normalized member %q gives no reason", fx.id(), member)
		}
		keys := map[string]bool{}
		for _, op := range cellsOf(fx) {
			keys[op.key()] = true
		}
		for key, r := range fx.refused {
			assert.True(t, keys[key], "%s: refused cell %q names no operation of the fixture", fx.id(), key)
			assert.NotEmpty(t, r.reason, "%s: refused cell %q gives no reason", fx.id(), key)
		}
		for key := range fx.refuse {
			assert.True(t, keys[key], "%s: refusals for %q name no operation of the fixture", fx.id(), key)
		}
	}
	for name := range pairs {
		assert.True(t, rows[name], "%s has a skeleton pair and no operations-matrix row", name)
	}
}
