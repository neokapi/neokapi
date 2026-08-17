package xliff_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/xliff"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1477. A `<target>` is written by substituting the block's runs into the
// `xliff:target-body` IR captured at read time, which is what gives an ordinary
// edit its inline-code fidelity. The IR is the shape, not the content: when a
// tool deletes a run the IR still has a node for, the node has to go with it.
//
// The witness is `whitespace-correct --trim-trailing`, because its trailing edge
// drops a whitespace-only text run outright rather than rewriting it — the one
// correction that makes the model's run sequence shorter than the IR's. Unit 1
// exercises it behind an inline code, where the surviving node is not the last
// thing in the body; unit 2 is the case that already worked, kept as the control
// that the fix did not buy the first at the second's expense.

const trimFixture = `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file original="save.txt" source-language="en" target-language="nb" datatype="plaintext">
    <body>
      <trans-unit id="1">
        <source>Save <g id="2">now</g></source>
        <target>Lagre  <g id="2">nå</g>   </target>
      </trans-unit>
      <trans-unit id="2">
        <source><x id="3"/> each</source>
        <target><x id="3"/>  per stk.  </target>
      </trans-unit>
    </body>
  </file>
</xliff>`

// trimTrailingOnly is the tool as the issue ran it: every other correction off,
// so nothing but the trailing trim can account for a difference in the output.
func trimTrailingOnly(t *testing.T, locale model.LocaleID) tool.Tool {
	t.Helper()
	cfg := tools.NewWhitespaceCorrectConfig(locale)
	cfg.TrimTrailing = true
	cfg.NormalizeSpaces = false
	cfg.MatchSourceWhitespace = false
	cfg.RemoveZeroWidthChars = false
	cfg.CorrectFullStop = false
	cfg.CorrectComma = false
	cfg.CorrectExclamation = false
	cfg.CorrectQuestion = false
	require.NoError(t, cfg.Validate())
	return tools.NewWhitespaceCorrectTool(cfg)
}

// roundtripThroughTool reads input with the document target locale attached,
// runs tl over every part, and writes through the same skeleton store — the
// round trip `kapi exec <tool>` takes.
func roundtripThroughTool(t *testing.T, input string, locale model.LocaleID, tl tool.Tool) string {
	t.Helper()
	ctx := t.Context()

	reader := xliff.NewReader()
	writer := xliff.NewWriter()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = locale
	require.NoError(t, reader.Open(ctx, doc))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NoError(t, reader.Close())

	in := testutil.PartsToChannel(parts)
	out := make(chan *model.Part, len(parts)+1)
	require.NoError(t, tl.Process(ctx, in, out))
	close(out)

	var processed []*model.Part
	for p := range out {
		processed = append(processed, p)
	}

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	writer.SetLocale(locale)
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(processed)))
	require.NoError(t, writer.Close())
	return buf.String()
}

func TestWriter_DeletedTargetRunDoesNotReturn(t *testing.T) {
	got := roundtripThroughTool(t, trimFixture, "nb", trimTrailingOnly(t, "nb"))

	assert.Contains(t, got, `<target>Lagre  <g id="2">nå</g></target>`,
		"the trailing run the tool deleted was re-emitted from the captured target body")
	assert.Contains(t, got, `<target><x id="3"/>  per stk.</target>`,
		"the case that already worked must keep working")

	// The inline codes the IR carries are exactly what the round trip is for, so
	// the deletion must not have cost them.
	assert.Contains(t, got, `<g id="2">`)
	assert.Contains(t, got, `<x id="3"/>`)
	// The sources are untouched by a target-only tool.
	assert.Contains(t, got, `<source>Save <g id="2">now</g></source>`)
	assert.Contains(t, got, `<source><x id="3"/> each</source>`)
}

// A tool that changes nothing must still write the document back byte for byte:
// the deletion rule keys on the runs, so a run sequence that still fills every
// IR node has to reproduce the capture exactly.
func TestWriter_NoEditRoundTripsByteForByte(t *testing.T) {
	assert.Equal(t, trimFixture, snippetRoundtripWithSkeleton(t, trimFixture))
}
