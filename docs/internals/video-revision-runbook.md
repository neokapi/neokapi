# Video revision runbook — up-first scene and demo revision (2026-07)

Companion to the scene/demo revision on `docs/scene-revision`. Every asset
below had its narration, commands, or structure revised toward the current CLI
surface (`kapi up` / gates / `exec` raw layer, post-#1180/#1187/#1188/#1191).
This runbook maps each asset to its re-record decision, the infrastructure the
session needs, the Norwegian narration status, and the publish step.

Every command line in the revised scenes was verified against `bin/kapi` built
from this branch (see "Verification" below). No recording was performed on
this branch — this is the authoring pass; recording follows this runbook.

Re-record vocabulary:

- **re-dub** — narration text changed, captured screen content still valid:
  re-run TTS + re-render (`make harness-videos`) from the existing captures.
- **re-capture** — commands, beats, or on-screen output changed: full
  `make harness-videos-staged` pass for the asset.
- **record-first-time** — no published capture exists yet.

## P1 — gate / up assets

| Asset | What changed | Re-record | Infra | Norwegian (nb) | Publish |
| --- | --- | --- | --- | --- | --- |
| `web/walkthroughs/kapi-up-loop` (NEW) | New scene: `up --plan` → `up` (produce, park) → `status --review` → `check --ship` exit 3 → `apply` → `check --ship` exit 0 → `status` shippable. The gate-blocks-then-clears asset. | record-first-time. `mode: video`: a curated VHS tape must be authored under `web/scenes/kapi-up-loop/` at record time (the generator does not emit tapes for video mode). Full command sequence verified natively end to end. | none — offline TM only, no provider, no server. Native `kapi` binary (wasm lacks `up`/`check`; see notes). | n/a — walkthrough scenes carry no nb narration | `make harness-videos-staged` → `make publish-cdn-videos` |
| `web/walkthroughs/kapi-project-workflow` | Restructured: leads with `kapi up` with `status` before/after; `ls` replaces `add` (a bare `add` records no target template, so coverage would not derive); `run`/`merge`/`extract` are now the closing "under the hood" beats; recipe flow renamed `tm-recycle` (no-shadowing rule). Commands + narration + structure. | full re-capture + re-dub | none — offline TM path | n/a | `make harness-videos-staged` → `make publish-cdn-videos` |
| `web/walkthroughs/kapi-under-the-hood` (NEW) | New plumbing-track scene: `exec recycle` (one tool), `run leverage-check` (one composed pass), `extract` → `merge -i` (interchange); each beat mapped to what `up` automates. Pairs with the docs page `/kapi/direct-execution-layer` (authored in parallel). | record-first-time | none — offline TM + deterministic qa | n/a | `make harness-videos-staged` → `make publish-cdn-videos` |
| `harness/demos/07-global-launch-many-languages` | Narration re-framed: one-pass multi-locale convergence via `kapi up`, `--plan` as the cost preview; per-language framing dropped. | full re-capture — the narration now asserts `up --plan` / one `up` pass; the prior capture ran per-language translates. | AI provider key (`needsAi: true`), Claude + kapi skill | n/a — no nb block | `make harness-videos-staged` → `make publish-cdn-videos` |
| `harness/demos/bowrain-cli-getting-started` | Narration only: `status` callouts for the server / terms / venue lines, `up` prints its venue first, `--plan` mention. Script/commands unchanged. | full re-capture recommended — the narration now points at the venue banner and the status server/terms/venue lines, so the capture must visibly show them. | server-stack: compose (Keycloak + bowrain-server), `make harness-seed` | n/a | `make harness-videos-staged` → `make publish-cdn-bowrain-videos` |

## P2 — revised kapi assets

| Asset | What changed | Re-record | Infra | Norwegian (nb) | Publish |
| --- | --- | --- | --- | --- | --- |
| `web/walkthroughs/kapi-review-and-approve` | Closing gating-consequence beat added (`check --ship` exits 3 until the decision lands; points at kapi-up-loop). Comment beat renders in the tape. | re-capture tape + dub the new closing line; existing narration lines unchanged | none | n/a | `make harness-videos-staged` → `make publish-cdn-videos` |
| `web/walkthroughs/kapi-pseudo-translate` | Pre-flight framing in narration; closing beat now points at `kapi translate` / `kapi up`. | re-capture tape + re-dub | none | n/a | same |
| `web/walkthroughs/kapi-terminology-pretranslation` | Command change: the retired top-level term-check spelling → `kapi exec term-check`; narration framed as the front half of `up`'s default flow. | full re-capture + re-dub | none | n/a | same |
| `web/walkthroughs/kapi-terminology-qa` | Same command change; narration ties term-check to `up`'s bound checks and the gate. | full re-capture + re-dub | none | n/a | same |
| `harness/demos/01-localize-landing-page` | Narration: `kapi translate` porcelain (TM reuse → AI → checks); hardcoded provider name dropped. | re-dub; spot-check the capture shows `kapi translate` — if the session ran a different spelling, re-capture (AI needed). | none for re-dub | n/a | `make harness-videos` → `make publish-cdn-videos` |
| `harness/demos/02-nextjs-zero-to-i18n` | Narration: pseudo-translate as pre-flight; "AI generated Japanese" beat re-framed to ship gate / review worklist. | re-dub | none | n/a | same |
| `harness/demos/04-i18n-react-catalogs` | Narration: `kapi translate` + built-in checks verify placeholders/plurals survived. | re-dub | none | n/a | same |
| `harness/demos/06-multi-format-publishing` | Narration: hardcoded provider name dropped. | re-dub | none | n/a | same |
| `harness/demos/08-mcp-tools` | Narration + caption: MCP tools are the same registry tools `up` and the porcelain drive; `exec` named as the CLI's raw doorway. Caption text changed → re-render required. | re-dub + re-render (`make harness-videos`); no re-capture (tool calls and stats artifact unchanged — stats already landed) | none | n/a | same |
| `harness/demos/kapi-desktop-content` | Narration (oneshot track): SYNC column callout (conditional phrasing — sample project is not server-connected, capture stays valid) and auto re-extract in the outro. | re-dub (full oneshot voice track) + re-render | none (no UI re-capture needed) | regenerate ids `files`, `outro` via the TM pipeline — stale nb entries removed in the demo.yaml | same |
| `harness/demos/kapi-desktop-flows` | Narration re-framed: built-in default flow is the zero-config hero; project flows are the escape hatch. Beats/screens unchanged. | re-dub (full oneshot track) + re-render | none | regenerate — most nb entries removed (English re-framed end to end); only three still-valid entries kept | same |
| `harness/demos/kapi-desktop-projects` | Narration: per-locale ladder (drafted → translated → reviewed) toward the gate; "Bring up to date" as the project's one main action. | re-dub (full oneshot track) + re-render | none | regenerate ids `project-home`, `content`, `outro` via the TM pipeline | same |

## P3 — bowrain assets

| Asset | What changed | Re-record | Infra | Norwegian (nb) | Publish |
| --- | --- | --- | --- | --- | --- |
| `harness/demos/bowrain-desktop-flows` | Narration: these flows are what a server-run `kapi up` executes (server-driven convergence). | re-dub + re-render | none | n/a — no nb block | `make harness-videos` → `make publish-cdn-bowrain-videos` |
| `harness/demos/bowrain-web-correction-loop` | Narration: kapi's checks are project-local; Bowrain versions them for the organisation. | re-dub + re-render | none | n/a | same |
| `harness/demos/bowrain-web-review` | Narration: single-player `status --review` / `apply` loop vs the team review surface; outro ties the round trip to the same review state. | re-dub + re-render | none | n/a | same |

## Record-first-time guardrails scenes

| Asset | What changed | Re-record | Infra | Norwegian (nb) | Publish |
| --- | --- | --- | --- | --- | --- |
| `web/walkthroughs/kapi-keep-source-on-brand` | Commands corrected to the current surface: `kapi check <file> --profile-file` carries the file-vs-profile check; `kapi brand rewrite` is stdin/`--input-text` term substitution (no `--diff`, no file positional). Former kit seeds (`product-page.md`, `brand.yaml`) did not exist in the playground kit — both are inline fixtures now, verified natively. Missing `.md` prompt file authored (was blocking embed regeneration). | record-first-time — `<PendingMedia>` is live on the public docs | none — offline brand checks | n/a | `make harness-videos-staged` → `make publish-cdn-videos` |
| `web/walkthroughs/kapi-rewrite-content` | The retired top-level `kapi rewrite` replaced by the inspect → apply pipeline (`inspect --jsonl`, `apply --diff`, `apply`, `check`), with real content hashes baked into the `edits.jsonl` fixture (verified against `kapi inspect`). The pre-existing missing-`.md` issue was still true; the `.md` prompt file is now authored on this branch. | record-first-time | none — no AI provider needed by any step | n/a | same |

## Verification performed on this branch

- `bin/kapi` built from this branch (`make build`, ICU 78 pkg-config). Every
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
- Embeds regenerated for all nine changed/new walkthrough ids via
  `scripts/walkthrough-gen/gen.ts` (smoke contracts synced by the generator).
  Note: this worktree has no frontend install; `node_modules` was resolved via
  the main checkout for the generator run — run `vp install` before touching
  frontend builds here.

## Known gaps / follow-ups (not addressed on this branch)

- **wasm build lacks the porcelain verbs**: `kapi/cmd/kapi-wasm-cli/main.go`
  wires `status`/`apply`/`run`/`extract`/`merge`/`exec`/… but not `up`,
  `check`, `inspect`, or the `brand` group. Affected scene steps are marked
  `offline: false` (tape + narration only); kapi-up-loop ships a pre-produced
  embed subset instead. Wiring `up`/`check` into the wasm build would let the
  project-workflow and up-loop embeds run the hero verbs live — code change,
  out of scope for this docs branch.
- **W7 snippet verifier not run here** (`make docs-verify-snippets` needs the
  wasm build + web deps). Run it before publishing; the synced smoke
  contracts follow the existing skip-list conventions (`.tmx` fixtures etc.).
- The stale comment in `kapi/cmd/kapi-wasm-cli/main.go` ("Top-level tool
  commands (pseudo-translate, term-check, …)") predates the exec-only cutover.
- `kapi apply --help` still references the retired `kapi rewrite` spelling in
  its long help — CLI help text, not a docs asset; flag for a CLI-side fix.
- Demo 07's artifact paths (`src/ja.json` etc.) assume the re-capture
  configures the project target template accordingly (`src/{lang}.json`);
  confirm during the re-record.
