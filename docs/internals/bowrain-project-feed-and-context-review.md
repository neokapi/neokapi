# Bowrain project feed and context review: design note

Status: proposal for the founder's review, 2026-11-14. Nothing here is built.
Every route, type and field named below was checked against `main` at
`a8b8e7328`. The dogfood figures come from `kapi context log` and
`kapi terms stats` on the local project; the server's copy of the same
context was not measured.

Two features for Bowrain web and the Bowrain mobile app. The server holds the
rules and both clients render the same API, as
[bowrain-mobile-review.md](bowrain-mobile-review.md) sets out for review.

- **A. The project feed:** one scrollable timeline of how a project changes.
- **B. Context review in the deck:** Keep, Retire and Fix cards for terms,
  term rules, voice rules and notes, both for suggestions waiting in a queue
  and for items already in force that nobody has read.

## The problem

**The feed.** A project's history is spread over the routes in the table
below, with offset, cursor and limit-only paging. Nobody can scroll one list and see
that a push added 40 blocks, a run drafted them, nb reached 90 percent, and an
agent session recorded six term rules. The workspace activity feed
(`GET /:ws/activities`) comes closest, and it leaves out review decisions,
recipe changes and every context write.

**The context.** On the dogfood project (workspace `bowrain-hq`, project
`uek630Q5`, from `kapi.yaml`'s `bowrain.url`):

| Measure | Value |
| --- | --- |
| Operations in the context log | 24, all `established` |
| Operations whose note reads "recorded as a person in a claude-code shell" | 19 |
| Operations carrying an agent session | 0 |
| Terms store | 67 concepts, 141 terms (74 en, 67 nb) |
| Term statuses | 125 preferred, 8 deprecated, 4 forbidden, 4 admitted |
| Log records naming terms | 1 import, "terms bundle .kapi/terms.json, 61 concepts" |
| Voice profile edits | 6, each recorded as "edited whole" |
| Content-memory bundles imported | 12 records, 2,444 entries |

So an agent wrote most of this context under the founder's identity, and the
log records it as a person's work. The 61 concepts arrived as one record, and
no record names the other six. Each voice edit replaced a whole profile, so
the log cannot say which rule changed. The founder has read none of it item
by item, and nothing on the platform can show which items a person has read.

## Scope

- **In scope:** the project feed (endpoint, event types, web and mobile
  screens); context cards in Pending and Replay modes for terms-store
  concepts, term rules, voice rules (patterns, rate ceilings, style measures,
  tone), kept notes, voice candidates and terms change sets; the attestation
  record; provenance on new context writes.
- **Out of scope:** content-memory pairs (2,444 entries; they are translation
  review and belong in the translation deck); the 13,205 imported translation
  decisions; workspace-level feeds (`/:ws/activities` stays as it is);
  notifications for feed entries; Kapi Desktop screens (the endpoints below
  serve them later, built in host per the standing rule); backfilling
  provenance that was never stored.

## Part A: the project feed

### What exists

| Route (under `/api/v1`) | Handler | Holds | Paging | Use for the feed |
| --- | --- | --- | --- | --- |
| `GET /:ws/activities` | `HandleListActivities` | `activities` rows written by `ActivityRecorder` (consumer group `activity-recorder`), filters `project_id`, `stream`, `actor_id`, `type`, `types` | cursor on `created_at` | the base table |
| `GET /pulse/:workspace/activity`, `/activity/heatmap` | `HandlePulseActivity`, `HandlePulseActivityHeatmap` | the same table, for the public Pulse surface (`BOWRAIN_PULSE_ENABLED`) | cursor, limit 100 | none; see gap 9 |
| `GET /:ws/audit-log`, `GET /:ws/:id/audit-log` | `HandleListWorkspaceAuditLog`, `HandleListAuditLog` | every bus event, as `event.AuditEntry` | offset | evidence behind an entry, never the feed itself |
| `GET /:ws/audit-log/verify` | `HandleVerifyWorkspaceAuditChain` | the hash chain check (`ChainVerification`) | none | none |
| `GET /:ws/projects/:id/recipe-changes` | `HandleListPendingRecipeChanges` | approved axes waiting for a pull to write them into `kapi.yaml` (`PendingRecipeChange`) | none | a pending state, not history |
| `GET /:ws/changesets`, `/:id/blast-radius` | `HandleListChangeSets`, `HandleChangeSetBlastRadius` | terms change sets (`knowledge.ChangeSet`: `CreatedBy`, `Origin`, `MergedBy`, `Impact`) | list | already mapped by the recorder |
| `GET /:ws/:id/context/objects` | `HandleListContextObjects` | the segments of the project's context ledger | none | none; writes publish no event |
| `GET /:ws/:id/convergence/runs`, `/:runID/events` | `HandleListConvergenceRuns`, `HandleConvergenceRunSSE` | runs; live progress over SSE | limit | completion is mapped by the recorder |
| `GET /:ws/:id/automations/history` | `HandleListAutomationHistory` | automation runs | cursor | out of scope for the first version |
| `GET /:ws/events` | `HandleWorkspaceEventsSSE` | the live change relay, `?project=` narrows it | live only | live prepend of new entries |
| `GET /:ws/concepts/:cid/story` | `HandleGetConceptStory` | one concept's merged timeline | none | precedent for a per-object merge |

`GET /api/admin/events` (`HandleAdminListEvents`) lists billing events and is
unrelated.

### Gaps in the code

1. **Context writes publish nothing.** `HandlePutContextObjects` and the
   Decide path (`HandleDecideRule` through `recordPlan`) never call
   `emitAudit` or `Publish`. A keep, a drop or a pushed import reaches
   neither the audit log nor the activity feed.
2. **Review decisions are per block.** `review.decided`
   (`EventReviewDecided`) fires once per block and locale, and the recorder
   does not map it. `review.bulk_approved` is also unmapped.
3. **Recipe changes carry no diff.** `project.setting_changed`
   (`EventProjectSettingChanged`, from `applyRecipeSettings` in
   `server/handlers_sync.go`) has before and after values and is audited, but
   unmapped. `EventEmittingStore.UpdateProject` publishes only the project's
   name, and the collection events carry an id and a name. Target languages
   added and coordinates changed (a collection's `Context` map) leave no
   record of what changed.
4. **`progress.milestone` is declared and never written.**
   `ActivityProgressMilestone` exists in `store/activity.go`; no code emits it.
5. **The summary is an English sentence.** `pushSummary` writes
   "pushed 3 files · 20 blocks" into `Activity.Summary`. A client cannot
   translate it or render its counts as counts.
6. **The cursor drops rows.** `ActivityStore.Create` writes `created_at`
   with `time.RFC3339` (whole seconds), and `List` pages with
   `created_at < cursor`. Rows that share a second across a page boundary are
   skipped. One push that changes three settings files three rows in the same
   second.
7. **Entries carry recording time.** `mapEventToActivity` leaves `CreatedAt`
   unset, so `Create` stamps the time the recorder ran, and a redelivered
   event sorts by its redelivery.
8. **Types are reused.** A deleted project files as `project.created`, a
   deleted stream as `stream.created`, a new collection as
   `project.updated`. A typed list cannot rest on these rows as they stand.
9. **Pulse queries by the wrong workspace key** (*unverified* in a running
   stack). `HandlePulseActivity` filters on the workspace's `ID`, while the
   recorder files rows under the workspace slug.

### Assembly: the feed table, extended

The `activities` table is already a feed table written by the existing
emitters: a durable consumer group on the event bus, idempotent on
`event_id`. The feed extends it rather than merging stores at query time.

A query-time merge would read six stores per page, each with its own paging
(offset, cursor, limit only) and its own permission check, and would decode
the context ledger from its objects on every request. No single cursor can
span those, so "the next 50 entries" would have no stable answer.

`EventEmittingStore` is the right emitter for row-level facts and the wrong
one for recipe diffs, because it sees one row write at a time. The push
handler holds the recipe state before and after, as `applyRecipeSettings`
already shows for settings, so the recipe diff is emitted there.

Changes to the table:

| Column | Type | Purpose |
| --- | --- | --- |
| `kind` | text | the feed kind below; empty for rows outside the project feed |
| `actor_kind` | text | `person`, `agent` or `tool` |
| `session` | text | the agent session, when there is one |
| `locales` | text[] | languages the entry touches |
| `coordinates` | jsonb | the point the entry sits at (`brand`, `product`, `channel`, `market`) |
| `counts` | jsonb | typed counts per kind (below) |
| index | | `(workspace_id, project_id, created_at DESC, id DESC)` |

`created_at` takes the event's own `Timestamp` at full precision. Rows
written before the change keep `kind` empty; the standing decision to prefer
a data reset over a migration applies, so no backfill is written.

### Event types

The project feed lists exactly these kinds. Everything else in `activities`
(streams, tasks, gates) stays visible through `/:ws/activities`.

| Kind | Source event | Emitter | Counts | Opens |
| --- | --- | --- | --- | --- |
| `push` | `connector.push.completed` | existing | `files`, `blocks` (in the event today as `files_count`, `blocks_count`); `blocks_added`, `blocks_changed`, `blocks_removed` to add | the stream at that push |
| `convergence_run` | `convergence.run.completed` | existing | `state` and `passes` (in the event today); `failing_checks` and the per-locale `standing` from `ConvergenceRun` to add | the run (`/convergence/runs/:runID`) |
| `review_summary` | `review.decided`, `review.bulk_approved` | existing, rolled up | `approved`, `fixed`, `rejected`, `bulk_approved`, per locale | the review deck for that language |
| `recipe_change` | new `recipe.changed`, and `project.setting_changed` | push handler | `collections_added`, `collections_removed`, `coordinates_changed`, `targets_added`, `targets_removed`, `settings_changed` | the recipe diff |
| `context_change` | new `context.recorded`, `voice.profile.updated`, `voice.rule_auto_promoted`, `voice.rule_rejected` | context ledger writes, voice handlers | per subject (`terms`, `notes`, `voice_rules`) and per operation (`suggested`, `kept`, `dropped`, `imported`, `edited`, `attested`) | the Replay or Pending deck at those items |
| `change_set` | `knowledge.EventChangeSet*` | existing | `ops` by type (`term.add`, `term.status`, …), `impact` from `ImpactSummary` | the change set card |
| `coverage_milestone` | new `coverage.milestone` | ship-state computation | `locale`, `percent`, `blocks` | the language page |

**Review roll-up.** The recorder folds `review.decided` into one row per
project, actor, locale and UTC hour. The row's id is derived from that key,
and each decision updates its counts in place. A reviewer who approves 300
blocks in a morning files three rows, never 300.

**Context roll-up.** One `context.recorded` event per ledger write, which is
one segment: a push of context objects, one Decide answer, one attestation
batch. Its counts are taken from the records in that segment.

**Milestones.** A locale crossing 50, 90 or 100 percent established, in
either direction. The thresholds are an open question.

### Endpoint

`GET /api/v1/:ws/:id/feed`

| Parameter | Meaning |
| --- | --- |
| `cursor` | opaque; encodes `(created_at, id)` of the last row |
| `limit` | default 50, maximum `maxListPageSize` |
| `kind` | comma-separated kinds from the list above |
| `locale` | entries touching this language |
| `coordinate` | `axis:value`, repeatable; entries at that point or below |
| `actor` | a user id |
| `actor_kind` | `person`, `agent` or `tool` |
| `since` | RFC 3339 |

Response:

```json
{
  "entries": [
    {
      "id": "act_01J…",
      "kind": "context_change",
      "at": "2026-11-10T06:27:40.979903Z",
      "actor": {"id": "usr_…", "kind": "agent", "name": "claude-code", "session": "0ab4e399", "host": "laptop"},
      "on_behalf_of": {"id": "usr_…", "name": "Asgeir"},
      "locales": ["en"],
      "coordinates": {"product": "kapi"},
      "counts": {"terms": 6, "suggested": 6},
      "link": {"screen": "context_deck", "mode": "pending", "items": ["0s6mjtsvhv…"]}
    }
  ],
  "next_cursor": "…"
}
```

`actor` is who did it as recorded; `on_behalf_of` is the signed-in member
whose token carried it, present when the two differ. `link.screen` names a
client screen from a closed list (`stream`, `run`, `review_deck`,
`recipe_diff`, `context_deck`, `change_set`, `language`), never a URL, so web
and mobile route it each in their own way.

The route needs `PermViewContent` on the project. The feed is a separate
route over the same table because its shape differs from `/:ws/activities`
(typed counts, actor kind, project scope), and the dashboard and workspace
layout already read the existing shape.

### Presentation

Page shape: **List and detail** (`packages/ui/docs/page-patterns.md`), `full`
width on the web. On the phone the list is one screen and the detail is the
next.

| Element | Rule |
| --- | --- |
| Day group | a sticky header per day in the viewer's time zone: Today, Yesterday, then the date |
| Row primary line | an icon for the kind and the object: "Push · main", "Run · converged", "nb · 90%" |
| Row meta line | at most three facts with a middle dot: the actor's mark and name, the main count ("+40 blocks", "6 terms"), the time |
| Actor mark | an avatar for a person, the host's icon for an agent, a gear for a tool |
| Filters | chips for kind, language and actor kind; coordinate in a sheet |
| Detail | the counts as a `StatRow`, the actor and session, the coordinates, and one button to the place `link` names |
| Live | the client subscribes to `GET /:ws/events?project=` and prepends new entries under "Today" |

All labels and count units come from the client's catalog, with the locale's
plural rules, which is why the server sends counts and never sentences. On
mobile the feed is a tab of the project screen and is cached through the
TanStack Query persister like the deck.

## Part B: context review in the deck

### Where the context lives

| Item | Store | Identity | Queue today |
| --- | --- | --- | --- |
| Term rule, note, memory pair, voice edit | context ledger (`contextop.Record`: `Kind`, `Subject`, `Scope`, `Actor`, `Status`, `Voice`) | operation id | `GET /:ws/:id/rules/:ref/decide`; workspace counts at `GET /:ws/context/queue` |
| Rules in force from any source | folded by `HandleKeptRules` (`screens.KeptRuleDTO`: `kind`, `rule`, `from`, `applies`, `kept_by`, `kept_at`) | none of its own | `GET /:ws/:id/rules/:ref/kept`, newest first |
| Concept and its terms per language | terms store; on the server `knowledge` with `ConceptRevision` (`Actor`, `ChangesetID`, `Rev`) | concept id and revision | `GET /:ws/changesets` (`in_review`) |
| Term rule fields | `profile.TermRule`: `Term`, `Replacement`, `Forms`, `Advisory`, `Competitor`, `ConceptID`, `DoNotTranslate`, `Scope` | none of its own | as its holder |
| Voice profile | `profile.VoiceProfile` with `Version`, `VersionNote`, `CreatedBy`; history in `voice_profile_versions` | profile id and version | none |
| Pattern rule | `profile.Pattern`: `Regex`, `Description`, `Fails`, `Advisory`, `Rate`, `Scope` | none | none |
| Correction candidate | voice loop: `GET /:ws/voice-profiles/suggested-rules`, `GET /:ws/voice-profiles/:id/candidates` | term within a profile | promote, demote, reject, evaluate-rule |
| Entity suggestion | review queue item, type `entity_review` | item id | `GET /:ws/:id/review-queue/:ref` |

A `TermRule` carries no language field. A rule tied to a concept takes the
language of the concept's term, which covers the source-language rules the
founder asked about (the rule that makes "content memory" the preferred
term, forbidden and avoided words, `competitor: true`) as well as the nb terms. A rule with no
`ConceptID` has no language the server can show.

### Provenance today

| Store | Actor | Actor kind | Session | Commit or PR | Item level |
| --- | --- | --- | --- | --- | --- |
| Context ledger, written by kapi | `contextop.Actor.Name` | yes | for an agent only | only on a `signal` (`Signal.Commit`, `Signal.PR`) | yes, except bundle imports and whole-profile voice edits |
| Context ledger, written by the server | the member's display name | always `person` | none | none | yes |
| Content changes | `change.Actor` | yes, in `content.changed` data `actor_kind` | yes | none | per change set |
| Terms change sets | `CreatedBy`, `MergedBy` user ids; `Origin` such as `kapi-push/concepts` | none | none | none | per op, and per `ConceptRevision` |
| Voice profiles | `CreatedBy` per version | none | none | none | per version, never per rule |
| Activities, audit log | user id | none | none | none | per event |

How the command line decides the kind (`host/contextactor.go`):
`KAPI_ACTOR` names it outright. Otherwise a host marker (`CLAUDECODE` with
`CLAUDE_CODE_SESSION_ID`, `CODEX_SESSION_ID`) means agent. `KAPI_ACTOR=person`
inside a marked shell is recorded as a person, and the override survives only
as text in the operation's note (`ResolvedActor.NoteWith`). That is how 19 of
the dogfood's 24 operations read as a person's.

What is missing:

1. **The override as a field.** "Person in an agent shell" lives in prose.
   The server cannot count or filter it. `contextop.Actor` needs the host and
   the override as fields, and the host's session id kept even when the kind
   is `person`.
2. **Actor kind and session on server-side stores.** A push under the
   founder's token files the change set, its ops and each `ConceptRevision`
   as the founder, with no kind or session. The push already resolves the
   actor for ledger operations; it should send the same `contextop.Actor` with
   concept and voice writes, and the server should store it beside the user id.
3. **Item-level records for bundle imports and voice edits.** One record for
   61 concepts, and "edited whole" for a profile. The server can still derive
   item cards: each concept has its own `ConceptRevision` rows, and successive
   `voice_profile_versions` snapshots can be compared rule by rule. The log
   itself should record one operation per item on future imports.
4. **A stable id for a pattern rule.** `profile.Pattern` has none, so a rule
   whose regex is edited looks like one rule removed and another added. A
   card, an attestation and a feed link all need the id.
5. **The commit behind a write.** Terms and voice files are git source. A
   push can stamp the commit it was made from on each change set and ledger
   segment, which gives Replay its "which PR" line.

### Two modes

**Pending** is the queue: ledger suggestions and contested rules, voice
candidates at `pending` or `approved`, terms change sets `in_review`, and
`entity_review` items. The server merges them into one deck for the project,
ordered as Decide orders its questions, with change sets after.

**Replay** is everything in force, oldest first, in the order it was added.
Cards are stacked by origin: one stack per agent session, per import, per
change set, with a header such as "claude-code · 0ab4e399 · 14 items ·
2026-09-26". The default filter is "not reviewed". Other filters: actor kind,
item kind, language, coordinate.

### The card

| Region | Content |
| --- | --- |
| Rule | the item as a person reads it, in the form `contextop.Subject.Phrase` writes, such as `"Quickcast", not "Quick cast"`; a concept shows each language's term with its status |
| Flags | badges: fails or advisory, competitor, do not translate, the forms it also matches, a rate ceiling |
| Where | the scope: project, workspace, or the coordinates (`brand`, `product`, `channel`, `market`) and validity dates |
| Provenance | the actor's mark and name, the host and session, "for" the member when the two differ, the commit or PR, the date |
| Blast radius | uses in the project's content (`Standing.Uses`, or `GET /:ws/concepts/:cid/blast-radius` for a concept), and for a voice rule the preview from `POST /:ws/voice-profiles/:id/evaluate-rule` |
| Review state | not reviewed, reviewed by whom and when, or changed since review |

Actions, with the same buttons and swipe directions as the translation deck:
Retire (swipe left, with a Fix button beside it), Later (swipe up), Keep
(swipe right).

### What each action does

| Item | Keep | Retire | Fix |
| --- | --- | --- | --- |
| Ledger suggestion (Pending) | `POST /:ws/:id/rules/:ref/decide` with `answer: keep` (a `keep` operation) | the same with `answer: drop` | keep with edits: `form` exists today; the request gains `replacement`, `advisory` and `scope`, which `kapi context review --keep --use --advisory --widen-to` already take |
| Ledger rule or note in force (Replay) | an attestation | a `drop` operation, which `kapi context review --drop` already accepts for a rule | an `edit` operation with the new rule; the old one is dropped in the same segment |
| Concept (Replay) | an attestation | a change set with `term.status` to `deprecated` or `forbidden`, or `concept.delete` | a change set with `term.update` or `term.add` |
| Terms change set (Pending) | `POST /:ws/changesets/:id/approve`, then `/merge` | `POST /:ws/changesets/:id/reject` | `POST /:ws/changesets/:id/ops` with the edited op; the set then needs another approver |
| Voice candidate (Pending) | `POST /:ws/voice-profiles/:id/promote-rule` | `POST /:ws/voice-profiles/:id/reject-rule` | promote with an edited `replacement` |
| Voice rule in force (Replay) | an attestation | a new profile version without the rule (`PUT /:ws/voice-profiles/:id`) | a new profile version with the edited rule |

Change sets carry no voice ops today (`knowledge.OpType` lists concept, term
and relation ops only), so a voice Retire or Fix lands at once, without the
review a terms change gets. Whether to add voice ops is an open question.

A Fix in Replay produces a new revision. The reviewer is its author, and the
separation-of-duties rules below decide whether it counts as reviewed.

### What "reviewed" records

An **attestation**: a person states that they read one item at one revision
and keep it. It changes nothing in force.

| Field | Meaning |
| --- | --- |
| `item` | a typed reference: `{kind: "op", id}`, `{kind: "concept", id, rev}`, `{kind: "voice_rule", profile, rule, version}` |
| `reviewer` | the person, always `person` kind |
| `verdict` | `keep`; Retire and Fix are their own operations and need no attestation |
| `basis` | `peer`, `agent_authored`, `solo_owner` or `self_warned` (below) |
| `at`, `note` | when, and an optional note |

It is recorded as a new context ledger operation kind, `attest`, beside
`keep` and `drop`. The ledger is the shared, append-only record that kapi,
Kapi Desktop and the server all read, so `kapi context log` shows reviewed
items too and the record travels with the context objects. An `attest` names
no subject of its own and folds onto the item it names, like `keep`.

An item counts as reviewed when its current revision carries an attestation
with a valid basis. An edit after the attestation makes it "changed since
review". The project shows three counts: in force, reviewed, not reviewed,
and the last split by actor kind.

### Separation of duties

The workspace policy is `SoDMode` (`off`, `warn`, `block`), and terms change
sets refuse a self-review unless the basis is `solo_owner`
(`knowledge.BasisSoloOwner`). Today both compare user ids. An agent working
under the founder's token is filed as the founder, so the founder's review of
that agent's work counts as self-review and `block` refuses it.

Proposed rules, all computed on the server and returned on each card as
`own_work` and the basis the action would record:

| Case | Basis | Result |
| --- | --- | --- |
| Item authored by another person | `peer` | reviewed |
| Item authored by an agent or tool, or by a person under an agent host's override, reviewed by the member it ran for | `agent_authored` | reviewed; the member stays on record as the principal |
| Item the reviewer wrote by hand | none | the Keep button is hidden; another person reviews it |
| Fix by the reviewer, policy `off` | `self_warned` | in force and reviewed |
| Fix by the reviewer, policy `warn` | `self_warned` | in force and reviewed, with a warning in the audit log |
| Fix by the reviewer, policy `block` | none | in force, not reviewed, waits for a second person |
| Fix by the reviewer, no other member holds `manage_terms` | `solo_owner` | reviewed, as change sets already allow |

The second row depends on gap 1 in provenance. Until the override is a field,
the server can read it only from the note text, and the 19 dogfood operations
are recognisable only that way.

The dogfood workspace has one member, so every founder Fix records
`solo_owner`.

### From the feed into review

A `context_change` entry links to the Replay or Pending deck with its items
first and the rest of its stack after. A `change_set` entry opens that change
set's card. A card's provenance line links back to the feed entry that
recorded it, so the reader can see what else that session did.

## What the server needs

1. **Feed table changes:** the columns above, the event's own timestamp, the
   `(created_at, id)` cursor, and distinct types where rows are reused today.
2. **New emitters:** `context.recorded` from every context ledger write;
   `recipe.changed` from the push handler with the diff; `coverage.milestone`
   from the ship-state computation; the review roll-up in the recorder.
3. **`GET /:ws/:id/feed`** as specified.
4. **`GET /:ws/:id/context/deck?mode=pending|replay`** with `cursor`, `kind`,
   `locale`, `coordinate`, `actor_kind`, `session` and `reviewed`. Each card
   carries its item reference, revision, provenance, blast radius summary,
   review state, `own_work` and the actions allowed.
5. **`POST /:ws/:id/context/deck/decide`** with the item reference, the
   revision the card showed, the action and a Fix payload. The server maps it
   to the flows in the action table and answers `409 stale` when the revision
   moved.
6. **The `attest` operation** in `core/contextop`, and the reviewed counts.
7. **Provenance fields:** host and override on `contextop.Actor`; actor kind,
   session and commit on change sets, concept revisions and voice versions; a
   stable id on `profile.Pattern`; one ledger operation per item on import.

## Build order

The proposal was to build the feed endpoint first. The code supports that,
with one change: the feed table already exists, so step one is a fix and a
new route rather than new storage.

1. **Feed over what is already emitted:** the cursor and timestamp fixes, the
   new columns, `/feed` for `push`, `convergence_run`, `change_set` and the
   review roll-up. Web and mobile screens follow. Small, and both clients
   gain a screen.
2. **Context and recipe emitters:** `context.recorded` and `recipe.changed`.
   Replay links and Pending counts depend on them.
3. **Replay, read only:** the deck endpoint in replay mode over `/kept`,
   concept revisions and voice versions, showing provenance as stored. The
   founder can read the dogfood context from here on.
4. **The attestation** and the reviewed counts.
5. **Retire and Fix** through the existing flows, with the separation-of-
   duties rules.
6. **Pending** as one merged deck.
7. **Provenance at write time**, so items recorded after this step carry
   kind, session and commit. It helps only new items, which is why it comes
   after the steps that serve the existing backlog.

Steps 1 and 3 are independent and can run in parallel.

## Open questions for the founder

1. Should Replay offer "keep all" for one session after a count
   confirmation, recorded as one attestation per item? The CLI has
   `kapi context review --session <id> --keep all` for suggestions.
2. Does a Retire take effect at once, as a ledger `drop` does today, or wait
   for a second person under `block`?
3. Should voice rules move into change sets (new `voice.*` ops), so a voice
   Retire or Fix gets the review a terms change gets?
4. Is `agent_authored` an acceptable basis: the founder reviewing work an
   agent did under the founder's token counts as review?
5. Should `profile.TermRule` gain a language field for rules with no concept?
6. Coverage milestones at 50, 90 and 100 percent established, or the
   project's ship bar?
7. Should the feed show `content_change` rows for edits made in the editor,
   or leave those to block history?

## Sources

- Routes: `bowrain/server/server.go`, `handlers_changesets.go`,
  `handlers_pulse.go`, `handlers_rules_decide.go`,
  `handlers_context_objects.go`, `handlers_concepts.go`, `recipe_changes.go`
- Feed table: `bowrain/store/activity.go`, `bowrain/store/migrations.go`,
  `bowrain/event/activity_recorder.go`
- Events: `bowrain/core/event/event.go`, `bowrain/event/store_events.go`,
  `bowrain/server/audit_events.go`, `bowrain/changes/record.go`
- Audit chain: `bowrain/event/audit.go`, `bowrain/event/audit_chain.go`
- Context ledger: `core/contextop/contextop.go`, `core/contextop/settle.go`,
  `bowrain/server/contextledger.go`, `host/contextactor.go`,
  `host/screens/kept.go`, `cli/context_review.go`
- Terms and voice: `core/profile/profile.go`, `bowrain/knowledge/types.go`,
  `bowrain/voice/postgres.go`, `bowrain/server/handlers_voice_loop.go`,
  `bowrain/server/handlers_voice_corrections.go`
- Separation of duties: `bowrain/core/auth/types.go`, `bowrain/review/gate.go`
- UI: `packages/ui/docs/page-patterns.md`
