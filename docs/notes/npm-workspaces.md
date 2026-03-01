---
sidebar_position: 11
title: "NPM Workspace Coordination"
---
# NPM Workspace Coordination

This note documents how the four frontend projects are coordinated via NPM workspaces, the build order constraints, and how the Makefile orchestrates frontend builds.

## Root Workspace Configuration

The root `package.json` declares an NPM workspace encompassing all four frontend projects:

```json
{
  "private": true,
  "workspaces": [
    "packages/ui",
    "bowrain/apps/web",
    "kapi/apps/kapi-web",
    "bowrain/apps/bowrain/frontend"
  ]
}
```

Note that `bowrain/apps/keycloak-theme` is intentionally excluded from the workspace. It manages its own dependencies independently because Keycloakify's build pipeline has specific dependency resolution requirements that conflict with workspace hoisting.

## The Four Frontend Projects

| Project | Package Name | Purpose | Key Dependencies |
|---------|-------------|---------|-----------------|
| `packages/ui` | `@gokapi/ui` | Shared React component library | shadcn-glass-ui, Tailwind v4, Radix UI |
| `bowrain/apps/web` | `gokapi-web` | SaaS web UI (bowrain-server) | React 19, Lexical, Tailwind v4 |
| `kapi/apps/kapi-web` | `gokapi-kapi-web` | Kapi serve web UI | React 19, Lexical, Tailwind v4 |
| `bowrain/apps/bowrain/frontend` | `bowrain` | Wails v3 desktop app frontend | React 19, Wails runtime, XYFlow, Lexical |

All four projects share React 19, Tailwind CSS v4, and TypeScript 5.8+. The three app projects consume `@gokapi/ui` through Vite path aliases rather than npm package resolution.

## @gokapi/ui Consumption Pattern

Rather than publishing `@gokapi/ui` as an npm package or using workspace protocol references, each app project resolves it through a Vite alias:

```ts
// vite.config.ts (each app)
resolve: {
  alias: {
    "@gokapi/ui": path.resolve(__dirname, "../../../packages/ui/src"),
  },
},
```

This means:
- Apps import directly from TypeScript source, not compiled output
- No separate build step for the UI package in development
- Changes to `packages/ui` are reflected immediately in Vite dev mode
- TypeScript compilation of `@gokapi/ui` happens as part of each app's build

However, for production builds, `packages/ui` must have its TypeScript declarations compiled first via `npx tsc` to ensure type checking passes.

## Build Order and Prerequisites

The build dependency graph is:

```
packages/ui (npm install)
    |
    +---> packages/ui (npx tsc)  [for type declarations]
    |         |
    |         +---> bowrain/apps/web (npm run build)
    |         |
    |         +---> kapi/apps/kapi-web (npm run build)
    |
    +---> bowrain/apps/bowrain/frontend (npm run build)
```

The critical constraint: **`packages/ui` dependencies must be installed before any app project can build**, because the Vite alias resolves to source files that import from `packages/ui/node_modules` (e.g., `shadcn-glass-ui`, `@radix-ui/react-slot`).

## Makefile Targets

The Makefile encodes these dependencies explicitly:

```makefile
# Shared UI package
ui-deps:                         ## Install shared UI package dependencies
	cd packages/ui && npm install

# SaaS web UI (bowrain-server)
web-deps:                        ## Install SaaS web UI dependencies
	cd $(WEB_DIR) && npm install

web-build: ui-deps web-deps      ## Build SaaS web UI for production
	cd packages/ui && npx tsc
	cd $(WEB_DIR) && npm run build

# Kapi web UI (kapi serve)
kapi-web-deps:                   ## Install kapi web UI dependencies
	cd $(KAPI_WEB_DIR) && npm install

kapi-web-build: ui-deps kapi-web-deps  ## Build kapi web UI for production
	cd packages/ui && npx tsc
	cd $(KAPI_WEB_DIR) && npm run build

# Bowrain desktop frontend
frontend-deps:                   ## Install frontend dependencies
	cd $(FRONTEND_DIR) && npm install

frontend-build: ui-deps frontend-deps  ## Build frontend for production
	cd $(FRONTEND_DIR) && npm run build
```

Key observations:
- Every app build target depends on `ui-deps` (which runs `npm install` in `packages/ui`)
- `web-build` and `kapi-web-build` run `cd packages/ui && npx tsc` before the app build to generate type declarations
- `frontend-build` (Bowrain desktop) does not run `npx tsc` on packages/ui separately because the Wails build handles TypeScript through its own pipeline

Server binaries that embed frontend assets chain through these targets:

```makefile
build-server: web-build          ## Build the Bowrain REST server
build-bowrain: frontend-build    ## Build the Bowrain desktop app
```

## Lock File Management

Each project maintains its own `package-lock.json`:

```
package-lock.json                           # Root workspace
packages/ui/package-lock.json               # Shared UI
bowrain/apps/web/package-lock.json          # Web app
kapi/apps/kapi-web/package-lock.json     # Kapi web
bowrain/apps/bowrain/frontend/package-lock.json  # Desktop frontend
bowrain/apps/keycloak-theme/package-lock.json    # Keycloak theme (outside workspace)
```

The root `package-lock.json` is the primary lock file for the NPM workspace. Individual lock files in workspace members may exist for historical reasons or for tools that look for a lock file relative to the package. The `node_modules/` directory and `dist/` output are gitignored globally.

## Pre-Commit Verification

Before committing frontend changes, the following checks must pass:

```bash
cd packages/ui && npm ci && npx tsc          # Shared UI builds
cd bowrain/apps/web && npm ci && npm run build   # Web app builds
cd kapi/apps/kapi-web && npm ci && npm run build  # Kapi web builds
```

The Bowrain desktop frontend build is verified separately through `make frontend-build`. The Keycloak theme build is verified via `make keycloak-theme`.

## Clean Target

The `make clean` target removes all `node_modules` and `dist` directories across the workspace:

```makefile
clean:
	rm -rf packages/ui/node_modules
	rm -rf $(FRONTEND_DIR)/dist $(FRONTEND_DIR)/node_modules
	rm -rf $(KAPI_WEB_DIR)/dist $(KAPI_WEB_DIR)/node_modules
	rm -rf $(WEB_DIR)/dist $(WEB_DIR)/node_modules
	rm -rf $(KC_THEME_DIR)/dist_keycloak $(KC_THEME_DIR)/node_modules
```
