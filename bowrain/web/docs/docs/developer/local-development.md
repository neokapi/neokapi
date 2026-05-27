---
title: Local Development
sidebar_position: 10
---

# Local Development

A full bowrain instance is a few cooperating processes: the **server** (REST +
gRPC API), an **async worker** (runs the auto-translate-on-push automation and
upstream machine translation), and backing services — PostgreSQL, NATS (job
queue + event bus), Redis, Keycloak (OIDC), and Mailpit (SMTP). The server and
worker share the job queue and a blob volume; push processing is asynchronous
([AD-009](../architecture-decisions/009-sync-protocol.md)).

There are three ways to run this locally, depending on what you are working on:

| Mode                            | What runs where                                                  | Entry point             | Command                |
| ------------------------------- | ---------------------------------------------------------------- | ----------------------- | ---------------------- |
| **A · Host + HMR**              | deps + Traefik in Docker; server + web on the host (hot reload)  | `https://bowrain.mymac` | `docker compose up` + `make dev-server`/`dev-web` |
| **B · Full Docker (shared)**    | server + worker + web in Docker, reusing mode A's deps + Traefik | `https://bowrain.mymac` | `make stack-up-shared` |
| **B · Full Docker (standalone)**| everything in Docker, plain HTTP, self-contained, no host setup  | `http://localhost:8080` | `make stack-up`        |

Modes A and the *shared* full-Docker mode are the **same Compose project** and
therefore share **one** Traefik — there is no `:80/:443` clash. You move between a
host-run server (for hot reload) and a containerized one by adding or removing the
app overlay; the container's router out-prioritizes the host file-routes while it
is up, and the same Traefik falls back to the host server when it is gone. The
*standalone* mode is a separate, self-contained project (its own deps, no Traefik)
for CI and quick starts.

For running a production instance, see [Self-Hosting](../server/self-hosting.md)
instead. All commands below run from the `bowrain/` directory.

## Prerequisites

- Docker (with Compose v2) and Go.
- The `kapi-bowrain` plugin, to drive a project with the `kapi` CLI:
  `brew install neokapi/tap/bowrain-cli`.
- For the `*.bowrain.mymac` (TLS) modes only, the one-time host setup — local DNS
  for `*.mymac` and a locally-trusted certificate:

  ```bash
  brew install dnsmasq mkcert
  echo 'address=/.mymac/127.0.0.1' >> "$(brew --prefix)/etc/dnsmasq.conf"
  sudo brew services restart dnsmasq
  sudo mkdir -p /etc/resolver && echo 'nameserver 127.0.0.1' | sudo tee /etc/resolver/mymac
  mkcert -install && make certs
  ```

  The standalone mode needs none of this.

## Mode A — host server + web (hot reload)

Backing services and the shared Traefik run in Docker; the server and Vite dev
server run on the host so changes reload immediately. Traefik routes
`bowrain.mymac` to the host's `:8080` (API) and `:5173` (web).

```bash
docker compose up -d --wait    # deps + Traefik (auto-loads compose.override.yaml)
make dev-server                # bowrain-server on host :8080
make dev-web                   # Vite HMR for apps/web on :5173
make dev-worker                # optional: the worker, for push → translate → pull
```

| Service  | URL                                      |
| -------- | ---------------------------------------- |
| Web + API| `https://bowrain.mymac`                  |
| Keycloak | `https://auth.bowrain.mymac` (admin/admin) |
| Mailpit  | `https://mail.bowrain.mymac`             |

## Mode B (shared) — full stack behind the same Traefik

Adds the server, worker, and web **containers** to mode A's deps + Traefik
project ([`compose.app.yaml`](https://github.com/neokapi/neokapi/blob/main/bowrain/compose.app.yaml)),
reachable at the same `https://bowrain.mymac`. No second proxy.

```bash
cp .env.example .env     # optional: configure a real translation provider
make stack-up-shared     # = docker compose -f compose.yaml -f compose.override.yaml -f compose.app.yaml up
```

Because this is the same project as mode A, you can leave the deps + Traefik
running and simply add or drop the app overlay to switch between a host-run and a
containerized server. `make stack-shared-down` tears it down.

## Mode B (standalone) — zero-setup self-contained stack

Everything, including its own backing services, built from source on plain-HTTP
`localhost` ([`compose.full.yaml`](https://github.com/neokapi/neokapi/blob/main/bowrain/compose.full.yaml)).
No `*.mymac`/TLS host setup — best for CI and quick starts.

```bash
make stack-up            # server + worker + deps on http://localhost:8080
make stack-up-web        # …also build and serve the SaaS web UI at http://localhost:8080/
```

Keycloak is on `http://localhost:8180` (admin/admin) and Mailpit on
`http://localhost:8025`. Useful targets: `make stack-ps`, `stack-logs`,
`stack-down`.

## The translation worker

The `bowrain-worker` runs the built-in auto-translate-on-push automation. Its
upstream provider for these platform jobs is configured by environment:

- `BOWRAIN_PLATFORM_PROVIDER` — `demo` (default), `gemini`, `openai`,
  `anthropic`, or `ollama`.
- The API key comes from `BOWRAIN_PLATFORM_API_KEY` or a provider-specific
  variable (`GEMINI_API_KEY`, `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`).
- `BOWRAIN_PLATFORM_MODEL` sets the default model.

The default `demo` provider is an offline, deterministic stub, so the full
pipeline works out of the box with no key. Set a real provider in `.env` (see
[`.env.example`](https://github.com/neokapi/neokapi/blob/main/bowrain/.env.example))
for genuine machine translation, e.g.:

```bash
BOWRAIN_PLATFORM_PROVIDER=gemini
BOWRAIN_PLATFORM_MODEL=gemini-2.5-flash
GEMINI_API_KEY=AIza...
```

## Driving it with the `kapi` plugin

Point `--server` at whichever entry point your mode exposes (`http://localhost:8080`
for standalone, `https://bowrain.mymac` for the TLS modes):

```bash
mkdir myapp && cd myapp
kapi init --server http://localhost:8080 --anonymous --source-locale en --target-locale fr,de
echo '{"greeting":"Hello"}' > en.json
kapi add en.json
kapi sync --locale fr,de        # push → wait for translations → pull
cat fr.json                     # translated catalog
```

`kapi init` scaffolds the project; when `--server` is given, the bowrain plugin
*contributes* the server connection — it writes the `server:` block and stores a
claim token (`--anonymous`), or creates the project under your account when you
are signed in (`kapi auth login`), or attaches to an existing one with
`--project <id>`. It is idempotent: running `kapi init --server …` inside an
existing local kapi project connects it to bowrain, and leaves an
already-connected project untouched.

The custom Keycloak login theme is optional — login renders with the default
theme until you build it with `make keycloak-theme`. For the no-auth path above
(`--anonymous`), no Keycloak login is involved at all.
