package mdx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The invariants here translate prose blocks of a real MDX document to a target
// locale, then assert spec-level properties on the OUTPUT and that it
// RE-PARSES cleanly through the Reader. They use only the stdlib + testify and
// the format's public API, so they always run. (Validity against the real
// @mdx-js/mdx compiler is asserted separately in the tool-gated
// acceptance_test.go.)

// opaqueRegions returns every opaque MDX region's verbatim content (the bytes
// the reader preserves for ESM, JSX, expressions, tables, and code-fence /
// markdown-opaque fallbacks), keyed by nothing — just the ordered slice.
func opaqueRegions(parts []*model.Part) []string {
	var out []string
	for _, p := range parts {
		if p.Type != model.PartData {
			continue
		}
		d, ok := p.Resource.(*model.Data)
		if !ok {
			continue
		}
		if c, ok := d.Properties["content"]; ok {
			out = append(out, c)
		}
	}
	return out
}

// proseBlockTexts returns the source text of every translatable prose block.
func proseBlockTexts(parts []*model.Part) []string {
	var out []string
	for _, p := range parts {
		if p.Type == model.PartBlock {
			out = append(out, p.Resource.(*model.Block).SourceText())
		}
	}
	return out
}

// pseudoTranslate produces a realistic target for a block: it copies the
// source RUNS and rewrites only the TEXT runs (wrapping their content in « »),
// preserving every protected inline-code / placeholder run verbatim. This
// mirrors what a real translation tool does — translate prose, round-trip the
// protected inline codes (Markdown link/code-span markers, etc.) untouched.
//
// A naive flatten-to-one-text-run translation would DROP those markers and, for
// docs prose that contains literal angle-bracket placeholders inside inline
// code (e.g. `<dir-name>`), would turn them into bare `<dir-name>` text that the
// MDX compiler then parses as an unclosed JSX tag. Preserving runs avoids that
// and matches production behaviour. Returns the rendered target text.
func pseudoTranslate(b *model.Block, locale model.LocaleID) string {
	srcRuns := b.SourceRuns()
	tgtRuns := make([]model.Run, len(srcRuns))
	copy(tgtRuns, srcRuns)
	for i := range tgtRuns {
		if tgtRuns[i].Text != nil {
			t := tgtRuns[i].Text.Text
			if t != "" {
				tgtRuns[i].Text = &model.TextRun{Text: "«" + t + "»"}
			}
		}
	}
	b.SetTargetRuns(locale, tgtRuns)
	return model.RenderRunsWithData(tgtRuns)
}

// TestInvariantProseTranslatedStructurePreserved translates every prose block
// of a real corpus MDX document and asserts:
//
//   - the output RE-PARSES cleanly through the Reader;
//   - every opaque region (ESM / JSX / expression / table / code fence) is
//     present BYTE-IDENTICAL in the output, in the same order;
//   - the translated prose appears in the output and the source prose is gone;
//   - the re-parsed opaque-region set is identical to the source's, proving the
//     translation touched only prose.
func TestInvariantProseTranslatedStructurePreserved(t *testing.T) {
	// Pick a corpus file rich in opaque constructs (imports + tables + code).
	path := filepath.Join("testdata", "corpus", "website-translation.mdx")
	src, err := os.ReadFile(path)
	require.NoError(t, err)

	parts, store := readParts(t, src)
	srcOpaque := opaqueRegions(parts)
	require.NotEmpty(t, srcOpaque, "expected opaque regions in the corpus document")

	const fr = model.LocaleID("fr-FR")
	var translated []string
	srcProse := proseBlockTexts(parts)
	require.NotEmpty(t, srcProse, "expected translatable prose in the corpus document")
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		// Only translate prose that contains a letter (skip pure-symbol blocks).
		if strings.ContainsAny(b.SourceText(), "abcdefghijklmnopqrstuvwxyz") {
			translated = append(translated, pseudoTranslate(b, fr))
		}
	}
	require.NotEmpty(t, translated, "expected at least one prose block to translate")

	out := writeParts(t, parts, store, fr)
	s := string(out)

	// Invariant 1: every opaque region survives byte-identical, in order.
	prev := 0
	for _, region := range srcOpaque {
		idx := strings.Index(s[prev:], region)
		assert.GreaterOrEqual(t, idx, 0,
			"opaque region must survive verbatim in translated output: %q", truncate(region))
		if idx >= 0 {
			prev += idx + len(region)
		}
	}

	// Invariant 2: the translated prose appears; representative source prose is
	// replaced (the « » markers prove the splice happened in place).
	assert.Contains(t, s, "«", "translated prose markers must appear in the output")
	for _, tr := range translated {
		assert.Contains(t, s, tr, "each translated prose block must be spliced into the output")
	}

	// Invariant 3: the output RE-PARSES cleanly and yields the SAME opaque-region
	// set (translation touched only prose).
	rrParts, _ := readParts(t, out)
	assert.Equal(t, srcOpaque, opaqueRegions(rrParts),
		"the opaque-region set must be identical after translate→write→re-parse")
}

// TestInvariantCodeFenceAndTableByteIdentical drills into the two highest-risk
// opaque constructs — fenced code blocks and GFM tables — on a real corpus file
// that contains both, asserting they appear in the translated output exactly as
// in the source (kapi must never normalise code or table padding under
// translation).
func TestInvariantCodeFenceAndTableByteIdentical(t *testing.T) {
	path := filepath.Join("testdata", "corpus", "kapi-cli-bilingual-workflow.mdx")
	src, err := os.ReadFile(path)
	require.NoError(t, err)

	// Extract the raw code-fence and table line spans from the source for later
	// equality checks.
	lines := strings.Split(string(src), "\n")
	var codeLines, tableLines []string
	inFence := false
	for _, ln := range lines {
		if strings.HasPrefix(ln, "```") {
			inFence = !inFence
			codeLines = append(codeLines, ln)
			continue
		}
		if inFence {
			codeLines = append(codeLines, ln)
		}
		if strings.HasPrefix(ln, "|") {
			tableLines = append(tableLines, ln)
		}
	}
	require.NotEmpty(t, codeLines, "fixture should contain fenced code")
	require.NotEmpty(t, tableLines, "fixture should contain a GFM table")

	parts, store := readParts(t, src)
	const fr = model.LocaleID("fr-FR")
	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			if strings.ContainsAny(b.SourceText(), "abcdefghijklmnopqrstuvwxyz") {
				pseudoTranslate(b, fr)
			}
		}
	}
	out := string(writeParts(t, parts, store, fr))

	for _, ln := range codeLines {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		assert.Contains(t, out, ln, "code-fence line must be byte-identical in translated output: %q", ln)
	}
	for _, ln := range tableLines {
		assert.Contains(t, out, ln, "table line (incl. cell padding) must be byte-identical: %q", ln)
	}
}

// TestInvariantInlineCodeMarkupPreservedUnderTranslation guards the specific
// real-world hazard the consumer-acceptance test surfaced: docs prose often
// contains literal angle-bracket placeholders inside inline code, e.g.
// `<dir-name>.kapi`. The inline-code backticks must survive translation; if
// they were dropped, `<dir-name>` would become a bare token the MDX compiler
// parses as an unclosed JSX tag. This test translates the prose with the
// inline-code runs preserved (as a real translation tool does) and asserts the
// backtick-fenced token reappears intact in the output.
func TestInvariantInlineCodeMarkupPreservedUnderTranslation(t *testing.T) {
	path := filepath.Join("testdata", "corpus", "bowrain-init.mdx")
	src, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(src), "`<dir-name>.kapi`",
		"fixture precondition: inline-code angle-bracket placeholder present")

	parts, store := readParts(t, src)
	const fr = model.LocaleID("fr-FR")
	var found bool
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		if strings.Contains(b.SourceText(), "<dir-name>.kapi") {
			pseudoTranslate(b, fr)
			found = true
		}
	}
	require.True(t, found, "expected to find the block carrying `<dir-name>.kapi`")

	out := string(writeParts(t, parts, store, fr))

	// The inline-code delimiters (backticks) must survive so the angle-bracket
	// placeholder stays FENCED. The code-span content itself is translatable
	// text, so a real translation may rewrite it (here wrapped in « »); what
	// must NOT happen is the backticks being dropped, leaving a bare
	// `<dir-name>` token the MDX compiler parses as an unclosed JSX tag.
	idx := strings.Index(out, "<dir-name>")
	require.GreaterOrEqual(t, idx, 0, "placeholder must still be present")
	// The nearest backtick before the placeholder must be closer than the
	// nearest newline before it, i.e. the placeholder sits inside a backtick
	// code span on its line.
	before := out[:idx]
	lastTick := strings.LastIndex(before, "`")
	lastNL := strings.LastIndex(before, "\n")
	assert.Greater(t, lastTick, lastNL,
		"the <dir-name> placeholder must remain inside a backtick code span after translation")
	after := out[idx:]
	assert.GreaterOrEqual(t, strings.Index(after, "`"), 0,
		"a closing backtick must follow the placeholder")
}

// truncate shortens a string for assertion messages.
func truncate(s string) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) > 60 {
		return s[:60] + "…"
	}
	return s
}
