package host

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// sigRuns renders a run sequence compactly so tests can assert on inline-code
// structure as well as text: text verbatim, <id>/</id> for a paired code,
// {id} for a placeholder, [id] for a subblock reference, and a plural as
// plural(form:…|form:…) with its forms in order.
func sigRuns(runs []model.Run) string {
	var b strings.Builder
	for _, r := range runs {
		switch {
		case r.Text != nil:
			b.WriteString(r.Text.Text)
		case r.PcOpen != nil:
			fmt.Fprintf(&b, "<%s>", r.PcOpen.ID)
		case r.PcClose != nil:
			fmt.Fprintf(&b, "</%s>", r.PcClose.ID)
		case r.Ph != nil:
			fmt.Fprintf(&b, "{%s}", r.Ph.ID)
		case r.Sub != nil:
			fmt.Fprintf(&b, "[%s]", r.Sub.ID)
		case r.Plural != nil:
			var forms []string
			for _, f := range []model.PluralForm{model.PluralOne, model.PluralOther} {
				if rs, ok := r.Plural.Forms[f]; ok {
					forms = append(forms, string(f)+":"+sigRuns(rs))
				}
			}
			fmt.Fprintf(&b, "plural(%s)", strings.Join(forms, "|"))
		case r.Select != nil:
			b.WriteString("select")
		}
	}
	return b.String()
}

// boldUglyRuns is "Hello <b>ugly</b> world" — the running example: a bold span
// (paired code id=1) wrapping the word "ugly".
func boldUglyRuns() []model.Run {
	return []model.Run{
		{Text: &model.TextRun{Text: "Hello "}},
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "b"}},
		{Text: &model.TextRun{Text: "ugly"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "b"}},
		{Text: &model.TextRun{Text: " world"}},
	}
}

// sedText runs prog over a block holding text alone and returns its text
// after.
func sedText(t *testing.T, prog sedProgram, text string) string {
	t.Helper()
	_, runs := sedProgOn(t, prog, []model.Run{{Text: &model.TextRun{Text: text}}})
	return model.SequenceText(runs)
}

// sedOn compiles script over runs and applies the operations it gives the way
// the change service does, returning the operations and the runs after them.
func sedOn(t *testing.T, runs []model.Run, scripts ...string) ([]change.Op, []model.Run) {
	t.Helper()
	prog, err := ParseSedProgram(scripts)
	require.NoError(t, err)
	return sedProgOn(t, prog, runs)
}

// sedProgOn is sedOn for a parsed program.
func sedProgOn(t *testing.T, prog sedProgram, runs []model.Run) ([]change.Op, []model.Run) {
	t.Helper()
	at := change.Ref{Doc: "page.html", Block: "p"}
	ops, err := prog.ops(at, model.RunsRevision(model.EditionKey{}, runs), runs)
	require.NoError(t, err)
	for _, op := range ops {
		assert.Equal(t, change.KindReplaceText, op.Kind)
		assert.Equal(t, at, op.At)
	}
	b := model.NewRunsBlock("p", runs)
	sim := make([]change.Op, len(ops))
	for i, op := range ops {
		sim[i] = op
		sim[i].At = change.Ref{}
		sim[i].IfMatch = change.AnyRevision
	}
	for _, r := range change.ApplyBlock(b, sim, change.BlockEnv{Actor: change.Actor{Kind: change.ActorPerson}}) {
		require.Nil(t, r.Error, "the service applies what ksed compiled")
	}
	ed, _ := b.Edition(model.EditionKey{})
	return ops, ed.Runs
}

// ksed compiles a substitution into replace_text operations over the text a
// block holds with its inline codes left out, so the codes around a change are
// kept, a match may span a code, and an emptied deletable span collapses.
func TestSedOpsPreserveCodes(t *testing.T) {
	tests := []struct {
		name   string
		script string
		runs   []model.Run
		want   string // sigRuns of the result
		ops    int
	}{
		{name: "edit inside a bold span keeps the span", script: "s/ugly/pretty/", runs: boldUglyRuns(), want: "Hello <1>pretty</1> world", ops: 1},
		{
			// The regex sees "Hello ugly world" (codes are invisible), so a phrase
			// spanning the bold boundaries matches. Bold is deletable, so the
			// emptied span collapses: no empty <b></b>.
			name: "match spanning the whole bold phrase drops the emptied span", script: "s/Hello ugly world/Hi/",
			runs: boldUglyRuns(), want: "Hi", ops: 1,
		},
		{name: "match crossing the opening code", script: "s/lo ug/X/", runs: boldUglyRuns(), want: "HelX<1>ly</1> world", ops: 1},
		{name: "global edit across and inside the span", script: "s/o/0/g", runs: boldUglyRuns(), want: "Hell0 <1>ugly</1> w0rld", ops: 1},
		{
			name: "placeholder preserved in place", script: "s/world/EARTH/",
			runs: []model.Run{{Text: &model.TextRun{Text: "Hello "}}, {Ph: &model.PlaceholderRun{ID: "1", Type: "icon"}}, {Text: &model.TextRun{Text: " world"}}},
			want: "Hello {1} EARTH", ops: 1,
		},
		{name: "backreference inside a span", script: `s/(ug)(ly)/\2\1/`, runs: boldUglyRuns(), want: "Hello <1>lyug</1> world", ops: 1},
		{
			name: "non-deletable code survives an emptying edit", script: "s/Hello world/Hi/",
			runs: []model.Run{
				{Text: &model.TextRun{Text: "Hello "}},
				{Ph: &model.PlaceholderRun{ID: "1", Type: "struct:break", Constraints: &model.RunConstraints{Deletable: false}}},
				{Text: &model.TextRun{Text: "world"}},
			},
			want: "{1}Hi", ops: 1,
		},
		{
			name: "deletable code dropped by an emptying edit", script: "s/Hello world/Hi/",
			runs: []model.Run{
				{Text: &model.TextRun{Text: "Hello "}},
				{Ph: &model.PlaceholderRun{ID: "1", Type: "fmt:icon", Constraints: &model.RunConstraints{Deletable: true}}},
				{Text: &model.TextRun{Text: "world"}},
			},
			want: "Hi", ops: 1,
		},
		{name: "no match compiles to nothing", script: "s/zzz/qqq/", runs: boldUglyRuns(), want: "Hello <1>ugly</1> world"},
		{name: "a replacement equal to the match compiles to nothing", script: "s/ugly/ugly/", runs: boldUglyRuns(), want: "Hello <1>ugly</1> world"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ops, got := sedOn(t, tc.runs, tc.script)
			assert.Len(t, ops, tc.ops)
			assert.Equal(t, tc.want, sigRuns(got))
		})
	}
}

// Each substitution of a program is one operation, applied in order, and
// every operation carries the revision ksed read. The first names its text by
// position in the content ksed read; a later one matches the text the
// substitutions before it left, and names it by find, which the service reads
// there too.
func TestSedOpsChainSubstitutions(t *testing.T) {
	runs := boldUglyRuns()
	ops, got := sedOn(t, runs, "s/Hello/Hi/", "s/world/earth/")
	require.Len(t, ops, 2)
	assert.Equal(t, "Hi <1>ugly</1> earth", sigRuns(got))
	rev := model.RunsRevision(model.EditionKey{}, runs)
	assert.Equal(t, rev, ops[0].IfMatch)
	assert.Equal(t, rev, ops[1].IfMatch)
	first := ops[0].Body.(*change.ReplaceText).Edits[0]
	assert.Equal(t, 0, *first.Start)
	second := ops[1].Body.(*change.ReplaceText).Edits[0]
	require.NotNil(t, second.Find)
	assert.Equal(t, "world", *second.Find)
	assert.Equal(t, 1, second.Occurrence)

	ops, got = sedOn(t, runs, "s/ugly/bad/", "s/bad/ugly/")
	assert.Len(t, ops, 2)
	assert.Equal(t, "Hello <1>ugly</1> world", sigRuns(got), "a program whose edits cancel leaves the codes as they were")

	_, got = sedOn(t, []model.Run{{Text: &model.TextRun{Text: "a cat, a car"}}}, "s/a/the/g", "s/the/one/g")
	assert.Equal(t, "one conet, one coner", sigRuns(got), "a later substitution counts the occurrences the earlier ones wrote")
}

// A later substitution whose match no find can name, an empty match here,
// makes the program one operation that leaves the edition as the whole
// program does.
func TestSedOpsAnEmptyLaterMatchGivesOneOperation(t *testing.T) {
	runs := boldUglyRuns()
	prog, err := ParseSedProgram([]string{"s/Hello/Hi/", "s/$/!/"})
	require.NoError(t, err)
	at := change.Ref{Doc: "page.html", Block: "p"}
	rev := model.RunsRevision(model.EditionKey{}, runs)
	ops, err := prog.ops(at, rev, runs)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	assert.Equal(t, at, ops[0].At)
	assert.Equal(t, rev, ops[0].IfMatch)

	b := model.NewRunsBlock("p", runs)
	ops[0].At = change.Ref{}
	for _, r := range change.ApplyBlock(b, ops, change.BlockEnv{Actor: change.Actor{Kind: change.ActorPerson}}) {
		require.Nil(t, r.Error)
	}
	assert.Equal(t, "Hi <1>ugly</1> world!", sigRuns(b.SourceRuns()))
}

// The regular expression reports byte offsets and a text edit counts code
// points, so a match after non-ASCII text lands on the text it matched.
func TestSedOpsAfterNonASCIIText(t *testing.T) {
	runs := []model.Run{
		{Text: &model.TextRun{Text: "Blåbær "}},
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "b"}},
		{Text: &model.TextRun{Text: "ugly"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "b"}},
		{Text: &model.TextRun{Text: " æøå ugly"}},
	}
	_, got := sedOn(t, runs, "s/ugly/pen/g")
	assert.Equal(t, "Blåbær <1>pen</1> æøå pen", sigRuns(got))
}

// A plural keeps its structure: each branch is matched on its own and edited
// through a path, and a match that would swallow the plural is left alone.
func TestSedOpsEditPluralBranches(t *testing.T) {
	plural := func() []model.Run {
		return []model.Run{
			{Text: &model.TextRun{Text: "You have "}},
			{Plural: &model.PluralRun{Pivot: "n", Forms: map[model.PluralForm][]model.Run{
				model.PluralOne:   {{Text: &model.TextRun{Text: "one item"}}},
				model.PluralOther: {{Text: &model.TextRun{Text: "many items"}}},
			}}},
			{Text: &model.TextRun{Text: " in your item list"}},
		}
	}
	ops, got := sedOn(t, plural(), "s/item/article/g")
	require.Len(t, ops, 1)
	assert.Equal(t, "You have plural(one:one article|other:many articles) in your article list", sigRuns(got))
	paths := map[string]bool{}
	for _, e := range ops[0].Body.(*change.ReplaceText).Edits {
		b, err := json.Marshal(e.Path)
		require.NoError(t, err)
		paths[string(b)] = true
	}
	assert.Equal(t, map[string]bool{"null": true, `[1,{"plural":"one"}]`: true, `[1,{"plural":"other"}]`: true}, paths)

	ops, got = sedOn(t, plural(), "s/have  in/had in/")
	assert.Empty(t, ops, "a match across the plural's position would delete it, so it is left alone")
	assert.Equal(t, "You have plural(one:one item|other:many items) in your item list", sigRuns(got))

	_, got = sedOn(t, plural(), "s/item/article/")
	assert.Equal(t, "You have plural(one:one item|other:many items) in your article list", sigRuns(got), "without g only the first match is replaced")
}

// sedCommand is the command a test runs ksed as.
func sedCommand() *EnvCommand { return NewEnvCommand(context.Background(), "sed") }

// End to end over a real HTML reader and writer: editing a word inside a <b>
// span keeps the span, and a phrase spanning the bold boundary matches.
func TestSedHTMLPreservesInlineFormatting(t *testing.T) {
	noProject(t)
	const html = `<!DOCTYPE html><html><body><p>Hello <b>ugly</b> world</p></body></html>`
	for name, tc := range map[string]struct {
		script string
		want   []string
		gone   []string
	}{
		"inside a span":         {"s/ugly/pretty/", []string{"<b>pretty</b>"}, []string{"ugly"}},
		"across an inline code": {"s/Hello ugly world/Hi there/", []string{"<p>Hi there</p>"}, []string{"ugly", "<b>"}},
	} {
		t.Run(name, func(t *testing.T) {
			app := newToolboxApp(t)
			path := filepath.Join(t.TempDir(), "page.html")
			require.NoError(t, os.WriteFile(path, []byte(html), 0o644))
			prog, err := ParseSedProgram([]string{tc.script})
			require.NoError(t, err)
			out, err := captureStdout(t, func() error {
				return app.RunSed(context.Background(), sedCommand(), []string{path}, prog, SedOptions{})
			})
			require.NoError(t, err)
			for _, w := range tc.want {
				assert.Contains(t, out, w)
			}
			for _, g := range tc.gone {
				assert.NotContains(t, out, g)
			}
			got, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, html, string(got), "printing an edit leaves the file as it is")
		})
	}
}

// --print-ops prints the change set ksed would apply, and kapi apply applying
// it writes the same bytes ksed -i writes.
func TestSedPrintOpsAppliesAsPrinted(t *testing.T) {
	noProject(t)
	const html = `<p>Read the <a href="https://old.example/guide">shop guide</a> before you <b>order</b>.</p>` + "\n<p>The shop opens at nine.</p>\n"
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.WriteFile("printed.html", []byte(html), 0o644))
	require.NoError(t, os.WriteFile("inplace.html", []byte(html), 0o644))
	app := newToolboxApp(t)
	prog, err := ParseSedProgram([]string{"s/shop/store/g"})
	require.NoError(t, err)

	printed, err := captureStdout(t, func() error {
		return app.RunSed(context.Background(), sedCommand(), []string{"printed.html"}, prog, SedOptions{PrintOps: true})
	})
	require.NoError(t, err)
	set, err := change.Decode(strings.NewReader(printed))
	require.NoError(t, err, printed)
	require.Len(t, set.Ops, 2, "one replace_text per block the substitution changes")
	for _, op := range set.Ops {
		assert.Equal(t, change.KindReplaceText, op.Kind)
		assert.Equal(t, "printed.html", op.At.Doc)
		assert.Regexp(t, `^r:[0-9a-f]{16}$`, op.IfMatch)
	}
	unchanged, _ := os.ReadFile("printed.html")
	assert.Equal(t, html, string(unchanged), "--print-ops changes nothing")

	_, _, err = runApply(t, app, NewEnvCommand(t.Context(), "apply"), printed, ApplyOptions{})
	require.NoError(t, err)
	require.NoError(t, app.RunSed(context.Background(), sedCommand(), []string{"inplace.html"}, prog, SedOptions{InPlace: true}))

	viaApply, _ := os.ReadFile("printed.html")
	viaSed, _ := os.ReadFile("inplace.html")
	assert.Equal(t, string(viaSed), string(viaApply))
	assert.Contains(t, string(viaApply), `<a href="https://old.example/guide">store guide</a>`)
}

// ksed -i edits a file in place through the change service and keeps a backup
// of what it replaced; standard input is edited to standard output.
func TestSedInPlaceAndStdin(t *testing.T) {
	noProject(t)
	app := newToolboxApp(t)
	prog, err := ParseSedProgram([]string{"s/Hello/Hi/"})
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "en.json")
	const src = `{"greeting":"Hello world","farewell":"Hello again"}`
	require.NoError(t, os.WriteFile(path, []byte(src), 0o644))
	require.NoError(t, app.RunSed(context.Background(), sedCommand(), []string{path}, prog, SedOptions{InPlace: true, BackupSuffix: ".bak"}))
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, `{"greeting":"Hi world","farewell":"Hi again"}`, string(got), "without g, the first match of each block")
	backup, err := os.ReadFile(path + ".bak")
	require.NoError(t, err)
	assert.Equal(t, src, string(backup))

	withStdin(t, src)
	out, err := captureStdout(t, func() error {
		return app.RunSed(context.Background(), sedCommand(), []string{StdinName}, prog, SedOptions{})
	})
	require.NoError(t, err)
	assert.Equal(t, `{"greeting":"Hi world","farewell":"Hi again"}`, out)
}

// A file no format claims is edited as plain text, as the rest of the toolbox
// reads it.
func TestSedEditsAnUnclaimedTextFileAsPlainText(t *testing.T) {
	noProject(t)
	app := newToolboxApp(t)
	prog, err := ParseSedProgram([]string{"s/colour/color/g"})
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "NOTES")
	require.NoError(t, os.WriteFile(path, []byte("The colour of the colour.\n"), 0o644))
	require.NoError(t, app.RunSed(context.Background(), sedCommand(), []string{path}, prog, SedOptions{InPlace: true}))
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "The color of the color.\n", string(got))
}

// --target edits the translation a bilingual file holds and leaves the source.
func TestSedTargetEditsTheTranslation(t *testing.T) {
	noProject(t)
	app := newToolboxApp(t)
	prog, err := ParseSedProgram([]string{"s/Bonjour/Salut/g"})
	require.NoError(t, err)
	const xlf = `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
<file source-language="en" target-language="fr" datatype="plaintext" original="a.txt">
<body>
<trans-unit id="1"><source>Bonjour means hello</source><target>Bonjour le monde</target></trans-unit>
</body>
</file>
</xliff>
`
	path := filepath.Join(t.TempDir(), "messages.xlf")
	require.NoError(t, os.WriteFile(path, []byte(xlf), 0o644))
	require.NoError(t, app.RunSed(context.Background(), sedCommand(), []string{path}, prog, SedOptions{InPlace: true, Target: "fr"}))
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(got), "<target>Salut le monde</target>")
	assert.Contains(t, string(got), "<source>Bonjour means hello</source>")
}

// withStdin feeds content to the process's standard input for one test.
func withStdin(t *testing.T, content string) {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, err = w.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig; _ = r.Close() })
}
