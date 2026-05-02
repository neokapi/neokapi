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
// it's deterministic, locale-agnostic, and exercises every engine's
// real merge logic.
//
// # The engines
//
//   - NativeEngine wires the format's registered reader through a
//     pseudo transform into the format's registered writer, all
//     in-process.
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
//     file.
//
// Tikal is the comparator, not a tested engine. Asserting tikal-
// against-itself would be circular; instead the harness runs tikal
// inline once per fixture to obtain the live reference output, then
// byte-compares each tested engine (native, bridge) against it.
//
// # Hard requirements
//
// Tikal AND okapi-bridge are mandatory dependencies for this suite.
// Tikal is checked at TestMain (set OKAPI_TIKAL, OKAPI_HOME, or put
// tikal on PATH). Bridge is required by the existing
// parity.AcquireBridgeDaemon path. If either is missing, the test
// binary aborts with a clear error — no silent skips.
//
// # Comparison strategy
//
// Each engine's merged output is compared byte-for-byte against
// tikal's output for the same fixture, produced inline in the same
// test run. There are no committed golden files: tikal's behavior
// IS the reference, captured fresh every test, so there is no risk
// of "the golden is from tikal v1.47, but we're on v1.48 now" drift.
//
// Compound zip formats (idml, openxml, epub) compare per-entry —
// byte-equal across uncompressed entry contents, ignoring zip
// metadata (mtime, central-directory order) that two correct
// round-trippers can legitimately differ on. Set Case.IsZip to true.
//
// # Architecture
//
//   - engine.go declares the Engine interface and the shared
//     PseudoSpec / Result types.
//   - pseudo.go implements the canonical pseudo transform that all
//     engines apply.
//   - native.go, bridge.go, tikal.go are one engine each.
//   - compare.go: byte / per-zip-entry comparators + Divergence type.
//   - harness.go: RunThreeWay — runs tikal as the comparator and
//     each tested engine against tikal's output.
//   - main_test.go: TestMain hard-requires tikal up front.
package roundtrip
