# neokapi

[![CI](https://github.com/neokapi/neokapi/actions/workflows/ci.yml/badge.svg)](https://github.com/neokapi/neokapi/actions/workflows/ci.yml)
[![Screenshots & Recordings](https://github.com/neokapi/neokapi/actions/workflows/screenshots-recordings.yml/badge.svg)](https://github.com/neokapi/neokapi/actions/workflows/screenshots-recordings.yml)
[![Docs](https://github.com/neokapi/neokapi/actions/workflows/docs.yml/badge.svg)](https://github.com/neokapi/neokapi/actions/workflows/docs.yml)

> **Experimental:** Neokapi is an ongoing experiment and should not be used in production.

An AI-native reimagining of the [Okapi Framework](https://okapiframework.org/) in Go. Format-aware document parsing, channel-based concurrent processing, and pluggable tools for localization and translation.

## Install

**Homebrew:**

```bash
brew install --cask neokapi/tap/kapi      # CLI
brew install --cask neokapi/tap/bowrain    # Desktop app (macOS)
```

Pre-built binaries for Linux, macOS, and Windows are on the [Releases](https://github.com/neokapi/neokapi/releases) page.

## Repository Layout

Six Go modules coordinated by `go.work`:

| Module          | Path           | Description                                                                               |
| --------------- | -------------- | ----------------------------------------------------------------------------------------- |
| **Framework**   | `core/`        | Content model, format readers/writers, processing tools, pipeline executor, plugin system |
| **CLI**         | `cli/`         | Shared CLI base, command factories, output formatting, app config                         |
| **Platform**    | `platform/`    | Shared platform types, auth, connector interfaces, REST client                            |
| **Kapi**        | `kapi/`        | Standalone CLI tool for local file processing                                             |
| **Bowrain CLI** | `bowrain-cli/` | Project sync companion CLI (init, push, pull, auth, status)                               |
| **Bowrain**     | `bowrain/`     | Server, desktop app, SQLite storage, auth, connectors                                     |

See [AD-001](docs/architecture-decisions/001-vision-and-modules.md) for the full rationale.

## Development Setup

### Prerequisites (one-time, macOS)

```bash
brew install dnsmasq mkcert

# Resolve *.mymac → 127.0.0.1
echo 'address=/.mymac/127.0.0.1' >> $(brew --prefix)/etc/dnsmasq.conf
sudo brew services restart dnsmasq
sudo mkdir -p /etc/resolver
echo 'nameserver 127.0.0.1' | sudo tee /etc/resolver/mymac

# Install local CA + generate TLS certs
mkcert -install
make certs
```

### Run locally

```bash
docker compose up -d --wait    # Traefik + Keycloak + Mailpit
make dev-server                # Build + run bowrain-server
make dev-web                   # Vite dev server with HMR
```

| Service  | URL                                      |
| -------- | ---------------------------------------- |
| Web app  | https://bowrain.mymac                    |
| API      | https://bowrain.mymac/api/*              |
| Keycloak | https://auth.bowrain.mymac (admin/admin) |
| Mailpit  | https://mail.bowrain.mymac               |
| Traefik  | https://traefik.bowrain.mymac            |

See [Docker Compose Development Setup](docs/notes/docker-compose.md) for details.

### CI mode (no Traefik, plain HTTP)

```bash
docker compose -f compose.yaml up -d --wait    # skips compose.override.yaml
```

## Makefile Targets

```
make build              Build kapi CLI → bin/kapi
make build-server       Build bowrain-server → bin/bowrain-server
make build-all          Build all Go binaries
make test               Run all tests (all four modules)
make test-framework     Framework tests only
make test-platform      Platform tests only
make test-kapi          Kapi tests only
make test-bowrain       Bowrain tests only
make check              fmt + vet + lint
make cover              Coverage report → coverage/coverage.html
make certs              Generate mkcert TLS certs for *.bowrain.mymac
make dev-server         Build + run server against Docker deps
make dev-web            Vite dev server with HMR
make dev-deps           Start Docker deps
make dev-deps-down      Stop Docker deps
```

Run a single test: `go test ./core/flow/ -run TestExecutorCancellation -v`

## Documentation

- **[Architecture Decisions](docs/ad/)** — one AD per architectural concern
- **[Implementation Notes](docs/notes/)** — schemas, protocols, algorithms
- **[Testing Strategy](docs/TESTING.md)** — test pyramid, patterns, CI config
- **[Website](website/)** — Docusaurus docs site (`cd website && npm start`)

## License

Apache 2.0 — see [LICENSE](LICENSE).
