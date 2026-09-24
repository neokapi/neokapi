// Package safeio provides resource limits and path confinement for untrusted
// input in the CLI, server and browser WASM builds. It is pure Go and supports
// both js/wasm and wasip1.
//
// # Consistent limits
//
// A format must apply the same [Budget] on every input path, including files,
// uploads and WASM buffers. Limiting only one path leaves the others exposed to
// oversized reads, decompression bombs or excessive recursion. Use [DefaultBudget]
// and [DefaultZipLimits], or pass the same customized budget through every path.
//
// # Primitives
//
//   - [LimitedReader] and [Budget.Reader] return [LimitError], wrapping
//     [ErrByteBudget], when input exceeds the limit. Unlike io.LimitedReader,
//     they distinguish the limit from ordinary EOF.
//   - [LimitedWriter] and [Budget.Writer] enforce output limits.
//   - [DepthGuard] tracks recursive entry and exit and returns [ErrTooDeep]
//     when the configured depth is exceeded.
//   - [ZipLimits] and [ZipGuard] limit uncompressed entry size, inflation ratio,
//     total size and entry count. Streaming checks enforce these limits even
//     when archive headers understate the expanded size.
//   - [SafeJoin] and [OpenInRoot] confine document-derived paths to a root,
//     using filepath.IsLocal and os.Root.
//   - [Budget] groups byte, depth and archive limits; With* methods customize them.
//   - [NoCopyString] avoids copying a document buffer. Callers must follow its
//     aliasing contract for the lifetime of the returned string.
//   - [Admission] limits the total estimated memory held by concurrent file jobs.
//     [DefaultMaxInflightBytes] supplies the default, [MaxInflightBytesEnv]
//     overrides it, and [FileWeight] estimates a file's weight from its size.
//
// # Reader integration
//
// Archive readers validate metadata with [ZipLimits.CheckReader] and read entries
// through [ZipLimits.ReadEntry] or [ZipLimits.OpenEntry]. Whole-input readers use
// [Budget.Reader] to bound buffering. Recursive walkers use [DepthGuard]; iterative
// walkers, such as the XML element stack, do not consume the Go call stack with
// each nested element.
//
// A child reader handling embedded content must apply its own limits. Integration
// is reader-specific; importing this package alone provides no protection. Track
// coverage through each format's security and hardening assessment (S0–S4).
package safeio
