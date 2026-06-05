//go:build parity

// Package tools holds the per-step parity tests that exercise every
// Okapi pipeline step in the okapi-bridge legacy plugin manifest. The
// shape mirrors cli/parity/formats/ — one row per step in the spec
// table, driven by a single TestParityTools test.
//
// The bar for "passing" depends on whether a Go counterpart exists:
//
//   - HEAD-TO-HEAD (NewTool set): run both the Okapi step via the
//     bridge and the neokapi tool against the same input parts; assert
//     they agree on output.
//   - BRIDGE-ONLY (NewTool nil): assert the bridge step accepts the
//     input and returns a non-empty stream. This is a stability gate,
//     not a correctness gate.
//   - SKIP (Skip non-empty): record the row as skipped with a
//     tracking-issue reference. Used for steps that require a richer
//     pipeline context than ProcessStep alone offers (TM matches,
//     scoping reports, multi-doc aligners) until per-step harness
//     work brings them online.
package tools

// SkipNeedsContext marks steps that ProcessStep alone cannot exercise
// because they depend on pipeline-wide state (TM matches, batch
// statistics, multi-doc inputs, RawDocument access, …). Tracked
// upstream as #454.
const SkipNeedsContext = "step needs pipeline context not provided by ProcessStep — see #454"

// SkipNeedsParams marks steps whose default parameters force an
// unconfigured fast-failure (e.g. XSLT needs a stylesheet path; search
// & replace needs a pattern). Tracked under #454.
const SkipNeedsParams = "step needs required parameters — see #454"

// ToolSpec describes one step parity row.
type ToolSpec struct {
	ID         string
	StepParams map[string]string
	Skip       string
}

// toolSpecs is the canonical list of every step declared in the
// okapi-bridge dist/plugin/manifest.json. Pinned to Okapi 1.48.0 by
// intent.
var toolSpecs = []ToolSpec{
	{ID: "b-o-m-conversion"},
	{ID: "batch-tm-leveraging", Skip: SkipNeedsContext},
	{ID: "batch-translation", Skip: SkipNeedsContext},
	{ID: "char-listing"},
	{ID: "character-count"},
	{ID: "characters-checker"},
	{ID: "cleanup"},
	{ID: "code-simplifier"},
	{ID: "codes-removal"},
	{ID: "concordance-character-count"},
	{ID: "concordance-word-count"},
	{ID: "convert-segments-to-text-units"},
	{ID: "copy-or-move", Skip: SkipNeedsContext},
	{ID: "copy-source-on-empty-target"},
	{ID: "create-target"},
	{ID: "desegmentation"},
	{ID: "diff-leverage", Skip: SkipNeedsContext},
	{ID: "encoding-conversion"},
	{ID: "enrycher", Skip: SkipNeedsParams},
	{ID: "exact-document-context-match-character-count"},
	{ID: "exact-document-context-match-word-count"},
	{ID: "exact-local-context-match-character-count"},
	{ID: "exact-local-context-match-word-count"},
	{ID: "exact-match-character-count"},
	{ID: "exact-match-word-count"},
	{ID: "exact-previous-version-match-character-count"},
	{ID: "exact-previous-version-match-word-count"},
	{ID: "exact-repaired-character-count"},
	{ID: "exact-repaired-word-count"},
	{ID: "exact-structural-match-character-count"},
	{ID: "exact-structural-match-word-count"},
	{ID: "exact-text-only-character-count"},
	{ID: "exact-text-only-previous-version-match-character-count"},
	{ID: "exact-text-only-previous-version-match-word-count"},
	{ID: "exact-text-only-unique-id-match-character-count"},
	{ID: "exact-text-only-unique-id-match-word-count"},
	{ID: "exact-text-only-word-count"},
	{ID: "exact-unique-id-match-character-count"},
	{ID: "exact-unique-id-match-word-count"},
	{ID: "external-command", Skip: SkipNeedsParams},
	{ID: "extraction-verification", Skip: SkipNeedsContext},
	{ID: "extraction", Skip: SkipNeedsContext},
	{ID: "filter-events-to-raw-document", Skip: SkipNeedsContext},
	{ID: "filter-events-writer", Skip: SkipNeedsContext},
	{ID: "format-conversion"},
	{ID: "full-width-conversion"},
	{ID: "fuzzy-match-character-count"},
	{ID: "fuzzy-match-word-count"},
	{ID: "fuzzy-previous-version-match-character-count"},
	{ID: "fuzzy-previous-version-match-word-count"},
	{ID: "fuzzy-repaired-character-count"},
	{ID: "fuzzy-repaired-word-count"},
	{ID: "fuzzy-unique-id-match-character-count"},
	{ID: "fuzzy-unique-id-match-word-count"},
	{ID: "g-m-x-alphanumeric-only-text-unit-character-count"},
	{ID: "g-m-x-alphanumeric-only-text-unit-word-count"},
	{ID: "g-m-x-exact-matched-character-count"},
	{ID: "g-m-x-exact-matched-word-count"},
	{ID: "g-m-x-fuzzy-match-character-count"},
	{ID: "g-m-x-fuzzy-match-word-count"},
	{ID: "g-m-x-leveraged-matched-character-count"},
	{ID: "g-m-x-leveraged-matched-word-count"},
	{ID: "g-m-x-measurement-only-text-unit-character-count"},
	{ID: "g-m-x-measurement-only-text-unit-word-count"},
	{ID: "g-m-x-numeric-only-text-unit-character-count"},
	{ID: "g-m-x-numeric-only-text-unit-word-count"},
	{ID: "g-m-x-protected-character-count"},
	{ID: "g-m-x-protected-word-count"},
	{ID: "g-m-x-repetition-matched-character-count"},
	{ID: "g-m-x-repetition-matched-word-count"},
	{ID: "general-checker"},
	{ID: "id-based-aligner", Skip: SkipNeedsContext},
	{ID: "id-based-copy", Skip: SkipNeedsContext},
	{ID: "image-modification", Skip: SkipNeedsParams},
	{ID: "inconsistency-check"},
	{ID: "inline-codes-checker"},
	{ID: "length-checker"},
	{ID: "leveraging", Skip: SkipNeedsContext},
	{ID: "line-break-conversion"},
	{ID: "localizable-checker"},
	{ID: "m-s-batch-translation", Skip: SkipNeedsParams},
	{ID: "m-t-character-count"},
	{ID: "m-t-word-count"},
	{ID: "merging", Skip: SkipNeedsContext},
	{ID: "paragraph-aligner", Skip: SkipNeedsContext},
	{ID: "patterns-checker"},
	{ID: "phrase-assembled-character-count"},
	{ID: "phrase-assembled-word-count"},
	{ID: "post-segmentation-code-simplifier"},
	{ID: "quality-check"},
	{ID: "r-t-f-conversion"},
	{ID: "raw-document-to-filter-events", Skip: SkipNeedsContext},
	{ID: "raw-document-to-output-stream", Skip: SkipNeedsContext},
	{ID: "raw-document-writer", Skip: SkipNeedsContext},
	{ID: "remove-target"},
	{ID: "repetition-analysis"},
	{ID: "resource-simplifier"},
	{ID: "scoping-report", Skip: SkipNeedsContext},
	{ID: "search-and-replace", Skip: SkipNeedsParams},
	{ID: "segmentation"},
	{ID: "sentence-aligner", Skip: SkipNeedsContext},
	{ID: "simple-word-count"},
	{ID: "space-check"},
	{ID: "t-m-import", Skip: SkipNeedsContext},
	{ID: "t-t-x-joiner", Skip: SkipNeedsContext},
	{ID: "t-t-x-splitter", Skip: SkipNeedsContext},
	{ID: "term-extraction"},
	{ID: "terminology-leveraging", Skip: SkipNeedsContext},
	{ID: "text-modification"},
	{ID: "tokenization"},
	{ID: "translation-comparison", Skip: SkipNeedsContext},
	{ID: "tu-filtering"},
	{ID: "uri-conversion"},
	{ID: "whitespace-correction"},
	{ID: "word-count"},
	{ID: "x-m-l-analysis"},
	{ID: "x-m-l-char-fixing"},
	{ID: "x-m-l-validation"},
	{ID: "x-s-l-transform", Skip: SkipNeedsParams},
	{ID: "xliff-joiner", Skip: SkipNeedsContext},
	{ID: "xliff-splitter", Skip: SkipNeedsContext},
	{ID: "xliff-w-c-splitter", Skip: SkipNeedsContext},
}
