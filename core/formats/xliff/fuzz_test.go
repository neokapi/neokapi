package xliff_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/neokapi/neokapi/core/formats/xliff"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// readXliff drives the XLIFF reader over input without ever calling t.Fatal, so
// it is safe on arbitrary fuzz input.
func readXliff(ctx context.Context, input []byte) (parts []*model.Part, hadErr bool) {
	reader := xliff.NewReader()
	if err := reader.Open(ctx, testutil.RawDocFromString(string(input), model.LocaleEnglish)); err != nil {
		return nil, true
	}
	defer reader.Close()
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			hadErr = true
			continue
		}
		if result.Part != nil {
			parts = append(parts, result.Part)
		}
	}
	return parts, hadErr
}

func xliffSourceTexts(parts []*model.Part) []string {
	var texts []string
	for _, b := range testutil.FilterBlocks(parts) {
		texts = append(texts, b.SourceText())
	}
	sort.Strings(texts)
	return texts
}

func xliffSeed(f *testing.F, names ...string) {
	f.Helper()
	for _, name := range names {
		if data, err := os.ReadFile(filepath.Join("testdata", name)); err == nil {
			f.Add(data)
		}
	}
}

// FuzzReadXliff asserts the XLIFF reader never panics and always terminates on
// arbitrary input — malformed XLIFF must surface as a channel error.
func FuzzReadXliff(f *testing.F) {
	xliffSeed(f, "simple.xlf")
	f.Add([]byte(`<?xml version="1.0"?><xliff version="1.2"><file><body></body></file></xliff>`))
	f.Add([]byte(``))
	f.Add([]byte(`<xliff><file><body><trans-unit id="1"><source>hi</source></trans-unit></body></file></xliff>`))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = readXliff(t.Context(), data)
	})
}

// FuzzRoundTripXliff asserts read → write → read is crash-free and idempotent
// in shape for XLIFF. The XML round-trip-mutation class (where decode/encode
// silently rewrites markup) is exactly the threat this catches against
// neokapi's faithfulness invariant.
func FuzzRoundTripXliff(f *testing.F) {
	xliffSeed(f, "simple.xlf")
	f.Add([]byte(`<?xml version="1.0"?><xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2"><file original="x" source-language="en" datatype="plaintext"><body><trans-unit id="1"><source>Hello</source></trans-unit></body></file></xliff>`))

	f.Fuzz(func(t *testing.T, data []byte) {
		ctx := t.Context()
		parts1, hadErr := readXliff(ctx, data)
		if hadErr || len(testutil.FilterBlocks(parts1)) == 0 {
			return
		}
		texts1 := xliffSourceTexts(parts1)

		var buf bytes.Buffer
		writer := xliff.NewWriter()
		if err := writer.SetOutputWriter(&buf); err != nil {
			return
		}
		if err := writer.Write(ctx, testutil.PartsToChannel(parts1)); err != nil {
			return
		}

		parts2, hadErr2 := readXliff(ctx, buf.Bytes())
		if hadErr2 {
			t.Fatalf("re-reading written XLIFF failed; round-trip is not stable\ninput:  %q\noutput: %q", data, buf.Bytes())
		}
		texts2 := xliffSourceTexts(parts2)
		if len(texts1) != len(texts2) {
			t.Fatalf("round-trip changed block count: %d -> %d\ninput:  %q\noutput: %q", len(texts1), len(texts2), data, buf.Bytes())
		}
		for i := range texts1 {
			if texts1[i] != texts2[i] {
				t.Fatalf("round-trip drift in source text\npass1: %q\npass2: %q\ninput: %q\noutput: %q", texts1, texts2, data, buf.Bytes())
			}
		}
	})
}
