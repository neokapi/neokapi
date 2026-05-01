//go:build parity

// Package roundtrip drives a deterministic pseudo-translation
// extract → translate → merge cycle against three engines (native
// in-process, okapi-bridge gRPC daemon, and the upstream tikal CLI)
// and compares the results.
//
// # Why this exists
//
// The spec.yaml runners verify reader contracts (Block count, source
// text, IDs) but never invoke a writer. They prove "we can read" —
// not "we can read, modify, and write back coherently". This package
// closes that gap with an end-to-end harness driven by a transform
// every engine can execute identically: wrap each translatable
// block's source text with a deterministic prefix/suffix and emit it
// as the target.
//
// Pseudo-translation is the canonical Okapi roundtrip vehicle (it's
// what RoundTripIt-style integration tests have always used) because
// it's:
//
//   - Deterministic — same input always yields same target.
//   - Locale-agnostic — no MT, no model dependency.
//   - Inline-aware — each engine has the freedom to keep paired-codes
//     and placeholders intact in the target while wrapping only the
//     text spans, exercising real merge logic.
//   - Comparable — every engine's output can be re-extracted with the
//     reference reader and diffed at the Block level.
//
// # The three engines
//
//   - NativeEngine wires the format's registered reader through a
//     PseudoTranslate tool into the format's registered writer, all
//     in-process. This is what `kapi pseudo-translate` does at the
//     CLI layer; the engine reuses the same plumbing.
//
//   - BridgeEngine drives the okapi-bridge daemon's Process RPC in
//     read-write mode. The daemon reads via the requested
//     okf_<format> filter, streams every translatable Block back over
//     gRPC, this side rewrites the target, and the daemon's writer
//     thread merges the result.
//
//   - TikalEngine shells out to the upstream tikal CLI: tikal -x to
//     extract to XLIFF, a Go-side XLIFF target rewrite, then
//     tikal -m to merge the translated XLIFF back into the original
//     file. This is the ground truth — any divergence between
//     bridge and tikal exposes a bug in the bridge daemon's filter
//     wiring (since bridge is supposed to be a thin shim over the
//     same Okapi filters tikal calls).
//
// # Comparison strategy
//
// Byte-equal comparison across engines is unrealistic — each filter
// emits subtly different whitespace, attribute ordering, and
// XML-decl quoting. Instead the harness re-extracts each engine's
// merged output through the native reader and compares the resulting
// Block source-text streams. This is the same level of fidelity the
// spec.yaml runners use, applied to the round-tripped output.
//
// Engines whose tooling is unavailable on the runner (tikal not
// installed, bridge daemon not reachable) skip with t.Skip rather
// than fail — the harness reports which engines participated.
//
// # Architecture
//
//   - engine.go declares the Engine interface and the shared
//     PseudoSpec / Result types.
//   - pseudo.go implements the canonical pseudo transform that all
//     engines apply; tikal and bridge invoke it directly, native
//     uses the equivalent built-in PseudoTranslate tool.
//   - native.go, bridge.go, tikal.go are one engine each.
//   - compare.go re-extracts and diffs Block streams.
//   - harness.go is the test entrypoint: RunThreeWay(t, cfg).
//
// All files are build-tagged `parity` to match the surrounding
// cli/parity/ harness.
package roundtrip
