package xliff2_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// TestRoundTrip_AllFixtures exercises the xliff2 reader+writer against
// every xliff2 fixture shipped by upstream okapi (both lib-xliff2 unit
// tests and the okapi-bridge integration suite). For each fixture the
// test runs a two-pass round-trip:
//
//	bytes  →  read  →  parts  →  write  →  pass1
//	pass1  →  read  →  parts2 →  write  →  pass2
//
// pass1 and pass2 must agree byte-for-byte. When the writer is idempotent
// on its own output (a "fixed point" round-trip), we have strong evidence
// that the reader and writer agree on the document's semantic content —
// no information is lost or rearranged across iterations. This is the
// xliff2 analogue of the okapi-reference parity check used for xliff 1.x
// (which isn't viable here — the okapi xliff2 round-trip is documented
// as lossy and the pseudo pipeline currently NPEs).
//
// pass1 itself is NOT compared against the original fixture because the
// xliff2 writer is DOM-based and intentionally normalizes formatting,
// attribute order, and namespace declarations — exact byte preservation
// of the original is not a goal (per the format owner's design).
//
// Failures here mean either the reader silently dropped data the writer
// can't reconstruct, or the writer emits non-idempotent output that
// re-reading would interpret differently. Both are real bugs.
//
// okapi: RoundTripXliff2IT#xliff2Files
// RoundTripXliff2IT#xliff2Files (roundtrip.integration) extracts→merges→
// re-extracts every .xlf/.xliff/.xlf2 in the xliff2 corpus and asserts the
// events match. This native test runs the same upstream xliff2 corpus
// (lib-xliff2 unit fixtures plus integration-tests/okapi/.../xliff2/) through
// a two-pass read→write→read→write round-trip and asserts byte-equal idempotent
// output — the native analogue of the Okapi event-compare double extraction.
func TestRoundTrip_AllFixtures(t *testing.T) {
	fixtures := collectXliff2Fixtures(t)
	if len(fixtures) == 0 {
		t.Skip("no xliff2 fixtures found under okapi-testdata; run `make parity-fetch` (or equivalent) to populate")
	}

	type result struct {
		name   string
		pass   bool
		reason string
		bytes1 int
		bytes2 int
	}
	var results []result

	for _, path := range fixtures {
		name, _ := filepath.Rel(fixtureRoot, path)
		t.Run(name, func(t *testing.T) {
			res := result{name: name}
			defer func() { results = append(results, res) }()

			raw, err := os.ReadFile(path)
			if err != nil {
				res.reason = "read fixture: " + err.Error()
				t.Fatal(res.reason)
			}

			pass1, err := readWrite(t, raw)
			if err != nil {
				res.reason = "pass 1: " + err.Error()
				t.Fatal(res.reason)
			}
			res.bytes1 = len(pass1)

			pass2, err := readWrite(t, pass1)
			if err != nil {
				res.reason = "pass 2: " + err.Error()
				t.Fatal(res.reason)
			}
			res.bytes2 = len(pass2)

			if !bytes.Equal(pass1, pass2) {
				res.reason = fmt.Sprintf("pass1 (%d bytes) != pass2 (%d bytes); writer is not idempotent", res.bytes1, res.bytes2)
				offset := firstByteDiff(pass1, pass2)
				if offset >= 0 {
					res.reason += fmt.Sprintf(" (first diff at offset %d)", offset)
				}
				t.Error(res.reason)
				return
			}
			res.pass = true
		})
	}

	// Summary report — useful when running with -v or as the only signal
	// from -run TestRoundTrip_AllFixtures$ aggregated runs.
	if !t.Failed() {
		return
	}
	fmt.Fprintln(os.Stderr, "\n# xliff2 self-round-trip report")
	fmt.Fprintln(os.Stderr, "| fixture | pass1 bytes | pass2 bytes | result |")
	fmt.Fprintln(os.Stderr, "|---|---:|---:|---|")
	for _, r := range results {
		status := "✓"
		if !r.pass {
			status = "✗ " + r.reason
		}
		fmt.Fprintf(os.Stderr, "| %s | %d | %d | %s |\n", r.name, r.bytes1, r.bytes2, status)
	}
}

// readWrite performs a single read → write pass: parses xliff2 bytes
// with neokapi's reader, then writes the parts back through neokapi's
// writer, returning the rendered bytes.
func readWrite(t *testing.T, in []byte) ([]byte, error) {
	t.Helper()
	ctx := context.Background()

	reader := xliff2.NewReader()
	if err := reader.Open(ctx, testutil.RawDocFromString(string(in), model.LocaleEnglish)); err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	var buf bytes.Buffer
	writer := xliff2.NewWriter()
	if err := writer.SetOutputWriter(&buf); err != nil {
		return nil, fmt.Errorf("setOutput: %w", err)
	}
	// No SetLocale — preserve whatever the source declared via trgLang.
	ch := testutil.PartsToChannel(parts)
	if err := writer.Write(ctx, ch); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}
	return buf.Bytes(), nil
}

// TestRoundTrip_ByteEqualUntouched is the v2 contract check: a single
// read → write pass on an unmodified XLIFF 2 input must produce
// byte-equal output. The writer's round-trip mode patches only segments
// where the model.Block's content differs from the source DOM; when
// nothing was modified, the source DOM is serialized verbatim.
//
// Failures here mean either the reader normalized something the writer
// didn't preserve, or the writer's "no patch needed" detection is
// over-eager. Test fixtures the reader-normalization wipes (e.g. CR
// entity refs collapsed to LF) are excluded from this check via
// notExpectedByteEqual — they still appear in TestRoundTrip_AllFixtures
// for the idempotency contract (pass1 == pass2).
func TestRoundTrip_ByteEqualUntouched(t *testing.T) {
	fixtures := collectXliff2Fixtures(t)
	if len(fixtures) == 0 {
		t.Skip("no xliff2 fixtures found under okapi-testdata")
	}
	excluded := notExpectedByteEqual()
	for _, path := range fixtures {
		name, _ := filepath.Rel(fixtureRoot, path)
		if _, skip := excluded[name]; skip {
			continue
		}
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			out, err := readWrite(t, raw)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(raw, out) {
				offset := firstByteDiff(raw, out)
				t.Errorf("byte-equal contract broken: input=%d bytes output=%d bytes (first diff at offset %d)",
					len(raw), len(out), offset)
			}
		})
	}
}

// notExpectedByteEqual lists fixtures where one or more reader
// normalizations make byte-equal output unattainable. Each entry has a
// brief reason; idempotency (pass1 == pass2) is still checked for all
// of them by TestRoundTrip_AllFixtures.
func notExpectedByteEqual() map[string]string {
	return map[string]string{
		// XML 1.1 declaration coerced to 1.0 on read (XLIFF 2 mandates 1.0).
		// The `<?xml version="1.1"?>` ProcInst is normalized to 1.0 by the
		// reader, so the byte form of the declaration can't round-trip; the
		// rest of the document is byte-identical.
		"integration-tests/okapi/src/test/resources/xliff2/original_en.xlf": "XML 1.1→1.0 coercion",
	}
}

// TestRoundTrip_CollidingUnitIDs pins the writer against units whose ids do
// not identify them uniquely. XLIFF 2 requires `id` on `<unit>` and requires it
// to be unique within the enclosing `<file>`, but kapi accepts documents that
// break either rule — they arrive from other tools, and in the packaged-handoff
// workflow from outside the project entirely.
//
// The writer resolved every unit element through a single id-keyed map, so a
// collision made one unit's content overwrite another's: not a dropped block
// but silent cross-unit corruption, on a document kapi had accepted, still
// valid XLIFF afterwards (#1599). There are two ways to collide — a repeated
// id, and an absent id, which keys every id-less unit on the empty string.
func TestRoundTrip_CollidingUnitIDs(t *testing.T) {
	const head = `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
`
	const tail = `  </file>
</xliff>
`
	cases := map[string]struct {
		units []string
		want  []string
	}{
		"repeated id": {
			units: []string{
				`<unit id="1"><segment><source>one</source></segment></unit>`,
				`<unit id="1"><segment><source>two</source></segment></unit>`,
			},
			want: []string{"one", "two"},
		},
		"absent id on every unit": {
			units: []string{
				`<unit><segment><source>0</source></segment></unit>`,
				`<unit><segment><source></source></segment></unit>`,
			},
			want: []string{"", "0"},
		},
		"absent id mixed with present": {
			units: []string{
				`<unit><segment><source>bare</source></segment></unit>`,
				`<unit id="u1"><segment><source>named</source></segment></unit>`,
				`<unit><segment><source>bare2</source></segment></unit>`,
			},
			want: []string{"bare", "bare2", "named"},
		},
		"three units sharing one id": {
			units: []string{
				`<unit id="x"><segment><source>a</source></segment></unit>`,
				`<unit id="x"><segment><source>b</source></segment></unit>`,
				`<unit id="x"><segment><source>c</source></segment></unit>`,
			},
			want: []string{"a", "b", "c"},
		},
		"collision inside a group": {
			units: []string{
				`<group id="g1">` +
					`<unit id="1"><segment><source>in-group-one</source></segment></unit>` +
					`<unit id="1"><segment><source>in-group-two</source></segment></unit>` +
					`</group>`,
			},
			want: []string{"in-group-one", "in-group-two"},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			input := []byte(head + "    " + strings.Join(tc.units, "\n    ") + "\n" + tail)

			pass1 := xliff2SourceTexts(t, input)
			if diff := cmpStrings(pass1, tc.want); diff != "" {
				t.Fatalf("read did not extract the expected units: %s", diff)
			}

			out, err := readWrite(t, input)
			if err != nil {
				t.Fatalf("readWrite: %v", err)
			}
			pass2 := xliff2SourceTexts(t, out)
			if diff := cmpStrings(pass2, tc.want); diff != "" {
				t.Errorf("round-trip changed unit content: %s\ninput:\n%s\noutput:\n%s", diff, input, out)
			}
		})
	}
}

// xliff2SourceTexts reads a document and returns its blocks' source texts,
// sorted so the comparison does not depend on emission order.
func xliff2SourceTexts(t *testing.T, in []byte) []string {
	t.Helper()
	ctx := context.Background()
	reader := xliff2.NewReader()
	if err := reader.Open(ctx, testutil.RawDocFromString(string(in), model.LocaleEnglish)); err != nil {
		t.Fatalf("open: %v", err)
	}
	defer reader.Close()
	var texts []string
	for _, b := range testutil.FilterBlocks(testutil.CollectParts(t, reader.Read(ctx))) {
		texts = append(texts, b.SourceText())
	}
	sort.Strings(texts)
	return texts
}

func cmpStrings(got, want []string) string {
	if len(got) == len(want) {
		same := true
		for i := range got {
			if got[i] != want[i] {
				same = false
				break
			}
		}
		if same {
			return ""
		}
	}
	return fmt.Sprintf("got %q, want %q", got, want)
}

// TestRoundTrip_StaleIRDetection verifies the writer's staleness
// auto-detection: when a tool modifies Segment.Runs without touching
// SegmentInlineAnnotation, the writer should still see the change and
// patch the DOM accordingly. Catches the footgun where the annotation
// would otherwise act as a stale "shadow" of the original content.
func TestRoundTrip_StaleIRDetection(t *testing.T) {
	const input = `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1">
      <segment id="s1">
        <source>Hello</source>
        <target>Bonjour</target>
      </segment>
    </unit>
  </file>
</xliff>`
	ctx := t.Context()

	reader := xliff2.NewReader()
	if err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)); err != nil {
		t.Fatal(err)
	}
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Mutate the target's Runs to "Salut" — but DON'T touch the
	// inline IR (UnitSegmentsAnnotation). The writer must detect this via
	// freshness comparison.
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		block := p.Resource.(*model.Block)
		for _, loc := range block.TargetLocales() {
			block.SetTargetRuns(loc, []model.Run{{Text: &model.TextRun{Text: "Salut"}}})
		}
	}

	var buf bytes.Buffer
	writer := xliff2.NewWriter()
	if err := writer.SetOutputWriter(&buf); err != nil {
		t.Fatal(err)
	}
	if err := writer.Write(ctx, testutil.PartsToChannel(parts)); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Salut") {
		t.Errorf("expected output to contain modified target 'Salut', got: %s", out)
	}
	if strings.Contains(out, "Bonjour") {
		t.Errorf("expected output NOT to contain stale 'Bonjour' (the writer trusted a stale annotation), got: %s", out)
	}
}

// fixtureRoot is the okapi-testdata subdirectory we walk for fixtures.
// Resolved at runtime (test binary cwd is the package dir).
var fixtureRoot string

// collectXliff2Fixtures returns absolute paths to every xliff2 fixture
// available under the okapi-testdata tree, walking both the lib-xliff2
// unit-test resources and the integration-test resources. Returns an
// empty slice when okapi-testdata isn't present (CI parity sandbox not
// populated) — callers should t.Skip in that case.
func collectXliff2Fixtures(t *testing.T) []string {
	t.Helper()
	repoRoot := findRepoRoot(t)
	if repoRoot == "" {
		return nil
	}
	candidates := []string{
		filepath.Join(repoRoot, "okapi-testdata", "1.48.0-v4", "okapi", "filters", "xliff2", "src", "test", "resources"),
		filepath.Join(repoRoot, "okapi-testdata", "1.48.0-v4", "integration-tests", "okapi", "src", "test", "resources", "xliff2"),
	}
	fixtureRoot = filepath.Join(repoRoot, "okapi-testdata", "1.48.0-v4")

	var out []string
	for _, dir := range candidates {
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".xlf" && ext != ".xlf2" {
				return nil
			}
			// Skip subfilter / gold / roundtrip directories — those carry
			// EXPECTED outputs from upstream okapi, not source fixtures.
			if strings.Contains(path, "/gold/") || strings.Contains(path, "/roundtrips/") {
				return nil
			}
			out = append(out, path)
			return nil
		})
	}
	sort.Strings(out)
	return out
}

// findRepoRoot walks up from the test binary's cwd to find the
// neokapi/ repo root (the dir containing okapi-testdata/). Returns ""
// when not found.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for range 10 {
		if _, err := os.Stat(filepath.Join(dir, "okapi-testdata")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
	return ""
}

// firstByteDiff returns the index of the first differing byte between
// a and b, or -1 when they are equal. Used for diagnostic messages
// when self round-trip detects writer non-idempotency.
func firstByteDiff(a, b []byte) int {
	n := min(len(b), len(a))
	for i := range n {
		if a[i] != b[i] {
			return i
		}
	}
	if len(a) != len(b) {
		return n
	}
	return -1
}
