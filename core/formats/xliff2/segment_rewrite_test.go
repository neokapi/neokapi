package xliff2_test

import (
	"bytes"
	"testing"

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
