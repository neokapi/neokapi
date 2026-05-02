//go:build parity

// Package roundtrip drives a deterministic pseudo-translation
// extract → translate → merge cycle against three engines (native
// in-process, okapi-bridge gRPC daemon, and an in-process upstream
// Okapi pipeline launched via the bridge JAR's `pseudo` subcommand)
// and compares the results.
//
// # Why this exists
//
// The spec.yaml runners verify reader contracts (Block count, source
// text, IDs) but never invoke a writer. They prove "we can read" —
// not "we can read, modify, and write back coherently". This package
// closes that gap with an end-to-end harness driven by a transform
// every engine can execute identically: substitute every Latin letter
// in each translatable block's source text with its Latin-extended
// counterpart (Okapi's TextModificationStep with TYPE_EXTREPLACE +
// SCRIPT_EXT_LATIN) and emit the result as the target.
//
// Pseudo-translation is the canonical Okapi roundtrip vehicle (it's
// what RoundTripIt-style integration tests have always used) because
// it's deterministic, locale-agnostic, and exercises every engine's
// real merge logic.
//
// # The engines
//
//   - NativeEngine wires the format's registered reader through the
//     pseudo transform into the format's registered writer, all
//     in-process.
//
//   - BridgeEngine drives the okapi-bridge daemon's Process RPC in
//     read-write mode. The daemon reads via the requested
//     okf_<format> filter, streams every translatable Block back over
//     gRPC, this side rewrites the target, and the daemon's writer
//     thread merges the result.
//
//   - OkapiEngine shells out to the okapi-bridge launcher's `pseudo`
//     subcommand, which composes upstream Okapi's
//     RawDocumentToFilterEventsStep → TextModificationStep
//     (TYPE_EXTREPLACE / SCRIPT_EXT_LATIN) →
//     FilterEventsToRawDocumentStep into a single in-process pipeline.
//     This produces the canonical upstream Okapi output for the same
//     filter + transform.
//
// The okapi engine is the comparator, not a tested engine.
// Asserting okapi-against-itself would be circular; instead the
// harness runs the okapi engine inline once per fixture to obtain
// the live reference output, then byte-compares each tested engine
// (native, bridge) against it.
//
// # Hard requirements
//
// The parity sandbox (built via `make parity-test` from the repo
// root) must contain a freshly built kapi binary plus the
// okapi-bridge plugin tarball. Both engines (bridge daemon and
// okapi reference engine) load from the same launcher inside the
// sandbox. If the sandbox is missing, the test binary aborts with a
// clear error — no silent skips.
//
// # Comparison strategy
//
// Each engine's merged output is compared byte-for-byte against the
// okapi reference output for the same fixture, produced inline in
// the same test run. There are no committed golden files: upstream
// Okapi behavior IS the reference, captured fresh every test, so
// there is no risk of "the golden is from Okapi v1.47, but we're on
// v1.48 now" drift.
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
//     engines apply (Latin-extended substitution, inline codes preserved).
//   - native.go, bridge.go, okapi.go are one engine each.
//   - compare.go: byte / per-zip-entry comparators + Divergence type.
//   - harness.go: RunThreeWay — runs the okapi engine as the
//     comparator and each tested engine against its output.
//   - main_test.go: TestMain hard-requires the okapi-bridge launcher
//     up front.
package roundtrip
