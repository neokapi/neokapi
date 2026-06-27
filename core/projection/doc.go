// Package projection builds a normalized, format-neutral render AST — the
// "generative projection spine" — from a document's Part/Block stream.
//
// # Why this exists
//
// neokapi has two legitimate ways to turn the content model back into bytes:
//
//  1. Skeleton replay — byte-exact same-format round-trip (docx/odf, and the
//     same-format modes of html/md/asciidoc). The reader retains the original
//     non-content bytes in a skeleton store and the writer replays them with
//     block text spliced back in. This path is uniform, already correct, and is
//     NOT the concern of this package. It must stay separate by design.
//
//  2. Generative / semantic projection — output to a *different* format, or
//     render a preview (which is itself a generative projection). Here nothing
//     foreign-skeleton survives; structure must be reconstructed from the
//     canonical role/vocabulary/geometry layer.
//
// Historically every generative writer (HTML, Markdown, AsciiDoc, DocLang,
// plaintext) and the TypeScript lab preview each re-derived that structure
// independently: four separate balanced-tag inline stacks, four separate table
// assemblers, four role→element tables, and a fifth re-derivation in the
// browser. Tables alone had four implementations and the preview none (it
// flattened runs and dropped table structure outright).
//
// This package is the one shared spine for path 2. [ProjectStream] walks the
// Part stream once and produces a tree of [RenderNode] — a format-neutral render
// model carrying roles, inline runs, table topology (rows/cells/spans), list
// nesting, and geometry. [ProjectBlock] is the block-first primitive: it
// projects a single block to a fragment, so `kapi inspect --project`, the
// convert-lab Blocks tab, and per-block preview all reuse the same logic that
// whole-document serialization uses. [WalkInline] is the shared inline decoder
// the per-format serializers build on, so each maps the canonical run
// vocabulary to its own markup without re-implementing run pairing.
//
// The render AST is a substrate, not a ceiling (tier 1 of the layered-fidelity
// model in strategy/preview-fidelity): a serializer reads as much of the full
// node — Props, run Attrs, geometry — as its format can express. Cross-format,
// only the canonicalized vocabulary transfers; same-format property round-trip
// belongs to skeleton replay, which this package deliberately does not touch.
package projection
