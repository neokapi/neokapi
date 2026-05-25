# neokapi: Test Strategy

## Table of Contents

- [Principles](#principles)
- [Test Structure](#test-structure)
- [Frontend Test Strategy](#frontend-test-strategy)
- [Porting Okapi Test Cases](#porting-okapi-test-cases)
- [Test Patterns](#test-patterns)
- [Integration Tests](#integration-tests)
- [Benchmarks](#benchmarks)
- [CI Configuration](#ci-configuration)

---

## Principles

1. **Every format and tool has tests** — No format reader/writer or tool ships without tests.
2. **Roundtrip is the gold standard** — For formats: read → write → compare with original.
3. **Port Okapi test data** — Use Okapi's test resource files as the source of truth.
4. **Table-driven tests** — Go's table-driven pattern for covering multiple inputs.
5. **Test at the interface boundary** — Test against `DataFormatReader`/`Tool` interfaces, not internals.
6. **Deterministic AI tests** — AI tools use mock providers in CI; real providers in manual integration tests.

---

## Test Structure

The project has two Go modules. Framework tests run from the root, platform
tests run from `bowrain/`. Both are exercised by `make test`.

```
neokapi/                              ── Framework Module Tests ──
├── model/
│   ├── model_test.go                # Block creation, targets, overlays
│   ├── run_test.go                  # Run sequence (canonical inline content)
│   └── identity_test.go             # Block identity / content hashing
├── flow/
│   ├── executor_test.go             # Flow execution, goroutine wiring, error propagation
│   └── builder_test.go              # Builder API
├── tool/
│   └── base_test.go                 # BaseTool dispatch, pass-through behavior
│
├── formats/
│   ├── plaintext/
│   │   ├── reader_test.go
│   │   ├── writer_test.go
│   │   └── testdata/
│   │       ├── simple.txt
│   │       ├── multiline.txt
│   │       ├── unicode.txt
│   │       └── empty.txt
│   ├── html/
│   │   ├── reader_test.go
│   │   ├── writer_test.go
│   │   └── testdata/
│   │       ├── simple.html
│   │       ├── inline_codes.html
│   │       ├── nested_tags.html
│   │       ├── attributes.html
│   │       ├── entities.html
│   │       └── utf8bom.html
│   └── ... (each format follows the same pattern)
│
├── tools/
│   ├── segmentation/
│   │   ├── tool_test.go
│   │   └── testdata/
│   │       ├── default.srx
│   │       └── sample_text.txt
│   └── ... (each tool follows the same pattern)
│
├── ai/
│   ├── tools/
│   │   ├── translate_test.go        # Uses mock provider
│   │   └── qualitycheck_test.go     # Uses mock provider
│   └── provider/
│       ├── mock.go                  # Mock LLM provider for testing
│       └── anthropic_test.go        # Integration test (requires API key)
│
├── plugin/
│   ├── host/
│   │   └── manager_test.go          # Plugin discovery, lifecycle
│   └── integration_test.go          # End-to-end plugin roundtrip
│
├── testdata/                         # Shared test data files
│   ├── html/
│   ├── xml/
│   ├── xliff/
│   ├── json/
│   ├── yaml/
│   ├── po/
│   ├── properties/
│   └── docx/                         # For Java bridge tests
│
├── testutil/                         # Shared test helpers (exported)
│   ├── helpers.go                    # Common test helpers
│   ├── mock_tool.go                  # Mock Tool implementation
│   ├── mock_reader.go                # Mock DataFormatReader
│   └── assert_parts.go              # Custom Part assertion helpers
│
└── bowrain/                          ── Platform Module Tests ──
    ├── store/                        # ContentStore + SQLite tests
    ├── auth/                         # OIDC, JWT, device flow tests
    ├── connector/                    # Connector integration tests
    ├── server/                       # HTTP/gRPC handler tests
    └── ...                           # Each platform package has *_test.go
```

---

## Frontend Test Strategy

The frontend spans three apps and a shared UI library. Testing is split into two complementary layers: **unit tests** for fast feedback on component logic, and **E2E tests** for full user-flow validation.

### Two-Layer Testing Model

| Layer    | Tool                           | Scope                                    | Speed              | Infrastructure               |
| -------- | ------------------------------ | ---------------------------------------- | ------------------ | ---------------------------- |
| **Unit** | Vitest + React Testing Library | Components, hooks, utilities             | ~2 s for 126 tests | None (jsdom)                 |
| **E2E**  | Playwright                     | Full user flows, screenshots, recordings | 30–60 s per suite  | Dev server or Docker backend |

Unit tests are the primary fast feedback loop for developers. They run in-memory with no browser or backend. E2E tests verify integration across the full stack and produce visual artifacts for documentation.

### Unit Tests (`bowrain/packages/ui`)

All shared UI components, contexts, hooks, and utilities live in `bowrain/packages/ui`. Unit tests are colocated in `bowrain/packages/ui/src/__tests__/`.

**Stack:** Vitest 4 + React Testing Library + jsdom

**Running:**

```bash
cd bowrain/packages/ui
npm test            # single run
npm run test:watch  # watch mode
```

**Configuration:** `bowrain/packages/ui/vitest.config.ts`

```typescript
export default defineConfig({
  test: {
    environment: "jsdom",
    setupFiles: ["./src/__tests__/setup.ts"],
  },
});
```

The setup file (`src/__tests__/setup.ts`) loads `@testing-library/jest-dom/vitest` for DOM matchers and registers explicit `cleanup()` in `afterEach` for reliable test isolation.

#### What to Unit Test

Tests are organized by what they exercise:

**Pure utilities** (no React, no mocking):

```
src/__tests__/codedText.test.ts      — Unicode marker parsing, segment roundtripping
src/__tests__/tagSemantics.test.ts   — Tag classification, pair building, validation, HTML preview
```

These are the highest-value tests: pure logic, fast, no dependencies.

**Context providers** (React, lightweight mocking):

```
src/__tests__/ThemeContext.test.tsx      — Theme persistence, system preference, DOM attributes
src/__tests__/AuthContext.test.tsx       — Authentication state transitions
src/__tests__/WorkspaceContext.test.tsx  — Workspace state management
src/__tests__/ApiContext.test.tsx        — Adapter injection
```

Each context test uses a small helper component that exposes the context value through `data-testid` elements, then asserts on DOM text content after `act()` interactions.

**Components** (React, render + interact):

```
src/__tests__/MainSidebar.test.tsx      — Navigation, collapse, theme toggle
src/__tests__/WorkspaceIcon.test.tsx    — Letter rendering, color hashing, active state
src/__tests__/WorkspaceRail.test.tsx    — Workspace list, selection, create button, avatar
src/__tests__/AccountMenu.test.tsx      — Dropdown open/close, sign-out callback
src/__tests__/TagValidationBar.test.tsx — Error/warning display, null handling
```

**Hooks** (React, mock API adapter):

```
src/__tests__/useLocales.test.tsx       — API fetch, loading state, display name resolution
```

#### What NOT to Unit Test

- **TranslationEditor** and **TargetCellEditor** — These integrate Lexical (rich text editor) which requires significant DOM infrastructure. Covered by E2E tests instead.
- **App shells** (`bowrain/apps/web/src/App.tsx`, etc.) — Thin wrappers that compose providers and route views. E2E tests cover the assembled behavior.
- **Semantic/status colors** — Hardcoded palette values in `tagSemantics.ts` and `WorkspaceIcon.tsx` are intentionally stable across themes. Visual correctness is verified by E2E screenshots.

#### Unit Test Patterns

**Helper component pattern** — Expose hook/context state through testable elements:

```tsx
function ThemeDisplay() {
  const { theme, resolvedTheme, setTheme } = useTheme();
  return (
    <div>
      <span data-testid="theme">{theme}</span>
      <span data-testid="resolved">{resolvedTheme}</span>
      <button data-testid="set-dark" onClick={() => setTheme("dark")}>
        Dark
      </button>
    </div>
  );
}
```

**Mock API adapter** — For hooks that depend on `useApi()`, create a mock adapter with `vi.fn()` stubs:

```tsx
const adapter = { getKnownLocales: vi.fn().mockResolvedValue(mockLocales), ... };
render(<ApiProvider adapter={adapter}><Component /></ApiProvider>);
await waitFor(() => expect(...));
```

**DOM attribute assertions** — Theme tests verify side effects on the real DOM:

```tsx
act(() => screen.getByTestId("set-dark").click());
expect(document.documentElement.dataset.theme).toBe("dark");
expect(localStorage.getItem("neokapi-theme")).toBe("dark");
```

#### Adding a New Unit Test

1. Create `src/__tests__/ComponentName.test.tsx` (or `.test.ts` for pure utilities).
2. Import from `vitest` and `@testing-library/react`.
3. Wrap components in the providers they need (ThemeProvider, ApiProvider, etc.).
4. Use `data-testid` attributes for stable selectors.
5. Run `npm test` to verify.

### E2E Tests (Playwright)

Each app has its own Playwright setup for integration-level testing against a running frontend (and optionally a backend).

#### Bowrain Desktop (13 spec files)

```
bowrain/apps/bowrain/frontend/e2e/
├── context-panel.spec.ts       — TM/term context panel in editor
├── flow-builder.spec.ts        — Visual flow builder
├── inline-codes.spec.ts        — Inline tag editing with coded text
├── project-dashboard.spec.ts   — Project creation and listing
├── project-view.spec.ts        — File management, upload, stats
├── recordings.spec.ts          — Screencast recordings for docs
├── rich-editor.spec.ts         — Lexical editor behavior
├── screenshots.spec.ts         — Static screenshots for docs
├── settings.spec.ts            — Settings page, theme toggle
├── term-explorer.spec.ts       — Terminology CRUD
├── tm-explorer.spec.ts         — Translation memory CRUD
├── tm-leverage.spec.ts         — TM leverage in translation
└── translation-editor.spec.ts  — Block editing, status, word count
```

**Running:**

```bash
cd bowrain/apps/bowrain/frontend
npx playwright test                    # all specs
npx playwright test e2e/settings.spec.ts  # single spec
```

**Configuration:** `playwright.config.ts` — uses Vite dev server with mock API routes. Tests seed data via `page.route()` interception, requiring no backend.

#### Web App (`bowrain/apps/web`)

```
bowrain/apps/web/e2e/
├── screenshots.spec.ts   — Dual-theme screenshots (dark + light)
├── recordings.spec.ts    — Dual-theme screencast recordings
└── helpers/
    └── api-client.ts     — Authentication, workspace/project creation, seeding
```

**Running:**

```bash
cd bowrain/apps/web
npm run e2e:screenshots   # requires Docker backend (bowrain-server)
npm run e2e:recordings    # requires Docker backend
```

**Configuration:** `playwright.config.ts` — connects to the real backend (defaults to `http://localhost:8080`, overridable via `BOWRAIN_URL` env var). Tests authenticate via device auth flow, create workspaces/projects, seed TM entries and terminology, then capture screenshots in both `dark/` and `light/` subdirectories for the documentation site.

#### kapi-web (`kapi/apps/kapi-web`)

```
kapi/apps/kapi-web/e2e/
├── screenshots.spec.ts   — Screenshots with mock API
└── mock-api.ts           — In-memory API mock via page.route()
```

**Running:**

```bash
cd kapi/apps/kapi-web
npx playwright test
```

**Configuration:** `playwright.config.ts` — auto-starts the Vite dev server. Uses `mock-api.ts` to intercept all `/api/v1/*` routes with in-memory stores. No backend required.

### Test Pyramid Summary

```
         ┌──────────────────────┐
         │   E2E (Playwright)   │  Full user flows, screenshots, recordings
         │   ~30 specs across   │  Requires dev server / Docker backend
         │   3 apps             │
         ├──────────────────────┤
         │                      │
         │  Unit (Vitest + RTL) │  Components, hooks, contexts, utilities
         │  126 tests in        │  No browser, no backend, ~2 seconds
         │  bowrain/packages/ui  │
         └──────────────────────┘
```

The unit tests in `bowrain/packages/ui` are designed to be the **primary fast feedback loop**. They validate all shared component logic that the three apps consume. The E2E tests then verify the assembled apps work end-to-end, including Lexical editor interactions, API flows, and visual theme correctness.

### Running All Frontend Tests

```bash
# Unit tests (fast, no infrastructure)
cd packages/ui && npm test

# E2E — Bowrain (mock API, no backend)
cd bowrain/apps/bowrain/frontend && npx playwright test

# E2E — kapi-web (mock API, no backend)
cd kapi/apps/kapi-web && npx playwright test

# E2E — web (requires Docker backend)
cd bowrain/apps/web && npm run e2e:screenshots
```

---

## Porting Okapi Test Cases

### Source of Test Data

Okapi's test resources are in its Git repository:

```bash
git clone https://gitlab.com/okapiframework/Okapi.git /tmp/okapi-tests
```

Test resources per filter:

```
Okapi/filters/<format>/src/test/resources/
```

### Porting Process

For each format:

1. **Identify test files**: Find representative test resource files in the Okapi filter's `src/test/resources/` directory.

2. **Copy test data**: Copy relevant files to `formats/<name>/testdata/` or `testdata/<name>/`.

3. **Translate assertions**: Convert Java JUnit assertions to Go `testing` + `testify` assertions.

**Java (Okapi):**

```java
@Test
public void testSimpleHtml() {
    String input = "<html><body><p>Hello</p></body></html>";
    IFilter filter = new HtmlFilter();
    filter.open(new RawDocument(input, "en"));

    Event event;
    assertTrue(filter.hasNext());
    event = filter.next();
    assertEquals(EventType.START_DOCUMENT, event.getEventType());

    assertTrue(filter.hasNext());
    event = filter.next();
    assertEquals(EventType.TEXT_UNIT, event.getEventType());
    TextUnit tu = event.getTextUnit();
    assertEquals("Hello", tu.getSource().toString());

    // ...
    filter.close();
}
```

**Go (neokapi):**

```go
func TestSimpleHTML(t *testing.T) {
    input := `<html><body><p>Hello</p></body></html>`
    reader := html.NewReader()
    err := reader.Open(ctx, testutil.RawDocFromString(input, "en"))
    require.NoError(t, err)
    defer reader.Close()

    parts := testutil.CollectParts(t, reader.Read(ctx))

    require.GreaterOrEqual(t, len(parts), 3) // layer start, block, layer end
    assert.Equal(t, model.PartLayerStart, parts[0].Type)

    block := testutil.FindFirstBlock(parts)
    require.NotNil(t, block)
    assert.Equal(t, "Hello", block.SourceText())
}
```

4. **Add roundtrip test**: For every format, add a roundtrip test.

5. **Add edge case tests**: Port Okapi's edge case tests (empty files, BOM handling, encoding issues, malformed input).

### Test Data Inventory

Files to port from Okapi (representative sample):

| Format     | Okapi Test Path                          | Key Files                                           |
| ---------- | ---------------------------------------- | --------------------------------------------------- |
| HTML       | `filters/html/src/test/resources/`       | Basic HTML, entities, inline codes, scripts, styles |
| XML        | `filters/xml/src/test/resources/`        | Simple XML, namespaces, CDATA, DTD references       |
| XLIFF      | `filters/xliff/src/test/resources/`      | XLIFF 1.2 files with various features               |
| XLIFF 2    | `filters/xliff2/src/test/resources/`     | XLIFF 2.0 with segments, notes                      |
| JSON       | `filters/json/src/test/resources/`       | Simple JSON, nested objects, arrays                 |
| YAML       | `filters/yaml/src/test/resources/`       | Scalars, multiline, anchors                         |
| PO         | `filters/po/src/test/resources/`         | Singular, plural, context, comments                 |
| Properties | `filters/properties/src/test/resources/` | Escapes, Unicode, multiline                         |

---

## Test Patterns

### Roundtrip Test

The most important test for any format: read a file, write it back, compare.

```go
func TestRoundTrip(t *testing.T) {
    tests := []struct {
        name string
        file string
    }{
        {"simple", "testdata/simple.html"},
        {"inline codes", "testdata/inline_codes.html"},
        {"nested tags", "testdata/nested_tags.html"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            original, err := os.ReadFile(tt.file)
            require.NoError(t, err)

            // Read
            reader := NewReader()
            err = reader.Open(ctx, testutil.RawDocFromFile(tt.file, "en"))
            require.NoError(t, err)

            parts := testutil.CollectParts(t, reader.Read(ctx))
            reader.Close()

            // Write
            var buf bytes.Buffer
            writer := NewWriter()
            writer.SetOutputWriter(&buf)
            writer.SetLocale(model.LocaleEnglish)

            ch := testutil.PartsToChannel(parts)
            err = writer.Write(ctx, ch)
            require.NoError(t, err)
            writer.Close()

            // Compare
            assert.Equal(t, string(original), buf.String())
        })
    }
}
```

### Extraction Test

Verify specific Blocks are extracted with correct content.

```go
func TestExtraction(t *testing.T) {
    reader := NewReader()
    err := reader.Open(ctx, testutil.RawDocFromFile("testdata/sample.html", "en"))
    require.NoError(t, err)
    defer reader.Close()

    blocks := testutil.CollectBlocks(t, reader.Read(ctx))

    require.Len(t, blocks, 3)
    assert.Equal(t, "Welcome", blocks[0].SourceText())
    assert.Equal(t, "Click here for more info", blocks[1].SourceText())
    assert.Equal(t, "Footer text", blocks[2].SourceText())
}
```

### Inline-code preservation test

Verify inline markup is carried as inline-code runs, not folded into the text.

```go
func TestInlineCodePreservation(t *testing.T) {
    input := `<p>Click <b>here</b> for <a href="url">info</a></p>`
    reader := NewReader()
    err := reader.Open(ctx, testutil.RawDocFromString(input, "en"))
    require.NoError(t, err)
    defer reader.Close()

    blocks := testutil.CollectBlocks(t, reader.Read(ctx))
    require.Len(t, blocks, 1)

    runs := blocks[0].SourceRuns()
    assert.Equal(t, "Click here for info", model.RunsText(runs))

    // Inline markup is carried as non-text (inline-code) runs — here two
    // paired codes (<b>…</b>, <a>…</a>) — never baked into the text.
    var inline []model.Run
    for _, r := range runs {
        if r.Text == nil {
            inline = append(inline, r)
        }
    }
    require.Len(t, inline, 4) // <b>, </b>, <a>, </a>

    require.NotNil(t, inline[0].PcOpen)
    assert.Equal(t, "fmt:bold", inline[0].PcOpen.Type)
    assert.Equal(t, "<b>", inline[0].PcOpen.Data)
}
```

### Flow Execution Test

Verify tools chain correctly in a Flow.

```go
func TestFlowExecution(t *testing.T) {
    // Create a flow with mock tools
    uppercaseTool := testutil.NewMockTool("uppercase", func(part *model.Part) *model.Part {
        if part.Type == model.PartBlock {
            block := part.Resource.(*model.Block)
            text := strings.ToUpper(block.SourceText())
            block.SetTargetText(model.LocaleFrench, text)
        }
        return part
    })

    f := flow.NewFlow("test").
        AddTool(uppercaseTool).
        Build()

    executor := flow.NewExecutor(reg)
    items := []*flow.Item{{
        Input:        testutil.RawDocFromString("Hello world", "en"),
        OutputPath:   "/dev/null",
        TargetLocale: model.LocaleFrench,
    }}

    err := executor.Execute(ctx, f, items)
    require.NoError(t, err)
    // Verify output contains "HELLO WORLD"
}
```

### Tool Dispatch Test

Verify BaseTool dispatches to correct handlers.

```go
func TestBaseToolDispatch(t *testing.T) {
    var handledTypes []model.PartType
    mockTool := &testutil.TrackingTool{
        OnBlock: func(p *model.Part) (*model.Part, error) {
            handledTypes = append(handledTypes, p.Type)
            return p, nil
        },
        OnData: func(p *model.Part) (*model.Part, error) {
            handledTypes = append(handledTypes, p.Type)
            return p, nil
        },
    }

    parts := []*model.Part{
        {Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc1"}},
        {Type: model.PartBlock, Resource: &model.Block{}},
        {Type: model.PartData, Resource: &model.Data{}},
        {Type: model.PartBlock, Resource: &model.Block{}},
        {Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc1"}},
    }

    testutil.RunToolOnParts(t, mockTool, parts)
    assert.Equal(t, []model.PartType{model.PartBlock, model.PartData, model.PartBlock}, handledTypes)
}
```

---

## Integration Tests

### End-to-End Format Integration

Test the full pipeline: format detection → read → flow → write.

```go
// integration_test.go (build tag: //go:build integration)

func TestEndToEndHTML(t *testing.T) {
    // 1. Detect format
    name, err := reg.DetectFormat("testdata/sample.html")
    require.Equal(t, "html", name)

    // 2. Build flow
    f := flow.NewFlow("e2e").
        AddTool(tools.NewSegmentationTool()).
        AddTool(tools.NewCopySourceTool()).
        Build()

    // 3. Execute
    executor := flow.NewExecutor(reg)
    err = executor.Execute(ctx, f, []*flow.Item{{
        Input:        testutil.RawDocFromFile("testdata/sample.html", "en"),
        OutputPath:   "testdata/output/sample_en.html",
        TargetLocale: model.LocaleEnglish,
    }})
    require.NoError(t, err)

    // 4. Verify output exists and is valid HTML
    output, err := os.ReadFile("testdata/output/sample_en.html")
    require.NoError(t, err)
    assert.Contains(t, string(output), "<html")
}
```

### Plugin Integration

```go
// //go:build integration

func TestPluginRoundTrip(t *testing.T) {
    // Build example CSV plugin
    exec.Command("go", "build", "-o", "testplugins/neokapi-format-csv",
        "./examples/plugin-format-csv").Run()

    // Load plugin
    mgr := plugin.NewPluginManager("testplugins/", "")
    mgr.DiscoverPlugins(reg, toolReg)

    // Verify CSV format is registered
    reader, err := reg.NewReader("csv")
    require.NoError(t, err)
    require.NotNil(t, reader)
}
```

### Java Bridge Integration

```go
// //go:build integration && java

func TestJavaBridgeDOCX(t *testing.T) {
    // Requires: Java runtime, built bridge JAR
    reader, err := bridge.NewJavaBridgeReader(
        "net.sf.okapi.filters.openxml.OpenXMLFilter",
    )
    require.NoError(t, err)

    err = reader.Open(ctx, testutil.RawDocFromFile("testdata/docx/sample.docx", "en"))
    require.NoError(t, err)

    blocks := testutil.CollectBlocks(t, reader.Read(ctx))
    require.NotEmpty(t, blocks)

    // Verify blocks contain expected content
    texts := testutil.BlockTexts(blocks)
    assert.Contains(t, texts, "Hello World")
}
```

---

## Benchmarks

### Format Reading Performance

```go
func BenchmarkHTMLRead(b *testing.B) {
    content, _ := os.ReadFile("testdata/large.html")

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        reader := html.NewReader()
        reader.Open(ctx, testutil.RawDocFromBytes(content, "en"))
        for range reader.Read(ctx) {
            // consume
        }
        reader.Close()
    }
}
```

### Flow Throughput

```go
func BenchmarkFlowThroughput(b *testing.B) {
    f := flow.NewFlow("bench").
        AddTool(tools.NewSegmentationTool()).
        AddTool(tools.NewWordCountTool()).
        Build()

    executor := flow.NewExecutor(reg)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        executor.Execute(ctx, f, items)
    }
}
```

### Native vs. Java Bridge

```go
func BenchmarkNativeVsBridge(b *testing.B) {
    b.Run("native-html", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            // Read with native HTML reader
        }
    })
    b.Run("bridge-html", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            // Read with Java bridge HTML reader
        }
    })
}
```

---

## CI Configuration

### GitHub Actions (`ci.yml`)

```yaml
name: CI
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        go: ["1.22", "1.23"]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go }}

      - name: Lint framework
        uses: golangci/golangci-lint-action@v4

      - name: Lint platform
        uses: golangci/golangci-lint-action@v4
        with:
          working-directory: bowrain

      - name: Framework tests
        run: go test ./... -race -coverprofile=framework.out

      - name: Platform tests
        run: cd bowrain && go test ./... -race -coverprofile=platform.out

      - name: Merge coverage
        run: |
          cat framework.out > coverage.out
          tail -n +2 bowrain/platform.out >> coverage.out

      - name: Upload Coverage
        uses: codecov/codecov-action@v4

  integration:
    runs-on: ubuntu-latest
    needs: test
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - uses: actions/setup-java@v4
        with:
          java-version: "17"
          distribution: "temurin"

      - name: Build Java Bridge
        run: cd plugin/bridge/java && mvn package -q

      - name: Framework integration tests
        run: go test ./... -tags=integration -race

      - name: Platform integration tests
        run: cd bowrain && go test ./... -tags=integration -race

  build:
    runs-on: ubuntu-latest
    needs: test
    strategy:
      matrix:
        goos: [linux, darwin, windows]
        goarch: [amd64, arm64]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - name: Build
        env:
          GOOS: ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
        run: |
          cd bowrain && go build -o kapi-${{ matrix.goos }}-${{ matrix.goarch }} ./cmd/kapi
```

### Test Tags

| Tag           | Purpose                         | Command                                                 |
| ------------- | ------------------------------- | ------------------------------------------------------- |
| (none)        | Unit tests only                 | `go test ./...` and `cd bowrain && go test ./...`       |
| `integration` | + plugin and format integration | `go test ./... -tags=integration` (both modules)        |
| `java`        | + Java bridge tests             | `go test ./... -tags="integration java"` (both modules) |
| `ai`          | + real AI provider tests        | `go test ./... -tags="integration ai"` (both modules)   |
