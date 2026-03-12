# Documentation Split Plan

This document describes how to split the current monorepo documentation into
**neokapi** (the open-source framework) and **bowrain** (the localization
platform) before the repository split runs.

All restructuring described here should be done in the monorepo and committed
**before** running `scripts/migrate-split.sh`.  The migration script's
`git filter-repo` path rules then place each file in the correct output repo
automatically.

---

## Guiding Principles

1. **neokapi is self-contained.**  A reader should be able to understand the
   framework, write a format reader, build a plugin, or run kapi without any
   knowledge of bowrain.
2. **bowrain links back to neokapi.**  Bowrain docs reference the framework
   (`github.com/neokapi/neokapi`) for concepts like content model, pipelines,
   and tools — they do not duplicate that content.
3. **No duplication.**  Every document lives in exactly one repo.
   Cross-cutting ADs are assigned to the repo that owns the majority of the
   content.  The other repo links to it via URL when needed.
4. **The website stays with neokapi** for now.  Bowrain gets its own minimal
   docs site later (or a subdirectory-based Docusaurus plugin when ready).

---

## Architecture Decisions (`docs/ad/`)

### → neokapi (framework)

| AD | Title | Reason |
|----|-------|--------|
| 002 | Content Model | Core framework primitive |
| 004 | Processing Engine | Channel-based pipeline, FlowExecutor |
| 006 | Tool System | BaseTool, tool dispatch |
| 007 | Plugin System and Okapi Bridge | go-plugin, gRPC bridge |
| 008 | AI Integration | LLMProvider interface |
| 009 | Translation Memory | TM interface and matching |
| 010 | Terminology | TermBase interface, concept model |
| 019 | MT Providers | MTProvider interface |

### → bowrain (platform)

| AD | Title | Reason |
|----|-------|--------|
| 003 | Content Store and Versioning | Server-side persistence |
| 005 | Connector System | CMS/Git/file connectors |
| 011 | Automation and Event System | Server-side automation |
| 012 | Bowrain Desktop App | Wails v3 desktop UI |
| 015 | Authentication and Workspaces | Multi-user platform |
| 016 | Bowrain Project Model | `.bowrain/` directories |
| 020 | Collaborative Editor | gRPC editor, real-time sync |
| 023 | Identity System | Short IDs, dual block identity |
| 024 | Streams | Git-like translation branching |

### Previously cross-cutting — assigned to one repo (no duplication)

| AD | Title | Destination | Reason |
|----|-------|-------------|--------|
| 001 | Vision | neokapi | Defines the framework mission; bowrain links back |
| 013 | CLI and Server | neokapi | Shared CLI base lives in neokapi; bowrain-specific server content moves to AD-012 |
| 014 | Testing and Documentation | neokapi | Testing strategy originates in the framework |
| 017 | CLI Output Format | neokapi | Shared CLI base owned by neokapi |
| 018 | Multi-Module Architecture | neokapi | Rewritten post-split to describe neokapi's module layout only |
| 021 | MCP Integration | neokapi | MCP tooling is a framework/kapi feature |
| 022 | Entity & Term Extraction | bowrain | Extraction pipeline is server-side orchestration |

**Action:** AD-018 should be rewritten after the split to describe the
`framework + cli + kapi` module layout.  Bowrain documents its own
`bowrain + bowrain-cli + platform` layout in its README or a new AD.

---

## Implementation Notes (`docs/notes/`)

### → neokapi

| File | Reason |
|------|--------|
| implementing-formats.md | Framework format development guide |
| plugin-bridge-protocol.md | Framework plugin protocol |
| tm-matching-algorithm.md | Framework TM implementation |
| terminology-data-model.md | Framework terminology model |
| cli-commands-reference.md | Shared CLI base commands (formats, tools, plugins) |
| mcp-tools-reference.md | MCP tools used by both CLIs |

### → bowrain

| File | Reason |
|------|--------|
| bowrain-ui-components.md | Bowrain React components |
| content-store-schema.md | SQLite/PostgreSQL schema |
| connector-interfaces.md | Bowrain connector implementation |
| docker-compose.md | Bowrain Docker infrastructure |
| glass-ui-theme.md | Bowrain UI theming |
| keycloak-theming.md | Bowrain auth provider theme |
| kapi-sync-protocol.md | Bowrain sync protocol |
| npm-workspaces.md | Bowrain npm workspace setup |
| skeleton-store.md | Bowrain storage layer |
| entity-term-extraction.md | Bowrain extraction pipeline |
| translation-job-queue.md | Bowrain server job queue |

---

## Top-Level Docs (`docs/`)

| File | Destination | Action |
|------|-------------|--------|
| ARCHITECTURE.md | neokapi | Keep as-is (describes framework architecture) |
| INTERFACES.md | neokapi | Keep (Go interface definitions) |
| RELEASE.md | neokapi | Bowrain writes its own release process post-split |
| TESTING.md | neokapi | Bowrain writes its own testing guide post-split |
| okapi-filter-frameworks.md | neokapi | Reference material for format porting |
| azure-deployment.md | bowrain | Server deployment guide |
| research/ | neokapi | Framework research |

---

## Website (`website/`)

The website goes to **neokapi**.  It currently has two sidebars: `gokapiSidebar`
(framework + kapi CLI) and `bowrainSidebar` (bowrain-specific).

### Pre-split actions

1. **Extract bowrain content from the website.**  Move the following sections
   out of `website/docs/` so that the neokapi website is clean and the content
   is ready for the bowrain Docusaurus site:
   - `bowrain-cli/` → bowrain website
   - `bowrain-desktop/` → bowrain website
   - `bowrain-getting-started/` → bowrain website
   - `bowrain-server/` → bowrain website

2. **Update `website/sidebars.ts`** to remove `bowrainSidebar`.

3. **Split `website/docs/developer/`** — move bowrain-specific pages:
   - `server.md` → bowrain
   - `connectors.md` → bowrain
   - `events.md` → bowrain
   - `content-store.md` → bowrain
   - Keep framework pages: `architecture.md`, `formats.md`, `interfaces.md`,
     `tools.md`, `plugins.md`, `java-bridge.md`, `testing.md`,
     `translation-memory.md`, `terminology.md`, `vocabularies.md`

4. **Keep `website/docs/features/`** in neokapi — these describe framework
   capabilities (formats, TM, terminology, AI, QA, MT).

5. **Keep `website/docs/getting-started/`** — kapi getting started.

6. **Keep `website/docs/kapi-cli/`** — kapi CLI reference.

7. **Update `website/docusaurus.config.ts`** — remove bowrain navbar item,
   update title/tagline from "gokapi" to "neokapi".

### Bowrain Docusaurus site (post-split)

Bowrain gets its own dedicated Docusaurus site under `website/` in the bowrain
repo.  Structure mirrors the neokapi site:

```
bowrain/
└── website/
    ├── docusaurus.config.ts   # bowrain branding, single sidebar
    ├── sidebars.ts            # bowrainSidebar (from current monorepo)
    ├── src/                   # Landing page, custom components
    └── docs/
        ├── getting-started/   # Bowrain getting-started content
        ├── bowrain-cli/       # Bowrain CLI reference
        ├── bowrain-server/    # Server admin & API docs
        ├── bowrain-desktop/   # Desktop app docs
        └── developer/         # server.md, connectors.md, events.md, content-store.md
```

Content sources:
- The bowrain-specific website pages extracted in pre-split step 1 above
- Bowrain-specific developer pages extracted in pre-split step 3 above
- Its own getting-started, CLI, server, and desktop docs
- Links back to the neokapi site for framework concepts (content model,
  pipelines, tools, formats)

---

## Execution Checklist

Before running `scripts/migrate-split.sh`:

- [ ] Move bowrain-only ADs to a `docs/ad-bowrain/` directory (or tag them)
- [ ] Move bowrain-only notes to `docs/notes-bowrain/`
- [ ] Remove bowrain website sections from `website/docs/`
- [ ] Update `website/sidebars.ts` (remove bowrainSidebar)
- [ ] Update `website/docusaurus.config.ts` (rename gokapi → neokapi)
- [ ] Move `docs/azure-deployment.md` to bowrain-owned path
- [ ] Rewrite AD-018 to describe the post-split module layout
- [ ] Update `scripts/migrate-split.sh` path rules if new directories are added
- [ ] Test the split with `--dry-run` to verify file placement

After the split:

- [ ] In neokapi: verify website builds (`cd website && npm run build`)
- [ ] In neokapi: verify all doc links resolve
- [ ] In neokapi: rewrite AD-018 for the `framework + cli + kapi` layout
- [ ] In bowrain: scaffold Docusaurus site (`website/`) with bowrain branding
- [ ] In bowrain: populate `website/docs/` from extracted bowrain content
- [ ] In bowrain: verify website builds (`cd website && npm run build`)
- [ ] In bowrain: document the `bowrain + bowrain-cli + platform` layout in a new AD
