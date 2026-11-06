# Bowrain 1.3.0

_Changes since [`bowrain-v1.2.0`](https://github.com/neokapi/neokapi/releases/tag/bowrain-v1.2.0)._

Bowrain 1.3.0 is the server-side companion to kapi 1.3.0. It contains the kapi-bowrain plugin, Bowrain Desktop and the server. A project's context, the rules kapi keeps beside the files, can now live on the server: every checkout reads the same decisions, the team decides from one queue in the browser, and the same screens a person sees in Kapi Desktop open on the server's data.

Like kapi 1.3.0, this release carries no migration from 1.2.0 data: a workspace is reset and its projects pushed again.

## A project's context on the server

- `kapi share --via bowrain <url>` connects a checkout to a Bowrain project. With `context: {backend: bowrain}` in the recipe, `kapi context sync` reads what the team decided and sends what this machine recorded, and the server keeps the project's context as its own objects.
- The server records every context change as an operation in the project's ledger, with the signed-in member as the actor. A decision needs the permission its kind takes, `manage_terms` for a name or a word, `manage_voice` for a note, `manage_memory` for a memory pair, and it has to sit inside the member's coordinates.
- Term candidates leave the review queue: the ledger is the one queue for what assistants notice.

## Map, Reader and Rules in the browser

- A project opens on its Map: the counts, the rules in force, what an assistant noticed, and one row per file. The Overview keeps its own address.
- Run on everything checks every file on the server as one job and needs the review permission. The Reader shows a file with its findings under each block and the rules that apply in the margin. Fix files a source proposal that keeps every inline code, and an approval applies it.
- Rules is the team queue. Each question shows who it is assigned to; the picker lists the members who can decide in the project, and Assigned to me narrows the queue to yours. Keep records the rule in your name, and Kept by lists every rule in force with who kept it, newest first. The review inbox counts the rules waiting per project and opens a project on your assignments.
- The platform's concepts, content memory, activity and review pages share the desktop's shapes: a list with a detail pane, counts and marks rather than sentences.

## The plugin and the desktop app

- Kapi Desktop's Share rung for Bowrain previews the project the way its Bowrain page shows it, from local data before the project is connected and from the server after, with the server's own refusal when a read is denied.
- The CLI, the plugin and the desktop app share one sign-in. The server allows a refresh token to be presented again within thirty seconds of its rotation, as a concurrent refresh from one of those apps, and the desktop re-reads the keychain before each refresh, so one does not sign the other out.
- A matched term carries its target-language wording, so the editor's term sidebar shows the approved term and term-enforce reports on the server's store.
- The block history records each translation's basis. Uploading a file replaces its block set, and the document preview marks blocks by the key the store knows.
- Passkey sign-up works again. The GitHub App can target an enterprise server.

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

The kapi CLI and Kapi Desktop are released separately, under the tag `v1.3.0`. To follow the beta channel, install `neokapi/tap/bowrain-cli-beta` or the `neokapi/tap/bowrain@beta` cask instead.

Server images are published as `ghcr.io/neokapi/bowrain-server:1.3.0`, `ghcr.io/neokapi/bowrain-worker:1.3.0`, `ghcr.io/neokapi/bowrain-web:1.3.0` and `ghcr.io/neokapi/bowrain-keycloak:1.3.0`.

Plugin archives for macOS, Linux and Windows, each with a signature bundle (`.sigstore.json`), and Bowrain Desktop for macOS and Linux are attached below. Verify a download against `checksums.txt`. The signed Windows build of Bowrain Desktop is added to this release after it is published.
