# kapi × Claude Code — demo harness

This harness **demonstrates and records kapi being driven by Claude Code**, as
narrated videos. Each demo runs a *real, headless `claude` session* against the kapi
Claude Code plugin, captures the transcript, screenshots the artifacts kapi produced,
generates a voice-over, and composes everything into an MP4 with [Remotion](https://remotion.dev).

Nothing here is mocked: the Claude sessions are live, the kapi commands run for real
(translating with Gemini, checking voice profile, importing terms…), and the
before/after artifacts are screenshots of kapi's actual output.

## What it produces

One narrated 1080p video per demo in `out/<id>.mp4`. Each video is structured as:

```
title card → real Claude Code terminal replay → artifact spotlights → outro
```

with a continuous British-English narration track explaining the story.

## The demos

Every directory under `demos/` (bar `_retired/`) is one demo. A row with no mode
tag runs a live Claude session; **shell** marks a scripted shell demo and
**desktop** one recorded against a running app.

| # | id | What it shows |
|---|----|----------------|
| 1 | `01-localize-landing-page`        | Translate a landing page to French: a plain request, and the kapi skill keeps the HTML intact |
| 2 | `02-nextjs-zero-to-i18n`          | A working English Next.js app wired up with `neokapi-i18n` and shipped in Japanese |
| 3 | `03-translate-docx`               | A Word announcement translated into Japanese, headings, lists and formatting preserved |
| 4 | `04-i18n-react-catalogs`          | An existing react-i18next project, and a request for two more languages |
| 5 | `05-ai-checks-guardrail`          | **shell.** `kapi check` reads an AI translation against its source and reports what would break in production |
| 6 | `06-multi-format-publishing`      | Two files in two formats, one request, each round-tripped back into its own format |
| 7 | `07-global-launch-many-languages` | One source file, four target languages, including a non-Latin script |
| 8 | `08-mcp-tools`                    | The same engine, exposed to the assistant as Model Context Protocol tools |
| 9 | `09-toolbox-find-replace`         | **shell.** kcat, kgrep and ksed work on the text kapi reads out of a document, not on raw bytes |
| 10 | `s0-northsea-governance`         | **shell.** One repository, one language, three surfaces: discovered, gated, corrected and converged, with no server and no model |
| 11 | `s1-compass-multilingual`        | **shell.** The same project at the same point in three more languages, converged, reviewed, and gated at the edge where a reader sees it |
| 12 | `s2-tidewatch-docs`              | **shell.** Four handbook pages, two more languages, and a CI job that reports what is behind instead of failing on it |
| 13 | `kapi-bilingual-workflow`        | **shell.** kapi emits a clean bilingual XLIFF, accepts the translated one back, and keeps the project content memory in the loop on both sides |
| 14 | `kapi-desktop-projects`          | **desktop.** Set up a multilingual content project in Kapi Desktop |
| 15 | `kapi-desktop-content`           | **desktop.** The files a project translates, and how they map |
| 16 | `kapi-desktop-flows`             | **desktop.** The pipelines a project runs over its content |
| 17 | `kapi-desktop-explorer`          | **desktop.** A visual tour of terminology and content memory in Kapi Desktop |
| 18 | `kapi-desktop-config`            | **desktop.** Appearance, AI providers, and plugins in one place |
| 19 | `bowrain-cli-getting-started`    | **shell.** `kapi init` connects local files to a Bowrain server; push and pull move content like git, and `kapi up` runs the loop on the server |
| 20 | `bowrain-cli-auth-and-workspaces` | **shell.** `kapi auth` and `kapi workspace` show who and where you are connected; pseudo-translate checks layout before you send for real translation |
| 21 | `bowrain-desktop-dashboard`      | **desktop.** The Bowrain desktop app as an equal, real-time client of the shared workspace |
| 22 | `bowrain-desktop-automations`    | **desktop.** Rules run the loop on the server; runs show each pass catching the project up |
| 23 | `bowrain-web-editor`             | **desktop.** The shared translation editor: every locale, with the team's memory and terms inline |
| 24 | `bowrain-web-review`             | **desktop.** Review and approve, the team workflow on content synced from kapi |
| 25 | `bowrain-web-governance`         | **desktop.** One governed source of voice, terms, and translations for a team |
| 26 | `bowrain-web-collaboration`      | **desktop.** Two people, one document, live |
| 27 | `bowrain-web-correction-loop`    | **desktop.** Every correction becomes a versioned check |
| 28 | `bowrain-sizzle`                 | **desktop.** A reel of the Bowrain platform: governance, collaboration, and quality |

Rows 1 to 13 exercise the task sections of the kapi skill
(`references/translate.md`, `references/i18n.md`, `references/voice.md`,
`references/toolbox.md`) plus the MCP tool surface. Rows 14 to 18 record Kapi
Desktop, and rows 19 to 28 record the Bowrain CLI, desktop app and web app.

### Scripted shell demos (no Claude)

A demo can set `terminal: shell` with a `script:` of commands instead of a Claude
`prompt`. The commands run for real in the sandbox (via `sh -c`, so globs expand)
and their output is recorded deterministically — no live `claude`, no billing, no
Gemini. The renderer frames them as a plain terminal (a `$` prompt, no Claude
banner or tool-call chrome) and the title/outro cards use the kapi-only lockup
(`brand: kapi`). Everything else — the macOS window, captions, voice-over, artifact
spotlights — is identical to the Claude demos. `09-toolbox-find-replace` is the
reference example.

Three manifest keys belong to this class:

- `fixturesFrom: samples/<name>` seeds the sandbox from a committed sample
  project instead of the demo's own `fixtures/`, so the tree in the recording is
  the tree a reader clones. A path that does not exist fails the capture rather
  than producing an empty sandbox.
- `project: true` turns kapi recipe discovery back on for a sandbox that *is* a
  kapi project, so the commands read the way a user types them instead of each
  carrying `-p`. The sandbox lives in `os.tmpdir()`, outside the repo, so the
  dogfood project is unreachable from it and the rest of the isolation contract
  still applies.
- `expectExit:` on a single `script` step declares the exit code — or codes — the
  command is recorded for. Every step is expected to exit 0 unless it says
  otherwise, and a step that exits any other way fails the capture stage by name,
  so a broken take cannot be narrated, rendered or published. Declare the
  exception where the non-zero result *is* the beat:

  ```yaml
  - command: kapi check --strict   # a gate that must fail on camera
    expectExit: 3
  - command: kgrep mooring *       # "found nothing" is a legitimate answer
    expectExit: [0, 1]
  ```

  The declaration binds both ways: a step that declares 3 and exits 0 fails the
  capture too, because the recording no longer shows what the demo says it shows.
  A non-zero exit still reads as a failure on screen whether or not it was
  declared — the declaration decides whether the take is sound, not how it looks.

`s0-northsea-governance` uses all three.

## How it works (pipeline)

Each demo is a folder under `demos/<id>/` with a `demo.yaml` manifest, a `fixtures/`
directory (the starting project Claude works on), and the narration script inline in
the manifest. The orchestrator runs four idempotent stages:

1. **capture** (`src/driver/capture.ts`) — copies `fixtures/` into a sandbox *outside
   the repo* (so Claude doesn't pick up this repo's `CLAUDE.md`), drops in a short
   `CLAUDE.md` that tells Claude how to call kapi, then runs:
   ```
   claude -p "<prompt>" --output-format stream-json --verbose \
          --permission-mode bypassPermissions --model sonnet \
          --plugin-dir <kapi-claude-plugin>
   ```
   The stream-json transcript is normalized into `public/<id>/capture.json`. (MCP demos
   instead pass `--mcp-config` running `kapi mcp` and disable Bash.)
2. **artifacts** (`src/driver/artifacts.ts`) — Playwright screenshots the visual
   results from the sandbox snapshot (rendered HTML before/after, or kapi JSON output
   rendered into a styled report card). → `public/<id>/artifacts/*.png`
3. **narrate** (`src/narrate/synth.ts`) — synthesizes each narration scene to audio.
   → `public/<id>/audio/*.wav` + `narration.json`
4. **render** (`src/remotion/`) — a Remotion composition replays the terminal, cuts to
   the artifacts, overlays captions, and plays the narration. → `out/<id>.mp4`

The capture step is the only non-deterministic / billed part; once captured, artifacts,
narration and render reproduce deterministically from `public/<id>/`.

## Narration backends (pluggable)

Secrets live in the **shared** `~/.config/neokapi/harness.env` (honours
`$XDG_CONFIG_HOME`) so every worktree reuses one `GEMINI_API_KEY`; a per-worktree
`harness/.env` (see `.env.example`) can override for one-off local tweaks. Default
is **Gemini TTS** (uses the same `GEMINI_API_KEY` as kapi), styled for a clear
British-English narrator voice. Narration pace/tone are committed code constants,
not env vars.

| `NARRATION_BACKEND` | Needs | Notes |
|---|---|---|
| `gemini` (default) | `GEMINI_API_KEY` | Neural TTS, prompted for a British narrator voice |
| `elevenlabs`       | `ELEVENLABS_API_KEY` (+ `ELEVENLABS_VOICE_ID`) | Studio quality |
| `openai`           | `OPENAI_API_KEY` | `gpt-4o-mini-tts` |
| `say`              | macOS only | Offline fallback, voice `Daniel` (en_GB) |

Switch with `NARRATION_BACKEND=elevenlabs pnpm run demo <id> -- --only=narrate --force`.

## Videos in other locales (`--locale`)

Demos carry their narration in English in `narration:` (the master — scene
structure, beats, holds and timing all come from it), and `demo.yaml` is
English-only. Translated narration lives in **generated** sidecar files next to
it — `demo.<locale>.yaml`, a content-memory-driven copy produced by the repo's
dogfood pipeline (the `l10n-*` make targets):

```bash
make -C .. l10n-demos    # regenerate the committed demo.<lang>.yaml sidecars
```

At load time the harness overlays a sidecar's translated narration text /
captions (and title/subtitle) onto the English master by scene `id`; sidecar
entries still identical to the English text are content memory misses (pending
translation) and simply fall back to English. Never hand-edit a sidecar or
author an inline `locales:` block (the loader rejects it) — corrections go
into the content memory seed `.kapi/memory/demo-narration-<lang>.memory.json`, then
regenerate.
Freshness is CI-gated by `make l10n-verify`.

Scenes without a translated override fall back to English — except for
published demos (`publishAs`), where the narrate stage requires full coverage
so a shipped video never mixes languages. Translations follow
`.kapi/voice.yaml` (nb override) and the project terms.

Select the locale with `--locale=<bcp47>` (or `HARNESS_LOCALE`) on the
narrate/render/publish stages. The default (`en`) keeps today's unsuffixed
filenames; any other locale suffixes every derived asset:

```bash
# Norwegian Bokmål pass for one published demo (after the English capture exists):
pnpm run demo kapi-desktop-explorer -- --locale=nb --only=narrate,render,publish --theme=both
#   narrate → public/<id>/narration-nb.json + audio-nb/
#   render  → out/<id>-nb.mp4 + out/<id>-nb-light.mp4
#   publish → web/static/video/kapi/<publishAs>-nb-{light,dark}.webm (+ .jpg posters)
```

The docs `ThemedVideo` component automatically prefers the `-<locale>` asset
variant when the Docusaurus page locale isn't `en`, and falls back to the
English asset when the translated one hasn't been published.

Narration TTS is locale-aware: the Gemini style prompt and Live narrator
instruction ask for a native narrator in the locale's language (the kapi /
Bowrain pronunciation hints are kept in every language), and the voice can be
pinned per locale with `GEMINI_TTS_VOICE_NB` (likewise
`ELEVENLABS_VOICE_ID_NB`, `OPENAI_TTS_VOICE_NB`, `SAY_VOICE_NB` — the macOS
`say` backend *requires* a per-locale voice since its voices are
single-language).

Desktop walkthroughs: `--locale` is passed through to the recorder
(`record-desktop.ts` `uiLocale`), which persists the language on the recording
backend (wbridge `SetUILanguage`) and appends `&lang=` to the recording URL.
Note the screencast files are NOT locale-suffixed — a non-English recording pass
overwrites `public/<id>/screencast.*`, so record → render → publish one locale
at a time. Recording a genuinely translated UI additionally needs (in
`apps/kapi-desktop`) the recorder entry `real-main.tsx` to honor `?lang=` via
`loadTranslations()` and a compiled catalog at
`frontend/public/translations/<locale>.json`; until then a non-English pass
records the English UI with fully translated narration/captions.

## Usage

```bash
# one-time: put GEMINI_API_KEY in ~/.config/neokapi/harness.env, then:
# build kapi (with fts5+icu4c), regenerate the plugin bundle,
# register the harness-gemini credential, install Playwright Chromium
pnpm install
pnpm run setup

# run the whole pipeline for one demo (or `all`)
pnpm run demo 01-localize-landing-page
pnpm run demo all

# run a single stage (each stage is idempotent; --force re-runs it)
pnpm run demo all -- --only=capture          # just the live Claude sessions
pnpm run demo all -- --only=artifacts,narrate,render
pnpm run demo 02-nextjs-zero-to-i18n -- --only=render --force

pnpm run list                                 # list demos
pnpm run studio                               # open the Remotion studio to preview

pnpm test                                     # unit tests (no kapi, no network, no stack)
pnpm run typecheck                            # tsc --noEmit
```

`make check` runs both.

Prerequisites: a logged-in `claude` CLI, Node ≥ 22, `ffmpeg`, Go + Homebrew `icu4c`
(for building kapi), and a `GEMINI_API_KEY` in `.env` for the AI demos.

### The TypeScript pin

The harness is a standalone pnpm project (`pnpm-workspace.yaml` with
`packages: []`), and its `typescript` devDependency is pinned to `~5.9.3`
instead of tracking the repo-wide TypeScript version.

Everything that bundles — `studio`, `render`, `videos`, and the render/publish
phases of `pnpm run demo` — goes through `@remotion/bundler`, whose esbuild
loader consumes the TypeScript **compiler API**: it reaches for `typescript.sys`
and `typescript.readConfigFile`. TypeScript 7.0 ships no compiler API at all, so
on 7.0 both are `undefined` and every bundle fails. The stable compiler API is
targeted for TypeScript 7.1, which no `@remotion/*` release runs on yet.

Upgrade trigger: **a `@remotion/*` release running on the 7.1 compiler API.**
At that point the pin is lifted and the harness rejoins the repo-wide TypeScript
version. Nothing else here needs 5.9 — `tsc --noEmit` type-checks the sources
under either major; the constraint is TypeScript as a *library* consumed by the
bundler.

## Adding a demo

1. `mkdir -p demos/<id>/fixtures` and add the starting project files.
2. Write `demos/<id>/demo.yaml` (see any existing demo). Pin output filenames in the
   `prompt` and point `artifacts[].path` at them. Keep prompts to the **reliable
   standalone** kapi surface: `translate`, `pseudo-translate`, `voice`, `terms`,
   `stats`, `formats`, `extract`, or the MCP tools.
3. `pnpm run demo <id>`.

`captures/`, `public/`, `out/`, `sandbox/` and `.env` are git-ignored. The authored
`demos/` are the source of truth — re-run the harness to regenerate everything else.

## Recording the real bowrain web app (`target: web`)

Some demos (`bowrain-web-*`) record the real bowrain SPA against a running stack
instead of the kapi-desktop wbridge. Bring the stack up from the current branch:

```bash
make -C bowrain stack-up-web        # serves SPA + API at http://localhost:8080
```

Auth is a device-flow JWT planted as the `bowrain_session` cookie. A seed script
prints the token + the route params; pass them to the capture stage, e.g.:

```bash
BOWRAIN_BACKEND_URL=http://localhost:8080 \
BOWRAIN_SESSION_TOKEN=<jwt> BOWRAIN_WORKSPACE_SLUG=<slug> \
  pnpm run demo bowrain-web-governance -- --only=capture --force --theme=both
```

### Two-user collaboration (`bowrain-web-collaboration`)

Collaboration is recorded with **two genuine authenticated users**. The recorded
camera is the first user (Alice); a second, off-camera Playwright context (Bob,
`BOWRAIN_PEER_TOKEN`) is a distinct workspace member who opens the **same**
Translate file. The bowrain collab WebSocket (`server/ws_collab.go`) relays Yjs
awareness between everyone in a room, so Bob's `PresenceAvatar` genuinely appears
on Alice's recorded screen — real multi-user presence, never mocked.

Seed both users (Alice owns the workspace, invites Bob, joins him) and capture:

```bash
node scripts/seed-collaboration.mjs > /tmp/collab.json   # prints both tokens + project/item/locale
# read the JSON and export the env it printed:
BOWRAIN_BACKEND_URL=http://localhost:8080 \
BOWRAIN_SESSION_TOKEN=<alice.token> BOWRAIN_PEER_TOKEN=<bob.token> \
BOWRAIN_PEER_NAME="<bob.name>" BOWRAIN_WORKSPACE_SLUG=<workspace> \
BOWRAIN_PROJECT_ID=<project_id> BOWRAIN_ITEM_ID=<item_id> BOWRAIN_COLLAB_LOCALE=<locale> \
  pnpm run demo bowrain-web-collaboration -- --only=capture --force --theme=both
# then narrate + render + publish (no tokens needed):
pnpm run demo bowrain-web-collaboration -- --only=narrate,render,publish
```

If `BOWRAIN_PEER_TOKEN` is unset the walk degrades to a single-user recording
(editor + governance frames) and skips the live-presence beats — it never
fabricates a teammate that isn't really connected.
