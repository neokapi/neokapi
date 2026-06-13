// Package safeio provides shared, resource-bounding primitives for parsing
// untrusted input across every neokapi trust context — the CLI, the
// multi-tenant bowrain server, and the cgo-less browser WASM build.
//
// # Why this package exists
//
// neokapi's ~49 native format readers consume attacker-controlled bytes. Go's
// memory safety removes the dominant RCE risk class, but the residual risks are
// real: decompression bombs, recursion-depth stack exhaustion, oversized
// reads, and path traversal on any filesystem write derived from document
// content. This package codifies the canonical Go mitigations for those
// classes as small, composable, panic-free helpers so every reader applies the
// *same* limits the *same* way.
//
// The package is pure Go (no cgo, no platform-specific syscalls beyond the
// std library), so it builds for js/wasm and wasip1 exactly as it does for the
// native CLI and server.
//
// # The identical-limits contract
//
// The most important rule, learned from the file-type CVE-2026-32630 disaster
// (a streaming code path bounded zip entry sizes while the buffer code path did
// not, so a 255 KB zip inflated to 257 MB through the unbounded entry point):
//
//	The same Budget MUST be applied on every input path — CLI file, server
//	upload, and WASM buffer — for a given format. There is exactly one
//	source of truth for the defaults: the package-level Default* values and
//	[DefaultBudget]. Do not bound one entry point and leave another open.
//
// Because the defaults are package-level and the readers reach for the same
// [DefaultBudget] / [DefaultZipLimits] regardless of caller, the limits are
// identical across contexts by construction. A caller that needs to tighten or
// loosen a bound composes a new [Budget] with the With* methods and threads it
// through — but it must do so on *all* paths, never just one.
//
// # Primitives
//
//   - [LimitedReader] / [Budget.Reader] — an io.Reader that returns a typed
//     [LimitError] (wrapping [ErrByteBudget]) once more than N bytes are read,
//     instead of io.LimitedReader's silent truncation-to-EOF.
//   - [LimitedWriter] / [Budget.Writer] — the output-side analogue.
//   - [DepthGuard] — an Enter/Leave recursion counter (and a [DepthGuard.Do]
//     helper) that returns [ErrTooDeep] past a configured maximum, so deeply
//     nested documents fail with an error rather than a non-recoverable Go
//     stack-overflow panic.
//   - [ZipLimits] / [ZipGuard] — per-entry uncompressed-size, inflate-ratio
//     (zip-bomb), total-size, and entry-count caps mirroring Apache POI's
//     ZipSecureFile semantics, checked on streaming read so a lying Zip64
//     header cannot evade them.
//   - [SafeJoin] / [OpenInRoot] — reject content-derived paths that escape a
//     root, using filepath.IsLocal and os.Root confinement.
//   - [Budget] — bundles the byte, depth, and zip limits with sane defaults
//     and composes via the With* methods.
//
// # Adoption status
//
// The budget primitives are wired into the genuinely high-risk readers — the
// ones that open archives, stream the whole input into memory, or recurse —
// not (yet) all ~49. Each is wired at the same point as its exemplar, reaching
// for [DefaultBudget] / [DefaultZipLimits] so the limits are uniform across the
// CLI, server, and WASM contexts.
//
// Archive (zip) readers — validate the archive up front with
// [ZipLimits.CheckReader] and read every entry through [ZipLimits.ReadEntry] /
// [ZipLimits.OpenEntry], so a lying Zip64 header, a zip bomb, or an entry
// flood is rejected before inflation:
//
//   - openxml, epub, idml, odf
//
// Whole-input streaming readers — bound the read that pulls the entire
// document into memory with [Budget.Reader] using [DefaultBudget], so an
// unbounded/oversized stream fails with a typed [LimitError] instead of
// exhausting memory:
//
//   - json, yaml, properties, po, csv, markdown, mdx, plaintext, paraplaintext,
//     xml, html
//
// Recursive-descent readers — bound the recursion with [DepthGuard] so a
// pathologically nested document degrades to a clean truncation instead of a
// Go stack-overflow panic:
//
//   - html bounds its recursive DOM and inline-element walk with a
//     [DepthGuard] from [DefaultBudget].
//
// The xml reader needs no [DepthGuard]: its element walk is iterative (an
// explicit element-frame stack driven by encoding/xml's streaming Decoder, not
// Go recursion), so deeply nested input cannot overflow the stack; it adopts
// only the byte budget. Likewise the odf and json subfilter recursion crosses
// into a child reader (xml / a configured subformat) that carries its own
// bounds, so the embedded-content path inherits the child's guards.
//
// Remaining readers (the smaller text/catalog formats — resx, androidxml,
// xcstrings, arb, i18next, designtokens, ts, srt/vtt/ttml, dtd, mif, rtf, …)
// are a follow-up: the adoption pattern is the one or two lines added to each
// reader above. Track per-reader adoption as part of the format
// security/hardening (S0–S4) axis.
package safeio
