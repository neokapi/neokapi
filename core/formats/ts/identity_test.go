package ts_test

import (
	"bytes"
	"encoding/xml"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// A block's Name is the context half of its identity (core/model/structural.go,
// core/reconcile). Qt TS has a real structural path — a message sits inside a
// named context — and a real key inside it, so these tests pin both: that the
// name says where the message is, and that an edit elsewhere leaves it alone.

func tsNames(t *testing.T, input string) []string {
	t.Helper()
	blocks := readTSBlocks(t, input)
	names := make([]string, len(blocks))
	for i, b := range blocks {
		names[i] = b.Name
	}
	return names
}

// tsDoc wraps message markup in the minimal TS envelope, under one context.
func tsDoc(contextName, messages string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.1" language="fr" sourcelanguage="en">
<context>
    <name>` + contextName + `</name>` + messages + `
</context>
</TS>`
}

const (
	msgAlpha = `
    <message>
        <source>Alpha</source>
        <translation>A</translation>
    </message>`
	msgBravo = `
    <message>
        <source>Bravo</source>
        <translation>B</translation>
    </message>`
	msgCharlie = `
    <message>
        <source>Charlie</source>
        <translation>C</translation>
    </message>`
)

// The name is the context and the message's own key, joined as a path — not the
// bare context, which used to give every message in a context the same name.
func TestMessageNameIsContextAndKey(t *testing.T) {
	t.Parallel()
	names := tsNames(t, tsDoc("MainWindow", msgAlpha+msgBravo))
	assert.Equal(t, []string{"MainWindow/Alpha", "MainWindow/Bravo"}, names)
}

// An explicit message id is the key an author can assign, and it wins over the
// source text — so rewording the string does not rename the block.
func TestExplicitMessageIDIsTheKey(t *testing.T) {
	t.Parallel()
	before := tsNames(t, tsDoc("MainWindow", `
    <message id="menu.file.open">
        <source>Open</source>
        <translation>Ouvrir</translation>
    </message>`))
	after := tsNames(t, tsDoc("MainWindow", `
    <message id="menu.file.open">
        <source>Open File</source>
        <translation>Ouvrir</translation>
    </message>`))
	assert.Equal(t, []string{"MainWindow/menu.file.open"}, before)
	assert.Equal(t, before, after, "an id-keyed message survives a source edit")
}

func TestTSDeletingAnEarlierMessageKeepsLaterNames(t *testing.T) {
	t.Parallel()
	before := tsNames(t, tsDoc("MainWindow", msgAlpha+msgBravo+msgCharlie))
	after := tsNames(t, tsDoc("MainWindow", msgAlpha+msgCharlie))

	require.Len(t, before, 3)
	require.Len(t, after, 2)
	assert.Equal(t, before[0], after[0])
	assert.Equal(t, before[2], after[1], "the surviving later message keeps its name")
}

// Deleting a whole context must not disturb the messages in another one.
func TestTSDeletingAnEarlierContextKeepsLaterNames(t *testing.T) {
	t.Parallel()
	const two = `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.1" language="fr" sourcelanguage="en">
<context>
    <name>Dialog</name>` + msgAlpha + `
</context>
<context>
    <name>MainWindow</name>` + msgBravo + `
</context>
</TS>`
	before := tsNames(t, two)
	after := tsNames(t, tsDoc("MainWindow", msgBravo))
	require.Len(t, before, 2)
	require.Len(t, after, 1)
	assert.Equal(t, before[1], after[0])
}

// The same string in two contexts is two messages, and Qt keeps them apart by
// context — so the names must differ.
func TestTSSameTextInTwoContextsGetsDistinctNames(t *testing.T) {
	t.Parallel()
	const two = `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.1" language="fr" sourcelanguage="en">
<context>
    <name>Dialog</name>` + msgAlpha + `
</context>
<context>
    <name>MainWindow</name>` + msgAlpha + `
</context>
</TS>`
	names := tsNames(t, two)
	require.Len(t, names, 2)
	assert.Equal(t, []string{"Dialog/Alpha", "MainWindow/Alpha"}, names)
}

// Within one context, Qt disambiguates two identical sources with a <comment>.
// That is the author's own disambiguator, so it goes in the name.
func TestTSDisambiguatingCommentSeparatesIdenticalSources(t *testing.T) {
	t.Parallel()
	names := tsNames(t, tsDoc("MainWindow", `
    <message>
        <source>Open</source>
        <comment>verb</comment>
        <translation>Ouvrir</translation>
    </message>
    <message>
        <source>Open</source>
        <comment>adjective</comment>
        <translation>Ouvert</translation>
    </message>`))
	assert.Equal(t, []string{"MainWindow/Open/verb", "MainWindow/Open/adjective"}, names)
}

// Identical sources with no disambiguating comment are indistinguishable by
// structure; an ordinal scoped to the repeated key is the last resort.
func TestTSIdenticalSourcesWithoutCommentGetDistinctNames(t *testing.T) {
	t.Parallel()
	names := tsNames(t, tsDoc("MainWindow", msgAlpha+msgAlpha))
	require.Len(t, names, 2)
	assert.Equal(t, "MainWindow/Alpha", names[0])
	assert.NotEqual(t, names[0], names[1])
}

func TestTSNamesAreStableAcrossReads(t *testing.T) {
	t.Parallel()
	input := tsDoc("MainWindow", msgAlpha+msgBravo)
	assert.Equal(t, tsNames(t, input), tsNames(t, input))
}

// A message outside any context still gets a name — just a one-segment one.
func TestTSMessageWithoutContextIsNamedByItsKey(t *testing.T) {
	t.Parallel()
	names := tsNames(t, `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.1" language="fr" sourcelanguage="en">
<context>
    <name></name>`+msgAlpha+`
</context>
</TS>`)
	assert.Equal(t, []string{"Alpha"}, names)
}

// A source with a line break in it must not smuggle the path separator or a
// newline into a name segment.
func TestTSMultilineSourceIsSanitisedIntoOneSegment(t *testing.T) {
	t.Parallel()
	names := tsNames(t, tsDoc("MainWindow", `
    <message>
        <source>first line
second line</source>
        <translation>x</translation>
    </message>`))
	require.Len(t, names, 1)
	assert.Equal(t, "MainWindow/first line second line", names[0])
	assert.NotContains(t, names[0], "\n")
}

// A block's Name is not only an internal identity: the XLIFF 2 writer stamps it
// as `<unit name="…">` on the `kapi extract` path, so it reaches translators.
// Since the fallback key here is the source text, these names now carry whatever
// the source carries — quotes, angle brackets, ampersands — straight into an XML
// attribute. Naming by the context alone could never produce those.
//
// This pins that the extract surface stays well-formed. It covers the mechanism
// for the other readers whose key is source text (TMX among them), because the
// escaping lives in the shared XLIFF writer.
func TestSourceKeyedNameSurvivesTheExtractSurface(t *testing.T) {
	t.Parallel()
	parts := readTS(t, tsDoc("Main", `
    <message>
        <source>Click "OK" &lt;b&gt;now&lt;/b&gt; &amp; wait</source>
        <translation type="unfinished"></translation>
    </message>`))

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1)
	// The path separator cannot appear inside a segment, so "</b>" reads as
	// "<-b>" — a deliberate flattening by model.StructuralPath, not a bug.
	assert.Equal(t, `Main/Click "OK" <b>now<-b> & wait`, blocks[0].Name)

	var buf bytes.Buffer
	w := xliff2.NewWriter()
	require.NoError(t, w.SetOutputWriter(&buf))
	w.SetLocale(model.LocaleID("fr"))
	require.NoError(t, w.Write(t.Context(), testutil.PartsToChannel(parts)))
	w.Close()

	assert.Contains(t, buf.String(),
		`name="Main/Click &quot;OK&quot; &lt;b&gt;now&lt;-b&gt; &amp; wait"`,
		"the name is escaped into the attribute, not injected raw")

	dec := xml.NewDecoder(bytes.NewReader(buf.Bytes()))
	for {
		if _, err := dec.Token(); err != nil {
			require.ErrorIs(t, err, io.EOF, "extract output must stay well-formed XML")
			break
		}
	}
}

// The name is what reconcile hashes as context, so two messages in one file must
// never produce the same context hash — otherwise a decision recorded about one
// can be claimed by the other.
func TestTSContextHashesAreDistinctWithinAFile(t *testing.T) {
	t.Parallel()
	blocks := readTSBlocks(t, tsDoc("MainWindow", msgAlpha+msgBravo+msgCharlie))
	require.Len(t, blocks, 3)
	seen := map[string]bool{}
	for _, b := range blocks {
		h := model.ComputeContextHash(b.Name, b.Type, b.Properties)
		assert.False(t, seen[h], "duplicate context hash for %q", b.Name)
		seen[h] = true
	}
}
