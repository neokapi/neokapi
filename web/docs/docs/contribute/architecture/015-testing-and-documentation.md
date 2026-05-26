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
ADs, and implementation notes from one deployment. Demo assets are produced
by the **walkthrough/scenes engine** (issue #425): an authored walkthrough
prompt drives recorder artifacts — VHS `.tape` files for terminal scenes,
Playwright `.spec.ts` files for UI scenes — which record `.webm` videos that
land under `web/docs/static/video/`. Recordings run against real systems and
regenerate on behavior changes.

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
                / E2E \           Playwright video, VHS terminal recordings
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

**E2E tests** use [Playwright](https://playwright.dev/) for GUI workflows
with video capture enabled. The recordings double as regression tests
and documentation demos. CLI demos use [VHS](https://github.com/charmbracelet/vhs)
to record from declarative tape files.

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
│   ├── get-started/
│   ├── framework/
│   ├── guides/
│   ├── reference/           # generated command/format/tool reference
│   ├── cli/
│   ├── react/
│   ├── desktop/
│   ├── walkthroughs/        # generated MDX (one per walkthrough prompt)
│   └── contribute/
│       ├── architecture/    # framework ADs (this document)
│       └── notes-internal/  # implementation notes
├── walkthroughs/            # authored prompts: {id}.md + {id}.scene.yaml
├── scenes/                  # recorder artifacts: {id}/0N-*.tape|.spec.ts + .webm
└── static/
    ├── img/                 # static images (logos, favicons, staged)
    └── video/               # recorded scene + harness videos (staged)
```

A single `@docusaurus/plugin-content-docs` instance serves all content
from `web/docs/docs/` with `routeBasePath: "/"`. Audience separation is by
top-level section rather than by separate plugin instances:

- **User-facing docs** — `get-started/`, `framework/`, `guides/`,
  `reference/`, `cli/`, `react/`, `desktop/`.
- **Contributor docs** — `contribute/architecture/` (framework ADs,
  Apache-2.0 scope) and `contribute/notes-internal/` (implementation
  notes extracted from ADs).

ADs are organized by architectural concern and updated in place as
subsystems evolve, rather than appended chronologically. Implementation
notes live in `contribute/notes-internal/` for tactical details (schemas,
algorithms, API routes) that would otherwise bloat the decision documents.

Hosting is GitHub Pages, deployed via GitHub Actions on push to `main`.

### Walkthrough/scenes engine

Demo assets for the kapi site are produced by the walkthrough/scenes engine
(issue #425), authored prompt → recorded scene → published page:

1. **The walkthrough prompt** is the authored unit. Each lives at
   `web/docs/walkthroughs/{id}.md` with YAML frontmatter declaring an ordered
   `scenes:` list (each scene has an `id`, a `kind` — `terminal` or `web` —
   the `binary`, `fixtures`, and a `smoke_contract` of commands to re-run for
   regression). The prose sections (`## Story`, `## Scene N`, `## Closing`)
   are the source of truth for everything the published page says.

2. **Scene recorder artifacts** live under `web/docs/scenes/{id}/`, one
   directory per walkthrough holding `0N-{scene-id}.{tape,spec.ts}` plus any
   `fixtures/`. Today every kapi walkthrough is a `kind: terminal` scene
   recorded with [VHS](https://github.com/charmbracelet/vhs); the engine also
   supports `kind: web` scenes recorded with Playwright `.spec.ts` against a
   real backend. The `walkthrough-scenes` skill regenerates these artifacts
   deterministically from the prompt — the same prompt yields byte-identical
   scene files.

A VHS tape is declarative:

```tape
Output "01-pseudo-translate.webm"

Set FontSize 16
Set Width 900
Set Height 550
Set Theme "Dracula"
Set Padding 20

Type "kapi pseudo-translate messages.json -o messages.fr.json"
Enter
Sleep 1500ms
```

For terminal walkthroughs, a unified `web/docs/walkthroughs/{id}.scene.yaml`
spec (W3, issue #661) is the single authored source that drives **both** the
VHS `.tape` and the interactive in-browser playground embed
(`web/docs/src/components/KapiPlayground/embeds/{id}.embed.ts`), so the video
and the live wasm playground cannot drift. `scripts/walkthrough-gen/gen.ts`
emits both from one edit and keeps the prompt's `smoke_contract` in sync.

3. **The published page** is regenerated by the `walkthrough-doc` skill into
   `web/docs/docs/walkthroughs/{id}.mdx`, interleaving the prompt prose with
   `ThemedVideo` embeds (and, for interactive terminal walkthroughs, a
   `KapiEmbed` playground as the primary artifact). Recorded `.webm` videos
   are referenced by site-relative path:

```mdx
import { ThemedVideo } from "@neokapi/docs-shared";

<ThemedVideo
  sources={{
    light: "/video/kapi/01-pseudo-translate.webm",
    dark: "/video/kapi/01-pseudo-translate.webm",
  }}
/>
```

`ThemedVideo` (from `@neokapi/docs-shared`) matches the active colour scheme;
until per-theme assets exist, both `light` and `dark` point at the same
`.webm`. Harness-rendered Claude-explainer videos (`harness/`) use the same
component. WebM is preferred for size and quality.

The kapi docs site no longer generates screenshots: `web/docs/static/img/`
carries only static assets (logos, favicons) plus any image set staged from
the assets tarball.

### Real systems, not mocks

Scenes run against real neokapi infrastructure — a terminal scene records the
real `kapi` binary, a web scene drives a real backend:

- **CLI** — terminal scenes invoke the built `bin/kapi` against fixtures
  under the scene directory; no command is mocked.
- **Authentication and identity** — web scenes use the real Keycloak OIDC
  provider via `compose.yaml` (auth via session-cookie injection in the
  spec). Never mock the auth flow.
- **Server** — for web scenes, the real platform server binary, never a mock API server.
- **Database and storage** — a real SQLite database (the server creates
  one automatically).
- **External integrations** outside the scope of neokapi (third-party MT
  providers, external LLM APIs) may be mocked for isolation.

The `smoke_contract` in each prompt is what `walkthrough-verify` re-runs to
prove the recorded commands still pass; a behavior change that breaks a
documented command fails the contract, forcing a regenerate.

### Asset generation and staging

Scene videos are recorded on the desktop (not in CI) and published to a
GitHub release named `docs-assets`. The Makefile exposes only the targets
that exist:

```bash
make kapi-scenes          # record every web/docs/scenes/*/*.tape with VHS
                          #   (needs `brew install vhs` + bin/kapi) and
                          #   stage the .webm under web/docs/static/video/kapi/
make harness-videos       # render the narrated Claude-explainer videos
                          #   (light + dark) → web/docs/static/video/kapi/
make publish-docs-assets  # merge web/docs/static/{img,video} into the
                          #   docs-assets release (never drops existing assets)
make fetch-docs-assets    # download the docs-assets tarball into static/
                          #   (transitional, until the engine covers everything)
```

Assets are not stored in git. CI **does not record** scenes: the
`docs-kapi.yml` deploy workflow downloads the `docs-assets` tarball and copies
its `video/` into `web/docs/static/video/` before building the site (the wasm
playground is built separately and downloaded as a workflow artifact). A
developer who only edits documentation text can rely on the prebuilt tarball
via `make fetch-docs-assets` and skip recording.

The walkthrough skills automate the per-walkthrough loop:
`walkthrough-scenes` regenerates the recorder artifacts from a prompt,
`walkthrough-verify` records them against the real backend and checks the MDX
builds, and `walkthrough-doc` regenerates the published `.mdx`.

### Verification checklist for CLI/UI changes

Before committing a change that affects documented behavior:

1. TypeScript + lint + typecheck pass (`make frontend-check-all`).
2. Frontend unit tests pass (`vp test` in each package).
3. Production builds succeed.
4. Go build succeeds (`make build`).
5. Affected walkthroughs re-verify against the real backend
   (`walkthrough-verify`), so the scene `smoke_contract` still passes.
6. Affected scene videos are re-recorded on the desktop (`make kapi-scenes`)
   and republished (`make publish-docs-assets`) when the recorded output
   changed.

## Consequences

- Documentation lives alongside code, encouraging updates with
  features.
- Two-audience separation (user / developer) provides clear
  navigation.
- Architecture decisions and implementation notes are accessible
  both in-repo and on the website.
- Demo videos are generated from actual commands and UI, preventing
  drift.
- One authored walkthrough prompt drives the scene recorders and the
  published MDX, so a documented command, its recorded video, and (for
  terminal walkthroughs) the interactive playground stay in lock-step.
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
- [AD-013: Kapi CLI](013-kapi-cli.md) — VHS tapes exercise the CLI
- [AD-014: Kapi Desktop](014-kapi-desktop.md) — Playwright specs
  exercise the desktop
