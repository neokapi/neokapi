---
id: 015-testing-and-documentation
sidebar_position: 15
title: "AD-015: Testing and Documentation"
description: "Architecture decision: neokapi follows a three-tier test pyramid — unit via testify, integration via format roundtrips and flow E2E, application E2E via Playwright for the desktop app and the web UI."
keywords: [testing, documentation, testify, roundtrip, Playwright, E2E, architecture decision, neokapi]
---

# AD-015: Testing and Documentation

## Summary

neokapi follows a three-tier test pyramid (unit via testify, integration
via format roundtrips and flow E2E, application E2E via Playwright for
GUIs). Documentation is a Docusaurus 3 site serving user docs, framework
ADs, and implementation notes from one deployment. Demo assets come from two
complementary pipelines: interactive in-browser walkthroughs, where an
authored `{id}.scene.yaml` is compiled by `scripts/walkthrough-gen/gen.ts`
into a `KapiPlayground` embed that runs the real kapi CLI as WebAssembly; and
narrated explainer videos, rendered by the `harness/` pipeline as per-theme
`.webm`s under `web/docs/static/video/`. Both run against real systems.

## Context

A localization framework with a CLI, a desktop app, and an integration
platform must cover a wide testing surface. Fast unit tests protect
refactors; roundtrip tests protect format fidelity; application E2E tests
protect user workflows. Documentation must stay synchronized with actual
behavior — a screenshot or terminal recording that shows the wrong
command defeats the purpose.

Demo recordings exercise real CLI commands and real UI workflows, which
makes testing and documentation tightly coupled: recordings serve as both
regression tests and user-facing content. Avoiding mocks in recordings
keeps the documented behavior honest.

The documentation consumer is split in two:

- **End users** — translators, localization engineers. They need
  quickstart guides, command references, and workflow tutorials.
- **Developers** — contributors implementing formats, tools, plugins,
  connectors. They need architecture documentation, interface
  specifications, and testing guides.

A single Docusaurus site with per-audience navigation covers both while
keeping deployment simple.

## Decision

### Test pyramid

```
                  /\
                 /  \
                / E2E \           Playwright (desktop + web UI suites)
               /------\
              / Integ  \          Format roundtrips, flow E2E, store tests
             /----------\
            /   Unit     \        Table-driven, testify
           /______________\
```

**Unit tests** use `github.com/stretchr/testify` (`assert` and `require`).
Table-driven tests are the standard pattern. Test files colocate with
implementation (`*_test.go`). Each test starts from fresh state — no
shared mutable fixtures across tests.

```go
tests := []struct {
    name    string
    input   string
    want    string
    wantErr bool
}{
    {"simple", "hello", "HELLO", false},
    {"empty", "", "", false},
}
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        got, err := Upper(tt.input)
        if tt.wantErr {
            require.Error(t, err)
            return
        }
        require.NoError(t, err)
        assert.Equal(t, tt.want, got)
    })
}
```

**Integration tests** validate format roundtrips (read → write → compare),
pipeline end-to-end flows with real tools, connector integration, and
store operations. They run as part of `make test` without the `-short`
flag.

**E2E tests** use [Playwright](https://playwright.dev/) for GUI workflows.
The web UI and desktop frontends keep their own suites (e.g.
`bowrain/apps/web/e2e`, `bowrain/apps/bowrain/frontend/e2e`, `bowrain/e2e/cloud`).
These are application tests, separate from the documentation demos described
below.

**Make targets:**

```bash
make test              # all tests
make test-unit         # unit tests only (-short flag)
make test-race         # tests with race detector
make test-verbose      # verbose output
make cover             # coverage report → coverage/coverage.html
```

Run a single test: `go test ./core/flow/ -run TestExecutorCancellation -v`.

### Documentation site

The site at `web/docs/` uses [Docusaurus](https://docusaurus.io/) 3 with
React 19:

```
web/docs/
├── docusaurus.config.ts     # site configuration (single docs instance at "/")
├── sidebars.ts              # docs sidebar
├── src/pages/               # custom React pages (landing)
├── docs/                    # all documentation, served at "/"
│   ├── kapi/                # CLI + desktop + get-started + guides + walkthrough MDX
│   ├── framework/
│   ├── react/
│   ├── toolbox/             # format-aware kgrep/ksed/kcat utilities
│   ├── reference/           # generated command/format/tool reference
│   └── contribute/
│       ├── architecture/    # framework ADs (this document)
│       └── notes-internal/  # implementation notes
├── walkthroughs/            # authored prompts: {id}.md + {id}.scene.yaml
├── scenes/                  # per-walkthrough WASM-embed fixtures: {id}/ (seeded in-browser)
└── static/
    ├── img/                 # static images (logos, favicons, staged)
    └── video/               # harness explainer videos (staged from the release)
```

A single `@docusaurus/plugin-content-docs` instance serves all content
from `web/docs/docs/` with `routeBasePath: "/"`. Audience separation is by
top-level section rather than by separate plugin instances:

- **User-facing docs** — `kapi/` (CLI, desktop, get-started, guides, and
  walkthrough MDX), `framework/`, `react/`, `toolbox/`, `reference/`.
- **Contributor docs** — `contribute/architecture/` (framework ADs,
  Apache-2.0 scope) and `contribute/notes-internal/` (implementation
  notes extracted from ADs).

ADs are organized by architectural concern and updated in place as
subsystems evolve, rather than appended chronologically. Implementation
notes live in `contribute/notes-internal/` for tactical details (schemas,
algorithms, API routes) that would otherwise bloat the decision documents.

Hosting is GitHub Pages, deployed via GitHub Actions on push to `main`.

### Walkthrough/embed engine

Demo assets for the kapi site come from the walkthrough engine (issue #425),
authored prompt → generated embed → published page:

1. **The walkthrough prompt** is the authored unit. Each lives at
   `web/docs/walkthroughs/{id}.md` with YAML frontmatter declaring an ordered
   `scenes:` list (each scene has an `id`, a `kind` — `terminal` or `web` —
   the `binary`, `fixtures`, and a `smoke_contract` of commands to re-run for
   regression). The prose sections (`## Story`, `## Scene N`, `## Closing`)
   are the source of truth for everything the published page says.

2. **The interactive embed** is generated from the companion
   `web/docs/walkthroughs/{id}.scene.yaml` (the unified spec, W3, issue #661).
   kapi walkthroughs are **embed-only**: `scripts/walkthrough-gen/gen.ts`
   compiles each scene into a `KapiPlayground` embed config
   (`web/docs/src/components/KapiPlayground/embeds/{id}.embed.ts`, plus the
   `index.ts` registry and `types.ts`) and keeps the prompt's `smoke_contract`
   in sync. Any fixture bytes the embed seeds in the browser live under
   `web/docs/scenes/{id}/`. The generator is deterministic — it formats output
   with `vp fmt` and writes no timestamps, so re-runs are idempotent.

   The embeds are committed, so the site builds straight from them; regenerate
   only when a `.scene.yaml` changes:

   ```bash
   node --experimental-strip-types scripts/walkthrough-gen/gen.ts <id>   # one walkthrough
   node --experimental-strip-types scripts/walkthrough-gen/gen.ts --all  # all walkthroughs
   node --experimental-strip-types scripts/walkthrough-gen/gen.ts --check # fail if any output is stale
   ```

   There is no make target, npm script, or CI step that invokes the generator;
   it is run by hand when a spec changes. The embeds render live in the browser
   against the kapi CLI compiled to WebAssembly (`make web-wasm-cli` →
   `web/docs/static/wasm/kapi-cli.wasm.gz`).

3. **The published page** interleaves the prompt prose into the recipe and
   get-started MDX under `web/docs/docs/kapi/`, embedding a `KapiPlayground`
   (the primary artifact for interactive terminal walkthroughs) and, where a
   narrated explainer exists, a `ThemedVideo`. Videos are referenced by
   site-relative path, with per-theme assets:

```mdx
import { ThemedVideo } from "@neokapi/docs-shared";

<ThemedVideo
  sources={{
    light: "/video/kapi/bilingual-workflow-light.webm",
    dark: "/video/kapi/bilingual-workflow-dark.webm",
  }}
/>
```

`ThemedVideo` (from `@neokapi/docs-shared`) matches the active colour scheme.
Harness-rendered explainer videos (`harness/`) supply matched light and dark
`.webm`s; WebM is preferred for size and quality.

The kapi docs site no longer generates screenshots: `web/docs/static/img/`
carries only static assets (logos, favicons) plus any image set staged from
the assets tarball.

### Real systems, not mocks

Demo assets run against real neokapi infrastructure — the embeds execute the
real `kapi` CLI (compiled to WebAssembly) and the harness videos drive a real
backend:

- **CLI** — embeds run the real kapi WASM CLI against fixtures under the scene
  directory; no command is mocked.
- **Authentication and identity** — harness web demos use the real Keycloak
  OIDC provider via `compose.yaml`. Never mock the auth flow.
- **Server** — harness web demos use the real platform server binary, never a
  mock API server.
- **Database and storage** — a real SQLite database (the server creates
  one automatically).
- **External integrations** outside the scope of neokapi (third-party MT
  providers, external LLM APIs) may be mocked for isolation.

The `smoke_contract` in each prompt is re-run by `make docs-verify-snippets`
(driving the WASM CLI) to prove the documented commands still pass; a behavior
change that breaks a documented command fails the contract.

### Asset generation and staging

Videos are rendered on the desktop (not in CI) and published to a GitHub
release named `docs-assets`. The Makefile exposes only the targets that exist:

```bash
make harness-videos       # render the narrated explainer videos (light + dark)
                          #   → web/docs/static/video/kapi/
make harness-videos-staged # full pass: stack up → seed → record → narrate → package
make publish-docs-assets  # merge web/docs/static/{img,video} into the
                          #   docs-assets release (never drops existing assets)
make fetch-docs-assets    # download the docs-assets tarball into static/
                          #   (transitional, until the engine covers everything)
make web-wasm-cli         # build the in-browser kapi CLI → static/wasm/kapi-cli.wasm.gz
```

The interactive embeds need no recording step: their generated
`embeds/*.embed.ts` are committed, and they render live against the WASM CLI.
Videos are not stored in git. CI **does not record or render**: the
`docs-kapi.yml` deploy workflow downloads the `docs-assets` tarball and copies
its `video/` into `web/docs/static/video/` before building the site (the wasm
playground is built separately and downloaded as a workflow artifact). A
developer who only edits documentation text can rely on the prebuilt tarball
via `make fetch-docs-assets`.

To regenerate an embed after editing its `.scene.yaml`, run
`scripts/walkthrough-gen/gen.ts <id>` by hand (see above); to refresh a video,
render it with the `harness-*` targets and republish with
`make publish-docs-assets`.

### Verification checklist for CLI/UI changes

Before committing a change that affects documented behavior:

1. TypeScript + lint + typecheck pass (`make frontend-check-all`).
2. Frontend unit tests pass (`vp test` in each package).
3. Production builds succeed.
4. Go build succeeds (`make build`).
5. Affected walkthrough `smoke_contract`s still pass under
   `make docs-verify-snippets` (the commands run in the WASM CLI).
6. Affected embeds are regenerated (`scripts/walkthrough-gen/gen.ts <id>`) and,
   when a demo video changed, re-rendered on the desktop (`make harness-videos`)
   and republished (`make publish-docs-assets`).

## Consequences

- Documentation lives alongside code, encouraging updates with
  features.
- Two-audience separation (user / developer) provides clear
  navigation.
- Architecture decisions and implementation notes are accessible
  both in-repo and on the website.
- Demo videos are generated from actual commands and UI, preventing
  drift.
- One authored walkthrough prompt drives both the generated interactive embed
  and the published MDX, so a documented command and its in-browser playground
  stay in lock-step.
- GitHub Pages hosting has no cost and integrates with the existing
  release workflow.
- The test pyramid enforces coverage at every level with appropriate
  speed and cost tradeoffs.
- Recording against real systems means breaking changes in auth or the
  server API cause recording failures — a useful canary for integration
  regressions.
- A single documentation stack shared across the CLI, desktop, and
  platform keeps the documentation single-sourced and avoids duplicated
  infrastructure.

## Related

- [AD-001: Vision and Modules](001-vision-and-modules.md) — module
  layout tested at the `GOWORK=off` level
- [AD-013: Kapi CLI](013-kapi-cli.md) — interactive embeds run the CLI as WASM
- [AD-014: Kapi Desktop](014-kapi-desktop.md) — Playwright suites test the desktop
