# kapi × Claude Code — demo harness

This harness **demonstrates and records kapi being driven by Claude Code**, as
narrated videos. Each demo runs a *real, headless `claude` session* against the kapi
Claude Code plugin, captures the transcript, screenshots the artifacts kapi produced,
generates a voice-over, and composes everything into an MP4 with [Remotion](https://remotion.dev).

Nothing here is mocked: the Claude sessions are live, the kapi commands run for real
(translating with Gemini, checking brand voice, importing glossaries…), and the
before/after artifacts are screenshots of kapi's actual output.

## What it produces

One narrated 1080p video per demo in `out/<id>.mp4`. Each video is structured as:

```
title card → real Claude Code terminal replay → artifact spotlights → outro
```

with a continuous British-English narration track explaining the story.

## The demos

| # | id | Aspect of the kapi skill it exercises |
|---|----|----------------------------------------|
| 1 | `01-localize-landing-page`        | AI translation + HTML format round-trip (zero-to-hero) |
| 2 | `02-brand-voice-guardrail`        | Brand voice: guide, check (0–100), rewrite, quality gate |
| 3 | `03-terminology-consistency`      | Terminology: import a glossary, look up terms, enforce them |
| 4 | `04-i18n-react-catalogs`          | i18n setup: react-i18next catalogs, presets, pseudo readiness, fr+de |
| 5 | `05-pseudo-translate-preflight`   | Pseudo-translation QA (offline) on a UI dialog |
| 6 | `06-multi-format-publishing`      | Format breadth: Markdown + Java `.properties` round-trip |
| 7 | `07-global-launch-many-languages` | Multi-locale incl. non-Latin (de/es/fr/**ja**) |
| 8 | `08-mcp-tools`                    | The MCP integration path: kapi run as an MCP server |
| 9 | `09-toolbox-find-replace`         | The toolbox (kcat/kgrep/ksed) — a **scripted shell** demo, no Claude |

Demos 1–8 cover all four sections of the kapi skill (`brand`, `localize`, `i18n`,
and the MCP/cloud path) plus the MCP tool surface.

### Scripted shell demos (no Claude)

A demo can set `terminal: shell` with a `script:` of commands instead of a Claude
`prompt`. The commands run for real in the sandbox (via `sh -c`, so globs expand)
and their output is recorded deterministically — no live `claude`, no billing, no
Gemini. The renderer frames them as a plain terminal (a `$` prompt, no Claude
banner or tool-call chrome) and the title/outro cards use the kapi-only lockup
(`brand: kapi`). Everything else — the macOS window, captions, voice-over, artifact
spotlights — is identical to the Claude demos. `09-toolbox-find-replace` is the
reference example.

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

Set in `.env` (see `.env.example`). Default is **Gemini TTS** (uses the same
`GEMINI_API_KEY` as kapi), styled for a clear British-English narrator voice.

| `NARRATION_BACKEND` | Needs | Notes |
|---|---|---|
| `gemini` (default) | `GEMINI_API_KEY` | Neural TTS, prompted for a British narrator voice |
| `elevenlabs`       | `ELEVENLABS_API_KEY` (+ `ELEVENLABS_VOICE_ID`) | Studio quality |
| `openai`           | `OPENAI_API_KEY` | `gpt-4o-mini-tts` |
| `say`              | macOS only | Offline fallback, voice `Daniel` (en_GB) |

Switch with `NARRATION_BACKEND=elevenlabs pnpm run demo <id> -- --only=narrate --force`.

## Usage

```bash
# one-time: build kapi (with fts5+icu4c), regenerate the plugin bundle,
# register the harness-gemini credential from .env, install Playwright Chromium
pnpm install
pnpm run setup

# run the whole pipeline for one demo (or `all`)
pnpm run demo 01-localize-landing-page
pnpm run demo all

# run a single stage (each stage is idempotent; --force re-runs it)
pnpm run demo all -- --only=capture          # just the live Claude sessions
pnpm run demo all -- --only=artifacts,narrate,render
pnpm run demo 02-brand-voice-guardrail -- --only=render --force

pnpm run list                                 # list demos
pnpm run studio                               # open the Remotion studio to preview
```

Prerequisites: a logged-in `claude` CLI, Node ≥ 22, `ffmpeg`, Go + Homebrew `icu4c`
(for building kapi), and a `GEMINI_API_KEY` in `.env` for the AI demos.

## Adding a demo

1. `mkdir -p demos/<id>/fixtures` and add the starting project files.
2. Write `demos/<id>/demo.yaml` (see any existing demo). Pin output filenames in the
   `prompt` and point `artifacts[].path` at them. Keep prompts to the **reliable
   standalone** kapi surface: `ai-translate`, `pseudo-translate`, `brand`, `termbase`,
   `word-count`, `formats`, `extract-content`, or the MCP tools.
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
