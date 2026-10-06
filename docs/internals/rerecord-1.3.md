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
- Kapi Desktop edits through the change service. The Review page's translation
  and source panes are the inline-code editor (codes as chips, a plural a form at
  a time) in place of a plain text box, its Save sends a `set_content` with the
  revision read, Approve and Reject send `decide`, and a unit that changed since
  it opened shows "Changed since you opened it" before anything is written. A
  save a failing rule refuses lists the findings and offers "Save anyway". A
  **Changes** card under Provenance lists the unit's recorded changes. The
  Checks panel's Apply fix sends the finding's fix, and the document view offers
  Edit on a focused unit. Each form of a plural is an inline-code editor of its
  own, and Approve clean checks each unit again, stopping at one that changed
  since the queue was listed or now has a finding.
- Kapi Desktop's project home lists the conflicts `kapi status` reports, when
  there are any, between the standing and the point map: each block with the
  wording held now and the wording that did not land, and buttons to keep
  either or write another. A sample with no conflict shows no such card.
- A source a `.kpz` carries is edited as `work.kpz!<name>`: `kapi inspect` reads
  it and `kapi apply` and the MCP edit tools write it, and `kapi info` then
  reports the workspace dirty.
- A `kapi check --json` finding whose rule names a replacement carries `fix`,
  the `replace_text` operation that applies it, naming its document by the path
  from the project root. A finding whose words have formatting among them, or
  that sits in a translation's file, carries none, and the Checks panel offers
  no Apply fix for it.
- Bowrain records an agent's pre-review. An assistant on the server's MCP
  endpoint sends `decide` with outcome `advise`; the review session's queue row
  shows "AI" and the score, and the block's checks card shows the score, the
  assistant's name and its reasons. Use match in the review session and the
  document's inspector saves the match with its codes.
- The Bowrain desktop's offline count reads "N not sent" and opens a list of
  the changes that did not reach the server, each with its wording, its file and
  language, and why; an edit an earlier version queued is listed as dropped.
- Every write through the change service passes the commit check. `ksed`
  refuses an edit that introduces a failing finding, prints `gate_failed` with
  the finding for each file, writes nothing, and exits 2. `audience-constraints`
  shows that refusal and then plants its violation with `perl`, as an edit made
  outside kapi would arrive.
- `kapi status --review` ends with the instruction to approve a block with a
  `decide` operation through `kapi apply`, on a translation or on the source.
- Block editions, KBF v2, the KPZ document home, the vault and sync state under
  `.kapi/`, and the browser engine's Worker and OPFS persistence change no step
  of the offline shell demos. They show in the in-browser walkthrough scenes,
  which `make docs-verify-snippets` checks and nobody records.

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

   The twelve offline shell demos were dry-run on 2026-10-06 under the full
   isolation environment against `main` at `dee55f220` with the change that
   updated this page, and every step exited as it declares. Record from a build
   that includes that change: `11-cli-terms-and-queue` shows the review queue's
   closing instruction, which it rewrote. The two Bowrain CLI demos need the
   stack and were not dry-run.

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
25. Each demo links its script, and each page is named by its URL on the `/next/`
docs channel, which every push to main deploys: `https://neokapi.github.io/next/`
for kapi and `https://bowrain.cloud/docs/next/` for Bowrain.

### 1. Embedded, and showing retired output or an earlier interface

| # | Demo | Publishes as | Embedded on (`/next/`) | Infrastructure | What changed |
| --- | --- | --- | --- | --- | --- |
| 1 | [`s0-northsea-checks`](../../harness/demos/s0-northsea-checks/demo.yaml) | `monolingual-governance` | [kapi/recipes/keep-source-on-brand](https://neokapi.github.io/next/kapi/recipes/keep-source-on-brand) | none | `kapi check --strict` is `kapi check`; findings read FAILS/REPORTS; the sample's voice patterns moved into `constraints:`, so no configuration warnings print; the `dock` decision is a `term` operation, and `kapi apply` closes with `change set applied: 1 applied`; the `findings.json` artifact gives each term finding with a replacement a `fix`, except one whose words have formatting among them |
| 2 | [`05-ai-checks-guardrail`](../../harness/demos/05-ai-checks-guardrail/demo.yaml) | `kapi-checks-guardrail` | [framework/checks/rule-checks](https://neokapi.github.io/next/framework/checks/rule-checks) | none | the findings table reads FAILS, not CRITICAL/MAJOR |
| 3 | [`kapi-bilingual-workflow`](../../harness/demos/kapi-bilingual-workflow/demo.yaml) | `bilingual-workflow` | [kapi/bilingual-workflow](https://neokapi.github.io/next/kapi/bilingual-workflow) | none | commands unchanged; each XLIFF unit now carries an `<mda:metadata>` with its `if-match` and `basis` revisions, which the `head -20` beat shows, and `kapi merge` writes the returned translations through the change service, from the source's skeleton |
| 4 | [`09-toolbox-find-replace`](../../harness/demos/09-toolbox-find-replace/demo.yaml) | `toolbox-explainer` | [toolbox/overview](https://neokapi.github.io/next/toolbox/overview) | none | commands and output unchanged; re-recorded for the September template. `ksed` applies its substitutions as change-set operations, and the demo's commands print the same output and leave the same bytes in all three files |
| 5 | [`kapi-desktop-projects`](../../harness/demos/kapi-desktop-projects/demo.yaml) | `kapi-desktop-projects` | [kapi/desktop/tour](https://neokapi.github.io/next/kapi/desktop/tour), [kapi/desktop/recipes/author-a-project-visually](https://neokapi.github.io/next/kapi/desktop/recipes/author-a-project-visually) | desktop recorder (wbridge, no server) | the app opens on the workspace; project home carries the context digest; the project home draws a conflicts card only when there are conflicts, and the sample has none |
| 6 | [`kapi-desktop-flows`](../../harness/demos/kapi-desktop-flows/demo.yaml) | `kapi-desktop-flows` | [kapi/desktop/tour](https://neokapi.github.io/next/kapi/desktop/tour), [kapi/desktop/recipes/build-a-flow-visually](https://neokapi.github.io/next/kapi/desktop/recipes/build-a-flow-visually) | desktop recorder | earlier Toolbox and home |
| 7 | [`kapi-desktop-config`](../../harness/demos/kapi-desktop-config/demo.yaml) | `kapi-desktop-config` | [kapi/desktop/tour](https://neokapi.github.io/next/kapi/desktop/tour), [kapi/desktop/recipes/store-ai-credentials](https://neokapi.github.io/next/kapi/desktop/recipes/store-ai-credentials) | desktop recorder | earlier settings layout |
| 8 | [`bowrain-cli-getting-started`](../../harness/demos/bowrain-cli-getting-started/demo.yaml) | `bowrain-cli-getting-started` | [the-loop](https://bowrain.cloud/docs/next/the-loop), [walkthroughs/bowrain-getting-started](https://bowrain.cloud/docs/next/walkthroughs/bowrain-getting-started) | stack + `make harness-seed` | `kapi init` proposes the catalog's collection, so the `kapi add` step is gone; status, push and up output |
| 9 | [`bowrain-cli-auth-and-workspaces`](../../harness/demos/bowrain-cli-auth-and-workspaces/demo.yaml) | `bowrain-cli-auth-and-workspaces` | [walkthroughs/bowrain-auth](https://bowrain.cloud/docs/next/walkthroughs/bowrain-auth) | stack + seed | output of the July take |
| 10 | [`bowrain-web-review`](../../harness/demos/bowrain-web-review/demo.yaml) | `bowrain-web-review` | [server/review](https://bowrain.cloud/docs/next/server/review), [server/web-overview](https://bowrain.cloud/docs/next/server/web-overview) | stack + `scripts/seed-collaboration.mjs` (two users) | one human rung, no sign-off controls; an approval of wording someone changed stops at the stale prompt; a correction the project's checks refuse stops at the findings prompt ("Save anyway"); a unit an assistant pre-reviewed shows "AI" and its score on the queue row and the score with its reasons in the checks card |
| 11 | [`bowrain-web-editor`](../../harness/demos/bowrain-web-editor/demo.yaml) | `bowrain-web-editor` | [server/translation-editor](https://bowrain.cloud/docs/next/server/translation-editor), [server/web-overview](https://bowrain.cloud/docs/next/server/web-overview) | stack + seed | status badges; a save over a translation someone changed stops at the stale prompt; the `edit` beat's save and the `memory` beat's Apply pass the project's checks, and one the seeded terms refuse stops at the findings prompt ("The checks found problems in this translation", "Save anyway"); Apply saves the match with its tags |
| 12 | [`bowrain-web-governance`](../../harness/demos/bowrain-web-governance/demo.yaml) | `bowrain-web-governance` | [server/context](https://bowrain.cloud/docs/next/server/context), [server/terminology](https://bowrain.cloud/docs/next/server/terminology), [server/translation-memory](https://bowrain.cloud/docs/next/server/translation-memory), [server/web-overview](https://bowrain.cloud/docs/next/server/web-overview) | stack + seed (`BOWRAIN_TERM_BLOCK_TEXT`, `BOWRAIN_TERM_TEXT`) | July take |
| 13 | [`bowrain-web-collaboration`](../../harness/demos/bowrain-web-collaboration/demo.yaml) | `bowrain-web-collaboration` | [introduction](https://bowrain.cloud/docs/next/introduction), [server/collaboration](https://bowrain.cloud/docs/next/server/collaboration), [server/web-overview](https://bowrain.cloud/docs/next/server/web-overview) | stack + `seed-collaboration.mjs` (two users) | July take; of two people saving one block, the second now meets the stale prompt (the walk saves none) |
| 14 | [`bowrain-web-correction-loop`](../../harness/demos/bowrain-web-correction-loop/demo.yaml) | `bowrain-web-correction-loop` | [server/context-voice](https://bowrain.cloud/docs/next/server/context-voice), [server/web-overview](https://bowrain.cloud/docs/next/server/web-overview) | stack + seed | July take |
| 15 | [`bowrain-desktop-dashboard`](../../harness/demos/bowrain-desktop-dashboard/demo.yaml) | `bowrain-desktop-dashboard` | [server/desktop-app](https://bowrain.cloud/docs/next/server/desktop-app) | stack + seed; the recorder's relay | July take; the reconnect fix (#2596) makes the offline beat recordable; the outbox queues change sets, and an offline edit someone overtook replays as not sent: the indicator reads "1 not sent" and its list shows the edit's wording and the refusal |

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

The server takes a content edit, a review decision, a note or an entity mark
only as a change set on `POST /:ws/projects/:id/streams/:stream/changes`, and
the web app, the Bowrain desktop and the seed scripts
(`harness/scripts/seed-bowrain.ts`, `harness/scripts/seed-collaboration.mjs`)
send their writes there, each save and decision naming the revision it was made
against. A save or a decision on a translation someone changed after the person
opened it stops at a prompt, "This translation changed since you opened it",
that shows the translation as it stands. The walks of rows 10, 11, 13 and 15
make no concurrent edit, so a take that shows the prompt opened a block before
the seed finished writing it. A save also passes the project's checks; one they
refuse stops at a second prompt that lists the findings and offers "Save
anyway". The seeds write with `gate: "report"`, so they never meet it, but the
`edit` and `memory` beats of row 11 and a correction in row 10 do whenever the
wording they save breaks a seeded term rule. Check each take for that prompt,
and choose wording the terms allow rather than recording the override. The desktop's offline beat in row 15 queues change
sets, and the queue drains as before when nobody else touched those blocks.

### 2. Placeholders on the page

Both are first publishes, and both count among the ten that were never
published.

| # | Demo | Publishes as | Placeholder on (`/next/`) | Infrastructure | What changed |
| --- | --- | --- | --- | --- | --- |
| 16 | [`kapi-desktop-review`](../../harness/demos/kapi-desktop-review/demo.yaml) | `kapi-desktop-review` | [kapi/recipes/review-and-approve](https://neokapi.github.io/next/kapi/recipes/review-and-approve), [kapi/recipes/translate-content](https://neokapi.github.io/next/kapi/recipes/translate-content) | desktop recorder | one human rung: Approve, with no sign-off control; the translation sits in the inline-code editor (codes as chips) rather than a text box, and Approve waits until the unit is read through the change service; a **Changes** card follows Provenance; every walk selector still renders |
| 17 | [`bowrain-desktop-automations`](../../harness/demos/bowrain-desktop-automations/demo.yaml) | `bowrain-desktop-automations` | [walkthroughs/bowrain-automation](https://bowrain.cloud/docs/next/walkthroughs/bowrain-automation) | stack + seed (#2597 gives the seed a source ready to run) | narration only: two sentences cut; the run row and the delivery panel's review link still render |

`kapi-desktop-projects` (row 5) also fills the video placeholders on
[kapi/get-started/add-languages](https://neokapi.github.io/next/kapi/get-started/add-languages) and [kapi/get-started/first-project](https://neokapi.github.io/next/kapi/get-started/first-project).
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

| # | Demo | Publishes as | Would fit on (`/next/`) | Infrastructure | What changed |
| --- | --- | --- | --- | --- | --- |
| 18 | [`s0-northsea-context`](../../harness/demos/s0-northsea-context/demo.yaml) | `monolingual-context` | [kapi/context](https://neokapi.github.io/next/kapi/context) | none | nothing in the script; the import reports the word rules it moved into terms, and each context answer ends with the #2989 recording guidance |
| 19 | [`10-cli-points-and-voice`](../../harness/demos/10-cli-points-and-voice/demo.yaml) | `cli-points-and-voice` | [kapi/projects](https://neokapi.github.io/next/kapi/projects) | none | the fixture's voice file sits under `context/`, like the samples', and `setup:` reads it with `kapi context import ./context`; the voice guide is asked for by path (`kapi voice guide partner/index.md`), and holds the portal's register and no terms; the brand coordinate is a `recipe` operation |
| 20 | [`11-cli-terms-and-queue`](../../harness/demos/11-cli-terms-and-queue/demo.yaml) | `cli-terms-and-queue` | [kapi/recipes/terminology-checks](https://neokapi.github.io/next/kapi/recipes/terminology-checks) | none | reads the same `context/` as row 19; the narration counts the four French rows it approves (it said twenty); the `--jq` filter writes one `decide` operation per row, and the narration says the queue names what a decision addresses; the French queue's closing line names the `decide` operation |
| 21 | [`audience-constraints`](../../harness/demos/audience-constraints/demo.yaml) | `audience-constraints` | [kapi/recipes/content-governance-for-ai](https://neokapi.github.io/next/kapi/recipes/content-governance-for-ai) | none | rewritten for the screen: text output in place of several hundred lines of JSON, `ksed` edits in place of `node -e`, and one `--jq` line for the findings' provenance; the sample's `emotion: reassuring` became `warm`, which removes a configuration warning. `ksed` now refuses the planted phrase at the commit check (`gate_failed`, exit 2), so the script shows that refusal, plants the phrase with `perl`, and the `violation` narration says both |
| 22 | [`s1-compass-converge`](../../harness/demos/s1-compass-converge/demo.yaml) | `multilingual-converge` | [kapi/convergence](https://neokapi.github.io/next/kapi/convergence) | none | the `ship.json` narration names Dutch's `not_governed` note |
| 23 | [`s1-compass-ship-gate`](../../harness/demos/s1-compass-ship-gate/demo.yaml) | `multilingual-ship-states` | [kapi/recipes/ship-gates-and-ci](https://neokapi.github.io/next/kapi/recipes/ship-gates-and-ci) | none | the first highlight lands on `parked` and `withheld`; nothing on screen said `blocked`; the two `--jq` filters write `decide` operations; the second `kapi up`'s plan line prices only the languages its pass works on, so the languages already shippable add no units to it |
| 24 | [`s2-tidewatch-build-output`](../../harness/demos/s2-tidewatch-build-output/demo.yaml) | `docs-site-convergence` | [kapi/recipes/translate-content](https://neokapi.github.io/next/kapi/recipes/translate-content) | none | nothing in the script |
| 25 | [`s2-tidewatch-ci`](../../harness/demos/s2-tidewatch-ci/demo.yaml) | `docs-coverage-in-ci` | [kapi/convergence-in-ci](https://neokapi.github.io/next/kapi/convergence-in-ci) | none | the finding that reports is on the integration page, which names the retired word where it explains `mooring_id`; the comment and narration said the API reference |

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
| [`08-mcp-tools`](../../harness/demos/08-mcp-tools/demo.yaml) | (not published) | Claude session, `kapi mcp --tools all` | the assistant records a pre-review; the queue keeps its length and shows the score; the queue narration and artifact subtitle address a row by its document, block and language. Driven by hand over stdio on 2026-10-06: `review_queue`, `review_block` and an `apply_edits` `decide` with outcome `advise` behave as the narration says, and the same call with `establish` is refused `not_permitted` |
| [`kapi-desktop-content`](../../harness/demos/kapi-desktop-content/demo.yaml) | `kapi-desktop-content` | desktop recorder | published in July, not embedded |
| [`kapi-desktop-explorer`](../../harness/demos/kapi-desktop-explorer/demo.yaml) | `kapi-desktop-explorer` | desktop recorder | the Context hub rail now lists Learned and Agent View above Terms and Content Memory |
| [`bowrain-sizzle`](../../harness/demos/bowrain-sizzle/demo.yaml) | `bowrain-sizzle` | renders from the web and desktop clips | render after rows 10 to 15 |
| [`02-nextjs-zero-to-i18n`](../../harness/demos/02-nextjs-zero-to-i18n/demo.yaml), [`03-translate-docx`](../../harness/demos/03-translate-docx/demo.yaml) | `claude-app-i18n`, `claude-translate-document` | Claude session (billed) | published in July, not embedded |
| [`01-translate-landing-page`](../../harness/demos/01-translate-landing-page/demo.yaml), [`04-i18n-react-catalogs`](../../harness/demos/04-i18n-react-catalogs/demo.yaml), [`06-multi-format-publishing`](../../harness/demos/06-multi-format-publishing/demo.yaml), [`07-global-launch-many-languages`](../../harness/demos/07-global-launch-many-languages/demo.yaml) | (not published) | Claude session (billed); AI provider for 01-07 | `04` and `07` render `kapi check --json` and `kapi status --json` artifacts from the take's sandbox, so `--only=artifacts,render` on the machine that holds the take refreshes them without a new session; a term finding with a replacement now carries `fix` in the check artifact |

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
| `s0-northsea-checks` | first check: 1 FAILS (`seamless`) and 4 REPORTS, exit 3; `findings.json` artifact shows `"schema": "kapi.check/v2"` and a `fix` on each of the five findings; after the `dock` decision 1 FAILS; the last check passes with 2 REPORTS |
| `05-ai-checks-guardrail` | 3 FAILS, exit 3; then `No findings.` and PASS |
| `kapi-bilingual-workflow` | the XLIFF excerpt shows the first unit's `<mda:metaGroup category="kapi">` with `if-match` and `basis`; the merge table reads applied for both files with `stale` and `skipped` at 0 and no `refused`; `fr-FR/messages.json` holds every key in the source's order |
| `s0-northsea-context` | the import reports 7 concepts, 1 voice profile and 8 word rules moved into terms; the two answers name `northsea/docs` and `northsea/landing`; the search lists `berth` preferred, `mooring` discouraged and `mooring_id` admitted |
| `10-cli-points-and-voice` | `ls` lists `context`; the two answers name `voltway/docs` and `voltway/portal` with `brand=helios`; the voice guide shows the portal's medium sentences and no Terms section; one FAILS on `partner/index.md` (`seamless`), exit 3, then PASS; the recipe artifact's comments say `advisory`, not severity |
| `11-cli-terms-and-queue` | the prompt excerpt lists `billing period` and `reading`; `kapi status` reads fr `blocked: checks` with one failing unit, because `billing.export` says "période comptable" and the recipe's `billing period` rule holds the gate; four French rows are approved; French goes from 0% to 100% established; the queue for French ends with the line naming the `decide` operation, then empties; the source rows stay |
| `audience-constraints` | both answers list the same two shared constraints; the first check passes with `requested and not run: voice.guidance`; the `ksed` attempt prints one `gate_failed` line per page, exits 2 and writes nothing; the `perl` plant gives 4 FAILS and exit 3; the jq line prints `harbor-help/no-unsupported-assurance v1` for each page; the restored pages pass; the contradicting sentence passes with `voice.guidance` still not run; no "Configuration warnings" block |
| `s1-compass-converge` | the first `ship.json` withholds all three; `kapi up` answers 37 of Norwegian's 38 units from content memory; afterwards `nb` and `de` are `translated` and `nl` is `blocked: checks` |
| `s1-compass-ship-gate` | `kapi up` parks `nl`; `ship.json`: `nl` withheld, then `translated`; twenty Dutch approvals; `nb` ends `established` |
| `s2-tidewatch-build-output` | `nb` ships `translated`, `nl` is `blocked: review`; the Norwegian front matter keeps the stub's marks on the description; `find` lists four Norwegian files and no Dutch ones |
| `s2-tidewatch-ci` | the jq line prints `established 77%` for nb and `established 0%` for nl; the check passes with one REPORTS, on `docs/integrating.md` (`mooring`) |
| `bowrain-desktop-automations` | Run now starts a run that appears at the top of the table; the row settles with a per-language summary; the dashboard's "Review pending translations" link opens the review session |
| `bowrain-cli-getting-started` | `kapi init` prints `collections (proposed from the files here...)` with `src/locales/en.json`; `kapi status` shows the `content` and `governance` lines; `kapi up` prints the venue first; `src/locales/fr.json` arrives |
| `08-mcp-tools` | the session reads the unit with `review_block` and sends `apply_edits` a `decide` with outcome `advise`, not an approval; the queue artifact still lists the unit, now with `aiScore` |
| `kapi-desktop-review` | the unit shows one human action (Approve) and no sign-off control; the history layer reads "Already approved"; the translation shows its codes as chips in the editor, and the `a` keystroke lands while the editor does not hold focus; the Changes card reads "No change recorded" for an untouched unit; the batch approval clears every clean unit with no "Batch approval stopped" notice, since nothing changes them during the take |
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

- The two Bowrain CLI demos (rows 8 and 9) were not dry-run: they push to and
  pull from a server, and no stack was up. Their flags were checked against
  `kapi init`, `kapi up` and the plugin's help. Run their capture stage once
  against the local stack and read the take before narrating them, since
  `kapi push`, `kapi status` and `kapi up` print what the server answers.
- The desktop and web walks (rows 5 to 7, 10 to 17, and the fourth group's
  desktop demos) were checked statically: every `data-slot` and `data-testid`
  the recorder and the scripts name, and every label it clicks by text, is
  still in the frontend source. Only the recorder shows whether each beat lands
  where the narration says, so read the recorder log for an inert crop or a
  selector that matched nothing.
- No walkthrough shows the context digest or the Learned section on its own;
  `kapi-desktop-explorer` passes the Learned rail entry on its way to Terms. A
  beat for it needs a sample with something learned to show.
- No walkthrough shows a conflict being decided in Kapi Desktop. A beat for it
  needs a sample whose workspace holds one: two machines' logs that each edited
  one parked draft's block, merged, or a parked draft a person edited whose
  translation's file was written since without it. The second is the easier
  fixture to seed. Neither the `kapi-kpz-workspace` guided embed nor any other
  walkthrough edits a source inside a `.kpz`; its narration is unchanged.
- The Claude demos in the fourth group spend a billed session, so none was
  captured for this pass. `08-mcp-tools` had its tool calls driven by hand
  instead (see its row); the others were checked by reading their prompts and
  narration against the commands they name.
