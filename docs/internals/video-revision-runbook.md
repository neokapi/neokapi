# Video revision runbook: up-first scene and demo revision (2026-07)

The scene and demo revision of 2026-07 moved every asset below toward the
current CLI surface (`kapi up`, the ship gates, the `exec` raw layer). This
runbook maps each asset to its re-record decision, the infrastructure the
session needs, the Norwegian narration status, and the publish step.

Every command line in the revised scenes was verified against `bin/kapi` (see
"Verification" below). Authoring and recording are separate passes; this
runbook drives the recording.

Re-record vocabulary:

- **re-dub**: narration text changed, captured screen content still valid:
  re-run TTS + re-render (`make harness-videos`) from the existing captures.
- **re-capture**: commands, beats, or on-screen output changed: full
  `make harness-videos-staged` pass for the asset.
- **record-first-time**: no published capture exists yet.

## P1: gate / up assets

| Asset | What changed | Re-record | Infra | Norwegian (nb) | Publish |
| --- | --- | --- | --- | --- | --- |
| `web/walkthroughs/kapi-up-loop` (NEW) | New scene: `up --plan` → `up` (produce, park) → `status --review` → `check --ship` exit 3 → `apply` → `check --ship` exit 0 → `status` shippable. The gate-blocks-then-clears asset. | record-first-time. `mode: video`: a curated VHS tape must be authored under `web/scenes/kapi-up-loop/` at record time (the generator does not emit tapes for video mode). Full command sequence verified natively end to end. | none: offline content memory only, no provider, no server. | n/a: walkthrough scenes carry no nb narration | `make harness-videos-staged` → `make publish-cdn-videos` |
| `web/walkthroughs/kapi-project-workflow` | Restructured: leads with `kapi up` with `status` before/after; `ls` replaces `add` (a bare `add` records no target template, so coverage would not derive); `run`/`merge`/`extract` are now the closing "under the hood" beats; recipe flow renamed `tm-recycle` (no-shadowing rule). Commands + narration + structure. | full re-capture + re-dub | none: offline content memory path | n/a | `make harness-videos-staged` → `make publish-cdn-videos` |
| `web/walkthroughs/kapi-under-the-hood` (NEW) | New plumbing-track scene: `exec recycle` (one tool), `run leverage-check` (one composed pass), `extract` → `merge -i` (interchange); each beat mapped to what `up` automates. Pairs with the docs page `/kapi/direct-execution-layer` (authored in parallel). | record-first-time | none: offline content memory + deterministic qa | n/a | `make harness-videos-staged` → `make publish-cdn-videos` |
| `harness/demos/07-global-launch-many-languages` | Narration re-framed: one-pass multi-locale convergence via `kapi up`, `--plan` as the cost preview; per-language framing dropped. | full re-capture: the narration asserts `up --plan` / one `up` pass; the prior capture ran per-language translates. | AI provider key (`needsAi: true`), Claude + kapi skill | n/a: no nb block | `make harness-videos-staged` → `make publish-cdn-videos` |
| `harness/demos/bowrain-cli-getting-started` | Narration only: `status` callouts for the server / terms / venue lines, `up` prints its venue first, `--plan` mention. Script/commands unchanged. | full re-capture recommended: the narration points at the venue banner and the status server/terms/venue lines, so the capture must visibly show them. | server-stack: compose (Keycloak + bowrain-server), `make harness-seed` | n/a | `make harness-videos-staged` → `make publish-cdn-bowrain-videos` |

## P2: revised kapi assets

| Asset | What changed | Re-record | Infra | Norwegian (nb) | Publish |
| --- | --- | --- | --- | --- | --- |
| `web/walkthroughs/kapi-review-and-approve` | Closing gating-consequence beat added (`check --ship` exits 3 until the decision lands; points at kapi-up-loop). Comment beat renders in the tape. | re-capture tape + dub the new closing line; existing narration lines unchanged | none | n/a | `make harness-videos-staged` → `make publish-cdn-videos` |
| `web/walkthroughs/kapi-pseudo-translate` | Pre-flight framing in narration; closing beat now points at `kapi translate` / `kapi up`. | re-capture tape + re-dub | none | n/a | same |
| `web/walkthroughs/kapi-terminology-pretranslation` | Command change: the retired top-level term-check spelling → `kapi exec term-check`; narration framed as the front half of `up`'s default flow. | full re-capture + re-dub | none | n/a | same |
| `web/walkthroughs/kapi-terminology-qa` | Same command change; narration ties term-check to `up`'s bound checks and the gate. | full re-capture + re-dub | none | n/a | same |
| `harness/demos/01-localize-landing-page` | Narration: `kapi translate` porcelain (content memory reuse → AI → checks); hardcoded provider name dropped. | re-dub; spot-check the capture shows `kapi translate`; if the session ran a different spelling, re-capture (AI needed). | none for re-dub | n/a | `make harness-videos` → `make publish-cdn-videos` |
| `harness/demos/02-nextjs-zero-to-i18n` | Narration: pseudo-translate as pre-flight; "AI generated Japanese" beat re-framed to ship gate / review worklist. | re-dub | none | n/a | same |
| `harness/demos/04-i18n-react-catalogs` | Narration: `kapi translate` + built-in checks verify placeholders/plurals survived. | re-dub | none | n/a | same |
| `harness/demos/06-multi-format-publishing` | Narration: hardcoded provider name dropped. | re-dub | none | n/a | same |
| `harness/demos/08-mcp-tools` | Narration + caption: MCP tools are the same registry tools `up` and the porcelain drive; `exec` named as the CLI's raw doorway. Caption text changed → re-render required. | re-dub + re-render (`make harness-videos`); no re-capture (tool calls and stats artifact unchanged; stats already landed) | none | n/a | same |
| `harness/demos/kapi-desktop-content` | Narration (oneshot track): SYNC column callout (conditional phrasing; the sample project is not server-connected, so the capture stays valid) and auto re-extract in the outro. | re-dub (full oneshot voice track) + re-render | none (no UI re-capture needed) | regenerate via `make l10n`; ids `files`, `outro` pending in the content memory (sidecar demo.nb.yaml falls back to EN for them) | same |
| `harness/demos/kapi-desktop-flows` | Narration re-framed: built-in default flow is the zero-config hero; project flows are the escape hatch. Beats/screens unchanged. | re-dub (full oneshot track) + re-render | none | regenerate via `make l10n`; three scenes recovered from the content memory, the re-framed rest pending (EN fallback in demo.nb.yaml) | same |
| `harness/demos/kapi-desktop-projects` | Narration: per-locale ladder (drafted → translated → reviewed) toward the gate; "Bring up to date" as the project's one main action. | re-dub (full oneshot track) + re-render | none | regenerate via `make l10n`; ids `project-home`, `content`, `outro` pending in the content memory (EN fallback in demo.nb.yaml) | same |

## P3: bowrain assets

| Asset | What changed | Re-record | Infra | Norwegian (nb) | Publish |
| --- | --- | --- | --- | --- | --- |
| `harness/demos/bowrain-desktop-automations` | Narration: these automations are what a server-run `kapi up` executes (server-driven convergence). | re-dub + re-render | none | n/a: no nb block | `make harness-videos` → `make publish-cdn-bowrain-videos` |
| `harness/demos/bowrain-web-correction-loop` | Narration: kapi's checks are project-local; Bowrain versions them for the organisation. | re-dub + re-render | none | n/a | same |
| `harness/demos/bowrain-web-review` | Narration: single-player `status --review` / `apply` loop vs the team review surface; outro ties the round trip to the same review state. | re-dub + re-render | none | n/a | same |

## Record-first-time guardrails scenes

| Asset | What changed | Re-record | Infra | Norwegian (nb) | Publish |
| --- | --- | --- | --- | --- | --- |
| `web/walkthroughs/kapi-keep-source-on-brand` | Commands corrected to the current surface: `kapi check <file> --profile-file` carries the file-vs-profile check; `kapi voice rewrite` is stdin/`--input-text` term substitution (no `--diff`, no file positional). Former kit seeds (`product-page.md`, `voice.yaml`) did not exist in the playground kit, both are inline fixtures now, verified natively. Missing `.md` prompt file authored (was blocking embed regeneration). | record-first-time; `<PendingMedia>` is live on the public docs | none: offline voice checks | n/a | `make harness-videos-staged` → `make publish-cdn-videos` |
| `web/walkthroughs/kapi-rewrite-content` | The retired top-level `kapi rewrite` replaced by the inspect → apply pipeline (`inspect --jsonl`, `apply --diff`, `apply`, `check`), with real content hashes baked into the `edits.jsonl` fixture (verified against `kapi inspect`). The pre-existing missing-`.md` issue was still true; the `.md` prompt file is now authored on this branch. | record-first-time | none: no AI provider needed by any step | n/a | same |

## Verification performed

- `bin/kapi` built with `make build` (ICU 78 pkg-config). Every
  scene command run in isolated sandboxes (`KAPI_NO_PROJECT`/`KAPI_CONFIG_DIR`/
  `XDG_*`/`KAPI_PLUGINS_DIR_ONLY` per CLAUDE.md) with fixtures matching the
  scene files: the full kapi-up-loop sequence (park → worklist → exit 3 →
  apply → exit 0 → shippable), the restructured project-workflow sequence
  (`up` converges in one pass; `run`/`merge`/`extract` beats), the
  under-the-hood beats (`exec recycle`, composed `leverage-check` flow,
  `extract` emitting `out/messages.en-to-fr.xliff`, `merge -i`), the brand
  check/rewrite beats, and the inspect → apply pipeline (hashes in
  `edits.jsonl` are the real ones).
- All changed/created scene and demo YAMLs parse (yq; PyYAML is absent on this
  machine).
- Retired spellings grep clean across `web/walkthroughs/` and `harness/demos/`
  (excluding `_retired/`): no retired verbs, no top-level raw-tool spellings,
  no retired count/check/report/analysis tools, no flowless `kapi run`.
- Embeds regenerated for the changed and new walkthrough ids via
  `scripts/walkthrough-gen/gen.ts` (smoke contracts synced by the generator).

## Desktop demo app: vocabulary sweep (2026-08-22), recording deferred

The recording-only desktop app (`apps/kapi-desktop/frontend/src/demo/`) is what
the `kapi-desktop-*` walkthroughs show on screen. Its copy and sample data were
swept to the fixed vocabulary, and two on-screen paths were wrong as well as
retired: the demo listed named stores under `~/.config/kapi/termbases/` and
`~/.config/kapi/tm/`, where `namedResourceDir` puts them under `terms/` and
`memory/` (`apps/kapi-desktop/backend/memory.go`). Every published desktop
capture therefore shows a directory that does not exist.

Authoring is done; **no recording was performed**. Narration needed no change;
the demo scripts were already clean, and their `beat`/step ids (`open-termbases`,
`open-glossary`) are recorded identifiers that stay.

| Asset | What changed on screen | Re-record | Infra | Publish |
| --- | --- | --- | --- | --- |
| `harness/demos/kapi-desktop-explorer` | Home copy ("behind your localisation" → "behind your content"), and the Terms list + opened-store subtitle now read `~/.config/kapi/terms/<name>.db` instead of the non-existent `termbases/`. Beats `intro`, `open-termbases`, `open-glossary` all show changed pixels. | full re-capture; **no re-dub**, narration text is unchanged | none: `demo.html` runs in-browser with sample data, no backend | `make harness-videos-staged` → `make publish-cdn-videos` |
| `harness/demos/kapi-desktop-{config,content,flows,projects}` | Not known to show the changed screens, but all five demos boot the same `demo.html` entry. | spot-check each capture's opening frames for the home copy before deciding; re-capture only those that show it | none | same |

Storybook fixtures were swept in the same pass and are **not** recorded, so they
carry no re-record debt.

## Known gaps / follow-ups

- **Browser-only steps.** The wasm build mirrors the native command set
  (`cli.BrowserCommandSet`), and only the verbs that need a subprocess, the
  keychain, the network or a socket (`plugin`, `models`, `credentials`,
  `telemetry`, `update`, `mcp`, `engine`) are stand-ins there. Scene steps that
  need AI, the network or SQLite stay `offline: false` (tape + narration only),
  and kapi-up-loop ships a pre-produced embed subset.
- **Run the W7 snippet verifier before publishing** (`make docs-verify-snippets`
  needs the wasm build + web deps); the synced smoke contracts follow the
  existing skip-list conventions (`.tmx` fixtures etc.).
- Demo 07's artifact paths (`src/ja.json` etc.) assume the re-capture
  configures the project target template accordingly (`src/{lang}.json`);
  confirm during the re-record.
- The desktop demo app's vocabulary sweep (section above) is authored but
  unrecorded: published `kapi-desktop-explorer` frames still show
  `~/.config/kapi/termbases/`, a path the app never used.
- `harness/src/driver/record-desktop.ts` still says "a localized recording
  pass" in its own prose. It is harness code rather than a swept surface, so
  `check-vocabulary.sh` does not see it; recast it when the recorder is next
  touched.
