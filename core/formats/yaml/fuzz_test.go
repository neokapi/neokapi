package yaml_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	yamlfmt "github.com/neokapi/neokapi/core/formats/yaml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// fuzzReadYAML drives the YAML reader over input without ever calling t.Fatal, so
// it is safe on arbitrary fuzz input.
func fuzzReadYAML(ctx context.Context, input []byte) (parts []*model.Part, hadErr bool) {
	reader := yamlfmt.NewReader()
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

func yamlSourceTexts(parts []*model.Part) []string {
	var texts []string
	for _, b := range testutil.FilterBlocks(parts) {
		texts = append(texts, b.SourceText())
	}
	sort.Strings(texts)
	return texts
}

func yamlSeed(f *testing.F, names ...string) {
	f.Helper()
	for _, name := range names {
		if data, err := os.ReadFile(filepath.Join("testdata", name)); err == nil {
			f.Add(data)
		}
	}
}

// seedDamagedYAML adds documents that are structurally damaged but still
// PARSEABLE — the region where a lenient parser silently loses content rather
// than erroring. See the seedDamagedPO comment in core/formats/po/fuzz_test.go
// for why this matters (#1588): a fuzzer seeded only with well-formed documents
// explores "malformed and correctly rejected" and rarely reaches here.
//
// YAML's structural carriers are indentation and key uniqueness, so those are
// what these seeds damage.
func seedDamagedYAML(f *testing.F) {
	f.Helper()
	for _, s := range []string{
		// Indentation that jumps two levels at once but still parses.
		"a:\n      deep: value\nb: top\n",
		// A key whose value is an empty block — the following keys may get
		// absorbed as children.
		"a:\nb: value\nc: value\n",
		// Anchor and alias: the alias duplicates content by reference, so a
		// naive extractor can double-count or drop it.
		"base: &anchor Shared text\nuse: *anchor\n",
		// Explicit document separators with content in each.
		"a: one\n---\nb: two\n",
		// A block scalar that swallows following lines into its body.
		"desc: |\n  line one\n  line two\nnext: value\n",
		// A folded scalar with an indentation indicator.
		"desc: >2\n   folded text\nnext: value\n",
		// Quoted key with a colon inside it.
		"\"a: b\": value\nc: value\n",
		// A merge key.
		"defaults: &d\n  title: Base\nitem:\n  <<: *d\n  title: Override\n",
		// Trailing content after a flow mapping closes.
		"a: {k: v} trailing\n",
	} {
		f.Add([]byte(s))
	}
}

// seedDuplicateKeyYAML holds the duplicate-key documents. The reader extracts a
// block per key occurrence and the skeleton write path reproduces the document
// byte-for-byte, both always correct. The rebuild path (no skeleton store) used
// to key its block map by block.Name — the YAML key path — so a repeated key
// collapsed and one already-translated string was silently dropped (#1597). It
// now keeps one entry per block, so these belong in the round-trip target too.
func seedDuplicateKeyYAML(f *testing.F) {
	f.Helper()
	f.Add([]byte("title: First\ntitle: Second\n"))
	f.Add([]byte("a:\n  k: one\n  k: two\n"))
	f.Add([]byte("k: a\nk: b\nk: c\n"))
	f.Add([]byte("k: one\nother: x\nk: two\n"))
}

// FuzzReadYaml asserts the YAML reader never panics and always terminates on
// arbitrary input.
func FuzzReadYaml(f *testing.F) {
	yamlSeed(f, "simple.yaml")
	f.Add([]byte("key: value\n"))
	f.Add([]byte(""))
	f.Add([]byte("list:\n  - a\n  - b\n"))
	f.Add([]byte("a:\n  b:\n    c: deep\n"))
	seedDamagedYAML(f)
	seedDuplicateKeyYAML(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = fuzzReadYAML(t.Context(), data)
	})
}

// FuzzRoundTripYaml asserts read → write → read is stable in content: a
// document that parses cleanly, written back and re-read, yields the same
// source texts. The writer rebuilds from blocks when no skeleton store is
// wired, so formatting legitimately changes; the text must not.
func FuzzRoundTripYaml(f *testing.F) {
	yamlSeed(f, "simple.yaml")
	f.Add([]byte("title: Hello\ndesc: World\n"))
	f.Add([]byte("nested:\n  key: Nested value\n"))
	// #1630: a block whose text begins with a newline must survive the
	// rebuild path — the default style drops the leading blank line.
	f.Add([]byte("a: \"\\nx\"\n"))
	seedDamagedYAML(f)
	seedDuplicateKeyYAML(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		ctx := t.Context()
		parts1, hadErr := fuzzReadYAML(ctx, data)
		if hadErr || len(testutil.FilterBlocks(parts1)) == 0 {
			return
		}
		texts1 := yamlSourceTexts(parts1)

		var buf bytes.Buffer
		writer := yamlfmt.NewWriter()
		if err := writer.SetOutputWriter(&buf); err != nil {
			return
		}
		if err := writer.Write(ctx, testutil.PartsToChannel(parts1)); err != nil {
			return
		}
		writer.Close()

		parts2, hadErr2 := fuzzReadYAML(ctx, buf.Bytes())
		if hadErr2 {
			t.Fatalf("re-reading written YAML failed; round-trip is not stable\ninput:  %q\noutput: %q", data, buf.Bytes())
		}
		texts2 := yamlSourceTexts(parts2)
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
