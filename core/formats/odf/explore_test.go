package odf_test

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/formats/odf"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/require"
)

const fixDir = "/Users/asgeirf/src/okapi/Okapi/okapi/filters/openoffice/src/test/resources/"

func TestExploreRoundtripReal(t *testing.T) {
	for _, name := range []string{"TestDocument01.odt", "TestDocument02.odt", "TestSpreadsheet01.ods", "TestPresentation01.odp"} {
		data, err := os.ReadFile(fixDir + name)
		require.NoError(t, err)
		ctx := t.Context()

		reader := odf.NewReader()
		require.NoError(t, reader.Open(ctx, testutil.RawDocFromReader(bytes.NewReader(data), name, model.LocaleEnglish)))
		parts := testutil.CollectParts(t, reader.Read(ctx))
		blocks1 := testutil.FilterBlocks(parts)
		reader.Close()

		var buf bytes.Buffer
		writer := odf.NewWriter()
		require.NoError(t, writer.SetOutputWriter(&buf))
		writer.SetOriginalContent(data)
		writer.SetLocale(model.LocaleEnglish)
		require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
		writer.Close()

		reader2 := odf.NewReader()
		require.NoError(t, reader2.Open(ctx, testutil.RawDocFromReader(bytes.NewReader(buf.Bytes()), name, model.LocaleEnglish)))
		blocks2 := testutil.CollectBlocks(t, reader2.Read(ctx))
		reader2.Close()

		match := len(blocks1) == len(blocks2)
		mismatches := 0
		if len(blocks1) == len(blocks2) {
			for i := range blocks1 {
				if blocks1[i].SourceText() != blocks2[i].SourceText() {
					match = false
					mismatches++
					if mismatches <= 5 {
						fmt.Printf("  MISMATCH[%d] %q != %q\n", i, blocks1[i].SourceText(), blocks2[i].SourceText())
					}
				}
			}
		}
		fmt.Printf("%-24s blocks1=%d blocks2=%d match=%v mismatches=%d\n", name, len(blocks1), len(blocks2), match, mismatches)
	}
}
