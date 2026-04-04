# shadcn Registry Pattern Evaluation

Evaluated April 2026 against shadcn v4.1.

## Context

The monorepo has a two-tier UI architecture:

- **`packages/ui`** (`@neokapi/ui-primitives`) — 37 shadcn primitives + domain components
- **`bowrain/packages/ui`** (`@neokapi/ui`) — 54+ domain components, consumes primitives
- **4 consuming apps**: kapi-desktop, bowrain desktop, bowrain web, bowrain ctrl

## Two Patterns Evaluated

### 1. Custom Registry (hosting registry.json at a URL)

The shadcn registry pattern lets you host a JSON registry at a URL and install
components via `shadcn add <url>`. It's designed for **distributing components to
external consumers** (open-source libraries, design systems used across orgs).

**Not adopted.** All packages are in the same monorepo. A custom registry would
add build steps (`shadcn build`), hosting infrastructure, and a JSON generation
pipeline with zero benefit over the current direct-import pattern.

### 2. Monorepo Pattern (components.json per workspace)

The shadcn monorepo pattern uses `components.json` in each workspace so the CLI
routes base primitives to the shared package and composed blocks to the app.

**Adopted.** Added `components.json` to all 6 workspaces:
- `packages/ui/components.json` — shared primitives (base target for `shadcn add`)
- `bowrain/packages/ui/components.json` — domain components
- `apps/kapi-desktop/frontend/components.json`
- `bowrain/apps/bowrain/frontend/components.json`
- `bowrain/apps/web/components.json`
- `bowrain/apps/ctrl/components.json`

This means `npx shadcn@latest add <component> -c <workspace>` works from any app.

## Import Pattern: Barrel Exports (not deep paths)

shadcn's monorepo template uses deep path imports (`@workspace/ui/components/button`).
We deliberately use **barrel imports** (`from "@neokapi/ui-primitives"`) instead:

- Simpler import statements (one import per file, not one per component)
- Single point of control for the public API
- Enforced by oxlint `no-restricted-imports` rule
- Tree-shaking works correctly with modern bundlers (Vite, esbuild)

## Revisit Triggers

- If shadcn adds first-class barrel export support to its monorepo template
- If the custom registry pattern adds local/workspace registry sources (no HTTP)
- If the component count in `packages/ui` exceeds ~80 and barrel import becomes a
  bundling bottleneck (unlikely with Vite's on-demand compilation)
