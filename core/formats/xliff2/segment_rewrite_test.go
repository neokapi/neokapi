package xliff2_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const twoSegmentUnit = `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1">
      <segment id="s1">
        <source>First.</source>
        <target>Premier.</target>
      </segment>
      <ignorable>
        <source> </source>
        <target> </target>
      </ignorable>
      <segment id="s2">
        <source>Second.</source>
        <target>Deuxieme.</target>
      </segment>
    </unit>
  </file>
</xliff>`

// A tool that rewrites part of a segmented target leaves every segment in
// place: the edited text is written in the segment it was found in, and the
// segment beside it keeps its own.
func TestWriter_PartialTargetRewriteKeepsSegments(t *testing.T) {
	ctx := t.Context()
	reader := xliff2.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(twoSegmentUnit, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NoError(t, reader.Close())

	tl := tools.NewSearchReplaceTool(&tools.SearchReplaceConfig{
		Pairs:        []tools.ReplacePair{{Search: "Deuxieme", Replace: "Second"}},
		Target:       true,
		ReplaceAll:   true,
		TargetLocale: "fr",
	})
	out := make(chan *model.Part, len(parts)+1)
	require.NoError(t, tl.Process(ctx, testutil.PartsToChannel(parts), out))
	close(out)
	var processed []*model.Part
	for p := range out {
		processed = append(processed, p)
	}

	var buf bytes.Buffer
	w := xliff2.NewWriter()
	require.NoError(t, w.SetOutputWriter(&buf))
	w.SetLocale("fr")
	require.NoError(t, w.Write(ctx, testutil.PartsToChannel(processed)))
	require.NoError(t, w.Close())
	got := buf.String()

	assert.Contains(t, got, "<target>Premier.</target>")
	assert.Contains(t, got, "<target>Second.</target>")
	assert.NotContains(t, got, "Deuxieme")
}

const codeEndsSegmentUnit = `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1">
      <segment id="s1">
        <source>We use it.<ph id="1"/></source>
        <target>Nous  employons cela.<ph id="1"/></target>
      </segment>
      <segment id="s2">
        <source>Then this.</source>
        <target>Puis cela.</target>
      </segment>
    </unit>
  </file>
</xliff>`

// A tool edit to a segmented target keeps each inline code in the segment it
// was read in. The code that ends the first segment stays there when the tool
// tidies the text before it, and the second segment is written as it was.
func TestSkeletonPathKeepsTheCodeThatEndsASegment(t *testing.T) {
	ctx := t.Context()
	reader := xliff2.NewReader()
	writer := xliff2.NewWriter()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(codeEndsSegmentUnit, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NoError(t, reader.Close())

	// Only the double space changes: every other correction is off.
	cfg := tools.NewWhitespaceCorrectConfig("fr")
	cfg.NormalizeSpaces = true
	cfg.MatchSourceWhitespace = false
	cfg.RemoveZeroWidthChars = false
	cfg.CorrectFullStop = false
	cfg.CorrectComma = false
	cfg.CorrectExclamation = false
	cfg.CorrectQuestion = false
	out := make(chan *model.Part, len(parts)+1)
	require.NoError(t, tools.NewWhitespaceCorrectTool(cfg).Process(ctx, testutil.PartsToChannel(parts), out))
	close(out)
	var processed []*model.Part
	for p := range out {
		processed = append(processed, p)
	}

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	writer.SetLocale("fr")
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(processed)))
	require.NoError(t, writer.Close())

	assert.Equal(t, strings.Replace(codeEndsSegmentUnit, "Nous  employons", "Nous employons", 1), buf.String())
}
