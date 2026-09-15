# Bowrain 1.2.0

_Changes since [`v1.1.0`](https://github.com/neokapi/neokapi/releases/tag/v1.1.0), the last stable release that included Bowrain._

Bowrain 1.2.0 is the first stable release on the Bowrain track, which now releases separately from kapi. It contains the kapi-bowrain plugin, Bowrain Desktop and the server images. The plugin connects a kapi project to a Bowrain server; use it with kapi 1.2.0.

## Sync a project with a server

- The plugin adds `kapi push`, `kapi pull`, `kapi auth`, `kapi workspace`, `kapi stream`, `kapi diff` and `kapi ui`. Each is also reachable as `kapi bowrain <command>`.
- In a recipe with a `bowrain:` block, `kapi up` runs the loop on the server and pulls the produced targets back. `kapi push` and `kapi pull` move content, and the server converges it.
- A push carries the project's declared structure, its voice profile and its terms, and the server applies it in one transaction. The plugin computes the change against the server's tree, so renames and deletions arrive as renames and deletions.
- `kapi pull` writes an approved recipe change into the working tree, and writes no translation for content without a target.
- `kapi up` prints the server's governance refusals for a push. Content declared only for its comments stays out of the push scope.
- The plugin is licensed Apache-2.0.

## Review and sign off

- Review is governed by default. Approving needs the review permission, and every decision is recorded.
- The review session gathers units from many items into one keyboard-driven queue, and approves in bulk the units that pass their checks.
- A reviewer can propose and approve a source fix that applies to every language.
- Sign off is available on the review surfaces for content that needs a final approval.
- Reviewers are notified by task, email, banner and badge, and see what approving a change-set affects.
- A unit a reviewer rejects is drafted again by the next pass. Approvals and sign-offs that arrive by push are held to the same governance.
- In-context review shows an item inside the component that ships it.

## Govern context

- The Context hub is organized by governance point, with one card per profile.
- A context scan reads pasted text, web pages, files or a repository, drafts a voice profile and candidate terms with evidence, and proposes the axes the material varies along. Nothing is stored until you approve it.
- A context refresh, and a change to a term's do-not-translate flag, go through a change-set you approve.
- Memberships and API token scopes can be bound to a region of the context space, such as `@brand=acme`.
- Insights shows how much shipped content was written under the project's context.

## Connect repositories and automate delivery

- The GitHub App setup connects a repository to a project. A push to a connected GitHub or GitLab repository starts convergence on the server, and the translations return as one pull request or merge request.
- Before large AI work, the server shows source readiness and a credit estimate and asks for consent. `kapi up` asks as well, unless you pass `--yes`.
- Per-locale ship states and a delivery panel show what can ship. A project can publish a public `ship.json` feed.
- An automation's `run_flow` action runs the named flow.

## Bowrain web app and Bowrain Desktop

- Bowrain Desktop runs the same app as the web, keeps working through an offline start, and reconnects when the network returns. `kapi ui` launches it.
- A project opens on its collections and where they sit in the context space.

## Accounts, billing and AI

- A workspace uses the platform's AI, metered in credits, or its own provider keys, which use no credits. Paid plans include monthly credits, and Free includes a one-time trial grant.
- An administrator can let a workspace choose its AI model.
- Passkeys are managed in user settings. Every plan includes API access.

## Run a server

- The server needs PostgreSQL, S3-compatible object storage, SQS-compatible queues and Redis. It no longer uses the Apache AGE extension or NATS.
- An error carries a reference id across the server, the web app and the CLI. Requests to the HTTP, gRPC, worker and MCP surfaces are traced and tagged with their workspace.
- Calls to AI providers and other external services degrade gracefully when those services fail.
- Tenancy and security checks cover object access, rate limits, bounds on uploads and listings, and CSRF protection for cookie sessions.

## Upgrading from 1.1.0

- `kapi sync` is removed. Use `kapi up`, or `kapi push` and `kapi pull`.
- `kapi push` and `kapi pull` no longer take `--concepts` or `--no-brand`. A push carries the whole project.
- The recipe's `server:` block is now `bowrain:`.
- The API names voice where it named brand: `/:ws/brand-profiles` is `/:ws/voice-profiles`, `/:ws/brand-scans` is `/:ws/context-scans`, the `brand.voice.*` events are `voice.*`, and the `manage_brand` permission is `manage_voice`.
- The push protocol changed, and `/push/diff` is removed. Upgrade the plugin and the server together.
- Approving a review needs the review permission. Creating a project needs workspace membership and `manage_files`.
- A self-hosted server needs the services listed under [Run a server](#run-a-server).

## Installation

**Homebrew (kapi and the bowrain plugin):**
```
brew install neokapi/tap/bowrain-cli
```

**Homebrew (Bowrain Desktop, macOS):**
```
brew install --cask neokapi/tap/bowrain
```

**Into an existing kapi install:**
```
kapi plugin install bowrain
```

The kapi CLI and Kapi Desktop are released separately, under the tag `v1.2.0`. To follow the beta channel, install `neokapi/tap/bowrain-cli-beta` or the `neokapi/tap/bowrain@beta` cask instead.

Server images are published as `ghcr.io/neokapi/bowrain-server:1.2.0`, `ghcr.io/neokapi/bowrain-worker:1.2.0`, `ghcr.io/neokapi/bowrain-web:1.2.0` and `ghcr.io/neokapi/bowrain-keycloak:1.2.0`.

Plugin archives for macOS, Linux and Windows, each with a signature bundle (`.sigstore.json`), and Bowrain Desktop for macOS and Linux are attached below. Verify a download against `checksums.txt`. The signed Windows build of Bowrain Desktop is added to this release after it is published.
