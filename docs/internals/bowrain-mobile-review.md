# Bowrain mobile, starting with review: design note

Status: approved by the founder on 2026-11-12, including the decisions at the
end. Nothing here is built yet. Versions and store rules were checked on
2026-11-12; items marked *unverified* were not
confirmed against a primary source.

A mock of four screens (language list, a fast-deck card, the reason sheet, a
stale card routed to the desk) is a private canvas shared with the founder;
it is not part of this repository.

## The problem

On the dogfood project, 3,815 nb blocks wait for review (`kapi status
--review`, 2026-11-12). Every one is an AI draft with status `translated`, and
none is approved. Their source text is short: the median is 52 characters and
none exceeds 80, because they are UI strings, CLI help, reference text and
narration. 57 carry check findings; the other 3,758 have none.

The recipe declares no ship gate, so nb ships unreviewed and the queue blocks
nothing. A gate such as `established: 100` for nb would hold the language back
behind this backlog. Either way approval coverage stays at zero, because every
review surface today assumes a desk: the Kapi Desktop Review page, Bowrain
web's review session, and on the CLI `kapi status --review` with a `decide`
operation sent to `kapi apply`.

Short, clean strings take a second or two to judge. A phone suits that work,
in the minutes a reviewer has between other things. This note proposes the
Bowrain mobile app, named Bowrain in both stores. Review is its first
feature: one card per block, swiped to decide. The app's structure (sign-in,
workspaces and projects, notifications, the offline queue) is built for the
features that follow review, and none of it is specific to review.

## Scope

- **In scope:** translation review, the blocks a pending-review listing
  returns, for people with the review permission for a language.
- **Later:** the entity queue (`/review-queue`, `entity_review` items: term
  and entity suggestions) suits the same card. It needs a card of its own.
- **Out of scope for the first version:** long prose (routed to the desk),
  project settings, source changes, notifications (phase 3), lot sampling,
  known-bad cards and pace (phases 2 and 4).

The app is a client. Every rule it applies (what counts as a decision, who
may decide, which cards come first, which cards go to the desk) is computed
by the server, so Bowrain web's focused reviewer and the app answer the same
way. Where a rule does not exist on the server yet, this note adds it there
first. This follows the standing rule that a surface never carries product
behaviour of its own.

## The card

One block per card:

- the source text and the translation, with placeholders and inline codes
  drawn as chips, never flattened to text;
- the key, the file and the collection;
- a screenshot or "where it is used" when the review context has one;
- for a stale block, the earlier approved translation beside the new draft,
  with the changed words marked;
- the AI pre-review score and the check findings, when there are any;
- the term and voice notes the review context returns for the block.

A card with no context says so. A reviewer who cannot see where a string
appears is guessing, and the card should make that plain.

## Gestures and buttons

| Action | Gesture | Button | Result |
| --- | --- | --- | --- |
| Fix | swipe left | Fix | the fix sheet (below): pick a ready-made fix or edit, and the corrected translation is approved in one change set |
| Skip | swipe up | Later | nothing is recorded; the card returns at the end of the deck |
| Approve | swipe right | Approve | a `decide` operation with outcome `establish` |
| Undo | the undo bar | Undo | the last decision is withdrawn while it is still queued, or reversed with a new decision once sent |

The buttons follow the rule every review surface uses: the negative or
corrective action first, Approve last at the trailing edge, with space
between them. Kapi Desktop, Bowrain web
and the app place them the same way, and the swipe direction matches: right
approves.

Every gesture has a button, for discoverability and for accessibility
(VoiceOver and TalkBack users act on the buttons). Haptics confirm approve and
fix.

### The fix sheet

A rejected block goes back for a new draft, waits in the queue and comes back
for review: two more round trips, often days, to change what is usually one or
two words. A correction settles the block at once, and the corrected wording
enters the content memory, so every later draft of the same text reuses it.
The fix sheet is therefore the main path for a translation that is not right.

1. **Why:** one tap on a reason chip (wrong term, meaning, tone, placeholder,
   grammar). The reason filters and orders the fixes below.
2. **Ready-made fixes,** each a diff against the draft, each one tap to apply:
   the term fix (the draft with a term rule's replacement applied), the
   content-memory alternatives the review context finds, and an AI rewrite
   that addresses the chosen reason. The server computes them (below), so
   Bowrain web and Kapi Desktop offer the same fixes.
3. **Edit:** a text field where placeholders and inline codes are locked
   chips that cannot be broken, with the diff shown before confirming.
4. **Reject with the reason:** when no fix fits. The decision carries the
   reason and an optional note, and the next draft reads them.

A fix is one change set: `set_content` with the corrected runs, then `decide`
with outcome `establish`, both with `if_match` on the revisions the card
showed. The server validates the content: a fix that breaks a placeholder is
refused as a guard failure, and the sheet shows why. Under the workspace's
separation-of-duties policy a reviewer approving their own correction is
self-approval: with `off` or `warn` the block settles at once; with `block`
the correction is kept and the block waits for a second reviewer, which is
still faster than a redraft. Corrections are sampled in quality review like
any other decision.

## The deck

The server orders the deck and returns it with a reason per card:

1. **Fast deck:** blocks with no check findings and a high pre-review score.
   These are the cards a swipe is right for.
2. **Look closer:** blocks with a low score, or a term or voice note.
3. **Needs the desk:** blocks with findings, stale blocks, rejected blocks
   with a new draft, and sources over a length threshold. The app lists them
   with "Open on desktop" and does not offer the swipe.

`kapi up --pre-review` and the server's pre-review already record the scores
the first split needs. The ordering is a new server endpoint (below), shared
with Bowrain web.

### The backlog before the first swipe

Swiping through 3,758 clean cards one by one is the wrong first step. The
recommended policy treats the backlog as lots and samples them, the way
acceptance sampling (ISO 2859-1, by acceptable quality level; ISO 2859-5 for
sequential plans) treats a production lot:

1. Group the clean, high-scoring drafts into lots: one collection and
   language each.
2. Draw a random sample from each lot into the fast deck, and review the
   sample.
3. If the sample's reject rate stays under the threshold, approve the rest of
   the lot with `approve-passing`. If it goes over, the whole lot goes back to
   review card by card.
4. Never approve a block with a failing check finding without a person.

The sample size and the threshold are project settings with defaults. No
published source applies acceptance sampling to translation review
specifically, so the defaults are a starting point to measure, not a
standard.

## Guarding against careless approval

- **Pace:** adults read silent non-fiction at about 238 words a minute, with
  a typical range of 175 to 300 (Brysbaert, 2019). A review card asks the
  reader to read the source and the translation. A decision that comes faster
  than about 0.25 seconds per word over both texts is too fast to have read
  them. The app does not refuse it: the reviewer sees "you are deciding faster
  than you can read", and the session report shows the rate.
- **Known-bad cards:** about 3 to 5 percent of a deck are cards whose correct
  answer is known (a deliberately wrong translation from a seeded set), spread
  evenly through the deck. The annotation literature gives no fixed rate; one
  heuristic sizes the gold set at about the square root of the items
  labelled. The pool rotates, because reused known items get recognised. A
  reviewer who approves one is told so on the spot. The result counts in the
  reviewer's accuracy, never in the content.
- **Separation of duties:** the workspace's existing policy applies; a person
  cannot approve a translation they wrote when the policy forbids it. The app
  hides the swipe on such cards.
- **Per-reviewer report:** approvals, rejections, reasons and pace per
  session, visible to the reviewer and to whoever manages the language.

## Decisions offline, and text that changes

A card carries the revisions it showed: the translation's revision
(`target_revisions` in the pending-review entry) and the source's. A swipe
becomes a `decide` operation whose `if_match` maps both editions to those
revisions, sent through the change contract
(`POST /:ws/projects/:id/streams/:stream/changes`). If the translation or the
source moved after the card was loaded, the server answers `409 stale` and the
approval does not land. The app tells the reviewer and offers the new version
as a fresh card.

Decisions made offline are paused mutations in TanStack Query, persisted on
the device, and sent in order when the phone reconnects or the app returns to
the foreground. Background sync on iOS is opportunistic, so the app does not
rely on it. Each decision is its own change set, so one stale card does not
hold back the rest. `bulk-review` is not used for this: it decides on the
translation the server reads at request time, so a late sync could approve
text the reviewer never saw.

## Notifications

- A digest per reviewer and language: "42 nb blocks are waiting", at most
  once per period the reviewer picks, never between their quiet hours.
- An event push when a block is assigned to the reviewer, or a gate is held
  back only by review.
- No content in a push payload: the count and the language only. The text is
  fetched after the person opens the app and signs in.
- Each notification links to the deck it describes.
- Sent through the Expo push service (below), which fronts APNs and FCM.

The push service decision: Expo's service is free, has one API for both
platforms, allows 600 notifications a second per project, and deletes a
payload once Apple or Google has accepted it. Expo staff may see payloads
while debugging, which is acceptable because payloads carry counts only.
Direct APNs and FCM become worth their extra work only if that rate limit or
a customer's compliance requirement applies; the worker job is written
against a small sender interface so the switch stays local.

## Sign-in and permissions

- OIDC authorization code flow with PKCE through `expo-auth-session`
  (`exchangeCodeAsync`, `refreshAsync`) against the workspace's identity
  provider: Cognito's hosted UI in production, Keycloak locally (which needs
  the `offline_access` scope for a refresh token). AWS Amplify is not needed.
- The refresh token is held in the platform keychain (`expo-secure-store`).
- The review permission is language-scoped (`PermReview`); the app shows only
  the languages a person may approve, and the server checks again on every
  decision.
- Apple requires apps with account creation to offer account deletion in the
  app (guideline 5.1.1(v)). The app only signs in to accounts created on the
  web, so a link to the account page may satisfy it (*unverified*; confirm
  with App Review before submission).

## What the server needs

1. **A deck endpoint:** the pending-review entries for a reviewer and
   language, ordered into the three groups above, each with the reason it is
   in its group and the revisions the card shows. Bowrain web's focused
   reviewer uses it too.
2. **The source revision on each pending-review entry,** beside
   `target_revisions`, so a decision can bind both halves.
3. **Lot sampling** for the backlog policy: lots, samples, the threshold, and
   `approve-passing` over a lot that passed.
4. **Fix candidates:** for one block and language, the term fix, the
   content-memory alternatives and, on request, an AI rewrite for a reason,
   each as runs with a diff against the draft. The computation lives in host,
   so Kapi Desktop and the CLI get the same candidates; the server exposes it.
5. **Device registration:** `POST`/`DELETE` a push token per device, with the
   person's digest period and quiet hours.
6. **A notification job** in the worker: digests and event pushes through the
   Expo push service, behind a sender interface.
7. **Known-bad cards and pace:** a seeded, rotating set per language, and the
   session report, both server-side.
8. **Reject reasons that reach the next draft.** The `decide` operation
   already carries `reasons`; the server keeps them with the decision, and the
   drafting step reads them as guidance when it drafts the block again.

## Stack (as of 2026-11)

| Part | Choice | Version | Why |
| --- | --- | --- | --- |
| Framework | Expo, development builds (not Expo Go) | SDK 57 (React Native 0.86, React 19.2); SDK 58 (React Native 0.88, released 2026-10-15) once its libraries settle | `create-expo-app` still starts on SDK 57; Expo Go is not for production work |
| Engine | Hermes V1, New Architecture | default since React Native 0.84; the legacy architecture is gone since SDK 55 | nothing to choose |
| Navigation | `expo-router` | follows the SDK | in the default template |
| Card and gestures | `react-native-gesture-handler` and Reanimated 4 with `react-native-worklets` | Reanimated 4.3 to 4.5 | the card stack is about 150 lines; no third-party swiper |
| Haptics | `expo-haptics` | follows the SDK | on swipe commit |
| Styling | Uniwind | current | Tailwind v4, the version Bowrain web uses, so the CSS tokens are shared; NativeWind v4 is on Tailwind v3 and v5 is still a release candidate |
| Components | React Native Reusables | current | shadcn-style primitives on Uniwind |
| Server state and offline | TanStack Query v5 with a persister and paused mutations | v5 | the offline queue is built in (`setMutationDefaults` lets paused mutations resume after a restart) |
| Local storage | `expo-sqlite` (with its key-value store) or MMKV | *unverified* | either holds the persisted query cache |
| Auth | `expo-auth-session` and `expo-secure-store` | follows the SDK | plain OIDC covers Cognito and Keycloak |
| Push | `expo-notifications` and the Expo push service | follows the SDK | see Notifications |
| Text | `react-i18next`, with the device language from Expo's locale module | current | JSON catalogs, which the kapi loop already writes |
| End-to-end tests | Maestro | current; the EAS Workflows Maestro job is in alpha | simpler than Detox |
| Unit tests | Jest with `jest-expo` | *unverified* | the template's default |
| Build and release | EAS Build, Submit, Update and Workflows | Starter plan ($19 a month); the free tier allows 15 iOS and 15 Android builds a month (*unverified*: from a third-party summary) | the fastest route to TestFlight |
| AI tooling | the Expo plugin for Claude Code | `claude plugin install expo@claude-plugins-official` | installs Expo's skills and its MCP server |

Universal web and native code (Expo web, react-strict-dom) is not used:
Bowrain web already exists, and the app shares types and the API client, not
screens.

## Building it with AI agents

- **Install the Expo plugin for Claude Code.** It brings Expo's agent skills
  and the Expo MCP server (`mcp.expo.dev`), which answers from the current
  SDK's documentation instead of the model's memory. SDK 58 adds an
  experimental `@expo/agent-cli`.
- **Start from `create-expo-app`'s default template** and keep the project in
  Continuous Native Generation: the `ios/` and `android/` folders are
  generated by prebuild and never committed or edited. Native settings live in
  `app.config.ts` and config plugins.
- **Use development builds from the start**, not Expo Go. A native module
  added later needs a new development build; agents often forget this and
  debug a stale binary.
- **Pitfalls agents hit:** editing generated native folders instead of
  writing a config plugin; testing in Expo Go; adding a native dependency
  without rebuilding; following blog posts for older Reanimated (its Babel
  plugin is now `react-native-worklets/plugin`).
- **A `bowrain/mobile/CLAUDE.md`** states the above, the licence boundary and
  the rule that the app holds no product behaviour, so every session starts
  from it.

## The app in this repository

- **Placement:** `bowrain/mobile/`, a pnpm workspace package.
- **Monorepo:** pnpm's isolated layout is supported since SDK 54, and
  `expo/metro-config` detects the workspace. If a library fails under it, the
  documented fallback is `nodeLinker: hoisted`, which is workspace-wide and
  would change the Vite apps too. The first spike checks that the isolated
  layout works with Vite+ in the same workspace.
- **Shared code:** the API client and the TypeScript types move to an Apache
  package under `packages/`, which both Bowrain web and the app import. The
  card itself is native UI; the web's DOM components are not reused.
- **Text:** the app's UI strings go through the same loop as every other
  Bowrain app: an `i18n/` directory the dogfood recipe reads, catalogs bundled
  at build time, and catalog updates shipped by EAS Update between store
  releases.
- **Analytics:** the same PostHog events Bowrain web sends for a decision
  (`review_approved`, `review_rejected`, batch size and locale), never
  content.

## Licence

`bowrain/` is AGPL-3.0. GPL-family licences conflict with App Store terms
(the 2011 removal of VLC from the App Store is the usual precedent), and no
current statement from Apple or the FSF changes that. Apache-2.0 raises no
such conflict, and Expo and its skills are MIT. The app is a client of the
server, like the `kapi-bowrain` plugin, so it takes the same placement: its
own directory under its own Apache-2.0 `LICENSE`, linking nothing from the
AGPL tree, `@neokapi/ui` included. `scripts/check-ts-license-boundary.sh`
refuses a reach into an AGPL file by path, package name, tsconfig alias or
dependency, and covers the new directory once it has its `LICENSE`.

## Release

- EAS Build and EAS Submit, run from EAS Workflows or CI beside
  `release-bowrain.yml`, on the `bowrain-v*` tags.
- EAS Update for JavaScript-only fixes between store releases. Apple allows
  over-the-air updates that do not change the app's primary purpose.
- Store requirements in 2026: Google Play requires target API level 36 for
  new apps and updates since 2026-08-31; Apple requires privacy manifests for
  the SDKs an app uses and the account-deletion rule above.
- Review times: Apple reports 90 percent of submissions reviewed within 24
  hours. TestFlight internal testing needs no review; external TestFlight
  builds go through a beta review.
- An Apple Developer account, a Google Play developer account and an Expo
  organization, all in the founder's name. Apple enrolment can take several
  days. Store listings, privacy labels and review notes are founder steps, as
  Windows signing is.

## The fastest path to a first build

About one to two weeks of work after the accounts exist (an estimate, not a
measurement):

1. The founder opens the Apple Developer, Google Play and Expo accounts.
2. `create-expo-app` in `bowrain/mobile/`, the Expo plugin, `eas init`, the
   monorepo spike.
3. Sign-in against the local Keycloak, then Cognito.
4. The deck and the card against the existing pending-review API, decisions
   through the change contract.
5. `eas build --profile development` for daily work.
6. `eas build -p ios` and `eas submit` to TestFlight internal testing, and an
   upload to the Play internal track.

The server's deck endpoint and the other additions follow; the first build
reads the pending-review listing as it is.

## Web fallback

A person who reviews once, such as an external translator sent a link, should
not have to install anything. The link in an invitation or digest email opens
Bowrain web's focused reviewer, which reads the same deck endpoint. The app is
for people who review regularly.

## Testing

End-to-end tests run against the real local stack (`bowrain/compose.yaml`:
Keycloak, the server, PostgreSQL), with Maestro flows driving a simulator.
The server endpoints get Go tests beside the existing review handler tests.
The push service is stubbed at the sender interface.

## Phases

1. **First version:** on the server, the deck endpoint (with source and
   target revisions) and fix candidates from host; in the app, sign-in,
   language list, deck, card, gestures, the fix sheet, decisions and fixes
   through the change contract, the offline queue; then TestFlight and the
   Play internal track.
2. **Server:** lot sampling, reject reasons read by the next draft. Bowrain
   web's focused reviewer moves to the deck endpoint and gains the fix sheet.
3. **Notifications:** device registration, the worker job, digests and
   assignment pushes.
4. **Safeguards:** known-bad cards, pace, session reports.
5. **Store release** of Bowrain with review, then the entity-review card and
   the next features.

## Decisions

1. **The backlog:** lot sampling, as in "The backlog before the first swipe":
   sample each lot of clean, high-scoring drafts, approve the lot with
   `approve-passing` when the sample passes, send it back to card-by-card
   review when it does not, and never approve a block with a failing finding
   without a person.
2. **Push:** the Expo push service, with counts-only payloads and the worker
   written against a sender interface; direct APNs and FCM only if the rate
   limit or a compliance requirement applies.
3. **Pace and known-bad cards:** warn below about 0.25 seconds per word over
   the source and the translation; 3 to 5 percent known-bad cards, spread
   evenly, from a rotating pool.
4. **Corrections:** in the first version, as the fix sheet: ready-made fixes
   first, editing second, reject with a reason last. A correction settles the
   block and teaches the content memory; a reject only asks for another
   draft. (The founder reversed the earlier "not in the first version" on
   2026-11-12.)
5. **The app's name:** decided: Bowrain. Review is the first feature, so
   the store subtitle and screenshots lead with it. Before submission, check
   that the name is free in both stores and that the trademark position
   (USPTO, EUIPO) covers the app category.

## Sources

- Expo SDK 57 and 58 changelogs: <https://expo.dev/changelog/sdk-57>,
  <https://expo.dev/changelog/sdk-58>
- React Native 0.84 (Hermes V1):
  <https://reactnative.dev/blog/2026/02/11/react-native-0.84>
- Reanimated 4 migration:
  <https://docs.swmansion.com/react-native-reanimated/docs/guides/migration-from-3.x/>
- NativeWind v5: <https://www.nativewind.dev/v5>; Uniwind:
  <https://uniwind.dev/>
- TanStack Query persistence:
  <https://tanstack.com/query/v5/docs/framework/react/plugins/persistQueryClient>
- Expo authentication guide: <https://docs.expo.dev/guides/authentication/>
- Expo push FAQ and security: <https://docs.expo.dev/push-notifications/faq/>,
  <https://expo.dev/security>
- Expo monorepos: <https://docs.expo.dev/guides/monorepos/>
- Expo end-to-end tests: <https://docs.expo.dev/tutorial/cicd/e2e-tests.md>
- Expo and Claude Code: <https://docs.expo.dev/agents/claude/>
- Over-the-air updates:
  <https://expo.dev/articles/updates-and-rollouts/over-the-air-updates>
- Google Play target API:
  <https://support.google.com/googleplay/android-developer/answer/11926878>
- Apple account deletion:
  <https://developer.apple.com/help/app-review/guideline-reference/5-1-1-account-deletion>
- Reading rate: Brysbaert (2019), "How many words do we read per minute?"
- Gold-standard items in annotation: <https://arxiv.org/pdf/2105.09457>,
  <https://arxiv.org/pdf/1801.02546>
