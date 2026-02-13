---
id: 014-testing-and-docs
sidebar_position: 14
title: "ADR-014: Testing, Documentation, and Website"
---
# ADR-014: Testing, documentation, and website

## Context

gokapi needs comprehensive documentation for two distinct audiences:

1. **End users** -- translators and localization engineers using the CLI, desktop
   app, or REST API. They need quickstart guides, command references, and
   workflow tutorials.

2. **Developers** -- contributors implementing new formats, tools, or plugins.
   They need architecture documentation, interface specifications, and testing
   guides.

Testing strategy must cover the full spectrum from fast unit tests through
integration roundtrips to E2E demos. Because demo recordings exercise real CLI
commands and UI workflows, testing and documentation are tightly coupled: demos
serve as both validation and user-facing content.

The documentation should be hosted for free (GitHub Pages), support versioning,
and include interactive demos that stay synchronized with the actual
implementation.

## Decision

### Test Pyramid

```
                  /\
                 /  \
                / E2E \           Screen recordings, Playwright
               /------\
              / Integ  \          Format roundtrips, flow E2E
             /----------\
            /   Unit     \        Table-driven, testify
           /______________\
```

**Unit tests** use `github.com/stretchr/testify` (assert/require). Table-driven
tests are the standard pattern. Format tests do roundtrip validation (read,
write, compare). Test files colocate with implementation (`*_test.go`).

Run: `make test` (all), `make test-unit` (short), `make test-race` (race
detector). Single test: `go test ./core/flow/ -run TestName -v`.

**Integration tests** validate format roundtrips, pipeline end-to-end flows,
connector integration, and store operations. They run as part of the full test
suite without the `-short` flag.

**E2E tests** use Playwright for Bowrain UI workflows with video capture enabled.
The recordings serve double duty as regression tests and documentation demos.

**Screen recordings** are automated demo generation from VHS tape files (CLI)
and Playwright specs (Bowrain). They exercise real commands and UI interactions,
preventing documentation drift.

### Documentation: Docusaurus 3

Static site at `website/` using [Docusaurus](https://docusaurus.io/) v3:

```
website/
├── docusaurus.config.ts    # Site configuration
├── sidebars.ts             # Main docs sidebar
├── sidebars-adr.ts         # ADR sidebar
├── src/pages/              # Custom React pages (landing page)
├── docs/                   # User and developer documentation
│   ├── getting-started/    # Quickstart, installation
│   ├── user-guide/         # CLI reference, formats, TM, AI
│   │   └── cli/            # Per-command documentation
│   ├── bowrain/            # Desktop app documentation
│   └── developer/          # Architecture, interfaces, testing
└── static/
    ├── img/bowrain/        # Bowrain screenshots
    └── video/              # Demo videos (CLI and Bowrain)
```

**User Guide** -- CLI reference, format guides, TM usage, AI configuration,
connector setup, and Bowrain documentation (see [ADR-012](012-bowrain.md)).

**Developer Guide** -- Architecture, interfaces, format implementation, tool
implementation, plugin development, testing patterns, and release process.

**ADRs** live in `docs/adr/` (repository root), included via a separate
Docusaurus plugin instance:

```typescript
plugins: [
  ['@docusaurus/plugin-content-docs', {
    id: 'adr',
    path: '../docs/adr',
    routeBasePath: 'docs/adr',
    sidebarPath: './sidebars-adr.ts',
  }],
],
```

ADRs are organized by architectural concern and updated in place as subsystems
evolve, rather than appended chronologically.

### Hosting: GitHub Pages

Deploy to `https://gokapi.github.io/` via GitHub Actions on push to `main`.

### CLI Demos: VHS

[VHS](https://github.com/charmbracelet/vhs) generates terminal screencasts from
declarative tape files in `website/tapes/`. Each tape specifies terminal
settings and a sequence of typed commands, producing WebM (web) and GIF (README)
outputs:

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

Generation workflow:

1. `test-cli.sh` validates that all CLI commands succeed before recording
2. VHS renders each tape to WebM and GIF formats
3. `generate.sh` copies output to `website/static/video/cli/`

Run: `make cli-recordings`. VHS can run in CI with Xvfb for headless rendering.

### Bowrain Demos: Playwright Video Recording

Desktop app demos are generated from Playwright e2e tests with video capture.
The recording spec uses a mock backend to simulate realistic user workflows
with human-like interaction helpers:

- `humanClick()` -- animated cursor movement to target
- `humanType()` -- character-by-character typing with realistic delays
- `pause()` -- visual pauses between actions
- `injectWindowChrome()` -- adds window title bar for context

Demo workflows include project creation, translation editing, focus view, TM
explorer, flow editor, and settings configuration (see [ADR-012](012-bowrain.md)
for Bowrain architecture).

Run: `make recordings`. Runs in CI with extended timeout for human-speed typing.

### Embedding in Documentation

Videos are embedded in MDX files using HTML5 video elements:

```mdx
<video controls autoPlay loop muted width="100%" style=\{{maxWidth: '900px'}}>
  <source src="/video/cli/convert.webm" type="video/webm" />
</video>
```

WebM is preferred for web (smaller, better quality). GIFs are generated for
README embeds where video is not supported.

### Asset Generation

All documentation assets are generated via Makefile targets:

```
make cli-recordings    # CLI demos via VHS
make recordings        # Bowrain demos via Playwright
make screenshots       # Bowrain screenshots
make docs-assets       # All of the above
```

## Alternatives Considered

**Documentation generators** -- MkDocs (simpler but less flexible for custom
React components), Hugo (fast but Go templates less expressive than React),
GitBook (hosted service with costs and less control). Docusaurus was chosen for
its React foundation (matches the Bowrain frontend), strong docs-specific
features, and active community.

**Demo generation** -- asciinema (terminal-only, no GIF export), manual screen
recording (does not stay in sync with implementation), animated GIFs from
screenshots (poor quality, large files). VHS was chosen for its declarative tape
format; Playwright video for Bowrain because tests already exist and the mock
backend enables reproducible demos.

## Consequences

- Documentation lives alongside code, encouraging updates with features
- Two-audience separation (user/developer) provides clear navigation
- ADRs are accessible both in-repo and on the website
- Demo videos are generated from actual commands and UI, preventing drift
- GitHub Pages hosting has no cost and integrates with existing workflow
- Test pyramid ensures coverage at every level with appropriate speed/cost
  tradeoffs
- All screenshots and recordings can be generated in CI (VHS uses Xvfb,
  Playwright has extended timeouts)
- Playwright demos are slower than real usage but provide consistent output
- Asset generation is available on-demand (workflow_dispatch), on release
  (tags), and nightly (schedule)
