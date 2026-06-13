package html_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// fuzzReadHTML drives the HTML reader over input without ever calling t.Fatal,
// so it is safe on arbitrary fuzz input. It returns whether any channel error
// surfaced; the contract under test is "no panic, bounded resources".
func fuzzReadHTML(ctx context.Context, input []byte) (parts []*model.Part, hadErr bool) {
	reader := htmlfmt.NewReader()
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

func htmlSeed(f *testing.F, names ...string) {
	f.Helper()
	for _, name := range names {
		if data, err := os.ReadFile(filepath.Join("testdata", name)); err == nil {
			f.Add(data)
		}
	}
}

// FuzzReadHtml asserts the HTML reader never panics and always terminates on
// arbitrary input. HTML is lenient by design (no "malformed" rejection), so the
// invariant is purely crash-freedom and bounded resources.
func FuzzReadHtml(f *testing.F) {
	htmlSeed(f, "simple.html", "inline_codes.html")
	f.Add([]byte(`<p>hello</p>`))
	f.Add([]byte(`<html><body><div><span>x</span></div></body></html>`))
	f.Add([]byte(``))
	f.Add([]byte(`<!-- unterminated`))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = fuzzReadHTML(t.Context(), data)
	})
}
