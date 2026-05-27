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

### Full local stack (everything in Docker)

For a self-contained stack — server **and** async worker **and** all backing
services in Docker on plain-HTTP localhost (no host DNS/TLS setup) — use
[`compose.full.yaml`](compose.full.yaml). This is the quickest way to exercise
real `push → translate → pull` cycles with the `kapi` plugin.

```bash
cp .env.example .env             # optional: set GEMINI_API_KEY for real MT
make stack-up                    # from bowrain/ (or: make -C bowrain stack-up)
# equivalently: docker compose -f compose.full.yaml up -d --build --wait
```

| Service        | URL                                   |
| -------------- | ------------------------------------- |
| Server (+ API) | http://localhost:8080                 |
| Health         | http://localhost:8080/api/v1/health   |
| Keycloak       | http://localhost:8180 (admin/admin)   |
| Mailpit        | http://localhost:8025                 |

The `bowrain-worker` runs the built-in auto-translate-on-push automation. Its
upstream provider defaults to the offline **`demo`** provider (deterministic, no
key) so the pipeline works out of the box; set `BOWRAIN_PLATFORM_PROVIDER=gemini`
+ `GEMINI_API_KEY` in `.env` for real translations (see `.env.example`).

Drive it with the `kapi` plugin (install the `kapi-bowrain` plugin first):

```bash
mkdir myapp && cd myapp
kapi init --server http://localhost:8080 --anonymous --source-locale en --target-locale fr,de
echo '{"greeting":"Hello"}' > en.json
kapi add en.json
kapi sync --locale fr,de        # push → wait for translations → pull
cat fr.json de.json             # translated catalogs
```

`kapi init` scaffolds the project; when `--server` is given, the bowrain plugin
*contributes* the server connection (writes the `server:` block + claim token).
It is idempotent — running `kapi init --server …` inside an **existing** local
kapi project simply connects it to bowrain, leaving an already-connected project
untouched. (`--anonymous` skips sign-in; omit it to create the project under
your account, or pass `--project <id>` to attach to an existing server project.)

Other targets: `make -C bowrain stack-ps`, `stack-logs`, `stack-down`,
`stack-up-web` (adds the SaaS web UI at http://localhost:8080).

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
