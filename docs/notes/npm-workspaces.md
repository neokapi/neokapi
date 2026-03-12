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
| `packages/ui` | `@neokapi/ui` | Shared React component library | shadcn-glass-ui, Tailwind v4, Radix UI |
| `bowrain/apps/web` | `neokapi-web` | SaaS web UI (bowrain-server) | React 19, Lexical, Tailwind v4 |
| `kapi/apps/kapi-web` | `neokapi-kapi-web` | Kapi serve web UI | React 19, Lexical, Tailwind v4 |
| `bowrain/apps/bowrain/frontend` | `bowrain` | Wails v3 desktop app frontend | React 19, Wails runtime, XYFlow, Lexical |

All four projects share React 19, Tailwind CSS v4, and TypeScript 5.8+. The three app projects consume `@neokapi/ui` through Vite path aliases rather than npm package resolution.

## @neokapi/ui Consumption Pattern

Rather than publishing `@neokapi/ui` as an npm package or using workspace protocol references, each app project resolves it through a Vite alias:

```ts
// vite.config.ts — bowrain/apps/web and kapi/apps/kapi-web (3 levels up)
resolve: {
  alias: {
    "@neokapi/ui": path.resolve(__dirname, "../../../packages/ui/src"),
  },
},

// vite.config.ts — bowrain/apps/bowrain/frontend (4 levels up)
resolve: {
  alias: {
    "@neokapi/ui": path.resolve(__dirname, "../../../../packages/ui/src"),
  },
},
```

This means:
- Apps import directly from TypeScript source, not compiled output
- No separate build step for the UI package in development
- Changes to `packages/ui` are reflected immediately in Vite dev mode
- TypeScript compilation of `@neokapi/ui` happens as part of each app's build

However, for production builds, `packages/ui` must have its TypeScript declarations compiled first via `npx tsc` to ensure type checking passes.

## Build Order and Prerequisites

The build dependency graph is:

```
packages/ui (npm install) [ui-deps]
    |
    +---> packages/ui (npx tsc) [ui-build]
              |
              +---> bowrain/apps/web (version.json + npm run build) [web-build]
              |
              +---> kapi/apps/kapi-web (version.json + npm run build) [kapi-web-build]
              |
              +---> bowrain/apps/bowrain/frontend (npm run build) [frontend-build]
```

The critical constraint: **`packages/ui` dependencies must be installed before any app project can build**, because the Vite alias resolves to source files that import from `packages/ui/node_modules` (e.g., `shadcn-glass-ui`, `@radix-ui/react-slot`).

## Makefile Targets

The Makefile encodes these dependencies explicitly:

```makefile
# Shared UI package
ui-deps:                         ## Install shared UI package dependencies
	cd packages/ui && npm install

ui-build: ui-deps                ## Compile shared UI TypeScript declarations
	cd packages/ui && npx tsc

# SaaS web UI (bowrain-server)
web-deps:                        ## Install SaaS web UI dependencies
	cd $(WEB_DIR) && npm install

web-build: ui-build web-deps     ## Build SaaS web UI for production
	@printf '{"version":"%s","commit":"%s","build_date":"%s","component":"web"}\n' \
	    "$(VERSION)" "$(COMMIT)" "$(BUILD_DATE)" > $(WEB_DIR)/public/version.json
	cd $(WEB_DIR) && npm run build

# Kapi web UI (kapi serve)
kapi-web-deps:                   ## Install kapi web UI dependencies
	cd $(KAPI_WEB_DIR) && npm install

kapi-web-build: ui-build kapi-web-deps  ## Build kapi web UI for production
	@printf '{"version":"%s","commit":"%s","build_date":"%s","component":"kapi-web"}\n' \
	    "$(VERSION)" "$(COMMIT)" "$(BUILD_DATE)" > $(KAPI_WEB_DIR)/public/version.json
	cd $(KAPI_WEB_DIR) && npm run build

# Bowrain desktop frontend
frontend-deps:                   ## Install frontend dependencies
	cd $(FRONTEND_DIR) && npm install

frontend-build: ui-build frontend-deps  ## Build frontend for production
	cd $(FRONTEND_DIR) && npm run build
```

Key observations:
- A dedicated `ui-build` target compiles `packages/ui` TypeScript declarations via `npx tsc`
- All three app build targets depend on `ui-build` (which itself depends on `ui-deps`)
- `web-build` and `kapi-web-build` generate a `version.json` in their `public/` directory before building (containing version, commit hash, and build date)
- `frontend-build` (Bowrain desktop) does not generate `version.json` because Wails provides version info through its own mechanism

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
