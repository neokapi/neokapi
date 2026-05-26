---
sidebar_position: 7
title: Testing Strategy
description: neokapi's test strategy — roundtrip gold-standard tests for formats, table-driven tests, parity tests against Okapi Framework data, mock AI providers for CI, and integration tests for flows.
keywords: [testing, roundtrip, table-driven tests, parity, Okapi, testify, mock providers, neokapi]
---

# Test Strategy

## Principles

1. **Every format and tool has tests** — No format reader/writer or tool ships without tests.
2. **Roundtrip is the gold standard** — For formats: read, write, compare with original.
3. **Port Okapi test data** — Use Okapi's test resource files as the source of truth.
4. **Table-driven tests** — Go's table-driven pattern for covering multiple inputs.
5. **Test at the interface boundary** — Test against `DataFormatReader`/`Tool` interfaces, not internals.
6. **Deterministic AI tests** — AI tools use mock providers in CI; real providers in manual integration tests.

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
│   └── tools_test.go               # AI tool tests — use mock provider
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
        Translate: func(v tool.TargetView) error {
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
go test ./core/flow/ -run TestFlowExecutorContextCancellation -v
```

## Test Tags

| Tag           | Purpose                                  | Command                            |
| ------------- | ---------------------------------------- | ---------------------------------- |
| (none)        | Unit tests only                          | `go test ./...`                    |
| `integration` | + plugin and format integration          | `go test ./... -tags=integration`  |
| `acceptance`  | + native-format consumer-toolchain tests | `go test ./... -tags=acceptance`   |
| `parity`      | + Okapi parity comparison tests          | `go test ./... -tags=parity`       |
| `e2e`         | + end-to-end tests                       | `go test ./... -tags=e2e`          |

## CI

Tests run automatically via GitHub Actions on every push and pull request. See `.github/workflows/ci.yml`.
