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

### Running Bowrain locally

Bowrain expects Keycloak (OIDC), Mailpit (email), a PostgreSQL store, NATS (job
queue + event bus), and an async worker. There are three ways to wire it up,
depending on what you're working on:

| Mode                       | What runs where                                  | Entry point           | Command                                                           |
| -------------------------- | ------------------------------------------------ | --------------------- | ----------------------------------------------------------------- |
| **A · Host + HMR**         | deps + Traefik in Docker; server + web on host   | https://bowrain.mymac | `docker compose up -d --wait` + `make dev-server` + `make dev-web` |
| **B · Full Docker, shared**| server + worker + web in Docker, **reusing A's deps + Traefik** | https://bowrain.mymac | `make stack-up-shared`                            |
| **B · Full Docker, standalone** | everything in Docker, plain HTTP, own project | http://localhost:8080 | `make stack-up` (`stack-up-web` for the SPA)                 |

Modes A and the *shared* full-Docker mode are the **same compose project** and
therefore share **one** Traefik — no `:80/:443` clash. You flip between a
host-run server (HMR) and a containerized one by adding/removing the app
overlay; the container's router out-prioritizes the host file-routes while it's
up, and the same Traefik falls back to the host server when it's gone. The
*standalone* mode is a separate self-contained project (its own deps, no Traefik).

Modes that use `*.bowrain.mymac` need the one-time host setup (dnsmasq + mkcert;
`make -C bowrain certs` writes the wildcard cert) — see
[the root README's Development Setup](../README.md). The standalone mode needs no
host setup at all.

**Mode A — active server/frontend development.** Backing services (and the
shared Traefik) run in Docker; the server and Vite dev server run on the host
with hot reload. Traefik routes `bowrain.mymac` → host `:8080`/`:5173`.

```bash
docker compose up -d --wait    # deps + Traefik (auto-loads compose.override.yaml)
make -C .. dev-server          # bowrain-server on host :8080
make -C .. dev-web             # Vite HMR for bowrain/apps/web on :5173
make -C .. dev-worker          # (optional) worker, for push→translate→pull
```

**Mode B (shared) — full stack in Docker behind the same Traefik.** Adds the
server + worker + web containers to Mode A's deps + Traefik project
([`compose.app.yaml`](compose.app.yaml)); reachable at the same
`https://bowrain.mymac`. No second proxy.

```bash
cp .env.example .env     # optional: set GEMINI_API_KEY for real MT
make stack-up-shared     # = docker compose -f compose.yaml -f compose.override.yaml -f compose.app.yaml up
```

**Mode B (standalone) — zero-setup self-contained stack.** Everything (incl. its
own deps) built from source on plain-HTTP localhost
([`compose.full.yaml`](compose.full.yaml)) — no `*.mymac`/TLS host setup. Best for
CI and quick starts.

```bash
make stack-up            # core stack on http://localhost:8080  (Keycloak :8180, Mailpit :8025)
make stack-up-web        # …also build + serve the SaaS web UI at http://localhost:8080/
```

The `bowrain-worker` runs the built-in auto-translate-on-push automation. Its
upstream provider defaults to the offline **`demo`** provider (deterministic, no
key) so the pipeline works out of the box; set `BOWRAIN_PLATFORM_PROVIDER=gemini`
+ `GEMINI_API_KEY` in `.env` for real translations (see `.env.example`).
Other targets: `make -C bowrain stack-ps`, `stack-logs`, `stack-down`,
`stack-shared-down`.

**Driving any mode with the `kapi` plugin** (install the `kapi-bowrain` plugin
first). Point `--server` at whichever entry point your mode exposes:

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
