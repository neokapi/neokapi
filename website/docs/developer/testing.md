---
sidebar_position: 7
title: Testing
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
├── model/
│   ├── block_test.go               # Block creation, segment management
│   ├── layer_test.go               # Layer nesting, embedded content
│   ├── fragment_test.go            # Fragment span encoding/decoding
│   └── skeleton_test.go            # Skeleton reconstruction
├── flow/
│   ├── executor_test.go            # Flow execution, error propagation
│   └── builder_test.go             # Builder API
│   └── tool/
│       └── base_test.go            # BaseTool dispatch, pass-through
│
├── formats/
│   ├── html/
│   │   ├── reader_test.go
│   │   ├── writer_test.go
│   │   └── testdata/               # Test fixtures
│   └── ... (each format follows the same pattern)
│
├── ai/
│   ├── tools/
│   │   ├── translate_test.go       # Uses mock provider
│   │   └── qualitycheck_test.go    # Uses mock provider
│   └── provider/
│       └── mock.go                 # Mock LLM provider
│
└── internal/
    └── testutil/
        ├── helpers.go               # Common test helpers
        ├── mock_tool.go             # Mock Tool implementation
        ├── mock_reader.go           # Mock DataFormatReader
        └── assert_parts.go          # Custom Part assertions
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
            err = reader.Open(ctx, testutil.RawDocFromFile(tt.file, "en"))
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
    reader := NewReader()
    err := reader.Open(ctx, testutil.RawDocFromFile("testdata/sample.html", "en"))
    require.NoError(t, err)
    defer reader.Close()

    blocks := testutil.CollectBlocks(t, reader.Read(ctx))
    require.Len(t, blocks, 3)
    assert.Equal(t, "Welcome", blocks[0].SourceText())
}
```

### Flow Execution Test

```go
func TestFlowExecution(t *testing.T) {
    uppercaseTool := testutil.NewMockTool("uppercase", func(part *model.Part) *model.Part {
        if part.Type == model.PartBlock {
            block := part.Resource.(*model.Block)
            text := strings.ToUpper(block.SourceText())
            block.SetTargetText(model.LocaleFrench, text)
        }
        return part
    })

    f := flow.NewFlow("test").AddTool(uppercaseTool).Build()
    executor := flow.NewExecutor(reg)
    err := executor.Execute(ctx, f, items)
    require.NoError(t, err)
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
go test ./flow/ -run TestExecutorCancellation -v
```

## Test Tags

| Tag           | Purpose                         | Command                                  |
| ------------- | ------------------------------- | ---------------------------------------- |
| (none)        | Unit tests only                 | `go test ./...`                          |
| `integration` | + plugin and format integration | `go test ./... -tags=integration`        |
| `java`        | + Java bridge tests             | `go test ./... -tags="integration java"` |
| `ai`          | + real AI provider tests        | `go test ./... -tags="integration ai"`   |

## CI

Tests run automatically via GitHub Actions on every push and pull request. See `.github/workflows/ci.yml`.
