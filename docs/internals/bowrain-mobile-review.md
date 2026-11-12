# Bowrain mobile review: design note

Status: proposal for founder review. Nothing here is built.

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
in the minutes a reviewer has between other things. This note proposes a
Bowrain mobile app built around one card per block, swiped to decide.

## Scope

- **In scope:** translation review, the blocks a pending-review listing
  returns, for people with the review permission for a language.
- **Later:** the entity queue (`/review-queue`, `entity_review` items: term
  and entity suggestions) suits the same card. It needs a card of its own.
- **Out of scope:** editing a translation beyond a one-line correction,
  long prose (routed to the desk), project settings, source changes.

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

## Gestures

| Action | Gesture | Button | Result |
| --- | --- | --- | --- |
| Approve | swipe right | Approve | a `decide` operation with outcome `establish` |
| Not approved | swipe left | Not approved | a reason sheet, then `decide` with outcome `reject` and the chosen reasons in its `reasons` |
| Skip | swipe up | Later | nothing is recorded; the card returns at the end of the deck |
| Undo | shake or the undo bar | Undo | the last decision is withdrawn while it is still queued, or reversed with a new decision once sent |

Every gesture has a button, for discoverability and for accessibility
(VoiceOver and TalkBack users act on the buttons). Haptics confirm approve and
reject. A reject always asks for a reason: one tap on a chip (wrong term,
meaning, tone, placeholder, grammar) or a short typed or dictated note. A bare
reject leaves the next draft no better than this one.

A one-line correction ("approve with this change") is a pencil button that
opens the translation for editing. Anything longer sends the card to the desk.

## The deck

The server orders the deck and returns it with a reason per card:

1. **Fast deck:** blocks with no check findings and a high pre-review score.
   These are the cards a swipe is right for.
2. **Look closer:** blocks with a low score, or a term or voice note.
3. **Needs the desk:** blocks with findings, stale blocks, rejected blocks
   with a new draft, and sources over a length threshold. The app lists them
   with "open on desktop" and does not offer the swipe.

`kapi up --pre-review` and the server's pre-review already record the scores
the first split needs. The ordering is a new server endpoint (below), shared
with Bowrain web.

Before anyone swipes through thousands of cards, the backlog itself needs a
policy decision. Approving every passing, high-scoring draft in one action is
what `approve-passing` already does. People then review what is left, and a
sample of what was approved automatically.

## Guarding against careless approval

- **Pace:** the server records decision timestamps. Approvals faster than a
  card can be read (a threshold per source length) are not refused, but the
  reviewer sees "you are approving faster than you can read" and the session
  report shows the rate.
- **Known-bad cards:** the deck occasionally includes a block whose correct
  answer is known (a deliberately wrong translation from a seeded set). A
  reviewer who approves it is told so on the spot. The result counts in the
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
revisions, sent through the change contract (`POST /:ws/projects/:id/streams/:stream/changes`). If the
translation or the source moved after the card was loaded, the server answers
`409 stale` and the approval does not land. The app tells the reviewer and
offers the new version as a fresh card.

Decisions made offline wait in a local queue (SQLite) and are sent in order
when the phone reconnects. Each is its own change set, so one stale card does
not hold back the rest. `bulk-review` is not used for this: it decides on the
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

## Sign-in and permissions

- OIDC with PKCE against the workspace's identity provider (Cognito in
  production, Keycloak locally), through `expo-auth-session`.
- The refresh token is held in the platform keychain (`expo-secure-store`).
- The review permission is language-scoped (`PermReview`); the app shows only
  the languages a person may approve, and the server checks again on every
  decision.

## What the server needs

1. **A deck endpoint:** the pending-review entries for a reviewer and
   language, ordered into the three groups above, each with the reason it is
   in its group and the revisions the card shows. Bowrain web's focused
   reviewer uses it too.
2. **The source revision on each pending-review entry,** beside
   `target_revisions`, so a decision can bind both halves.
3. **Device registration:** `POST`/`DELETE` a push token per device, with the
   person's digest period and quiet hours.
4. **A notification job** in the worker: digests and event pushes through
   APNs and FCM (or Expo's push service, which fronts both).
5. **Known-bad cards and pace:** a seeded set per language, and the session
   report, both server-side.
6. **Reject reasons that reach the next draft.** The `decide` operation
   already carries `reasons`; the server keeps them with the decision, and the
   drafting step reads them as guidance when it drafts the block again.

## The app

- **Stack:** Expo (React Native) with `expo-router`,
  `react-native-gesture-handler` and `react-native-reanimated` for the card,
  `expo-notifications`, `expo-secure-store`, `expo-sqlite`, and NativeWind so
  the colour and type tokens are shared with the web apps.
- **Shared code:** the API client and the TypeScript types come from the
  packages Bowrain web already uses. The card itself is new native UI; the
  web's DOM components are not reused.
- **Monorepo:** the app joins the pnpm workspace. Metro has to resolve the
  workspace's symlinked packages; current Expo releases support this with a
  short `metro.config.js`. This needs a spike against the Vite+ setup before
  the app is committed to.
- **Text:** the app's UI strings go through the same loop as every other
  Bowrain app, with an `i18n/` directory the dogfood recipe reads.
- **Analytics:** the same PostHog events Bowrain web sends for a decision
  (`review_approved`, `review_rejected`, batch size and locale), never
  content.

## Licence and placement

`bowrain/` is AGPL-3.0, and GPL-family licences conflict with App Store
terms. The app is a client of the server, like the `kapi-bowrain` plugin, so
it takes the same placement: its own directory, `bowrain/mobile/`, under its
own Apache-2.0 `LICENSE`, linking nothing from the AGPL tree.
`scripts/check-ts-license-boundary.sh` already refuses a reach into an AGPL
file by path, package name, tsconfig alias or dependency, and covers the new
directory once it has its `LICENSE`. Shared types the app needs move to an
Apache package under `packages/`.

## Release

- EAS Build and EAS Submit, run from CI beside `release-bowrain.yml`, on the
  `bowrain-v*` tags.
- EAS Update for JavaScript-only fixes between store releases.
- An Apple Developer team listing and a Google Play developer account, both in
  the founder's name. Store listings, privacy labels and review notes are
  founder steps, as Windows signing is.

## Web fallback

A person who reviews once, such as an external translator sent a link, should
not have to install anything. The link in an invitation or digest email opens
Bowrain web's focused reviewer, which reads the same deck endpoint. The app is
for people who review regularly.

## Testing

End-to-end tests run against the real local stack (`bowrain/compose.yaml`:
Keycloak, the server, PostgreSQL), with Maestro flows driving a simulator.
The server endpoints get Go tests beside the existing review handler tests.
External push services are stubbed at the provider boundary.

## Phases

1. **Server:** the deck endpoint, the source revision on entries, reject
   reasons kept and read by the next draft. Bowrain web's focused reviewer moves to the deck endpoint.
2. **App, read and decide:** sign-in, language list, deck, card, gestures,
   offline queue, TestFlight and internal Play track.
3. **Notifications:** device registration, the worker job, digests and
   assignment pushes.
4. **Safeguards:** known-bad cards, pace, session reports.
5. **Store release** and the entity-review card.

## Open decisions

1. The backlog policy before the first swipe: run `approve-passing` over
   passing, high-scoring drafts, then sample what it approved?
2. Expo's push service, or APNs and FCM directly? Expo's is simpler; direct
   keeps the push path inside Bowrain's own infrastructure.
3. The pace threshold and the share of known-bad cards in a deck.
4. Whether "approve with a one-line change" belongs in the first version.
5. The app's name in the stores.
