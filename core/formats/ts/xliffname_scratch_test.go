package ts_test

import (
	"bytes"
	"encoding/xml"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

func TestScratchXliffUnitNameEscaping(t *testing.T) {
	snippet := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.1" language="fr" sourcelanguage="en">
<context>
    <name>Main</name>
    <message>
        <source>Click "OK" &lt;b&gt;now&lt;/b&gt; &amp; wait</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>`
	parts := readTS(t, snippet)
	for _, b := range testutil.FilterBlocks(parts) {
		t.Logf("block name = %q", b.Name)
	}

	var buf bytes.Buffer
	w := xliff2.NewWriter()
	require.NoError(t, w.SetOutputWriter(&buf))
	w.SetLocale(model.LocaleID("fr"))
	require.NoError(t, w.Write(t.Context(), testutil.PartsToChannel(parts)))
	w.Close()

	t.Logf("xliff:\n%s", buf.String())

	// Well-formedness: the whole document must parse.
	d := xml.NewDecoder(bytes.NewReader(buf.Bytes()))
	for {
		_, err := d.Token()
		if err != nil {
			require.ErrorContains(t, err, "EOF")
			break
		}
	}
}
