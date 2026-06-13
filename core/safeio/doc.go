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
// As of this package's introduction the budget primitives are wired into a set
// of exemplar readers, not all 49:
//
//   - ZIP-container readers openxml, epub, idml, and odf validate the archive
//     up front with [ZipLimits.CheckReader] and read every entry through
//     [ZipLimits.ReadEntry] / [ZipLimits.OpenEntry].
//   - The json streaming reader bounds its initial read with
//     [Budget.Reader] using [DefaultBudget].
//
// Remaining readers (yaml and the other text/XML formats, the remaining
// zip-family parsers, and the recursive descent parsers that should adopt
// [DepthGuard]) are a follow-up: the adoption pattern is the two lines added to
// each exemplar reader. Track per-reader adoption as part of the format
// security/hardening (S0–S4) axis.
package safeio
