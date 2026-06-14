package doclang_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	doclangfmt "github.com/neokapi/neokapi/core/formats/doclang"
	doclingfmt "github.com/neokapi/neokapi/core/formats/docling"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// Conformance: validate the DocLang writer's output against the OFFICIAL DocLang
// XML Schema (vendored from the standard, testdata/conformance/doclang.xsd) with
// xmllint. This is the external oracle for "is our DocLang actually valid
// DocLang" — without it, our own tests only prove the writer is self-consistent.
// Building this gate is what surfaced three real writer bugs (RoleTitle emitted
// as a non-existent <title> body element; a table caption emitted as an illegal
// <text> child of <table>; a list child not preceded by <ldiv/>); each case
// below is a regression guard for one of them.
//
// The test self-skips when xmllint (libxml2) is absent so `go test ./...` stays
// green on minimal machines; CI installs libxml2-utils so the gate is real
// there (see the docs-/parity-style "ensure the tool is present" note).

const vendoredXSD = "testdata/conformance/doclang.xsd"

func xmllintPath(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("xmllint")
	if err != nil {
		t.Skip("xmllint (libxml2) not found — skipping DocLang XSD validation")
	}
	return p
}

// renderDocLang reads `data` with `reader`, then serializes the resulting Part
// stream through the DocLang writer, returning the produced XML.
func renderDocLang(t *testing.T, reader format.DataFormatReader, data string) []byte {
	t.Helper()
	ctx := context.Background()
	if err := reader.Open(ctx, testutil.RawDocFromString(data, model.LocaleEnglish)); err != nil {
		t.Fatalf("open reader: %v", err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	parts := testutil.CollectParts(t, reader.Read(ctx))

	w := doclangfmt.NewWriter()
	var buf bytes.Buffer
	if err := w.SetOutputWriter(&buf); err != nil {
		t.Fatalf("set writer output: %v", err)
	}
	ch := make(chan *model.Part, len(parts)+1)
	for _, p := range parts {
		ch <- p
	}
	close(ch)
	if err := w.Write(ctx, ch); err != nil {
		t.Fatalf("doclang write: %v", err)
	}
	return buf.Bytes()
}

// assertValidDocLang writes xml to a temp file and validates it against the
// vendored XSD with xmllint, failing with the validator's message on error.
func assertValidDocLang(t *testing.T, xmllint string, xml []byte) {
	t.Helper()
	xsd, err := filepath.Abs(vendoredXSD)
	if err != nil {
		t.Fatalf("resolve xsd: %v", err)
	}
	f := filepath.Join(t.TempDir(), "out.dclg.xml")
	if err := os.WriteFile(f, xml, 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	out, err := exec.CommandContext(t.Context(), xmllint, "--noout", "--schema", xsd, f).CombinedOutput()
	if err != nil {
		t.Errorf("DocLang output is not XSD-valid:\n%s\n--- output ---\n%s", out, xml)
	}
}

// TestDocLangWriter_RoundTripIsSchemaValid: reading each vendored upstream
// DocLang fixture and writing it back must produce schema-valid DocLang.
func TestDocLangWriter_RoundTripIsSchemaValid(t *testing.T) {
	xmllint := xmllintPath(t)
	fixtures, err := filepath.Glob("testdata/corpus/*.dclg.xml")
	if err != nil || len(fixtures) == 0 {
		t.Fatalf("no vendored DocLang corpus fixtures (%v)", err)
	}
	for _, fx := range fixtures {
		t.Run(filepath.Base(fx), func(t *testing.T) {
			data, err := os.ReadFile(fx)
			if err != nil {
				t.Fatal(err)
			}
			out := renderDocLang(t, doclangfmt.NewReader(), string(data))
			assertValidDocLang(t, xmllint, out)
		})
	}
}

// TestDocLangWriter_NativeProjectionIsSchemaValid: projecting native sources
// (here DoclingDocument JSON, which carries titles + captioned tables — the
// exact shapes that triggered the writer bugs) to DocLang must be schema-valid.
func TestDocLangWriter_NativeProjectionIsSchemaValid(t *testing.T) {
	xmllint := xmllintPath(t)
	// Our own sample exercises a title + a captioned OTSL table + a picture
	// caption in one document; the vendored corpus adds more real shapes.
	inputs := []string{
		"../docling/testdata/sample.docling.json",
		"../docling/testdata/corpus/flattened.json",
		"../docling/testdata/corpus/page_without_pic.json",
	}
	for _, in := range inputs {
		t.Run(filepath.Base(in), func(t *testing.T) {
			data, err := os.ReadFile(in)
			if err != nil {
				t.Fatal(err)
			}
			out := renderDocLang(t, doclingfmt.NewReader(), string(data))
			assertValidDocLang(t, xmllint, out)
		})
	}
}
