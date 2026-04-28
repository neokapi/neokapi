// Package parity is the test harness that proves neokapi's Go ports of
// Okapi filters and steps behave like the Java reference implementation.
//
// Tests are gated behind the build tag "parity" and require:
//   - A locally built kapi binary (Makefile target: build)
//   - A locally installed okapi-bridge plugin (Makefile target: parity-sandbox)
//
// The Makefile target [parity-test] orchestrates both, sets the
// KAPI_PARITY_SANDBOX environment variable, and runs
// `go test -tags parity ./core/parity/...`. Tests called outside this
// orchestration (e.g. directly via `go test`) are skipped with an
// explanatory message — there is no implicit fallback to a system-wide
// kapi or pre-installed plugins, by design.
//
// # Architecture
//
//	┌─────────────────────────────────────────────────────────────────────┐
//	│  parity test (build tag: parity)                                    │
//	│                                                                     │
//	│  RunNative(...) ──────────────► neokapi format reader (in-process)  │
//	│                                          │                          │
//	│                                  []model.Part                       │
//	│                                          ▼                          │
//	│  RunBridge(...) ──────────────► daemon.Process RPC                  │
//	│       │                                  │                          │
//	│       └─► pluginhost.DaemonPool ─► kapi-okapi-bridge daemon (JVM)   │
//	│                                  []model.Part                       │
//	│                                          ▼                          │
//	│  CompareEvents(t, native, bridge)                                   │
//	│  CompareBytes(t, want, got)                                         │
//	└─────────────────────────────────────────────────────────────────────┘
//
// # Environment isolation
//
// The harness intentionally never reads the user's
// $XDG_DATA_HOME/kapi/plugins/ or honours an inherited $KAPI_PLUGINS_DIR.
// `make parity-test` builds a sandbox at $REPO/.parity/ and exports
// $KAPI_PARITY_SANDBOX to point Go tests at it. Without that variable,
// every test SkipNow's. This guarantees CI runs and local runs see the
// same plugin install — the freshly built one — and that no developer
// shortcut accidentally measures pre-installed versions.
//
// # Reporting
//
// Tests opt in to the JSON test-comparison report via the [Report]
// helper. The report collector writes one record per filter/step into
// $KAPI_PARITY_REPORT (default $REPO/.parity/test-comparison.json). The
// scripts/testcompare/ tool consumes that JSON to produce the docs page.
package parity
