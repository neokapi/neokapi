---
sidebar_position: 7
title: Testing Strategy
description: "neokapi's test strategy: roundtrip gold-standard tests for formats, table-driven tests, spec suites driven by each format's spec.yaml, mock AI providers for CI, and integration tests for flows."
keywords: [testing, roundtrip, table-driven tests, spec.yaml, Okapi, testify, mock providers, neokapi]
---

# Test Strategy

## Principles

1. **Every format and tool has tests.** No format reader/writer or tool ships without tests.
2. **Roundtrip is the gold standard.** For formats: read, write, compare with original.
3. **Port Okapi test data.** Use Okapi's test resource files as the source of truth.
4. **Table-driven tests.** Go's table-driven pattern for covering multiple inputs.
5. **Test at the interface boundary.** Test against `DataFormatReader`/`Tool` interfaces, not internals.
6. **Deterministic AI tests.** AI tools use mock providers in CI; real providers in manual integration tests.

## Test Structure

```
neokapi/
├── core/model/
│   ├── model_test.go               # Block creation, targets, overlays
│   └── run_test.go                 # Run sequence (canonical inline content)
├── core/flow/
│   ├── executor_test.go            # Flow execution, error propagation
│   └── steps_test.go               # StepsToGraph compilation
├── core/tool/
│   └── base_test.go                # BaseTool dispatch, pass-through
│
├── core/formats/
│   ├── html/
│   │   ├── reader_test.go
│   │   ├── writer_test.go
│   │   └── testdata/               # Test fixtures
│   └── ... (each format follows the same pattern)
│
├── core/ai/tools/
│   └── tools_test.go               # AI tool tests; use mock provider
├── providers/ai/
│   └── mock.go                     # Mock LLM provider
│
└── core/internal/testutil/
    └── helpers.go                   # Common test helpers (RawDocFrom*, CollectParts/Blocks, PartsToChannel)
```

## Test Patterns

### Roundtrip Test

The most important test for any format:

```go
func TestRoundTrip(t *testing.T) {
    tests := []struct {
        name string
        file string
    }{
        {"simple", "testdata/simple.html"},
        {"inline codes", "testdata/inline_codes.html"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            original, err := os.ReadFile(tt.file)
            require.NoError(t, err)

            reader := NewReader()
            err = reader.Open(ctx, testutil.RawDocFromReader(
                bytes.NewReader(original), tt.file, model.LocaleEnglish))
            require.NoError(t, err)
            parts := testutil.CollectParts(t, reader.Read(ctx))
            reader.Close()

            var buf bytes.Buffer
            writer := NewWriter()
            writer.SetOutputWriter(&buf)
            writer.Write(ctx, testutil.PartsToChannel(parts))
            writer.Close()

            assert.Equal(t, string(original), buf.String())
        })
    }
}
```

### Extraction Test

Verify specific Blocks are extracted with correct content:

```go
func TestExtraction(t *testing.T) {
    data, err := os.ReadFile("testdata/sample.html")
    require.NoError(t, err)

    reader := NewReader()
    err = reader.Open(ctx, testutil.RawDocFromReader(
        bytes.NewReader(data), "testdata/sample.html", model.LocaleEnglish))
    require.NoError(t, err)
    defer reader.Close()

    blocks := testutil.CollectBlocks(t, reader.Read(ctx))
    require.Len(t, blocks, 3)
    assert.Equal(t, "Welcome", blocks[0].SourceText())
}
```

### Flow Execution Test

Build a tool by embedding `tool.BaseTool` and setting handler fields, assemble a
flow with the `flow.NewFlow(...).AddTool(...).Build()` builder (which returns
`(*Flow, error)`), and drive it with `ExecuteWithChannels` for channel-level
control:

```go
func TestFlowExecution(t *testing.T) {
    uppercase := &tool.BaseTool{
        ToolName: "uppercase",
        Produce: func(v tool.VariantView) error {
            if v.Translatable() {
                v.SetTargetText(model.LocaleFrench, strings.ToUpper(v.SourceText()))
            }
            return nil
        },
    }

    f, err := flow.NewFlow("test").AddTool(uppercase).Build()
    require.NoError(t, err)

    executor := flow.NewExecutor()
    in, out, wait := executor.ExecuteWithChannels(t.Context(), f)

    go func() {
        in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "hello")}
        close(in)
    }()
    for range out { /* drain */ }
    require.NoError(t, wait())
}
```

## Running Tests

```bash
make test               # All tests
make test-unit          # Unit tests only (-short)
make test-race          # With race detector
make test-verbose       # Verbose output
make cover              # Coverage report
```

Single test:

```bash
go test -tags fts5 ./core/flow/ -run TestFlowExecutorContextCancellation -v
```

Pass `-tags fts5` whenever you bypass `make`. The build succeeds without it, and
only a query that reaches FTS5 (content memory, terms) fails at runtime with
`no such function: fts5`. Spell every tag in one comma-separated flag
(`-tags "fts5,integration"`): `go` keeps only the last `-tags` occurrence.

## Test Tags

| Tag           | Purpose                                  | Command                                    |
| ------------- | ---------------------------------------- | ------------------------------------------ |
| (none)        | Unit tests only                          | `go test -tags fts5 ./...`                 |
| `integration` | + plugin and format integration          | `go test -tags "fts5,integration" ./...`   |
| `acceptance`  | + native-format consumer-toolchain tests | `go test -tags "fts5,acceptance" ./...`    |
| `e2e`         | + end-to-end tests                       | `go test -tags "fts5,e2e" ./...`           |

The head-to-head comparison of the Go ports against the Okapi Framework is a
separate suite in the [okapi-bridge repository](https://github.com/neokapi/okapi-bridge)
(`parity/`). It compiles against a released neokapi module and runs on a
schedule there; see [A-02](/contribute/architecture/assurance/a-02-parity).

## Gate Conformance Suite

The gate conformance suite holds the built `kapi` binary to what the gate
claims. Each case states which findings `kapi check` reports and which it
leaves alone, what `kapi fix` writes, what `kapi context <path>` names among a
file's rules, and what the Claude Code plugin's Stop hook decides after an
edit. Run it with:

```bash
make gate-conformance
```

The target runs `TestGateConformance` in `kapi/e2e/`, so it uses the binary and
the isolation environment that package's `TestMain` sets up, and gives each case
a workspace of its own. It prints a matrix with one line per case and surface
(`check`, `fix`, `context`, `hook`): `pass`, `fail`, `expected-fail` with the
reason, or `not covered`.

Each case is a directory under `kapi/e2e/gate/testdata/<case>/`. The directory
is a small project: a `kapi.yaml`, its content files, and, for a case that
needs rules, a `voice.yaml` under `.kapi/`. That file is the artifact a
`kapi store import` step reads into the project's store: its `terms:` into the
terms store and its patterns into the voice profile. Beside them, `expect.yaml`
lists stages. Each stage runs its `steps` and then checks its expectations:

```yaml
description: A kept name rule fails a check on each rejected form.
stages:
  - name: kept
    steps:
      - actor: agent
        kapi: [context, note, --term, Sign in, --instead-of, Log in]
      - keep: Write Sign in, not Log in, Sign-in, Signin or SignIn.
    files:
      - path: docs/guide.md
        check:
          failing: 1
          report:
            - {rule: terms.vocabulary, block: "welcome/p", quote: Log in, fails: true}
          absent:
            - {quote: Sign in}
        fix:
          failing: 0
          after: |
            # Welcome

            Sign in with your account.
        context:
          names: ['Sign in, not "Log in"']
    hook:
      edit: {path: docs/guide.md, append: "\nLog in again.\n"}
      blocks: true
      names: [Forbidden term "Log in" found]
      log: {failing: 1, caught: 0}
```

A step is a `kapi` command line (run in the project, as the person or as an
`agent`), a `keep` or `narrow` of a rule named by the sentence `kapi rules`
prints, or a `write` to a file. A `fix` with no `after` asserts that the file
keeps its bytes; `args: []` runs `kapi fix` over the whole project, and
`unchanged` names other files whose bytes must not move. A hook's `log` reads
`kapi context log --session` after the run: `failing` is the count the run
recorded, and `caught` is how many findings the session's runs have caught
so far.

To add a case, copy the nearest directory, change the project and the
expectations, and run `make gate-conformance`. A failing expectation prints the
full `kapi check --json` report, or the fix output and both versions of the
file. Find the block ids and quoted words there.

An expectation the current code does not meet carries `xfail: <reason>`. It is
listed as `expected-fail` with that reason and does not fail the run. When the
behaviour lands, the expectation passes, the run fails and names the `xfail` to
remove. Never mark an expectation as skipped instead.

The fixtures contain banned words on purpose. The repository's own `kapi.yaml`
excludes `**/testdata/**`, so `kapi check` on the repository never reads them.

## CI

Tests run automatically via GitHub Actions on every push and pull request. See `.github/workflows/ci.yml`.
