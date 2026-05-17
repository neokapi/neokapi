---
id: 015-testing-and-documentation
sidebar_position: 15
title: "AD-015: Testing and Documentation"
---

# AD-015: Testing and Documentation

## Summary

neokapi follows a three-tier test pyramid (unit via testify, integration
via format roundtrips and flow E2E, application E2E via Playwright for
GUIs). Documentation is a Docusaurus 3 site with separate plugin
notes. Screenshots and recordings are generated from real systems —
regenerate on UI changes and land under `web/docs/static/`.

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
website/
├── docusaurus.config.ts     # site configuration
├── sidebars.ts              # main docs sidebar
├── sidebars-ad.ts           # framework AD sidebar
├── src/pages/               # custom React pages (landing)
├── docs/                    # user and developer documentation
│   ├── getting-started/
│   ├── user-guide/
│   ├── kapi-desktop/
│   └── developer/
└── static/
    ├── img/                 # screenshots (by app and theme)
    └── video/               # demo videos
```

The site uses multiple Docusaurus plugin instances to keep content
sources separate while serving them from a single deployment:

- **Main docs** — `web/docs/docs/` at `/`.
- **Framework ADs** — `docs/architecture-decisions/` at
  `/architecture-decisions/`. Apache-2.0 framework scope.
- **Notes** — `docs/notes/` at `/docs/notes/`. Implementation details
  extracted from ADs.

Example plugin configuration:

```typescript
plugins: [
  ['@docusaurus/plugin-content-docs', {
    id: 'architecture-decisions',
    path: '../docs/architecture-decisions',
    routeBasePath: 'architecture-decisions',
    sidebarPath: './sidebars-ad.ts',
  }],
  ['@docusaurus/plugin-content-docs', {
    id: 'notes',
    path: '../docs/notes',
    routeBasePath: 'docs/notes',
  }],
],
```

ADs are organized by architectural concern and updated in place as
subsystems evolve, rather than appended chronologically. Implementation
notes live in `docs/notes/` for tactical details (schemas, algorithms,
API routes) that would otherwise bloat the decision documents.

Hosting is GitHub Pages, deployed via GitHub Actions on push to `main`.

### Screenshot systems

Screenshots are captured via Playwright and written directly to
`web/docs/static/img/`. Two systems:

1. **Kapi Desktop screenshots** — in
   `apps/kapi-desktop/frontend/e2e/screenshots.spec.ts`. Self-contained
   (auto-starts a Vite dev server). Output:
   `web/docs/static/img/kapi-desktop/{dark,light}/`.
   `web/docs/static/img/web-app/{dark,light}/`.

Each screenshot spec runs in dark and light themes. Test suites capture
multiple views per run.

### Recording systems

Four independent recording pipelines:

1. **Kapi Desktop** — Playwright video capture in
   `apps/kapi-desktop/frontend/e2e/recordings.spec.ts`, dark + light
   themes.
2. **Kapi CLI** — [VHS](https://github.com/charmbracelet/vhs) terminal
   recordings from `.tape` files in `web/docs/tapes/`. No server
   required.
   4.VHS tape files are declarative:

```tape
Output output/convert.webm
Output output/convert.gif

Set FontSize 16
Set Width 900
Set Height 500
Set Theme "Dracula"

Type "kapi convert -i samples/messages.json -o output.yaml"
Enter
Sleep 1500ms
```

Playwright recordings use human-like interaction helpers:

- `humanClick()` — animated cursor movement to target.
- `humanType()` — character-by-character typing with realistic delays.
- `pause()` — visual pauses between actions.
- `injectWindowChrome()` — adds a window title bar for context.

Videos embed in MDX via HTML5 video elements:

```mdx
<video controls autoPlay loop muted width="100%">
  <source src="/video/kapi-cli/convert.webm" type="video/webm" />
</video>
```

WebM is preferred (smaller, better quality); GIFs are generated for
README embeds where video is not supported.

### Real systems, not mocks

All screenshots and recordings run against real neokapi infrastructure:

- **Authentication and identity** — the real Keycloak OIDC provider via
  `compose.yaml`. Never mock the auth flow.
  mock API server.
  creates one automatically).
- **External integrations** outside the scope of neokapi (third-party MT
  providers, external LLM APIs) may be mocked for isolation.

This rule makes documentation assets immediately obsolete when behavior
changes — the test run fails, forcing a regenerate.

### Asset generation

All documentation assets regenerate via Make targets:

```bash
make screenshots                   # kapi-desktop screenshots
make recordings                    # kapi-desktop recordings
make kapi-recordings               # kapi CLI tapes → webm/gif
make docs-assets                   # all of the above
make fetch-docs-assets             # download pre-built tarball
```

Assets are not stored in git. They are built in CI and uploaded to a
GitHub release named `docs-assets`; the docs deploy workflow fetches
that tarball before building the site. Developers who only edit
documentation text can skip asset generation locally and rely on the
prebuilt tarball.

A GitHub Actions workflow (`.github/workflows/screenshots-recordings.yml`)
runs asset generation:

- **On demand** — `workflow_dispatch`.
- **On release** — triggered by version tags.
- **Nightly** — scheduled at 02:00 UTC.

All four recording systems run in parallel jobs. A `publish-assets` job
creates a tarball and uploads it to the `docs-assets` release.

### Verification checklist for UI changes

Before committing any UI-related change:

1. TypeScript checks pass for all frontend projects.
2. All frontend unit tests pass (`vp test` in each package).
3. All production builds succeed.
4. All screenshots regenerated to `web/docs/static/img/`.
5. All recordings regenerated and copied to `web/docs/static/video/`.
6. Go build succeeds (`make build build-server`).

## Consequences

- Documentation lives alongside code, encouraging updates with
  features.
- Two-audience separation (user / developer) provides clear
  navigation.
- Architecture decisions and implementation notes are accessible
  both in-repo and on the website.
- Demo videos are generated from actual commands and UI, preventing
  drift.
- GitHub Pages hosting has no cost and integrates with the existing
  release workflow.
- The test pyramid enforces coverage at every level with appropriate
  speed and cost tradeoffs.
  means breaking changes in auth or the server API cause recording
  failures — a useful canary for integration regressions.
  share the same testing and documentation stack) keeps the
  documentation single-sourced and avoids duplicated infrastructure.

## Related

- [AD-001: Vision and Modules](001-vision-and-modules.md) — module
  layout tested at the `GOWORK=off` level
- [AD-013: Kapi CLI](013-kapi-cli.md) — VHS tapes exercise the CLI
- [AD-014: Kapi Desktop](014-kapi-desktop.md) — Playwright specs
  exercise the desktop
