package json_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	jsonfmt "github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// readParts drives the JSON reader over input and returns the parts it emitted
// plus whether any PartResult.Error surfaced. It never calls t.Fatal, so it is
// safe to call from a fuzz target on arbitrary (malformed) input: the contract
// under test is "no panic, bounded resources, errors surface on the channel".
func readParts(ctx context.Context, input []byte) (parts []*model.Part, hadErr bool) {
	reader := jsonfmt.NewReader()
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

func sourceTexts(parts []*model.Part) []string {
	var texts []string
	for _, b := range testutil.FilterBlocks(parts) {
		texts = append(texts, b.SourceText())
	}
	sort.Strings(texts)
	return texts
}

// seedFromFixtures registers the package's valid testdata fixtures as fuzz
// seeds so the corpus always starts from real, parseable documents.
func seedFromFixtures(f *testing.F, names ...string) {
	f.Helper()
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			continue
		}
		f.Add(data)
	}
}

// FuzzReadJson asserts the JSON reader never panics and always terminates
// (bounded resources) on arbitrary input — malformed input must surface as a
// channel error, not a crash or hang.
func FuzzReadJson(f *testing.F) {
	seedFromFixtures(f, "simple.json")
	f.Add([]byte(`{"a":"b"}`))
	f.Add([]byte(`[1,2,3]`))
	f.Add([]byte(``))
	f.Add([]byte(`{"nested":{"deep":{"deeper":"value"}}}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		// A panic here fails the fuzz input automatically; we just must not
		// hang. The reader closes its channel via defer, so the range
		// terminates.
		_, _ = readParts(t.Context(), data)
	})
}

// FuzzRoundTripJson asserts read → write → read is crash-free and idempotent in
// shape: when the input parses cleanly and the writer round-trips it, the set
// of source texts recovered on the second read equals the first. This catches
// the round-trip-drift class (content silently mutated across a read/write
// cycle) in addition to crashes.
func FuzzRoundTripJson(f *testing.F) {
	seedFromFixtures(f, "simple.json")
	f.Add([]byte(`{"title":"Hello","desc":"World"}`))
	f.Add([]byte(`{"items":["a","b","c"]}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		ctx := t.Context()
		parts1, hadErr := readParts(ctx, data)
		if hadErr || len(testutil.FilterBlocks(parts1)) == 0 {
			return // not a clean parse with content; only the no-panic contract applies
		}
		texts1 := sourceTexts(parts1)

		var buf bytes.Buffer
		writer := jsonfmt.NewWriter()
		if err := writer.SetOutputWriter(&buf); err != nil {
			return
		}
		if err := writer.Write(ctx, testutil.PartsToChannel(parts1)); err != nil {
			return // writer legitimately rejected reconstructed parts; no panic = pass
		}

		parts2, hadErr2 := readParts(ctx, buf.Bytes())
		if hadErr2 {
			t.Fatalf("re-reading written JSON failed; round-trip is not stable\ninput:  %q\noutput: %q", data, buf.Bytes())
		}
		texts2 := sourceTexts(parts2)
		if !slicesEqual(texts1, texts2) {
			t.Fatalf("round-trip drift in source texts\ninput:  %q\noutput: %q\npass1:  %q\npass2:  %q", data, buf.Bytes(), texts1, texts2)
		}
	})
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
