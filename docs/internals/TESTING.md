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

The project is a multi-module monorepo. Go test files colocate with the code
they exercise (`*_test.go`), and each module runs from its own directory. The
framework runs from the repo root; the shared CLI, kapi, kapi-desktop, and the
bowrain modules run from their own roots. `make test` exercises them all.

```
neokapi/                              ── Framework Module Tests (repo root) ──
├── core/
│   ├── model/
│   │   ├── model_test.go            # Block creation, targets, overlays
│   │   ├── run_test.go              # Run sequence (canonical inline content)
│   │   └── identity_test.go         # Block identity / content hashing
│   ├── flow/
│   │   └── executor_test.go         # Flow execution, goroutine wiring, error propagation
│   ├── tool/
│   │   └── base_test.go             # BaseTool dispatch, pass-through behavior
│   ├── formats/
│   │   ├── plaintext/
│   │   │   ├── reader_test.go
│   │   │   ├── writer_test.go
│   │   │   └── testdata/            # simple.txt, multiline.txt, unicode.txt, empty.txt, …
│   │   ├── html/
│   │   │   ├── reader_test.go
│   │   │   ├── writer_test.go
│   │   │   └── testdata/            # simple.html, inline_codes.html, entities.html, …
│   │   └── …                         # each format follows the same pattern
│   ├── tools/                        # Built-in utility tools, with *_test.go + testdata/
│   └── internal/testutil/            # Shared test helpers (INTERNAL to the framework module)
│       └── helpers.go                # RawDocFromString, CollectParts, CollectBlocks, FindFirstBlock, …
├── sievepen/                         # TM tests (in-memory + SQLite, matching)
├── termbase/                         # Terminology tests (in-memory + SQLite, import/export)
├── providers/
│   ├── ai/                           # package aiprovider — provider + AI tool tests (demo provider for CI)
│   └── mt/                           # package mtprovider — provider + MT tool tests
│
├── cli/                              ── CLI Module Tests ──
│   └── pluginhost/                   # Manifest discovery, dispatch, daemon-pool tests
├── kapi/                             ── Kapi Module Tests ──
│   ├── cmd/kapi/                     # Root command + MCP tool wiring tests
│   └── e2e/                          # CLI end-to-end tests (isolated config/data/cache)
├── apps/kapi-desktop/                ── Kapi Desktop Module Tests ──
│   └── backend/                      # Go backend tests
│
└── bowrain/                          ── Bowrain Module Tests ──
    ├── core/                         # bowrain/core module
    ├── cli/                          # bowrain/cli module (kapi-bowrain plugin)
    ├── plugin/                       # bowrain/plugin module
    ├── store/ auth/ connector/ server/   # server-side package tests
    └── …                              # each package has *_test.go
```

The Okapi Java bridge has no Go package in this repo. It lives in the separate
[okapi-bridge](https://github.com/neokapi/okapi-bridge) repository, ships as a
plugin binary, and is exercised through `cli/pluginhost` (see
[Bridge protocol](../../web/docs/docs/contribute/notes-internal/plugin-bridge-protocol.md)).

---

## Frontend Test Strategy

The frontend spans three apps and a shared UI library. Testing is split into two complementary layers: **unit tests** for fast feedback on component logic, and **E2E tests** for full user-flow validation.

### Two-Layer Testing Model

| Layer    | Tool                           | Scope                                    | Speed              | Infrastructure               |
| -------- | ------------------------------ | ---------------------------------------- | ------------------ | ---------------------------- |
| **Unit** | Vitest + React Testing Library | Components, hooks, utilities             | seconds            | None (jsdom)                 |
| **E2E**  | Playwright                     | Full user flows, screenshots, recordings | tens of seconds    | Dev server or Docker backend |

Unit tests are the primary fast feedback loop for developers. They run in-memory with no browser or backend. E2E tests verify integration across the full stack and produce visual artifacts for documentation.

### Unit Tests (`bowrain/packages/ui`)

All shared UI components, contexts, hooks, and utilities live in `bowrain/packages/ui`. Unit tests are colocated in `bowrain/packages/ui/src/__tests__/`.

**Stack:** Vitest 4 + React Testing Library + jsdom

**Running:**

```bash
cd bowrain/packages/ui
vp test            # single run
vp run test:watch  # watch mode
```

**Configuration:** `bowrain/packages/ui/vite.config.ts`

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
5. Run `vp test` to verify.

### E2E Tests (Playwright)

Each app has its own Playwright setup for integration-level testing against a running frontend (and optionally a backend).

#### Bowrain Desktop

Specs live under `bowrain/apps/bowrain/frontend/e2e/`, one `*.spec.ts` per
user flow — for example:

```
bowrain/apps/bowrain/frontend/e2e/
├── context-panel.spec.ts       — TM/term context panel in editor
├── flow-builder.spec.ts        — Visual flow builder
├── inline-codes.spec.ts        — Inline tag editing with coded text
├── project-dashboard.spec.ts   — Project creation and listing
├── project-view.spec.ts        — File management, upload, stats
├── rich-editor.spec.ts         — Lexical editor behavior
├── settings.spec.ts            — Settings page, theme toggle
├── term-explorer.spec.ts       — Terminology CRUD
├── tm-explorer.spec.ts         — Translation memory CRUD
├── tm-leverage.spec.ts         — TM leverage in translation
└── translation-editor.spec.ts  — Block editing, status, word count
```

**Running:**

```bash
cd bowrain/apps/bowrain/frontend
vpx playwright test                    # all specs
vpx playwright test e2e/settings.spec.ts  # single spec
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
vp run e2e:screenshots   # requires Docker backend (bowrain-server)
vp run e2e:recordings    # requires Docker backend
```

**Configuration:** `playwright.config.ts` — connects to the real backend (defaults to `http://localhost:8080`, overridable via `BOWRAIN_URL` env var). Tests authenticate via device auth flow, create workspaces/projects, seed TM entries and terminology, then capture screenshots in both `dark/` and `light/` subdirectories for the documentation site.

#### kapi-react (`packages/kapi-react`)

`@neokapi/kapi-react` is the React component library (the in-browser kapi
engine wrapper). It is tested with Vitest, not Playwright; specs live under
`packages/kapi-react/tests/` and exercise the engine directly (extract,
transform, ICU plural roundtrip, hash parity, …).

**Running:**

```bash
cd packages/kapi-react
vp test
```

### Test Pyramid Summary

```
         ┌──────────────────────┐
         │   E2E (Playwright)   │  Full user flows, screenshots, recordings
         │   one spec per flow  │  Requires dev server / Docker backend
         │   across the apps    │
         ├──────────────────────┤
         │                      │
         │  Unit (Vitest + RTL) │  Components, hooks, contexts, utilities
         │  colocated in        │  No browser, no backend, seconds
         │  packages/ui, …       │
         └──────────────────────┘
```

The unit tests in `bowrain/packages/ui` are designed to be the **primary fast feedback loop**. They validate all shared component logic that the three apps consume. The E2E tests then verify the assembled apps work end-to-end, including Lexical editor interactions, API flows, and visual theme correctness.

### Running All Frontend Tests

```bash
# Unit tests (fast, no infrastructure)
cd packages/ui && vp test
cd packages/kapi-react && vp test

# E2E — Bowrain (mock API, no backend)
cd bowrain/apps/bowrain/frontend && vpx playwright test

# E2E — web (requires Docker backend)
cd bowrain/apps/web && vp run e2e:screenshots
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
            err = reader.Open(ctx, testutil.RawDocFromReader(
                bytes.NewReader(original), tt.file, "en"))
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
    f, err := os.Open("testdata/sample.html")
    require.NoError(t, err)
    defer f.Close()

    reader := NewReader()
    err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/sample.html", "en"))
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

    executor := flow.NewExecutor() // functional options; defaults to sequential
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
    src, err := os.Open("testdata/sample.html")
    require.NoError(t, err)
    defer src.Close()

    executor := flow.NewExecutor()
    err = executor.Execute(ctx, f, []*flow.Item{{
        Input:        testutil.RawDocFromReader(src, "testdata/sample.html", "en"),
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

### Plugin Discovery and Dispatch

Plugins are out-of-process binaries discovered from on-disk `manifest.json`
files; the host-side runtime lives in `cli/pluginhost`. `Discover` reads the
manifests under each plugin root (no subprocess is launched to enumerate),
and `NewHost` folds them into dispatch tables for commands, MCP tools,
formats, and recipe-schema extensions. Tests point discovery at a temp root
via `DiscoverOptions.EnvPluginsDir` and disable the user/system roots:

```go
func TestDiscover(t *testing.T) {
    tmp := t.TempDir()
    // write tmp/<plugin>/manifest.json …

    plugins := pluginhost.Discover(pluginhost.DiscoverOptions{
        EnvPluginsDir: tmp, // $KAPI_PLUGINS_DIR
        XDGDataHome:   "",  // disable per-user root
        HomeDir:       "/nonexistent",
        SystemDirs:    []string{}, // disable system roots
    })
    require.Len(t, plugins, 1)
    assert.Equal(t, "demo", plugins[0].Manifest.Plugin)

    host := pluginhost.NewHost(plugins, func(msg string) { t.Log(msg) })
    _ = host // assert on the dispatch tables
}
```

See the [plugin model](../../web/docs/docs/contribute/notes-internal/plugin-model.md)
note for the in-process registry contract and
[AD-007: Plugin system](../../web/docs/docs/contribute/architecture/007-plugin-system.md)
for discovery and the A/B/C transport modes.

### Okapi Java bridge

The Okapi Java bridge is a Mode-C plugin daemon hosted in the separate
[okapi-bridge](https://github.com/neokapi/okapi-bridge) repository — there is
no `bridge.NewJavaBridgeReader` and no Java package in this repo. The host
side (`cli/pluginhost`) spawns the daemon, connects over a Unix-socket gRPC
`BridgeService`, and converts between neokapi Parts and Okapi Events via
`core/plugin/protoconvert`. Bridge format tests exercise it through the same
discovery/dispatch path as any other plugin; the wire protocol is documented
in the [bridge protocol](../../web/docs/docs/contribute/notes-internal/plugin-bridge-protocol.md)
note.

---

## Benchmarks

### Format Reading Performance

```go
func BenchmarkHTMLRead(b *testing.B) {
    content, _ := os.ReadFile("testdata/large.html")

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        reader := html.NewReader()
        reader.Open(ctx, testutil.RawDocFromReader(
            bytes.NewReader(content), "large.html", "en"))
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

    executor := flow.NewExecutor()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        executor.Execute(ctx, f, items)
    }
}
```

### Native vs. Java bridge

Compares a native Go reader against the same format served by the Okapi bridge
plugin (a Mode-C daemon — the bridge path pays JVM/gRPC cost, amortized by the
long-lived daemon pool).

```go
func BenchmarkNativeVsBridge(b *testing.B) {
    b.Run("native-html", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            // Read with the native HTML reader
        }
    })
    b.Run("bridge-html", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            // Read with the Okapi bridge plugin (via cli/pluginhost daemon)
        }
    })
}
```

---

## CI Configuration

### GitHub Actions (`ci.yml`)

The real `ci.yml` runs one test job per module (framework, cli, kapi,
kapi-desktop, bowrain, bowrain/core, bowrain/cli, bowrain/plugin) plus
frontend and lint jobs. It pins Go `1.26.0` and additionally tries `stable` on
pushes. The shape below is illustrative:

```yaml
name: CI
on: [push, pull_request]

jobs:
  test-framework:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        go-version: ["1.26.0", "stable"]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go-version }}

      - name: Framework tests
        run: go test ./... -race

  test-bowrain:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.26.0"
      - name: Bowrain tests
        run: cd bowrain && go test ./... -race

  build:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        goos: [linux, darwin, windows]
        goarch: [amd64, arm64]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.26.0"
      - name: Build kapi
        env:
          GOOS: ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
        run: cd kapi && go build -o kapi-${{ matrix.goos }}-${{ matrix.goarch }} ./cmd/kapi
```

The Okapi Java bridge is **not** built in this CI — it is a separate repo
([okapi-bridge](https://github.com/neokapi/okapi-bridge)) released as a plugin
binary, so no `setup-java` / `mvn package` step exists here.

### Test Tags

| Tag           | Purpose                         | Command                                            |
| ------------- | ------------------------------- | -------------------------------------------------- |
| (none)        | Unit tests only                 | `go test ./...` (per module)                       |
| `integration` | + plugin and format integration | `go test ./... -tags=integration`                  |
| `ai`          | + real AI provider tests        | `go test ./... -tags="integration ai"`             |

Some packages need build tags for native dependencies — for example `fts5`
for SQLite full-text search (used by the kapi e2e suite).
