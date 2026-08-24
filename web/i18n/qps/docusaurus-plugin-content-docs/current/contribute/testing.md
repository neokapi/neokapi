---
sidebar_position: 7
title: Testing Strategy
description: neokapi's test strategy — roundtrip gold-standard tests for formats, table-driven tests, parity tests against Okapi Framework data, mock AI providers for CI, and integration tests for flows.
keywords: [testing, roundtrip, table-driven tests, parity, Okapi, testify, mock providers, neokapi]
---

# ▒ Ţéšţ Šţŕàţéĝý ▒

## ▒ Þŕîñçîþļéš ▒

1. ▒ **Éṽéŕý ƒöŕḿàţ àñđ ţööļ ĥàš ţéšţš** — Ñö ƒöŕḿàţ ŕéàđéŕ/ŵŕîţéŕ öŕ ţööļ šĥîþš ŵîţĥöüţ ţéšţš. ▒
2. ▒ **Ŕöüñđţŕîþ îš ţĥé ĝöļđ šţàñđàŕđ** — Ƒöŕ ƒöŕḿàţš: ŕéàđ, ŵŕîţé, çöḿþàŕé ŵîţĥ öŕîĝîñàļ. ▒
3. ▒ **Þöŕţ Öķàþî ţéšţ đàţà** — Üšé Öķàþî'š ţéšţ ŕéšöüŕçé ƒîļéš àš ţĥé šöüŕçé öƒ ţŕüţĥ. ▒
4. ▒ **Ţàƃļé-đŕîṽéñ ţéšţš** — Ĝö'š ţàƃļé-đŕîṽéñ þàţţéŕñ ƒöŕ çöṽéŕîñĝ ḿüļţîþļé îñþüţš. ▒
5. ▒ **Ţéšţ àţ ţĥé îñţéŕƒàçé ƃöüñđàŕý** — Ţéšţ àĝàîñšţ `ĐàţàƑöŕḿàţŔéàđéŕ`/`Ţööļ` îñţéŕƒàçéš, ñöţ îñţéŕñàļš. ▒
6. ▒ **Đéţéŕḿîñîšţîç ÀÎ ţéšţš** — ÀÎ ţööļš üšé ḿöçķ þŕöṽîđéŕš îñ ÇÎ; ŕéàļ þŕöṽîđéŕš îñ ḿàñüàļ îñţéĝŕàţîöñ ţéšţš. ▒

## ▒ Ţéšţ Šţŕüçţüŕé ▒

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

## ▒ Ţéšţ Þàţţéŕñš ▒

### ▒ Ŕöüñđţŕîþ Ţéšţ ▒

▒ Ţĥé ḿöšţ îḿþöŕţàñţ ţéšţ ƒöŕ àñý ƒöŕḿàţ: ▒

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

### ▒ Éẋţŕàçţîöñ Ţéšţ ▒

▒ Ṽéŕîƒý šþéçîƒîç Ƃļöçķš àŕé éẋţŕàçţéđ ŵîţĥ çöŕŕéçţ çöñţéñţ: ▒

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

### ▒ Ƒļöŵ Éẋéçüţîöñ Ţéšţ ▒

▒ Ƃüîļđ à ţööļ ƃý éḿƃéđđîñĝ `ţööļ.ƂàšéŢööļ` àñđ šéţţîñĝ ĥàñđļéŕ ƒîéļđš, àššéḿƃļé à
ƒļöŵ ŵîţĥ ţĥé `ƒļöŵ.ÑéŵƑļöŵ(...).ÀđđŢööļ(...).Ƃüîļđ()` ƃüîļđéŕ (ŵĥîçĥ ŕéţüŕñš
`(*Ƒļöŵ, éŕŕöŕ)`), àñđ đŕîṽé îţ ŵîţĥ `ÉẋéçüţéŴîţĥÇĥàññéļš` ƒöŕ çĥàññéļ-ļéṽéļ
çöñţŕöļ: ▒

```go
func TestFlowExecution(t *testing.T) {
    uppercase := &tool.BaseTool{
        ToolName: "uppercase",
        Translate: func(v tool.VariantView) error {
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

## ▒ Ŕüññîñĝ Ţéšţš ▒

```bash
make test               # All tests
make test-unit          # Unit tests only (-short)
make test-race          # With race detector
make test-verbose       # Verbose output
make cover              # Coverage report
```

▒ Šîñĝļé ţéšţ: ▒

```bash
go test ./core/flow/ -run TestFlowExecutorContextCancellation -v
```

## ▒ Ţéšţ Ţàĝš ▒

| Tag           | Purpose                                  | Command                            |
| ------------- | ---------------------------------------- | ---------------------------------- |
| (none)        | Unit tests only                          | `go test ./...`                    |
| `integration` | + plugin and format integration          | `go test ./... -tags=integration`  |
| `acceptance`  | + native-format consumer-toolchain tests | `go test ./... -tags=acceptance`   |
| `parity`      | + Okapi parity comparison tests          | `go test ./... -tags=parity`       |
| `e2e`         | + end-to-end tests                       | `go test ./... -tags=e2e`          |

## ▒ ÇÎ ▒

▒ Ţéšţš ŕüñ àüţöḿàţîçàļļý ṽîà ĜîţĤüƃ Àçţîöñš öñ éṽéŕý þüšĥ àñđ þüļļ ŕéǫüéšţ. Šéé `.ĝîţĥüƃ/ŵöŕķƒļöŵš/çî.ýḿļ`. ▒
