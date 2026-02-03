---
id: 013-website-and-documentation
sidebar_position: 13
title: "ADR-013: Website and Documentation"
---
# ADR-013: Website and documentation system

## Context

gokapi needs comprehensive documentation for two distinct audiences:

1. **End users** — translators and localization engineers using the CLI, desktop
   app, or REST API. They need quickstart guides, command references, and
   workflow tutorials.

2. **Developers** — contributors implementing new formats, tools, or plugins.
   They need architecture documentation, interface specifications, and testing
   guides.

Additionally, the documentation should:

- Be hosted without infrastructure costs (GitHub Pages)
- Support versioning as gokapi evolves
- Include interactive demos that stay in sync with the actual implementation
- Reference Architecture Decision Records (ADRs) from the repository

## Decision

### Static Site Generator: Docusaurus

Use [Docusaurus](https://docusaurus.io/) v3 for the documentation website.
Docusaurus provides:

- Markdown/MDX authoring with frontmatter metadata
- Built-in versioning for docs
- Search, dark mode, and responsive design out of the box
- React components for custom pages and interactive elements
- Mermaid diagram support via `@docusaurus/theme-mermaid`

The website source lives in `website/` with this structure:

```
website/
├── docusaurus.config.ts    # Site configuration
├── sidebars.ts             # Main docs sidebar
├── sidebars-adr.ts         # ADR sidebar
├── src/
│   ├── pages/              # Custom React pages (landing page)
│   └── css/                # Custom styles
├── docs/                   # User and developer documentation
│   ├── getting-started/    # Quickstart, installation
│   ├── user-guide/         # CLI reference, formats, TM, AI
│   │   └── cli/            # Per-command documentation
│   ├── bowrain/            # Desktop app documentation
│   └── developer/          # Architecture, interfaces, testing
└── static/                 # Images, videos, and other assets
    ├── img/bowrain/        # Bowrain screenshots
    └── video/              # Demo videos (CLI and Bowrain)
```

### Hosting: GitHub Pages

Deploy to GitHub Pages at `https://gokapi.github.io/`. The site is built and
deployed via GitHub Actions on push to `main`. Configuration:

```typescript
// docusaurus.config.ts
url: 'https://gokapi.github.io',
baseUrl: '/',
organizationName: 'gokapi',
projectName: 'gokapi',
```

### Documentation Separation

#### User Guide (`docs/user-guide/`, `docs/getting-started/`, `docs/bowrain/`)

Task-oriented documentation for end users:

- **Getting Started** — Installation, quickstart, introduction
- **CLI Reference** — Per-command docs (`convert`, `translate`, `extract`,
  `merge`, `pack`, `unpack`, `plugins`)
- **Use Cases** — Real-world workflow tutorials (website translation, etc.)
- **Formats** — Supported file formats with configuration options
- **Translation Memory** — Pensieve usage, TMX import/export
- **AI Translation** — LLM provider configuration, prompt customization
- **Connectors** — DeepL, Google, Microsoft, MyMemory integration
- **Bowrain** — Desktop app overview, getting started, feature documentation

#### Developer Guide (`docs/developer/`)

Contributor-focused documentation:

- **Architecture** — System overview, package layout, streaming pipeline
- **Interfaces** — Core interfaces (`DataFormatReader`, `Tool`, `FlowExecutor`)
- **Formats** — How to implement a new format reader/writer
- **Tools** — How to implement a new tool
- **Plugins** — Plugin system, gRPC protocol, Java bridge
- **Testing** — Test patterns, roundtrip validation, test utilities
- **Release** — Release process, versioning, changelog

#### Architecture Decision Records (`../docs/adr/`)

ADRs live in the repository at `docs/adr/` (not inside `website/docs/`). They
are included in the website via a separate Docusaurus plugin instance:

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

This allows ADRs to:

- Be edited alongside the code they document
- Use standard markdown without Docusaurus-specific features
- Be referenced by developers who clone the repo but don't run the website
- Appear in the website navigation via a sidebar link

### Demo and Screencast Generation

Documentation includes animated demos that stay synchronized with the actual
CLI and UI implementation through automated generation.

#### CLI Demos: VHS

[VHS](https://github.com/charmbracelet/vhs) generates terminal screencasts from
declarative "tape" files. Tape files live in `docs/tapes/`:

```
docs/tapes/
├── overview.tape           # kapi --help, formats, tools
├── convert.tape            # Format conversion demo
├── word-count.tape         # Word counting for estimation
├── pseudo-translate.tape   # Pseudo-translation demo
├── create-project.tape     # Project creation demo
├── generate.sh             # Generation script
├── test-cli.sh             # Pre-recording validation
├── samples/                # Sample files for demos
│   ├── messages.json
│   └── landing-page.html
└── output/                 # Generated videos (gitignored)
```

Each tape file specifies terminal settings and a sequence of typed commands:

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

1. `test-cli.sh` validates that all CLI commands work before recording
2. VHS renders each tape to WebM (web) and GIF (README) formats
3. `generate.sh` copies output to `website/static/video/cli/`

Run with `make cli-recordings`. VHS requires a local TTY (not available in CI).

#### Bowrain Demos: Playwright Video Recording

Desktop app demos are generated from Playwright e2e tests with video capture
enabled. The recordings spec (`apps/bowrain/frontend/e2e/recordings.spec.ts`)
uses a mock backend to simulate realistic user workflows:

```typescript
// playwright.recordings.config.ts
use: {
  video: { mode: 'on', size: { width: 1280, height: 800 } },
  // Slower actions for better video visualization
  actionTimeout: 10000,
}
```

Demo tests use human-like interaction helpers:

- `humanClick()` — Animated cursor movement to target
- `humanType()` — Character-by-character typing with realistic delays
- `pause()` — Visual pauses between actions
- `injectWindowChrome()` — Adds window title bar for context

Each test represents a user workflow:

| Test | Workflow |
|------|----------|
| `create-project-flow` | New project wizard |
| `translation-editor-workflow` | Grid → split view → translate |
| `focus-view-editing` | Single-segment editing |
| `tm-explorer` | Translation memory search |
| `flow-editor` | Visual flow builder |
| `settings-configuration` | AI provider setup |

Generation workflow:

1. Playwright runs tests with video capture enabled
2. Videos are saved to `recordings-output/`
3. `copy-recordings.sh` renames and copies to `website/static/video/bowrain/`

Run with `make recordings`. Skipped in CI due to human-speed typing timeouts.

### Embedding Videos in Documentation

Videos are embedded in MDX files using HTML5 video elements:

```mdx
<video controls autoPlay loop muted width="100%" style={{maxWidth: '900px'}}>
  <source src="/video/cli/convert.webm" type="video/webm" />
</video>
```

WebM format is preferred for web (smaller, better quality). GIFs are generated
for README embeds where video isn't supported.

## Alternatives Considered

### Documentation Generators

- **MkDocs**: Python-based, simpler but less flexible for custom components
- **Hugo**: Fast but requires Go templates; less ecosystem for docs
- **GitBook**: Hosted service with costs; less control over customization
- **Plain GitHub Wiki**: No versioning, limited formatting, separate from repo

Docusaurus was chosen for its React foundation (matches Bowrain frontend),
strong docs-specific features, and active community.

### Demo Generation

- **asciinema**: Terminal-only, no GIF export, requires hosted player
- **terminalizer**: Less maintained, limited customization
- **Manual screen recording**: Doesn't stay in sync with implementation
- **Animated GIFs from screenshots**: Poor quality, large file sizes

VHS was chosen for CLI demos due to its declarative tape format and dual
WebM/GIF output. Playwright video recording was chosen for Bowrain demos
because tests already exist and the mock backend enables reproducible demos.

## Consequences

- Documentation lives alongside code, encouraging updates with features
- Two-audience separation (user/developer) provides clear navigation
- ADRs are accessible both in-repo and on the website
- Demo videos are generated from actual commands and UI, preventing drift
- GitHub Pages hosting has no cost and integrates with existing workflow
- Docusaurus updates may require migration effort (mitigated by LTS releases)
- VHS demos require local TTY, limiting CI automation
- Playwright demos are slower than real usage but provide consistent output
