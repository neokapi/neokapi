package ts_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/ts"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const numerusDoc = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE TS []>
<TS version="2.1" language="fr" sourcelanguage="en">
<context>
    <name>MainWindow</name>
    <message numerus="yes">
        <source>We use %n item(s)</source>
        <translation>
            <numerusform>Nous employons %n article</numerusform>
            <numerusform>Nous employons %n articles</numerusform>
        </translation>
    </message>
</context>
</TS>
`

// editNumerus reads numerusDoc through the skeleton path, hands the message
// to edit, and writes it back. It returns what was written and the writer's
// error.
func editNumerus(t *testing.T, edit func(*model.Block)) (string, error) {
	t.Helper()
	ctx := t.Context()
	reader, writer := ts.NewReader(), ts.NewWriter()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(numerusDoc, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NoError(t, reader.Close())
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && edit != nil {
			edit(b)
		}
	}

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	werr := writer.Write(ctx, testutil.PartsToChannel(parts))
	require.NoError(t, writer.Close())
	return buf.String(), werr
}

// A numerus message holds its plural forms as spans over one translation. An
// edit written as one text has no form boundaries, so the runs it rebuilds can
// merge across one, and the writer used to cut the new runs at the old spans:
// "Nous utilisons %n articleNous utilisons " landed in the first form and
// "%n articles" in the second. The writer now refuses that message, before it
// writes anything; an edit that keeps each form's runs writes each form.
func TestNumerusFormsAreWrittenOrRefused(t *testing.T) {
	replaceInRuns := func(b *model.Block) {
		runs := b.TargetRuns("fr")
		out := make([]model.Run, len(runs))
		copy(out, runs)
		for i := range out {
			if out[i].Text != nil {
				txt := *out[i].Text
				txt.Text = strings.ReplaceAll(txt.Text, "employons", "utilisons")
				out[i].Text = &txt
			}
		}
		b.SetTargetRuns("fr", out)
	}
	replaceInEditText := func(b *model.Block) {
		runs := b.TargetRuns("fr")
		text := strings.ReplaceAll(model.RunsEditText(runs), "employons", "utilisons")
		b.SetTargetRuns("fr", model.ParseRunsEditText(text, runs))
	}

	tests := []struct {
		name    string
		edit    func(*model.Block)
		want    string
		refused bool
	}{
		{name: "untouched", want: numerusDoc},
		{
			name: "each form edited in its own runs",
			edit: replaceInRuns,
			want: strings.ReplaceAll(numerusDoc, "Nous employons", "Nous utilisons"),
		},
		{name: "the forms edited as one text", edit: replaceInEditText, refused: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := editNumerus(t, tc.edit)
			if tc.refused {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "plural forms no longer line up")
				assert.Empty(t, out, "a refused write writes nothing")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, out)
		})
	}
}
