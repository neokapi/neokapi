package po_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/neokapi/neokapi/core/formats/po"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// readPO drives the PO reader over input without ever calling t.Fatal, so it is
// safe on arbitrary fuzz input. testutil.CollectParts is deliberately not used:
// it require.NoError's every PartResult, which turns an expected parse error
// into a fuzz failure.
func readPO(ctx context.Context, input []byte) (parts []*model.Part, hadErr bool) {
	reader := po.NewReader()
	doc := testutil.RawDocFromString(string(input), model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	if err := reader.Open(ctx, doc); err != nil {
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

func poSourceTexts(parts []*model.Part) []string {
	var texts []string
	for _, b := range testutil.FilterBlocks(parts) {
		texts = append(texts, b.SourceText())
	}
	sort.Strings(texts)
	return texts
}

func poSeed(f *testing.F, names ...string) {
	f.Helper()
	for _, name := range names {
		if data, err := os.ReadFile(filepath.Join("testdata", name)); err == nil {
			f.Add(data)
		}
	}
}

// seedDamagedPO adds documents that are structurally damaged but still
// PARSEABLE. This is the corpus shape that matters here, and it is not what a
// fuzzer reaches on its own from well-formed seeds.
//
// #1588 is the reason. A one-byte change to a valid XLIFF file — corrupting the
// '<' of a closing tag — made the lenient tokenizer swallow every following
// unit: three units became one block, and nothing errored. The xliff package had
// both a read fuzzer and a round-trip fuzzer at the time and neither found it,
// because both were seeded only with well-formed documents. Random mutation
// almost always lands in "malformed, correctly rejected"; the interesting region
// is "still parses, but a separator or terminator is gone", where a lenient
// parser silently absorbs the rest of the file.
//
// PO's separator is the blank line between entries, and its terminator is the
// closing quote. Those are the analogues seeded below.
func seedDamagedPO(f *testing.F) {
	f.Helper()
	for _, s := range []string{
		// Entries run together with no blank-line separator. If the parser
		// treats the blank line as the entry boundary, this is the #1588 shape:
		// two entries silently becoming one.
		"msgid \"one\"\nmsgstr \"un\"\nmsgid \"two\"\nmsgstr \"deux\"\n",
		// Unterminated closing quote on the last msgstr — the rest of the file
		// is a candidate to be swallowed into the string.
		"msgid \"one\"\nmsgstr \"un\"\n\nmsgid \"two\"\nmsgstr \"deux\n",
		// msgstr with no msgid.
		"msgstr \"orphan\"\n",
		// Two msgids in a row, no msgstr between.
		"msgid \"a\"\nmsgid \"b\"\nmsgstr \"b\"\n",
		// A continuation line with nothing to continue.
		"\"dangling continuation\"\nmsgid \"a\"\nmsgstr \"b\"\n",
		// Duplicate msgid — last-wins would be silent content loss.
		"msgid \"dup\"\nmsgstr \"one\"\n\nmsgid \"dup\"\nmsgstr \"two\"\n",
		// Obsolete-entry marker mid-file; everything after it may be skipped.
		"msgid \"live\"\nmsgstr \"vivant\"\n\n#~ msgid \"dead\"\n#~ msgstr \"mort\"\n\nmsgid \"after\"\nmsgstr \"apres\"\n",
		// Header with a truncated charset declaration.
		"msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=\"\n\nmsgid \"a\"\nmsgstr \"b\"\n",
		// Plural entry missing msgstr[1].
		"msgid \"one\"\nmsgid_plural \"many\"\nmsgstr[0] \"un\"\n",
	} {
		f.Add([]byte(s))
	}
}

// seedSentinelPO holds a msgid containing a literal U+0001. It is seeded into
// the read target, where the reader handles it correctly, but NOT into the
// round-trip target: it fails there, and the failure is a real defect filed as
// #1600 rather than something to assert around.
//
// The writer brackets verbatim regions with escapeSkipStart = '\x01' and
// escapeSkipEnd = '\x02' (writer.go:593-599), on the stated assumption that
// these are "control bytes that never appear in PO source text". They can: the
// reader accepts a literal 0x01 inside a quoted msgid, so content and delimiter
// become indistinguishable and escapePO consumes the content byte. Sweeping
// every byte 0x01-0x7f through a msgid finds exactly this one does not survive.
func seedSentinelPO(f *testing.F) {
	f.Helper()
	f.Add([]byte("msgid \"a\x01b\"\nmsgstr \"x\"\n"))
}

// FuzzReadPo asserts the PO reader never panics and always terminates on
// arbitrary input. PO's Open does real work — byte-budgeted reads, BOM
// detection and charset transcoding — so this covers the encoding-detection
// surface as well as the entry parser.
func FuzzReadPo(f *testing.F) {
	poSeed(f, "simple.po")
	seedSentinelPO(f)
	f.Add([]byte("msgid \"Hello\"\nmsgstr \"Bonjour\"\n"))
	f.Add([]byte(""))
	f.Add([]byte("#: ref.js:1\nmsgid \"x\"\nmsgstr \"\"\n"))
	seedDamagedPO(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = readPO(t.Context(), data)
	})
}

// FuzzRoundTripPo asserts read → write → read is stable: a document that parses
// cleanly, once written back and re-read, yields the same source texts. Crashes
// are the floor; this is the property that actually matters for a format engine
// whose contract is faithful write-back.
func FuzzRoundTripPo(f *testing.F) {
	poSeed(f, "simple.po")
	f.Add([]byte("msgid \"Hello\"\nmsgstr \"Bonjour\"\n"))
	f.Add([]byte("msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\nmsgid \"a\"\nmsgstr \"b\"\n"))
	seedDamagedPO(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		ctx := t.Context()
		parts1, hadErr := readPO(ctx, data)
		if hadErr || len(testutil.FilterBlocks(parts1)) == 0 {
			return // only the no-panic contract applies to a non-clean parse
		}
		texts1 := poSourceTexts(parts1)

		var buf bytes.Buffer
		writer := po.NewWriter()
		if err := writer.SetOutputWriter(&buf); err != nil {
			return
		}
		writer.SetLocale(model.LocaleFrench)
		if err := writer.Write(ctx, testutil.PartsToChannel(parts1)); err != nil {
			return // writer legitimately rejected the parts; no panic = pass
		}
		writer.Close()

		parts2, hadErr2 := readPO(ctx, buf.Bytes())
		if hadErr2 {
			t.Fatalf("re-reading written PO failed; round-trip is not stable\ninput:  %q\noutput: %q", data, buf.Bytes())
		}
		texts2 := poSourceTexts(parts2)
		if len(texts1) != len(texts2) {
			t.Fatalf("round-trip changed block count: %d -> %d\ninput:  %q\noutput: %q\npass1:  %q\npass2:  %q",
				len(texts1), len(texts2), data, buf.Bytes(), texts1, texts2)
		}
		for i := range texts1 {
			if texts1[i] != texts2[i] {
				t.Fatalf("round-trip drift in source text\npass1: %q\npass2: %q\ninput: %q\noutput: %q",
					texts1, texts2, data, buf.Bytes())
			}
		}
	})
}
