package ts_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/ts"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const numerusMessage = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE TS>
<TS version="2.1" language="fr" sourcelanguage="en">
<context>
    <name>Files</name>
    <message numerus="yes">
        <source>%n file(s)</source>
        <translation>
            <numerusform>un fichier</numerusform>
            <numerusform>des fichiers</numerusform>
        </translation>
    </message>
</context>
</TS>
`

// A numerus target is carved into its forms by a segmentation layer. A tool
// that rewrites one form leaves every form in place, the edited one holding
// the new wording.
func TestWriter_PartialNumerusRewriteKeepsEveryForm(t *testing.T) {
	ctx := t.Context()
	reader := ts.NewReader()
	writer := ts.NewWriter()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(numerusMessage, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NoError(t, reader.Close())

	tl := tools.NewSearchReplaceTool(&tools.SearchReplaceConfig{
		Pairs:        []tools.ReplacePair{{Search: "fichiers", Replace: "documents"}},
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
	require.NoError(t, writer.SetOutputWriter(&buf))
	writer.SetLocale("fr")
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(processed)))
	require.NoError(t, writer.Close())
	got := buf.String()

	assert.Contains(t, got, "<numerusform>un fichier</numerusform>")
	assert.Contains(t, got, "<numerusform>des documents</numerusform>")
}
