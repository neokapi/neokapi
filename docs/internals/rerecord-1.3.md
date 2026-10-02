# Re-recording the walkthroughs for 1.3

The walkthrough videos on both docs sites were published on 2026-07-13, and
`monolingual-governance` on 2026-08-15 (the `Last-Modified` of each object on
the CDN). The takes recorded on 2026-09-08 were rendered and never published.
Since then 1.3 changed what several of them show:

- One status vocabulary. A source unit is `written` or `established`; a target
  unit is `draft`, `translated` or `established`. Ship states are
  `established`, `translated`, `withheld` and `not_gated`. Sign-off is gone, and
  an assistant records a pre-review and never establishes a unit.
- `kapi check` reports `kapi.check/v2`, marks each finding `FAILS` or
  `REPORTS`, and fails on a failing finding. There are no severity grades and
  no `--strict`.
- Word rules are terms. `kapi context import` keeps the voice and moves a voice
  file's word rules into the terms store.
- `.kapi/` is a cache. The context is shared with `kapi context pull` and
  `kapi context push`; `defaults.translate_after` replaces
  `defaults.source_gate`.
- `kapi init` proposes collections and writes an MCP entry and one short skill.
- `kapi apply` reads a kapi.change/v1 change set: a term is
  `{"op":"term","action":"upsert",...}`, a recipe field `{"op":"recipe",...}`,
  and a review decision `{"op":"decide",...}` naming the unit's document,
  block and language. `kapi inspect` prints each block's reference and
  revision, and `ksed` writes through the same contract.
- Kapi Desktop opens on the workspace, carries the context digest and a
  Learned section in the Context hub, and its review surface has one human rung.

This page is the ordered list of what to record, how, on what infrastructure,
and what to check in each result. The authored sources (demo scripts,
narration, fixtures, samples and the text around each embed) are already
updated. The general procedure is in
[regenerating docs assets](regenerating-docs-assets.md); this page names only
what differs per video.

## Before you start

1. Build from the commit you are releasing, so the recording shows the release:

   ```bash
   make build build-bowrain-plugin    # bin/kapi, bin/kapi-bowrain
   make harness-deps                  # once per checkout
   ```

   The shell demos were dry-run against both the `v1.3.0-rc2` release binary
   and `main` at `18c8f65ad`, and printed the same output.

2. Put `GEMINI_API_KEY` in `~/.config/neokapi/harness.env` for narration.

3. For the Bowrain videos, bring up the local stack. Never point a recording
   at production.

   ```bash
   make -C bowrain stack-up-web       # Keycloak, PostgreSQL, bowrain-server, SPA at http://localhost:8080
   export BOWRAIN_BACKEND_URL=http://localhost:8080
   make harness-seed                  # BowMart workspace + record tokens in harness/.env
   ```

   Device-flow tokens are short-lived: mint them again right before a capture
   session.

Every command below runs from `harness/`. `--force` redoes a stage whose output
exists; `--theme=both` renders light and dark. The publish stage writes
`<publishAs>-{light,dark}.webm` into `web/static/video/kapi/` or the Bowrain
docs tree, chosen by the demo's brand.

For a desktop demo, narrate first and then record, so each beat stays on camera
as long as its narration:

```bash
vpx tsx src/cli/run.ts <id> --only=narrate --force
vpx tsx src/cli/run.ts <id> --only=capture,artifacts,render,publish --force --theme=both
```

Every other demo runs the whole pipeline in one call:

```bash
vpx tsx src/cli/run.ts <id> --force --theme=both
```

## The order

The first group replaces published videos that docs pages embed today. The
second fills pages that show a placeholder. The third holds the demos that have
never been published and have no page yet. The fourth has updated sources and
no embed.

Ten demos have never been published (the CDN answers 403 for each): rows 16 to
25. Their scripts were checked step by step against 1.3, and every shell demo
among them was dry-run under the full isolation environment against the
`v1.3.0-rc2` release binary and `main` at `7c52a73f0`. Each step exited as it
declares, and the two binaries differ only in the recording guidance a context
answer ends with (#2989).

### 1. Embedded, and showing retired output or an earlier interface

| # | Demo | Publishes as | Embedded on | Infrastructure | What changed |
| --- | --- | --- | --- | --- | --- |
| 1 | `s0-northsea-checks` | `monolingual-governance` | `kapi/recipes/keep-source-on-brand.mdx` | none | `kapi check --strict` is `kapi check`; findings read FAILS/REPORTS; the sample's voice patterns moved into `constraints:`, so no configuration warnings print; the `dock` decision is a `term` operation, and `kapi apply` closes with `change set applied: 1 applied` |
| 2 | `05-ai-checks-guardrail` | `kapi-checks-guardrail` | `framework/checks/rule-checks.md` | none | the findings table reads FAILS, not CRITICAL/MAJOR |
| 3 | `kapi-bilingual-workflow` | `bilingual-workflow` | `kapi/bilingual-workflow.mdx` | none | output of the July take; commands unchanged |
| 4 | `09-toolbox-find-replace` | `toolbox-explainer` | `toolbox/overview.mdx` | none | commands and output unchanged; re-recorded for the September template. `ksed` now applies its substitutions as change-set operations, and the demo's commands print the same output and leave the same bytes in all three files as before |
| 5 | `kapi-desktop-projects` | `kapi-desktop-projects` | `kapi/desktop/tour.mdx`, `kapi/desktop/recipes/author-a-project-visually.mdx` | desktop recorder (wbridge, no server) | the app opens on the workspace; project home carries the context digest |
| 6 | `kapi-desktop-flows` | `kapi-desktop-flows` | `kapi/desktop/tour.mdx`, `kapi/desktop/recipes/build-a-flow-visually.mdx` | desktop recorder | earlier Toolbox and home |
| 7 | `kapi-desktop-config` | `kapi-desktop-config` | `kapi/desktop/tour.mdx`, `kapi/desktop/recipes/store-ai-credentials.mdx` | desktop recorder | earlier settings layout |
| 8 | `bowrain-cli-getting-started` | `bowrain-cli-getting-started` | Bowrain `getting-started/the-loop.mdx`, `walkthroughs/bowrain-getting-started.mdx` | stack + `make harness-seed` | `kapi init` proposes the catalog's collection, so the `kapi add` step is gone; status, push and up output |
| 9 | `bowrain-cli-auth-and-workspaces` | `bowrain-cli-auth-and-workspaces` | Bowrain `walkthroughs/bowrain-auth.mdx` | stack + seed | output of the July take |
| 10 | `bowrain-web-review` | `bowrain-web-review` | Bowrain `server/review.mdx`, `server/web-overview.mdx` | stack + `scripts/seed-collaboration.mjs` (two users) | one human rung, no sign-off controls |
| 11 | `bowrain-web-editor` | `bowrain-web-editor` | Bowrain `server/translation-editor.mdx`, `server/web-overview.mdx` | stack + seed | status badges |
| 12 | `bowrain-web-governance` | `bowrain-web-governance` | Bowrain `server/context.mdx`, `server/terminology.mdx`, `server/translation-memory.mdx`, `server/web-overview.mdx` | stack + seed (`BOWRAIN_TERM_BLOCK_TEXT`, `BOWRAIN_TERM_TEXT`) | July take |
| 13 | `bowrain-web-collaboration` | `bowrain-web-collaboration` | Bowrain `getting-started/introduction.mdx`, `server/collaboration.mdx`, `server/web-overview.mdx` | stack + `seed-collaboration.mjs` (two users) | July take |
| 14 | `bowrain-web-correction-loop` | `bowrain-web-correction-loop` | Bowrain `server/context-voice.mdx`, `server/web-overview.mdx` | stack + seed | July take |
| 15 | `bowrain-desktop-dashboard` | `bowrain-desktop-dashboard` | Bowrain `server/desktop-app.mdx` | stack + seed; the recorder's relay | July take; the reconnect fix (#2596) makes the offline beat recordable |

Rows 1 to 4 need nothing but the build. Rows 5 to 7 start their own isolated
backend. Rows 8 to 15 need the stack only while they capture. Narrate them
first (the web and desktop walks read each beat's length from the narration),
capture in one stack session, then quit Docker Desktop before rendering (see
"Desktop/web render reliability" in
[regenerating docs assets](regenerating-docs-assets.md)):

```bash
WEB="bowrain-web-editor bowrain-web-governance bowrain-web-correction-loop bowrain-desktop-dashboard"
TWO="bowrain-web-collaboration bowrain-web-review"
CLI="bowrain-cli-getting-started bowrain-cli-auth-and-workspaces"
vpx tsx src/cli/run.ts $WEB $TWO --only=narrate --force          # no stack needed
vpx tsx src/cli/run.ts $CLI $WEB --only=capture,artifacts --force --theme=both
node scripts/seed-collaboration.mjs > /tmp/collab.json           # export the env it prints
vpx tsx src/cli/run.ts $TWO --only=capture,artifacts --force --theme=both
# stack down, quit Docker Desktop, then:
vpx tsx src/cli/run.ts $CLI --only=narrate --force
vpx tsx src/cli/run.ts $CLI $WEB $TWO --only=render,publish --force --theme=both
```

`bowrain-web-review` also takes `BOWRAIN_PEER_BLOCK_ID` and
`BOWRAIN_SELF_BLOCK_ID` from the collaboration seed; without them the
separation-of-duties beats act on whichever unit is in focus.

### 2. Placeholders on the page

Both are first publishes, and both count among the ten that were never
published.

| # | Demo | Publishes as | Placeholder on | Infrastructure | What changed |
| --- | --- | --- | --- | --- | --- |
| 16 | `kapi-desktop-review` | `kapi-desktop-review` | `kapi/recipes/review-and-approve.mdx`, `kapi/recipes/translate-content.mdx` | desktop recorder | one human rung: Approve, with no sign-off control; every walk selector still renders |
| 17 | `bowrain-desktop-automations` | `bowrain-desktop-automations` | Bowrain `walkthroughs/bowrain-automation.mdx` | stack + seed (#2597 gives the seed a source ready to run) | narration only: two sentences cut; the run row and the delivery panel's review link still render |

`kapi-desktop-projects` (row 5) also fills the video placeholders on
`kapi/get-started/add-languages.mdx` and `kapi/get-started/first-project.mdx`.
Replacing a `<PendingMedia>` with a `<ThemedVideo>` is an edit to the page after
the asset is on the CDN.

### 3. Never published, no page yet

Record these after the first two groups. Each one runs offline in a sandbox
with the `demo` provider or none, so a take needs nothing but the build:

```bash
vpx tsx src/cli/run.ts <id> --force --theme=both
```

No page links any of them. The page named is where each one fits, as a
proposal; embedding it is a page edit after the asset is on the CDN.

| # | Demo | Publishes as | Would fit on | Infrastructure | What changed |
| --- | --- | --- | --- | --- | --- |
| 18 | `s0-northsea-context` | `monolingual-context` | `kapi/context.mdx` | none | nothing in the script; the import reports the word rules it moved into terms, and each context answer ends with the #2989 recording guidance |
| 19 | `10-cli-points-and-voice` | `cli-points-and-voice` | `kapi/projects.mdx` | none | the fixture's voice file sits under `context/`, like the samples', and `setup:` reads it with `kapi context import ./context`; the voice guide is asked for by path (`kapi voice guide partner/index.md`), and holds the portal's register and no terms; the brand coordinate is a `recipe` operation |
| 20 | `11-cli-terms-and-queue` | `cli-terms-and-queue` | `kapi/recipes/terminology-checks.mdx` | none | reads the same `context/` as row 19; the narration counts the four French rows it approves (it said twenty); the `--jq` filter writes one `decide` operation per row, and the narration says the queue names what a decision addresses |
| 21 | `audience-constraints` | `audience-constraints` | `kapi/recipes/content-governance-for-ai.mdx` | none | rewritten for the screen: text output in place of several hundred lines of JSON, `ksed` edits in place of `node -e`, and one `--jq` line for the findings' provenance; the sample's `emotion: reassuring` became `warm`, which removes a configuration warning |
| 22 | `s1-compass-converge` | `multilingual-converge` | `kapi/convergence.mdx` | none | the `ship.json` narration names Dutch's `not_governed` note |
| 23 | `s1-compass-ship-gate` | `multilingual-ship-states` | `kapi/recipes/ship-gates-and-ci.mdx` | none | the first highlight lands on `parked` and `withheld`; nothing on screen said `blocked`; the two `--jq` filters write `decide` operations |
| 24 | `s2-tidewatch-build-output` | `docs-site-convergence` | `kapi/recipes/translate-content.mdx` | none | nothing in the script |
| 25 | `s2-tidewatch-ci` | `docs-coverage-in-ci` | `kapi/convergence-in-ci.mdx` | none | the finding that reports is on the integration page, which names the retired word where it explains `mooring_id`; the comment and narration said the API reference |

Rows 1, 19, 20 and 23 were changed after the dry runs above, when `kapi apply`
moved to kapi.change/v1: their apply steps now write operations. Dry-run those
four against a build of the release commit before recording them.

To take all eight in one call:

```bash
vpx tsx src/cli/run.ts s0-northsea-context 10-cli-points-and-voice 11-cli-terms-and-queue \
  audience-constraints s1-compass-converge s1-compass-ship-gate \
  s2-tidewatch-build-output s2-tidewatch-ci --force --theme=both
```

### 4. Sources updated, no embed

Record these once the first three groups are out. None of them is linked from a
page, so nothing a reader sees is stale in the meantime.

| Demo | Publishes as | Infrastructure | Note |
| --- | --- | --- | --- |
| `08-mcp-tools` | (not published) | Claude session, `kapi mcp --tools all` | the assistant records a pre-review; the queue keeps its length and shows the score |
| `kapi-desktop-content` | `kapi-desktop-content` | desktop recorder | published in July, not embedded |
| `kapi-desktop-explorer` | `kapi-desktop-explorer` | desktop recorder | the Context hub rail now lists Learned and Agent View above Terms and Content Memory |
| `bowrain-sizzle` | `bowrain-sizzle` | renders from the web and desktop clips | render after rows 10 to 15 |
| `02-nextjs-zero-to-i18n`, `03-translate-docx` | `claude-app-i18n`, `claude-translate-document` | Claude session (billed) | published in July, not embedded |
| `01`, `04`, `06`, `07` | (not published) | Claude session (billed); AI provider for 01-07 | `04` and `07` render `kapi check --json` and `kapi status --json` artifacts from the take's sandbox, so `--only=artifacts,render` on the machine that holds the take refreshes them without a new session |

The web walkthrough scenes (`web/walkthroughs/*.scene.yaml`) are interactive
embeds that run the wasm build live on the page, so they need no recording. The
ones a page embeds (`kapi-overview`, `kapi-explain-prompts`,
`kapi-kpz-workspace`, `kapi-terminology-pretranslation`,
`kapi-terminology-checks`, `kapi-review-and-approve`) are checked with
`make docs-verify-snippets`. `kapi-up-loop` is still a video scene with no tape
authored, and is not embedded.

## What to check in each result

Every shell take fails its capture stage when a command exits other than its
step declares, so a take that finishes is at least the take the script meant.
Beyond that, look at the rendered video:

- **Any check output** reads `FAILS` and `REPORTS`. No finding says critical,
  major or minor, and no run prints a "Configuration warnings" block.
- **Any status output** uses `draft`, `translated` and `established`, and a ship
  column of `translated`, `established`, `withheld`, `not gated` or `blocked`.
  Nothing says reviewed, approved as a state, signed off or governed.
- **Terminal paths**: `kapi memory import`, `extract` and `merge` print the
  absolute sandbox path (`/private/tmp/...`). The harness rewrites it only for
  Bowrain demos, so check `kapi-bilingual-workflow` and decide whether the line
  is acceptable before publishing.
- **Highlights** land on a word the output contains: `FAILS`, `REPORTS`,
  `blocked`, `parked`, `withheld`, `false`, `seamless`, `dock`, `risk-free`,
  `voice.guidance`. A highlight on a word that is not on screen draws nothing.

Per video:

| Demo | Check |
| --- | --- |
| `s0-northsea-checks` | first check: 1 FAILS (`seamless`) and 4 REPORTS, exit 3; `findings.json` artifact shows `"schema": "kapi.check/v2"`; after the `dock` decision 1 FAILS; the last check passes with 2 REPORTS |
| `05-ai-checks-guardrail` | 3 FAILS, exit 3; then `No findings.` and PASS |
| `s0-northsea-context` | the import reports 7 concepts, 1 voice profile and 8 word rules moved into terms; the two answers name `northsea/docs` and `northsea/landing`; the search lists `berth` preferred, `mooring` discouraged and `mooring_id` admitted |
| `10-cli-points-and-voice` | `ls` lists `context`; the two answers name `voltway/docs` and `voltway/portal` with `brand=helios`; the voice guide shows the portal's medium sentences and no Terms section; one FAILS on `partner/index.md` (`seamless`), exit 3, then PASS; the recipe artifact's comments say `advisory`, not severity |
| `11-cli-terms-and-queue` | the prompt excerpt lists `billing period` and `reading`; `kapi status` reads fr `blocked: checks` with one failing unit, because `billing.export` says "période comptable" and the recipe's `billing period` rule holds the gate; four French rows are approved; French goes from 0% to 100% established; the queue for French empties; the source rows stay |
| `audience-constraints` | both answers list the same two shared constraints; the first check passes with `requested and not run: voice.guidance`; the planted phrase gives 4 FAILS and exit 3; the jq line prints `harbor-help/no-unsupported-assurance v1` for each page; the restored pages pass; the contradicting sentence passes with `voice.guidance` still not run; no "Configuration warnings" block |
| `s1-compass-converge` | the first `ship.json` withholds all three; `kapi up` answers 37 of Norwegian's 38 units from content memory; afterwards `nb` and `de` are `translated` and `nl` is `blocked: checks` |
| `s1-compass-ship-gate` | `kapi up` parks `nl`; `ship.json`: `nl` withheld, then `translated`; twenty Dutch approvals; `nb` ends `established` |
| `s2-tidewatch-build-output` | `nb` ships `translated`, `nl` is `blocked: review`; the Norwegian front matter keeps the stub's marks on the description; `find` lists four Norwegian files and no Dutch ones |
| `s2-tidewatch-ci` | the jq line prints `established 77%` for nb and `established 0%` for nl; the check passes with one REPORTS, on `docs/integrating.md` (`mooring`) |
| `bowrain-desktop-automations` | Run now starts a run that appears at the top of the table; the row settles with a per-language summary; the dashboard's "Review pending translations" link opens the review session |
| `bowrain-cli-getting-started` | `kapi init` prints `collections (proposed from the files here...)` with `src/locales/en.json`; `kapi status` shows the `content` and `governance` lines; `kapi up` prints the venue first; `src/locales/fr.json` arrives |
| `08-mcp-tools` | the session reads the unit with `review_block` and sends `apply_edits` a `decide` with outcome `advise`, not an approval; the queue artifact still lists the unit, now with `aiScore` |
| `kapi-desktop-review` | the unit shows one human action (Approve) and no sign-off control; the history layer reads "Already approved" |
| `kapi-desktop-explorer` | the Context hub opens on Learned; the walk then opens Terms, then Content Memory |
| desktop and web demos | the recorder log reports no inert crop and no selector that matched nothing |

## After publishing

1. `make publish-cdn-videos` for the kapi site and
   `make publish-cdn-bowrain-videos` for Bowrain, then compare each object's
   `Content-Length` on the CDN with the local file.
2. Remove the `outdated` and `outdatedNote` props from every `<ThemedVideo>`
   whose video was re-recorded (rows 1 to 15), and replace the `<PendingMedia>`
   placeholders filled by rows 5, 16 and 17.
3. Dispatch the docs builds for `/next/` and production as in
   [regenerating docs assets](regenerating-docs-assets.md).

## Norwegian narration

The English narration changed in these demos: `01`, `05`, `08`, `10`, `11`,
`audience-constraints`, `bowrain-cli-getting-started`,
`bowrain-desktop-automations`, `bowrain-web-review`,
`s0-northsea-checks`, `s0-northsea-context`, `s1-compass-converge`,
`s1-compass-ship-gate`, `s2-tidewatch-build-output`, `s2-tidewatch-ci`. Their
`demo.nb.yaml` sidecars are loop output, and the next dogfood convergence in CI
translates the changed scenes. The narrate stage refuses a published demo's
Norwegian pass until every scene is translated, so record English first and run
`--locale=nb --only=narrate,render,publish` once the sidecars have caught up.

## Open before recording

No walkthrough shows the context digest or the Learned section on its own;
`kapi-desktop-explorer` passes the Learned rail entry on its way to Terms. A
beat for it needs a sample with something learned to show.
