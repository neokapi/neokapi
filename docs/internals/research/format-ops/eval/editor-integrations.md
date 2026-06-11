# Editor-embedding integrations inventory (neokapi repo)

Repo root: `/Users/asgeirf/src/neokapi/neokapi/.claude/worktrees/format-process` (all paths below relative to it unless absolute).

**Scope caveat (load-bearing):** the workspace add-ins and the google-workspace/microsoft365 connectors are **NOT on main/HEAD**. They live on open **PR #776** (`feat(bowrain): Google Workspace + Microsoft 365 workspace add-ons`, branch `origin/worktree-google-ms-addon`, head commit `3967be87b`; feature commits `0efa504c9` + `681f6228b`). `git merge-base --is-ancestor 0efa504c9 origin/main` → NOT in main. Everything labeled **[PR #776]** below was read via `git show origin/worktree-google-ms-addon:<path>`. Everything else is on HEAD.

---

## 1. bowrain/addin — shared in-editor add-in backend [PR #776]

`bowrain/addin/service.go` (326 lines). Package doc (lines 1–18) is the spec:

> "shared backend for Bowrain's in-product workspace add-ins — the Google Workspace add-on (Docs/Sheets/Slides) and the Microsoft 365 Office add-in (Word/Excel/PowerPoint). Both surfaces call the same three operations against the document the user is editing: **Check** — score text against a brand voice profile … **Terms** — surface the approved/forbidden/competitor terms … **Translate** — translate text on-brand into a target locale."

- Transport-agnostic `Service` with injected `LoadProfile` (defaults `packs.Load`, default pack `professional-b2b`) and `NewProvider` (defaults to keyless demo `aiprovider.Demo`; server overrides with `BOWRAIN_PLATFORM_PROVIDER`). Reuses framework engines: `core/brand`, `core/brand/packs`, `core/check`, `core/ai/tools`, `core/tools`, `providers/ai` (service.go imports, lines 22–36).
- **Office task pane REST surface**: `bowrain/addin/handlers.go` — `Service.RegisterRoutes(g)` mounts `POST /check`, `POST /terms`, `POST /translate` (Echo).
- **Google Card-JSON surface**: `bowrain/addin/google.go` (250 ln) + `bowrain/addin/cards.go` (319 ln) — Google's add-on runtime POSTs a `GoogleEvent` (`commonEventObject.hostApp` = `DOCS|SHEETS|SLIDES`, `authorizationEventObject.userOAuthToken`, `docs/sheets/slides` editor contexts); handlers build CardService JSON. `google.go` imports `bowrain/connector` — it reuses the **google-workspace connector** (with the user's OAuth token) to read/write the active document server-side.
- **Server wiring**: `bowrain/server/addin.go` `registerAddinRoutes(v1)` mounts under `/api/v1/addin` — fail-closed: REST half only when `JWTSecret` configured (behind `AuthMiddleware`); Google half only when `BOWRAIN_GOOGLE_ADDON_AUDIENCE` set, behind Google system ID-token verification (`bowrain/server/google_verify.go`, JWKS + issuer + audience + optional SA email).
- **Content exchange model**: plain **text** (selection or whole-doc) over JSON — no DataFormat, no openxml, no skeleton on this path. The doc on the branch (`bowrain/docs/integrations/workspace-addons.md`) draws the two-layer architecture: in-product add-ins (live, selection-level) vs server-side connectors (bulk file sync).

### 1a. Office task pane app [PR #776]
`bowrain/apps/office-addin/` — Vite + React task pane (`src/App.tsx`, 135 ln), sideload manifests `manifest.xml` (classic) + `manifest.json` (unified). Host integration in `src/office.ts` uses the **Office.js Common API only**:

> "The Common API (getSelectedDataAsync / setSelectedDataAsync) works uniformly across Word, Excel, and PowerPoint, so one implementation drives all three hosts."

So: read selection as text → call `/addin/check|terms|translate` → `replaceSelection()` writes the translation back over the selection (`setSelectedDataAsync`, coercion `"text"`). Auth: Office SSO `Office.auth.getAccessToken` with MSAL fallback noted in README. **Editors touched: Word, Excel, PowerPoint (selection-level text, live). Formats: none of neokapi's DataFormats — host API text only.**

### 1b. Google Workspace add-on [PR #776]
Two deployment modes, same backend:
- HTTP card backend: `bowrain/apps/google-workspace-addon/deployment.json` → bowrain-server's `/api/v1/addin/google/*` endpoints (cards.go builds the UI; google.go reads the active doc via the connector with the event's `userOAuthToken`).
- No-server Apps Script: `bowrain/apps/google-workspace-addon/apps-script/Code.gs` (135 ln) — reads the active document with `DocumentApp` / `SpreadsheetApp` / `SlidesApp` (`activeText_()` flattens Docs body text, Sheet display values, Slide shape text), calls the add-in REST API via `UrlFetchApp`, renders `CardService` cards.

**Editors touched: Google Docs, Sheets, Slides (sidebar, live). Content: whole-document text (Apps Script) or per-file via Workspace REST (HTTP mode).**

---

## 2. Connectors reaching editors/CMSes (`bowrain/connector/`)

Registry & taxonomy: `bowrain/core/connector/connector.go:23-29` — categories `file, code, cms, design, marketing, tms` (+ `productivity` added on PR #776). Interface: `Fetch / Publish / List / Status / Configure` returning `[]*platconn.ContentItem` (`Format` is a **free string**, `Blocks []*model.Block`). Registration split in `bowrain/connector/register.go`: `RegisterAll` (server/worker: local + remote) vs `RegisterRemote` (desktop: wordpress/figma/hubspot only) — local-FS connectors are server-side only per the product boundary.

| Connector | File | Editor/CMS | Fetch | Publish (write-back) | Native format use | Depth |
|---|---|---|---|---|---|---|
| **wordpress** | `bowrain/connector/wordpress.go` (205 ln) | WordPress REST `wp-json/wp/v2/posts` | title/content/excerpt → one Block each via `makeBlock` (wordpress.go:89-96); `Format:"html"` label (line 103) but the HTML is stored as a **raw string in a single block — the html DataFormat reader is never run** | `POST /wp-json/wp/v2/posts/{id}` with title/content/excerpt strings (lines 112-155) | none (label only) | field-level export/import round-trip; no inline-markup model |
| **figma** | `bowrain/connector/figma.go` (177 ln) | Figma file API `GET /v1/files/{key}` | recursive TEXT-node walk → blocks with `DisplayHint{MaxLength: bbox.Width/6}` (lines 157-177); `Format:"figma"` | **read-only**: `Publish` returns `errors.New("figma publish not yet supported via REST API")` (lines 105-109, needs Plugin/Variables API) | none | extract-only |
| **hubspot** | `bowrain/connector/hubspot.go` (198 ln) | HubSpot CMS `cms/v3/pages/site-pages` | page `htmlTitle` + `metaDescription` only (lines 82-101); `Format:"html"` label | `PATCH` htmlTitle/metaDescription (lines 105-156) | none | metadata-only round-trip (page body untouched) |
| **file** | `bowrain/connector/file.go` (226 ln) | local directory (server host) | full `registry.FormatRegistry` detection by extension + native reader (file.go:94-113) | native `NewWriter(item.Format)` (file.go:164-166) | **all registered DataFormats** (~49 under `core/formats/`) | true native round-trip |
| **git** | `bowrain/connector/git.go` (343 ln) | cloned repo | wraps an internal file connector (`ensureFileConnector`, git.go:216-226) | via file connector | all registered DataFormats | true native round-trip |
| **google-workspace** [PR #776] | `bowrain/connector/google.go` (711 ln) | Google Docs/Sheets/Slides via Drive/Docs/Sheets/Slides REST | per-kind extractors `fetchDoc/fetchSheet/fetchSlides` (lines 343, 430, 527); `gws_kind` metadata routes write-back; RevisionId for optimistic concurrency (line ~310) | Docs/Slides: `documents|presentations.batchUpdate` `replaceAllText`; Sheets: `values:batchUpdate` (lines 394-, 475-, 568-) | **deliberately none** — google.go:74-78: "there is no DataFormat round-trip because the structured Docs/Sheets/Slides APIs *are* the format" | live structured-API round-trip (text replacement) |
| **microsoft365** [PR #776] | `bowrain/connector/microsoft365.go` (519 ln) | OneDrive/SharePoint via MS Graph | Graph file bytes → **native `openxml` reader** (`officeExtensions`: .docx/.docm/.xlsx/.xlsm/.pptx/.pptm, lines 35-39; `fetchItem` line 289) | re-downloads original, then **faithful skeleton rebuild**: `renderItem` (lines 367-410) sets `SetOriginalContent(original)` via local `originalContentSetter` interface (lines 27-32) + `populateSkeleton` re-reads the original through a `coreformat.SkeletonStoreEmitter` so the faithful writer splices translated runs into the live archive (lines 416-440); uploads regenerated bytes | **openxml DataFormat, byte-faithful** | full native round-trip against live cloud files |

Server REST surface for connectors (HEAD): `bowrain/server/server.go:1094-1099` — `GET/POST /connectors`, `/connectors/:id/status|fetch|publish`.

---

## 3. Bowrain's own editor (platform editor = the deepest surface on HEAD)

This is in-*Bowrain*-editor, not in-native-editor, but it is the platform's embedded editing stack and a candidate E2 yardstick.

**Server API** — `bowrain/server/editor.go` (1280+ ln): `editorGetBlocks` (:615), `editorUpdateBlockTarget` (:634), `editorUpdateBlockTargetRuns` (:647, Run-native targets), `editorPseudoTranslate` (:659), `editorAITranslate` (:707), `editorTMTranslate` (:762), `editorLookupTMForBlock` (:850), `editorLookupTermsForBlock` (:895), word count (:807). Per-workspace persistent TM/TB (`workspaceStores`, :43-100).
**Visual preview** — `bowrain/server/handlers_preview.go`: `GET /editor/projects/:pid/file-preview/*?locale=xx` (:20) and `GET /editor/projects/:pid/blocks/:bid/html` (:73), rendered with `core/editor` (kat-block-marked HTML); QA: `handlers_qa.go` (:33, :60). Route mount e.g. `server.go:1286` `g.GET("/:id/preview/:ref", s.HandleRenderDocumentPreview)`.
**Real-time** — `bowrain/server/grpc_editor.go`: `EditorGRPCServer` with a `presenceStore` (live presence/co-editing events; see also `grpc_editor_events_test.go`).

**Shared UI** — `bowrain/packages/ui/src/components/`:
- `TranslationEditor.tsx` (grid editor; mounts `editor/TableView` at :745), `UnifiedTargetEditor.tsx`, `ReviewSurface.tsx`, `PreProcessSurface.tsx`; `editor/EditorSurfaceTabs.tsx` switches the three per-file surfaces "Pre-process · Translate · Review" (one strip drives **both** web and desktop, per its doc comment).
- `editor/DocumentPreview.tsx` — sandboxed iframe of the server-rendered preview; postMessage protocol (`kat-iframe-ready`, block-click selection at :137-, `kat-select-block`/scroll/`kat-update-block` live updates) so editing in the side panel updates the rendered document in place.
- `editor/VisualEditorCard.tsx` + `GridTargetRenderer/HighlightedSource/TermSidebar/ProblemsPanel/EntityPopover/TermCreationPopover` — block-level editing with TM matches, term hits, QA, history.
- Note: this editor still authors **coded-text** bridged via `codedToRuns.ts` (`DocumentPreview.tsx:12-50` works on `source_coded` + 0xE001-0xE003 span markers) — tracked in #695.

**Hosts**: web `bowrain/apps/web/src/routes/workspace/translate.tsx:54` mounts `TranslationEditor`; desktop `bowrain/apps/bowrain/frontend/src/components/DesktopTranslateView.tsx:2,86` mounts the same plus `useCollaboration` + `PresenceAvatars`. Desktop backend mirror: `bowrain/apps/bowrain/backend/preview.go` (`RenderDocumentPreview` from stored `PreviewHTML`/`BlockIndex` via `editor.BuildPreviewFromBlockIndex`).

**Formats covered**: whatever was ingested into the ContentStore (via file/git/CMS connectors or push) — block-level editing is format-agnostic; the *visual document preview* is format-aware only as far as `core/editor` goes (next section).

---

## 4. core/editor — the framework's preview substrate (HEAD)

- Dispatch: `core/editor/preview.go:11-16` — `BuildPreview(parts, reader)` delegates to `format.PreviewBuilder` if the reader implements it, else `buildGenericPreview` (escaped block list, `preview_generic.go`).
- Optional interface: `core/format/preview.go:12` `PreviewBuilder { BuildPreview([]*model.Part) string }`.
- **Implementations: exactly 3 of ~49 native formats** — `core/formats/html/preview.go:14`, `core/formats/markdown/preview.go:14`, `core/formats/mdx/preview.go:16` (delegating to `editor.BuildHTMLPreview` / `BuildMarkdownPreview` in `core/editor/preview_html.go` / `preview_markdown.go`).
- The kat-block iframe protocol lives here too: `PreviewBoilerplateStart/End` + `previewScript` (`preview.go:18-60`), sandbox note "innerHTML only for trusted, server-generated preview HTML… iframe is sandboxed".
- Consumers (grep `core/editor` --include=\*.go): `bowrain/server/{editor,handlers_preview}.go`, `bowrain/apps/bowrain/backend/{preview,project}.go`, `bowrain/plugin/connector/source.go` (`editor.ParseItem` at :1019 builds the Part stream for push), `apps/kapi-desktop/backend/inspect.go`, `kapi/cmd/kapi-wasm-cli/{lab,lab_annotate}.go` (WASM lab).

---

## 5. kapi-desktop editor surfaces (`apps/kapi-desktop/`) (HEAD)

- **FilePreview** — `frontend/src/components/FilePreview.tsx` (doc comment :33-44): "reuses the docs PreviewKit's DocumentViewer (Preview · Blocks · Stats · Download, with a source↔target toggle and annotation highlighting) … driven by the desktop's full native engine via the InspectFileAnnotated binding rather than the WASM runtime." Backend: `backend/inspect.go` `InspectFile` (:33) / `InspectFileAnnotated` (:54) — parses with the project's FormatRegistry (any registered format), overlays project targets (:152) and terminology/brand/QA annotations (:225). **Read-only**: no `UpdateBlockTarget`-style binding exists anywhere in `apps/kapi-desktop` (verified by grep) — kapi-desktop previews and runs flows; it does not edit blocks.
- **ContentPage.tsx** — project content collections (globs, formats, target paths) + opens FilePreview; **FormatConfigEditor.tsx** — edits format *configs* (SchemaForm), not content; **FlowPage/FlowsPage/RunnerPage** — flow composition via `@neokapi/flow-editor` (process editing, not document editing).

## 6. Shared preview kit `@neokapi/ui-primitives/preview` (`packages/ui/src/components/preview/`) (HEAD)

- `DocumentViewer.tsx` — "the shared preview editor": tabs Preview (FormatPreview) / Blocks (BlockInspector) / Raw (CodeView) / Stats / Download; source↔target toggle; overlay highlighting. Input = `ContentTree` from `kapi inspect` / `labInspectAnnotated`.
- `renderDoc.ts` — **the one place format→visual-shape knowledge lives in TS**: `RenderKind = "slides" | "sheet" | "doc" | "pages" | "list" | "sections"` (:36) and the data-driven `STRUCTURE_RULES` table (:375, dispatch :436-477; comment :364-366: "Add a new format's shape here — everything else degrades"). Recognized shapes today: PPTX (`ppt/slides/slideN.xml` layers), XLSX (`xl/worksheets/sheetN.xml`, cell refs), DOCX doc, pages; everything else degrades to doc/list/sections by format family.
- Consumers: kapi-desktop FilePreview; docs site `web/src/components/TryNeokapi/ModalBody.tsx:308` ("driven by the REAL kapi WASM"); `web/src/components/Lab/SegmentationPreviewInner.tsx:193`.

## 7. Docs lab explorers (`packages/kapi-lab/`, `web/src/`) (HEAD)

Teaching/sandbox surfaces running the real engine in WASM (`kapi/cmd/kapi-wasm-cli/lab.go`); read-only or sandbox-edit, not integrations into third-party editors: `AnatomyExplorer`, `PipelineExplorer`, `ToolLab`, `RoundTripExplorer`, `FlowBuilderRunner`, `FlowTracePlayer`, `KlfExplorer`/`KlfConformance`, `WorkspaceExplorer`, `ProjectExplorer`, `ScriptLab` (+`ScriptCodeEditor` — a code editor for kapi scripts, not content) — exports in `packages/kapi-lab/src/index.ts:11-52`. **`@neokapi/kapi-react` is NOT an editor embed** — it is build-time i18n for React JSX extracting KLF (`packages/kapi-react/package.json:5`).

---

## Per-surface summary table

| Surface | Formats covered | Depth | Key files |
|---|---|---|---|
| Office task pane (Word/Excel/PowerPoint) [PR #776] | host text only (no DataFormat) | **embedded** (live selection read/write in native editor) | `bowrain/apps/office-addin/src/{App,office,api}.tsx/ts`, `manifest.xml`, `bowrain/addin/handlers.go`, `bowrain/server/addin.go` |
| Google Workspace add-on (Docs/Sheets/Slides) [PR #776] | host text / Workspace structured APIs | **embedded** (sidebar cards; scan + translate active doc) | `bowrain/addin/{google,cards}.go`, `bowrain/apps/google-workspace-addon/apps-script/Code.gs`, `deployment.json`, `bowrain/server/google_verify.go` |
| microsoft365 connector [PR #776] | openxml (.docx/.docm/.xlsx/.xlsm/.pptx/.pptm) | **round-trip** (byte-faithful skeleton rebuild against live Graph files) | `bowrain/connector/microsoft365.go:255,327,367,416` |
| google-workspace connector [PR #776] | Docs/Sheets/Slides structured APIs (explicitly no DataFormat) | **round-trip** (batchUpdate replaceAllText / values write; revision-checked) | `bowrain/connector/google.go:209,261,394,475,527`, `oauth.go` |
| wordpress connector (HEAD) | "html" label; raw strings, html reader unused | round-trip (3 fields/post) | `bowrain/connector/wordpress.go:81,112` |
| hubspot connector (HEAD) | "html" label; title+meta only | round-trip (metadata only) | `bowrain/connector/hubspot.go:75,105` |
| figma connector (HEAD) | "figma" (TEXT nodes + DisplayHint) | **extract-only** (Publish errors) | `bowrain/connector/figma.go:86,105,157` |
| file/git connectors (HEAD, server-only) | all ~49 registered DataFormats | round-trip (native readers/writers) | `bowrain/connector/{file,git,register}.go` |
| Bowrain web+desktop translation editor (HEAD) | format-agnostic blocks; visual preview format-aware for html/markdown/mdx only | **embedded platform editor** (block edit + TM/terms/QA + live iframe preview + gRPC presence) | `bowrain/server/editor.go:615-905`, `handlers_preview.go:20,73`, `grpc_editor.go:25`, `bowrain/packages/ui/src/components/{TranslationEditor.tsx,editor/*}`, `bowrain/apps/web/src/routes/workspace/translate.tsx`, `bowrain/apps/bowrain/frontend/src/components/DesktopTranslateView.tsx` |
| core/editor preview substrate (HEAD) | PreviewBuilder: html, markdown, mdx; generic fallback for the rest | preview | `core/format/preview.go:12`, `core/editor/preview*.go`, `core/formats/{html,markdown,mdx}/preview.go` |
| kapi-desktop FilePreview (HEAD) | all registered formats (native engine) | **preview** (read-only; overlays, source↔target, raw, download) | `apps/kapi-desktop/frontend/src/components/FilePreview.tsx`, `apps/kapi-desktop/backend/inspect.go:33-54` |
| Shared PreviewKit (HEAD) | structure-aware shapes: pptx slides, xlsx sheet, doc, pages; generic for rest | preview | `packages/ui/src/components/preview/{DocumentViewer.tsx,renderDoc.ts:364-477,FormatPreview.tsx}` |
| Docs lab explorers (HEAD) | engine formats via WASM; sandbox only | preview/sandbox | `packages/kapi-lab/src/index.ts`, `web/src/components/TryNeokapi/ModalBody.tsx`, `kapi/cmd/kapi-wasm-cli/lab.go` |

---

## What an E0–E3 editor-embedding axis can deterministically measure today — and what's missing

A workable rubric grounded in existing artifacts:

- **E0 (none)** — format only gets the generic escaped block-list preview. Default.
- **E1 (preview)** — format renders a structure-aware preview. Deterministically detectable two ways: (a) Go: reader implements `format.PreviewBuilder` (`core/format/preview.go:12`) — assertable in a `core/formats/maturity_test.go`-style guardrail with a compile-time/`interface{}` probe per registered reader (today: html, markdown, mdx = 3/49); (b) TS: format/layer shape matched by a `STRUCTURE_RULES` entry in `packages/ui/src/components/preview/renderDoc.ts:375` (today: openxml pptx/xlsx/docx shapes). The TS table is **invisible to Go tooling** — it would need to be exported as a JSON artifact (the repo already has the `//go:generate`-committed-artifact convention) to be scored from one place.
- **E2 (faithful round-trip editing)** — translated/edited blocks can be spliced back into the original document bytes. Deterministically detectable: writer implements `coreformat.SkeletonStoreConsumer` and/or `SetOriginalContent` — but note the `originalContentSetter` interface is **locally re-declared** in `bowrain/connector/microsoft365.go:27-32` instead of exported from `core/format`, so today there is no single authoritative interface to probe. Roundtrip byte-equality tests per format already exist (format maturity framework) and can gate this level.
- **E3 (embedded in the native editor)** — a live in-editor surface (task pane / add-on / sidebar) or live structured-API write-back. Today this is **only enumerable by convention**: add-in manifests (`bowrain/apps/office-addin/manifest.{xml,json}`, `bowrain/apps/google-workspace-addon/{appsscript.json,deployment.json}`) and connector registrations (`bowrain/connector/register.go` `RegisterAll`/`RegisterRemote` + `Category` in `bowrain/core/connector/connector.go:23-29`). Nothing machine-readable links them to formats.

**Missing for determinism (the gaps an axis would need closed):**
1. **No integration registry/manifest.** Connectors register by name+category via Go `init`-style calls; add-in apps aren't indexed anywhere (not in `registry/plugins.json` either). A small committed JSON (connector/add-in → editors touched → formats → capabilities) generated from the registries would make E3 scoreable.
2. **`ContentItem.Format` is a free string** (`bowrain/core/connector/connector.go`), not a `registry.FormatID` — wordpress/hubspot claim `"html"` without running the html DataFormat, figma invents `"figma"`. The axis cannot trust this field.
3. **No capability declaration on connectors** — figma's read-only-ness is a runtime `errors.New` in `Publish` (`figma.go:105-109`), not a `CanPublish()` capability; depth (extract-only vs round-trip vs live) is undiscoverable without executing.
4. **Preview depth split across Go (`PreviewBuilder`) and TS (`STRUCTURE_RULES`)** with no shared dataset; and the bowrain visual editor's iframe preview only goes beyond generic for 3 formats.
5. **The whole E3 layer is unmerged** (PR #776) — on main, the only editor-adjacent integrations are wordpress/figma/hubspot (shallow) and the platform's own editor.
