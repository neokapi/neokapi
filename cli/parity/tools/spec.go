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

// SKIP_NEEDS_CONTEXT marks steps that ProcessStep alone cannot exercise
// because they depend on pipeline-wide state (TM matches, batch
// statistics, multi-doc inputs, RawDocument access, …). Tracked
// upstream as #454.
const SKIP_NEEDS_CONTEXT = "step needs pipeline context not provided by ProcessStep — see #454"

// SKIP_NEEDS_PARAMS marks steps whose default parameters force an
// unconfigured fast-failure (e.g. XSLT needs a stylesheet path; search
// & replace needs a pattern). Tracked under #454.
const SKIP_NEEDS_PARAMS = "step needs required parameters — see #454"

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
	{ID: "batch-tm-leveraging", Skip: SKIP_NEEDS_CONTEXT},
	{ID: "batch-translation", Skip: SKIP_NEEDS_CONTEXT},
	{ID: "char-listing"},
	{ID: "character-count"},
	{ID: "characters-checker"},
	{ID: "cleanup"},
	{ID: "code-simplifier"},
	{ID: "codes-removal"},
	{ID: "concordance-character-count"},
	{ID: "concordance-word-count"},
	{ID: "convert-segments-to-text-units"},
	{ID: "copy-or-move", Skip: SKIP_NEEDS_CONTEXT},
	{ID: "copy-source-on-empty-target"},
	{ID: "create-target"},
	{ID: "desegmentation"},
	{ID: "diff-leverage", Skip: SKIP_NEEDS_CONTEXT},
	{ID: "encoding-conversion"},
	{ID: "enrycher", Skip: SKIP_NEEDS_PARAMS},
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
	{ID: "external-command", Skip: SKIP_NEEDS_PARAMS},
	{ID: "extraction-verification", Skip: SKIP_NEEDS_CONTEXT},
	{ID: "extraction", Skip: SKIP_NEEDS_CONTEXT},
	{ID: "filter-events-to-raw-document", Skip: SKIP_NEEDS_CONTEXT},
	{ID: "filter-events-writer", Skip: SKIP_NEEDS_CONTEXT},
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
	{ID: "id-based-aligner", Skip: SKIP_NEEDS_CONTEXT},
	{ID: "id-based-copy", Skip: SKIP_NEEDS_CONTEXT},
	{ID: "image-modification", Skip: SKIP_NEEDS_PARAMS},
	{ID: "inconsistency-check"},
	{ID: "inline-codes-checker"},
	{ID: "length-checker"},
	{ID: "leveraging", Skip: SKIP_NEEDS_CONTEXT},
	{ID: "line-break-conversion"},
	{ID: "localizable-checker"},
	{ID: "m-s-batch-translation", Skip: SKIP_NEEDS_PARAMS},
	{ID: "m-t-character-count"},
	{ID: "m-t-word-count"},
	{ID: "merging", Skip: SKIP_NEEDS_CONTEXT},
	{ID: "paragraph-aligner", Skip: SKIP_NEEDS_CONTEXT},
	{ID: "patterns-checker"},
	{ID: "phrase-assembled-character-count"},
	{ID: "phrase-assembled-word-count"},
	{ID: "post-segmentation-code-simplifier"},
	{ID: "quality-check"},
	{ID: "r-t-f-conversion"},
	{ID: "raw-document-to-filter-events", Skip: SKIP_NEEDS_CONTEXT},
	{ID: "raw-document-to-output-stream", Skip: SKIP_NEEDS_CONTEXT},
	{ID: "raw-document-writer", Skip: SKIP_NEEDS_CONTEXT},
	{ID: "remove-target"},
	{ID: "repetition-analysis"},
	{ID: "resource-simplifier"},
	{ID: "scoping-report", Skip: SKIP_NEEDS_CONTEXT},
	{ID: "search-and-replace", Skip: SKIP_NEEDS_PARAMS},
	{ID: "segmentation"},
	{ID: "sentence-aligner", Skip: SKIP_NEEDS_CONTEXT},
	{ID: "simple-word-count"},
	{ID: "space-check"},
	{ID: "t-m-import", Skip: SKIP_NEEDS_CONTEXT},
	{ID: "t-t-x-joiner", Skip: SKIP_NEEDS_CONTEXT},
	{ID: "t-t-x-splitter", Skip: SKIP_NEEDS_CONTEXT},
	{ID: "term-extraction"},
	{ID: "terminology-leveraging", Skip: SKIP_NEEDS_CONTEXT},
	{ID: "text-modification"},
	{ID: "tokenization"},
	{ID: "translation-comparison", Skip: SKIP_NEEDS_CONTEXT},
	{ID: "tu-filtering"},
	{ID: "uri-conversion"},
	{ID: "whitespace-correction"},
	{ID: "word-count"},
	{ID: "x-m-l-analysis"},
	{ID: "x-m-l-char-fixing"},
	{ID: "x-m-l-validation"},
	{ID: "x-s-l-transform", Skip: SKIP_NEEDS_PARAMS},
	{ID: "xliff-joiner", Skip: SKIP_NEEDS_CONTEXT},
	{ID: "xliff-splitter", Skip: SKIP_NEEDS_CONTEXT},
	{ID: "xliff-w-c-splitter", Skip: SKIP_NEEDS_CONTEXT},
}
