# Bowrain

[![Docs — bowrain](https://github.com/neokapi/neokapi/actions/workflows/docs-bowrain.yml/badge.svg)](https://github.com/neokapi/neokapi/actions/workflows/docs-bowrain.yml)
[![Web Landing](https://github.com/neokapi/neokapi/actions/workflows/web-landing.yml/badge.svg)](https://github.com/neokapi/neokapi/actions/workflows/web-landing.yml)
[![Pages Deploy](https://github.com/neokapi/neokapi/actions/workflows/pages-deploy.yml/badge.svg)](https://github.com/neokapi/neokapi/actions/workflows/pages-deploy.yml)

Bowrain is a full-stack localization platform built on top of the [neokapi framework](../README.md): a sync CLI for developers, a web app for translators, a desktop app for visual workflows, and a server that holds it all together.

This subtree (`bowrain/`) is licensed AGPL-3.0. The neokapi framework at the repository root is Apache-2.0; see the [root README](../README.md) for that side.

## Install

```bash
brew install neokapi/tap/bowrain-cli      # CLI plugin (commands run as `kapi <cmd>`)
brew install --cask neokapi/tap/bowrain   # Desktop app (macOS)
```

## Layout

```
bowrain/
├── core/            Shared platform types (auth, store, connector, project, event)
├── cli/             Bowrain CLI plugin (`kapi-bowrain`) — commands run as `kapi init/push/pull/sync/...`
├── auth/            OIDC + AuthStore + SQLite/PostgreSQL auth
├── server/          REST/gRPC server (Echo v4)
├── service/         Auth, project, connector, flow services
├── store/           Project + content storage (SQLite + PostgreSQL)
├── connector/       Connector implementations (file, git, ...)
├── billing/         Stripe + workspace credits
├── jobs/            Background worker queue
├── event/           Event bus + automation
├── brand/           Brand voice management
├── proto/           gRPC protobuf definitions
├── cmd/
│   ├── bowrain-server/   Echo REST API server entrypoint
│   └── bowrain-worker/   Background worker entrypoint
├── apps/
│   ├── bowrain/     Desktop app (Wails v3 + React/TS)
│   ├── web/         SaaS web UI
│   ├── ctrl/        Admin control panel
│   ├── pulse/       Real-time dashboard
│   └── keycloak-theme/   Custom Keycloak theme
├── packages/ui/     @neokapi/ui — bowrain-flavored shadcn primitives (AGPL)
├── storybook/       Bowrain Storybook (port 6006)
├── e2e/             End-to-end tests
├── emails/          Email templates
├── web/
│   ├── landing/     bowrain.cloud landing site (Vite + React)
│   └── docs/        Docusaurus docs site → /web/bowrain/docs/
└── deploy/          Deployment configs
```

## Go modules

| Module          | Path          | Purpose                                                   |
| --------------- | ------------- | --------------------------------------------------------- |
| **Bowrain Core**| `core/`       | Shared types — used by both bowrain CLI and server        |
| **Bowrain CLI** | `cli/`        | `bowrain` binary — project sync companion                 |
| **Bowrain**     | `.`           | Server, worker, desktop app, REST/gRPC handlers           |

Coordinated via the root `go.work`.

## Quick start

```bash
# Build everything
make -C .. build-all

# Or piecewise
cd .. && make build-bowrain-cli      # bin/bowrain CLI
cd .. && make build-server           # bin/bowrain-server
cd apps/bowrain && wails3 build      # native desktop app
```

### Local dev environment

Bowrain expects Keycloak (OIDC), Mailpit (email), and a Postgres or SQLite store.

```bash
docker compose up -d --wait    # Traefik + Keycloak + Mailpit
make -C .. dev-server          # Build + run bowrain-server against the deps
make -C .. dev-web             # Vite dev server with HMR for bowrain/apps/web
```

| Service  | URL                                      |
| -------- | ---------------------------------------- |
| Web app  | https://bowrain.mymac                    |
| API      | https://bowrain.mymac/api/*              |
| Keycloak | https://auth.bowrain.mymac (admin/admin) |
| Mailpit  | https://mail.bowrain.mymac               |
| Traefik  | https://traefik.bowrain.mymac            |

The first-time host setup (dnsmasq + mkcert for `*.mymac`) is the same as the framework root — see the steps in the [root README's Development Setup](../README.md).

### Tests

```bash
make -C .. test-bowrain         # All bowrain Go tests
go test ./server/... -v         # Single package
cd apps/bowrain/frontend && vp test       # Frontend unit tests (vitest)
cd apps/bowrain/frontend && vp test:e2e   # End-to-end (Playwright)
```

## Make targets (this subtree)

```
make -f bowrain/Makefile build-server   Build bowrain-server → bin/bowrain-server
make -f bowrain/Makefile build-worker   Build bowrain-worker → bin/bowrain-worker
make -f bowrain/Makefile build-bowrain  Build native desktop app
make -f bowrain/Makefile test           Run all bowrain Go tests
make -f bowrain/Makefile lint           golangci-lint
```

Most are also reachable from the repo root as `make -C bowrain <target>`.

## Documentation

- **[bowrain docs](https://neokapi.github.io/web/bowrain/docs/)** — published Docusaurus site
- **[bowrain landing](https://neokapi.github.io/web/bowrain/)** — marketing/intro site
- **[Architecture decisions](web/docs/docs/architecture-decisions/)** — bowrain-specific ADs
- **[Server reference](web/docs/docs/server/)**, **[CLI reference](web/docs/docs/cli/)**, **[Desktop reference](web/docs/docs/desktop/)**

## License

AGPL-3.0. See [LICENSE](LICENSE) (top of this subtree). The neokapi framework at the repository root is Apache-2.0; see [`../LICENSE`](../LICENSE).
