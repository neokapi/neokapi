package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parsePO is a small round-trip helper — extract writes a PO, the test
// parses it back with the merge parser, asserts the shape.
func TestPO_WriteAndParseRoundTrip(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "test.po")
	f, err := os.Create(out)
	require.NoError(t, err)

	blocks := []*model.Block{
		{
			ID:           "tu1",
			Translatable: true,
			Source:       []model.Run{{Text: &model.TextRun{Text: "Hello"}}},
			Targets: map[model.VariantKey]*model.Target{
				model.Variant("fr-FR"): {Runs: []model.Run{{Text: &model.TextRun{Text: "Bonjour"}}}},
			},
			Properties: map[string]string{"kapi-tm-match": "exact"},
		},
		{
			ID:           "tu2",
			Translatable: true,
			Source:       []model.Run{{Text: &model.TextRun{Text: "Goodbye"}}},
			Targets: map[model.VariantKey]*model.Target{
				model.Variant("fr-FR"): {Runs: []model.Run{{Text: &model.TextRun{Text: "Au revoir"}}}},
			},
			Properties: map[string]string{"kapi-tm-match": "fuzzy"},
		},
	}

	require.NoError(t, writePOExtract(f, "fr-FR", "batch-xyz", "src/en/app.json", "sha256:abc", blocks))
	require.NoError(t, f.Close())

	parsed, err := readPOForMerge(out)
	require.NoError(t, err)
	assert.Equal(t, "batch-xyz", parsed.BatchID)
	assert.Equal(t, "src/en/app.json", parsed.SourceFile)
	assert.Equal(t, "sha256:abc", parsed.SourceHash)
	require.Len(t, parsed.Blocks, 2)

	// Block 1 is the exact match — no fuzzy flag.
	assert.Equal(t, "tu1", parsed.Blocks[0].BlockID)
	assert.Equal(t, "Hello", parsed.Blocks[0].MsgID)
	assert.Equal(t, "Bonjour", parsed.Blocks[0].MsgStr)
	assert.False(t, parsed.Blocks[0].Fuzzy)

	// Block 2 is fuzzy-prefilled — flag must be present.
	assert.Equal(t, "tu2", parsed.Blocks[1].BlockID)
	assert.True(t, parsed.Blocks[1].Fuzzy)

	// Raw file also contains the kapi sentinel comments.
	raw, err := os.ReadFile(out)
	require.NoError(t, err)
	s := string(raw)
	assert.Contains(t, s, "#. kapi-batch: batch-xyz")
	assert.Contains(t, s, "#. kapi-source-file: src/en/app.json")
	assert.Contains(t, s, "#. kapi-source-hash: sha256:abc")
	assert.Contains(t, s, "#. kapi-block: tu1")
	assert.Contains(t, s, "#, fuzzy")
}

func TestPO_UnquoteHandlesEscapes(t *testing.T) {
	cases := map[string]string{
		`"hello"`:          "hello",
		`"line1\nline2"`:   "line1\nline2",
		`"tab\there"`:      "tab\there",
		`"quote\"inside"`:  `quote"inside`,
		`"back\\slash"`:    `back\slash`,
		`"empty is empty"`: "empty is empty",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, poUnquote(in))
		})
	}
}

func TestExtract_POFormatWritesFileWithBatchComments(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := extractProjectFixture(t, real, []model.LocaleID{"fr-FR"})
	writeJSONSource(t, real, "src/locales/en/app.json", `{"k":"Hello"}`)

	out, err := runExtractCmd(t, recipe, "--format", "po")
	require.NoError(t, err, "stdout: %s", out)

	entries, err := os.ReadDir(filepath.Join(real, "out"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	// Naming: .po extension.
	assert.True(t, strings.HasSuffix(entries[0].Name(), ".po"), "got %s", entries[0].Name())

	raw, err := os.ReadFile(filepath.Join(real, "out", entries[0].Name()))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "#. kapi-batch: ")
	assert.Contains(t, string(raw), "#. kapi-source-file: src/locales/en/app.json")
	assert.Contains(t, string(raw), "#. kapi-source-hash: sha256:")
	assert.Contains(t, string(raw), `msgid "Hello"`)
}

// End-to-end: PO extract → simulate translator → PO merge → verify
// translated target source has the translation.
func TestMerge_POFormatRoundTrip(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := mergeProjectFixture(t, real)
	writeJSONSource(t, real, "src/locales/en/app.json", `{"greeting":"Hello"}`)

	// Extract with --format po.
	out, err := runExtractCmd(t, recipe, "--format", "po")
	require.NoError(t, err, "extract stdout: %s", out)

	// Edit the msgstr.
	entries, err := os.ReadDir(filepath.Join(real, "out"))
	require.NoError(t, err)
	poPath := filepath.Join(real, "out", entries[0].Name())
	raw, err := os.ReadFile(poPath)
	require.NoError(t, err)
	// Target the "Hello" entry specifically — the first `msgstr ""` in the
	// file is on the PO header and must stay empty.
	const beforePair = "msgid \"Hello\"\nmsgstr \"\""
	const afterPair = "msgid \"Hello\"\nmsgstr \"Bonjour\""
	require.Contains(t, string(raw), beforePair, "PO extract should leave msgstr empty for new strings")
	edited := strings.Replace(string(raw), beforePair, afterPair, 1)
	require.NoError(t, os.WriteFile(poPath, []byte(edited), 0o644))

	// Merge.
	mergeOut, err := runMergeCmd(t, recipe, "-i", poPath, "--no-tm-update")
	require.NoError(t, err, "merge stdout: %s", mergeOut)

	// Target file at src/locales/fr-FR/app.json.
	mergedPath := filepath.Join(real, "src", "locales", "fr-FR", "app.json")
	data, err := os.ReadFile(mergedPath)
	require.NoError(t, err, "expected merged file at %s", mergedPath)
	assert.Contains(t, string(data), "Bonjour")
}

// PO extract must refuse when segmentation is on — follow-up for #422.
func TestExtract_POFormatErrorsWhenSegmentationOn(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := filepath.Join(real, "app.kapi")
	proj := &project.KapiProject{
		Version: "v1",
		Name:    "POSeg",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"fr-FR"},
			Segmentation:    project.SegmentationDefaults{Source: true},
		},
		Content: []project.ContentCollection{
			{Path: "src/locales/en/*.json", Format: &project.FormatSpec{Name: "json"}},
		},
	}
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(real, project.StateDirName), 0o755))
	writeJSONSource(t, real, "src/locales/en/app.json",
		`{"k":"This is a sentence. Another sentence."}`)

	// PO extract with segmentation-on should fail hard — the inner
	// segmentation-on error is printed to os.Stderr as a per-pair
	// failure, then Execute returns an aggregate error. We assert the
	// failure mode without coupling to exact stderr wording.
	out, err := runExtractCmd(t, recipe, "--format", "po")
	require.Error(t, err, "expected PO+segmentation combo to error; stdout: %s", out)
	assert.Contains(t, err.Error(), "failed")
	// No .po file is produced for the failing pair.
	poEntries, _ := os.ReadDir(filepath.Join(real, "out"))
	for _, e := range poEntries {
		assert.NotEqual(t, ".po", filepath.Ext(e.Name()), "no PO output should be written when the pair failed: %s", e.Name())
	}
}
