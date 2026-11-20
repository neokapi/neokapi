# Review by leverage

Working note, 2026-11-19. How a person reviews a large project without
reviewing it block by block, and how Bowrain guides a new project there. The
founder asked for it after the neokapi dogfood project reached 27,788 blocks
waiting, a number no reviewer will work through, and one any project adopting
neokapi can reach in a week.

## The problem

`kapi status` on the dogfood project, 2026-11-19:

| What | Count |
| --- | --- |
| Source blocks | 27,788, all written, none established |
| Of which documentation (neokapi-docs, bowrain-docs) | 21,996 |
| Interface collections (CLI, engine, mobile) | 3,502 |
| Blocks failing the bound checks | 593 |
| Ship gates declared | none |

Every block is waiting because the only way a translation or a source block
becomes established is a person deciding it. What exists today:

- `priority:` (high, normal, low) on a collection orders the deck and decides
  nothing.
- Approve passing establishes every target that clears the checks, the terms
  and the voice bar, in one action, scoped to a stream and locales.
- The deck groups cards (fast, closer, desk) and collapses identical copies.
- The inbox lists context before languages, by order only.
- Nothing samples, nothing records a policy, and nothing reviews the source
  except a person establishing it.

The deeper problem is the order. A reviewer who starts on strings reviews the
consequences of decisions nobody has made: which product a string belongs to,
what a term is called, how the voice sounds. Those decisions generate the
text. A term decided once governs every block that uses it, in the source and
in every translation, and a voice rule decided once governs every block drafted
under it. A string decided once governs one block.

## The principle

**Decide what generates the text, then confirm the text.** The weight of a
decision is its reach: how many blocks, at how many coordinates, in how many
languages it governs. A reviewer works from the widest reach down, and the
source language comes first at every level, because the source is what every
translation is made from. The same reviewer carries each decision into the
languages the profile is configured for, while it is fresh.

Reviewing "the interfaces first" or "the docs first" is the wrong axis. The
interfaces and the docs share the same terms and voice rules. Deciding them once, at the
coordinates it spans, is what makes both right.

## The pyramid

| Level | What is decided | Unit | Reach of one decision |
| --- | --- | --- | --- |
| 1. Coordinates | products, channels, brand, languages; which voice governs each point; what each language receives | one decision per axis or binding | every block at the point |
| 2. Context | terms (the source term, its forms, its translation per language, do-not-translate), names, voice rules per product | one card per concept or rule | every block that uses it |
| 3. Exceptions | blocks that fail a check against the context, grouped by the rule they break | one decision per rule, then per block | the blocks the rule names |
| 4. Interfaces | complete components, sections and pages, read in context | one page | the strings on the page |
| 5. Settled by policy | everything else | none | |

Level 1 is a handful of decisions. Level 2 is tens to a few hundred cards,
ranked by reach. Level 3 shrinks as level 2 is decided, because most findings
are one rule broken many times. Level 4 is where a person confirms that the
decisions above produce text that reads right where it ships. Level 5 is the
bulk of the corpus, and no person reviews it block by block.

### Level 1: coordinates

The axes the recipe declares: `profiles` (products) and their `channels`,
`defaults.coordinates` (brand and any declared axis), `target_languages`, and
the voice each profile binds. A context scan proposes axes for a project that
declares none (`InferAxes`). The decisions are few and each one moves
everything under it, so they come first and they are confirmed explicitly,
even when the recipe already states them: a recipe written by an agent is a
proposal until a person keeps it.

### Level 2: context

The context deck, ordered by reach instead of by kind. For each concept: the
source term and its forms, then the term in each target language the reviewer
covers, each with the number of blocks that use it and at which coordinates.
For each voice rule: the rule and the blocks it reached. Source-language
decisions come first; a concept's translations are offered right after its
source term, to the same reviewer, so the reviewer decides "workspace" and
"arbeidsområde" together.

A key term with no translation in a configured language is a decision to make,
not a gap to discover later in a string.

### Level 3: exceptions

The blocks that fail the bound checks, grouped by the rule they break. Most
findings are one rule broken many times: the reviewer decides whether the rule
is right (and changes the rule, which clears every finding at once) or the
source is wrong (and files a source fix, which the engine applies everywhere).
Only what remains after that is decided block by block.

### Level 4: interfaces in context

Complete interfaces, read where they ship: a component's strings in its running
story, a document section with its heading, a page. The reviewer confirms the
source reading first and then each target reading of the same page. How many
pages a reviewer is asked to read is set by the gate (below): every page, a
sample, or only pages with a finding.

### Level 5: settled by policy

A block is established by policy when it clears every bar the gate names: its
checks pass, its terms comply, its voice score meets the bar, and the context it
was produced under is approved at its current revision. The establishment is a
decision like any other: a decide operation in the ledger, attributed to the
person who set the policy, with the reason `review policy`, and reopened by any
later rejection. The person decided a rule; the server applied it.

### Context sessions

Bowrain is a team tool, so a project has one context queue. The phone works
it in sessions (`POST /:ws/:id/context/sessions`, `bowrain/server/context_sessions.go`):

- A session deals up to ten pending cards the caller can keep or reject,
  ranked by reach. A concept and the proposed translations that follow it
  rank as one group. A card with no reach of its own (a kept concept with no
  scan count, a term rule, a voice rule that lists its words) gets one
  `CountBlocks` aggregate over the source blocks holding its words, for at
  most 30 cards per start.
- The session holds its cards (`context_session_holds`, one row per project
  and item) until a decision releases one, the owner ends it
  (`POST .../sessions/:sid/end`), or 30 minutes pass with no decision. A
  hold row's key decides who takes a card, so two sessions started at once
  get disjoint cards. Starting a session ends the caller's previous one.
- The deck listing, the inbox count and the decide path exclude cards
  another person's live session holds; a decision on such a card answers
  409 `held`.
- "What changed" compares against the team's last ended session, not a
  baseline per person: candidates filed since it ended, concepts whose reach
  grew against the reach the session recorded when it started, and the
  decisions other people made in sessions since. Decisions made outside a
  session are not in that count. What others decided reaches a person
  through the feed, never as work.
- The summary counts kept, changed (kept in the reviewer's wording) and
  rejected cards, the blocks the kept terms reach (each concept once, as
  dealt), and the source findings the last check counted for term rules the
  session turned down. Translations that will be drafted again are not
  counted: the server has no figure for them, so the summary leaves it out.
- A second review happens only under the workspace's separation-of-duties
  setting (`GetSoDMode`), which the decide path applies when a card retires
  a rule in force. Nothing else asks a second person to look at a decided
  card.

Sessions make no AI calls: they deal candidates and proposals that already
exist.

## How terms enter the context: term extraction today

The context cards are only as good as the candidates behind them, and
on a large project the candidates are the weakest link. What exists:

| Path | Reads | Proposes | Lands as |
| --- | --- | --- | --- |
| `term-extract` (AI, `core/ai/tools/terminology.go`) | one block | term, definition, domain | a block property; the overlay the schema promises is never written |
| DeeperLook (`kapi context suggest --ai`, `host/contextdeeper.go`) | the first 12 chunks of 2,000 characters, in file order | the 10 terms used in the most blocks | a term rule when the term appears in two spellings, otherwise a note |
| Context scan for a project (`bowrain/jobs/contextscan_project.go`, `bowrain/termcandidate`) | a sample drawn from every collection in turn, in the project's source language | terms plus the drafted voice's preferred forms, each with its reach counted over the whole stream | proposed concepts, scoped to the coordinates their blocks share, dealt first in the context deck |
| `entity-extract` (AI) | one block | text, category, translatability, confidence | term-candidate overlay spans no queue reads |
| Spelling variants (no AI, `host/contextsuggest.go`) | the whole corpus | words and two-word phrases written two or more ways | ledger suggestions with a block count and the shared coordinates |

What this means for a project the size of neokapi:

- **The sample was the front of the corpus.** A project scan now samples every
  collection; DeeperLook still reads the first 24k characters, and a scan
  started without a project the first 72k. On neokapi that is a slice of whichever collection sorts
  first, so the terms of the CLI, the apps and most of the docs is
  never seen.
- **Candidates live in four places**, each with its own shape: ledger
  suggestions, proposed concepts, the scan's result, overlay spans.
- **A key term with no variants becomes a note.** Keeping a note creates no
  concept, so the term never governs anything.
- **Nothing ranks by reach or by coordinate.** Scan candidates sit at the
  default point; the context deck orders by kind and time and does not show
  a concept's coordinates.
- **Translations are proposed only on Bowrain.** A kept term gets a proposed
  term per target language there; in kapi, target terms arrive by hand or by
  import, and `kapi terms expand` adds forms to terms that already exist.

The fix, in the order it pays:

1. **Sample across coordinates.** Draw the extraction sample from every point
   (each collection at its coordinates), weighted by size, instead of the
   front of the corpus. Run in the source language, whatever it is.
2. **One candidate shape in the ledger.** Every path files a term suggestion
   with the term, its forms, a definition, the blocks it occurs in per
   coordinate (counted by the engine's matcher over the whole corpus, not the
   sample) and examples. A single-variant key term is a term suggestion, not a
   note.
3. **Keep creates a concept, scoped to the coordinates it spans**: the
   project's point when it occurs everywhere, the product or channel when it
   occurs only there. Reject records "not a term", so it is not proposed again.
4. **Rank the deck by reach**, and show each card's coordinates.
5. **Propose translations**: after a source term is kept, propose its term in
   each target language (AI, with the concept's definition and examples, and
   the forms through `kapi terms expand`), dealt to the same reviewer as the
   next card.

## Gates by involvement

A project says how much its reviewers can be involved, and the gate follows
from that, rather than from the kind of content:

| Involvement | The reviewer decides | Settled by policy |
| --- | --- | --- |
| `context` | levels 1 to 3, and a sample of interfaces | every block that clears the bars |
| `interfaces` | levels 1 to 3, and every interface page | the blocks the pages do not cover |
| `every` | everything | nothing |

The default is `context`. It is the only setting a single founder can sustain
on a project of this size, and it is the setting where the founder's attention
has the most reach.

Involvement is a Bowrain project setting, set by a manager on the project's
languages screen. It is never a recipe key: team settings live on the Bowrain
project.

### Involvement per coordinate

The project's involvement is the default. A manager may override it at a
point of the project's context space: `product=docs` at `context`,
`product=app` with `channel=ui` at `every`
(`bowrain/server/review_plan_involvement.go`).

- **Storage.** The project property `review_involvement_overrides` holds the
  list as JSON: `[{"coordinates": {"product": "app"}, "involvement":
  "every"}]`. Each review action merges its own property keys in a single
  database update. Concurrent scan, settle and involvement writes preserve
  each other's results. The review plan owns the involvement, job references
  and attribution fields. An edit through the general project settings route
  cannot change them.
- **API.** `GET /:ws/:id/review/plan/involvement` answers the involvement, the
  overrides with the collections each decides, and the axes an override may
  name with the values the main stream's collections hold. `PUT
  .../involvement/overrides {"overrides": [...]}` replaces the list and
  answers the same. It needs `manage_project`, refuses an axis no collection
  declares, a value no collection holds, a level other than the three, an
  override with no axis, and two overrides at the same point (400), and
  records the caller in `review_involvement_set_by`, as setting the
  involvement does.
- **Resolution.** An override applies to a collection when every axis it
  names has the collection's value. Of those that apply, the one naming the
  most axes wins; between two naming as many, the deeper involvement wins
  (`every`, then `interfaces`, then `context`), so a tie never leaves people
  less to review than either override asks. A collection no override
  reaches, and a block in no collection, takes the project's involvement.
- **Where it applies.** Involvement resolves once per collection, from the
  collection's coordinates, never per block. The plan's points carry the
  involvement in force (`coordinates.points[].involvement`), and the plan
  lists the overrides (`overrides`). The deck counts run once, or twice
  when some collections are under `every` and others are not: once over the
  collections a hold reaches, once over the `every` ones
  (`ReviewDeckQuery.OnlyCollections`). Held translations, `settleable` and
  the language decks count the first part only; a held deck deals the desk
  group outside the `every` collections and every group inside them
  (`ReviewDeckQuery.Hold`). The page sample applies to the collections under
  `context`, every page elsewhere. The settle pass, by hand or after a run,
  leaves the items of the `every` collections alone, and `policy.settle` is
  `none` only when the project and every translated collection are under
  `every`. At 70,000 blocks and five languages the plan stays near 215 ms
  with two overrides in force (`TestReviewPlan_Scale`).

Before a gate is set, the review plan shows what each involvement would ask of
the reviewer at the project's current size: how many context cards, how many
exception groups, how many pages. A founder chooses with the numbers in front
of them.

## Source first

The source is reviewed through the same pyramid and ahead of the targets:

- the context is decided in the source language first;
- source exceptions (a source block breaking a source-language rule) come
  before target exceptions;
- an interface page is read in the source before its translations;
- a block's translation is settled by policy only when its source is
  established, by a person or by the same policy.

Translation keeps running while the source settles: drafting is cheap, and a
redraft after a source fix is automatic. What waits is establishment.

## The review plan

One view, the same on web (the project Overview) and mobile (the Review tab):
the five levels, each with what is waiting, what is decided, and the next
action, plus the involvement in force and what it leaves for policy. It replaces
"N of 5 first steps" on a Bowrain project. Each level opens the surface that
already decides it: the coordinates and bindings, the context deck, the
exceptions grouped by rule, the page deck, and the policy record.

The plan is computed on the server, so web, mobile and kapi show the same
numbers. Reach (blocks per concept and per rule, by coordinate) is computed by
the engine's own term lookup over the project's blocks, in batches, and kept per
terms snapshot and content revision.

## The plan is the queue

The plan decides what a language's review deck holds. Under `context` and
`interfaces`, while the context has open decisions (a context card
waiting for anyone, or a concept with no term in a target language), the deck
deals only what the plan leaves for people at that point:

- the exceptions: translations a check fails, the deck's desk group;
- the page sample, through the page deck.

The other translations are held. The deck answers with `held: {count,
reason: "context"}` in place of their cards, and `counts.held` beside
`counts.waiting`. A translation the deck has not classified yet counts as
held until a deck request classifies it and finds a failing check. The
deck's collections, the plan's languages (`languages[].held`, and `held` on
the plan) and the review inbox count the same way, so the Review tab never
lists held work as waiting. A project whose translations are all held stays in
the inbox's `projects` with its plan in brief, whose `held` lists the count
per followed language. Under `every` nothing is held.

The exceptions stay reviewable while the context is open. A failing check
names a rule, and a reviewer can decide that rule whatever terms are still open.

### Review plan counts

The main-stream review plan and the inbox share a stored snapshot per
project. It holds deck counts by language and involvement, page counts,
source check totals, and project-wide context counts. The server adds the
caller's followed languages and the context decisions they can make on
each request. `as_of` gives the time of the counts.

Project and workspace events mark affected snapshots for refresh. The
server groups changes and claims a database lease before each background
refresh. A change during the refresh schedules another count. A request
counts immediately if the snapshot is missing or its inputs have changed.
Those inputs include the deck policy, source and target languages, collection
scope and priority, and the resolved involvement of each collection.
Changing a collection from `context` to `interfaces` updates the page
count on the next read. Branch plans count on each request.

Language deck holds and settle passes read current project data. Both use
`planPolicy` to count open context cards and concepts that lack a target
term. A failed read from a configured terms store stops the decision.
The automatic settle pass evaluates the policy while holding the same
project lock as a manual pass.

### Step states

Each level of the plan reports a state in `steps[].state`, on the plan route
and in the inbox's plan in brief:

| State | Meaning |
| --- | --- |
| `not_started` | nothing to work from yet |
| `open` | something waits |
| `done` | nothing waits |
| `attention` | the team finished the step and it changed after, such as coordinates changed since the last confirmation |

The context level is `not_started` while the project has no concept with a
source term, no context card waits for the caller, and no term scan has
completed. It is never `done` at zero. Exceptions are `not_started` before
any project-wide check when no translation waits or is held.

### The next step is the first one a person can act on

`next`, on the plan and on the inbox's plan in brief, names the first level
the caller can act on, computed per request where the caller's context cards
and followed languages join the snapshot's counts
(`nextPlanLevel`, `handlers_review_plan.go`). A level with work ahead and
nothing to decide yet is skipped and listed in `waiting`, each entry a
`level`, a `reason` and, for `waiting_quota`, `quota_resets_at`. Today only
the context level can wait this way: it has not started and its term scan is
`queued` or `processing`, or cannot start (`waiting_content`,
`waiting_quota`, `waiting_credits`, `no_ai`). A context that waits for the
next push, or whose scan failed, stays `next`, because a person can start a
scan. A target term missing in a language the caller does not follow is not
the caller's step. When every level waits or is done, `next` is `""` and
`waiting` says why.

The phone's Home tab opens on the same step, for the project the person last
opened when it has one, else the first project in the inbox with one: the
setup while the person has chosen no languages, the coordinates map, the
context cards, the language deck for exceptions and pages, or the settle
pass. The waiting levels sit beside it as one row each. With no step to act
on, the card names what waits; with nothing waiting, it reads as caught up.
The Review tab's plan card shows the same `next` and rows.

A context level that has not started does not hold translations. A project
that cannot run a scan (a scan needs AI credits) would otherwise hold every
translation with nothing a reviewer could do about it.

### The term scan starts on a push

Nobody presses a button to start the term scan. The server subscribes to
`connector.push.completed` (consumer group `term-scan-on-push`,
`bowrain/server/review_plan_autoscan.go`) and, for a push to the project's
default stream, starts a scan when one is due:

- the project has never had a term scan, and the workspace has no concept
  with a source term;
- the last scan failed;
- a collection exists that the last scan did not read;
- the translatable source blocks grew by 500, or by a fifth of the blocks the
  last scan read, whichever comes first.

Starting a scan records the job in the project property
`review_term_scan_job`, and the blocks and collection names it reads in
`review_term_scan_blocks` and `review_term_scan_collections`. A scan never
starts while another is queued or processing. It also never starts when it
would fail on arrival: no platform AI provider, the workspace over its
monthly AI limit (`jobs.QuotaStore`), or no AI credits left. The next push
decides again.

While the context has not started and no scan runs, the plan says why
in `context.scan_waiting` and on the context step's `scan`:
`waiting_content`, `waiting_push` (content exists and no scan has run; the
next push starts it), `waiting_quota` with `quota_resets_at`, the first of the
next month in UTC, `waiting_credits` or `no_ai`. `POST
/:ws/:id/review/plan/scan` scans again on request and records the same
coverage; the web Overview offers it as **Scan again**.

### Target terms are proposed on their own

The server subscribes to `context.scan.completed` (published by the worker
when a term scan of a project completes), `recipe.changed` with a target
language added by a member, and `connector.push.completed` on the default
stream (consumer group `term-proposals-auto`,
`bowrain/server/review_plan_autopropose.go`). The triggers for one project
are coalesced: the job starts once they have been quiet for 30 seconds. It
starts the whole-project proposal job when a concept of the project with a
source term has no proposal and no usable term in a target language
(`termcandidate.NeedsTranslation`).

- The job is the platform's, on behalf of the project. It records no person;
  its proposals carry the identity `tool/term-proposal`, and the reviewer who
  keeps one is the person the keep is credited to.
- It is recorded in the project property `review_term_proposal_job`. A job
  never starts while the last one is queued or processing.
- It never starts with no platform AI provider, with the workspace over its
  monthly AI limit, or with no AI credits left. The plan counts the waiting
  pairs in `context.proposable`, carries the job in `context.proposals`, and
  says why none starts in `context.proposals_waiting` (`waiting_quota` with
  `quota_resets_at`, `waiting_credits`, `no_ai`). The context step carries the
  same as `steps[].proposals`: `queued` or `processing` while the job runs,
  or the reason it waits.

Keeping a proposed target term starts a term-forms job
(`jobs.TermFormsRequest`) for that concept and language, under the same
guard. The worker asks the platform AI for the forms through
`terms/expand`, the code behind `kapi terms expand`, bills the
tokens like a scan phase, and writes the forms onto the term as read when
the answer arrives. A term that declares forms already is not asked about.
The concept's revision time stays as the keep left it, so the context deck
does not deal the concept again.

### The settle pass runs after a run

The server subscribes to `convergence.run.completed` (consumer group
`review-settle-after-run`, `bowrain/server/review_plan_autosettle.go`). For a
run on the plan's stream that converged or parked after at least one
production pass, it runs the settle step's own code (`settleReviewPlan`) when
`policy.settle` is `first_pass` or `policy`, over every target language the
credited person may review.

- **Who it is credited to.** The person who last set the involvement
  (`review_involvement_set_by`, recorded by `PUT .../review/plan/involvement`),
  else the person who confirmed the coordinates. That person must be a
  member who holds the review permission on the project when the pass runs.
  The plan's `policy.auto` names them (`by`, `source`: `involvement` or
  `coordinates`), or says why no pass runs (`skip`: `no_one` or
  `no_permission`).
- **One pass at a time.** A pass claims the project on the instance and
  takes a PostgreSQL advisory lock keyed by the project, held on its own
  connection for the pass. A pass that finds the project claimed does not
  run: the manual step answers 409, and the pass after a run is skipped; the
  next run settles what it left. The lock is per project, so passes over
  different projects run side by side.
- **What the plan shows.** `policy.last` is the last pass, by hand or after a
  run: `at`, `approved`, `by`, and `automatic`. The phone shows it with the
  person the pass after a run is credited to, and has no settle button; the
  web Overview keeps the button.
- **The feed.** The pass files the same `review.bulk_approved` record per
  language as a pass by hand, with the credited person as the actor, so the
  feed's review summary counts it.

A completing run, the one a review that empties the queue starts, has no
production pass and is not followed by a settle pass.

## What the founder reviews on neokapi

In order, with the counts from 2026-11-19:

1. **Coordinates (about ten decisions).** The products `neokapi`, `bowrain` and
   `source`; their channels (`cli`, `engine`, `desktop`, `docs`, `demos`;
   `app`, `ctrl`, `pulse`, `docs`, `email`, `landing`, `mobile`; `comments`);
   the voice bound to each (`neokapi-documentation`, `bowrain-platform`,
   `source-comments`); the target language `nb` (with `qps` as a demo locale,
   not a translation); and that `source/comments` is source only.
2. **Context (tens of cards).** The project's concepts (57 in the last
   local snapshot), ranked by reach, each with its Norwegian term; the voice
   rules of the three profiles; the pending terms change set.
3. **Exceptions (593 blocks, expected to collapse to a few dozen rules).** The
   blocks that fail the bound checks, grouped by rule.
4. **Interfaces.** Under `context`: a sample of pages from each product's
   high-priority channels (CLI, desktop, app, mobile, landing). Under
   `interfaces`: every page of those channels.
5. **Settled by policy.** The documentation (21,996 blocks) and every other
   block that clears the bars.

## Decisions taken

The founder accepted each recommendation on 2026-11-19:

1. The default involvement for a new project is `context`.
2. A settle pass is attributed to the person who runs it, and each decision
   carries the reason `review policy` in the ledger. The pass the server runs
   after a run is attributed to the person who set the involvement, or who
   confirmed the coordinates. There is no `policy` actor.
3. A project settles on the checks alone once, for its first pass. After
   that, a settle pass waits until the context is decided: no context
   card waiting for anyone, and a target term for every concept in every
   target language. The plan reports this as `policy.settle`: `first_pass`,
   `policy`, `held` or `none`.
4. Under `context`, a reviewer reads one page in ten of each collection, the
   first of every ten in deck order, plus every page holding a translation
   with a failing finding. The plan counts that sample, and the deck by page
   answers it with `sample=true`.
5. `priority:` stays as the order within a level; involvement decides depth.
6. Keeping a candidate term makes it preferred directly; the keep is the
   review, and the attestation records it.
7. A kept term applies in the project it was kept in. Built. A concept carries
   the projects it is limited to in its `project_id` property (one id or
   several, comma-separated, read by `bowrain/termscope`); a concept without
   it applies in the whole workspace. Every surface that applies terms to a
   project reads the store through `termscope.StoreView`: the checks, the
   term gate, the translate prompts, the context deck, the plan's counts, the
   term scan and term proposals, and `GET /:ws/concepts?project=`, which a
   client's `kapi pull` asks with. `POST /:ws/concepts/:cid/widen`
   (`manage_terms`) makes a term apply in every project; the web Concepts
   page offers it as **Apply in every project**.

## Build order

1. The review plan endpoint (server), with the levels and counts. Built.
2. The plan on mobile and on the web Overview. Built.
3. Term candidates: sampling across coordinates, one candidate shape (a
   proposed concept with its reach), keep promotes it to preferred, reject
   is remembered on the project. Built for the project scan. A push starts
   the project scan when one is due (see "The term scan starts on a push"),
   and the plan's context step shows its state. AI-proposed
   translations of kept terms: built. Keeping a candidate starts a job that
   proposes the term in each target language (prompt `term.translate`, with
   the definition and up to three source blocks that use the term); the
   proposal sits on the concept as the property `term_proposal.<locale>`
   until a reviewer keeps it, edits it or turns it down
   (`term_proposal_rejected.<locale>`), and the deck deals it right after
   the source-term card, in the reviewer's languages.
   `POST /:ws/:id/context/term-proposals` runs the same job for the whole
   project, and the server runs it on its own (see "Target terms are
   proposed on their own"). Keeping a proposal, as filed or in the
   reviewer's wording, starts a job that writes the kept term's forms through
   the code behind `kapi terms expand` (`terms/expand`).
4. Context deck ordered by reach. Built for candidate terms; the rest of the
   deck keeps its order by kind. Sessions over the deck: built (see
   [Context sessions](#context-sessions)).
5. Exceptions grouped by rule. Built: the project-wide check's tally and
   each translation's deck facts keep a count per rule (rule id and subject:
   the term or the voice pattern, with the concept when a word rule comes
   from one), the plan's `exceptions.by_rule` names the top rules for the
   source and per language, and the review deck takes `?rule=` to narrow a
   language to one rule. The phone links each language's rule to that deck;
   the web Overview lists the rules without a link, because the web has no
   card deck to narrow.
6. Settling: built. The plan's last step (`POST /:ws/:id/review/plan/settle`)
   runs the approve passing path with the ledger reason `review policy`, under
   the first-pass rule above, credited to the caller. The server runs the same
   pass after each run (see "The settle pass runs after a run").
7. The interface sample under `context`. Built.
8. User guide: reviewing a large project. Built (bowrain/web/docs, getting started).
9. The coordinates map. Built. `GET /:ws/:id/review/coordinates` returns the
   plan's coordinates and the cells: one per product and channel, with the
   collections there, their translatable blocks, the voices bound, whether
   the cell is source only, and per target language the share of blocks
   established. Two grouped queries count it
   (`CollectionCoverageStore.CountCollectionCoverage`); no block is read.
   The phone draws it as products by channels, with a coverage bar per
   language, and the plan's coordinates step opens it.

   The baseline is shared, per project (founder rule: no cross-review unless
   a process asks for it). Confirming stores the main stream's map on the
   project as `review_coordinates_snapshot`: the points by collection, their
   coordinates, voice and source-only flag, and the target languages, with
   no counts. The plan's `coordinates.changes` lists what differs from that
   snapshot (`point_added`, `point_removed`, `point_moved`, `voice_changed`,
   `source_only_changed`, `language_added`, `language_removed`) and
   `coordinates.changed` counts them. A voice is compared by profile id, so
   renaming a profile is no change. While `changed` is above zero,
   `next` is `coordinates` for everyone; one confirmation by anyone records
   a new snapshot and clears it for the team. A project confirmed before the
   snapshot existed reads as confirmed with no changes, and its next
   confirmation records one. A branch stream is not compared with the
   snapshot. The phone passes the languages a person follows as the screen's
   `languages` parameter, which narrows the coverage bars.
10. Followed languages and the plan in the Review tab. Built. A person follows
    a list of target languages per project (`followed_languages`, Postgres
    migration 54, SQLite 45), read and saved through `GET` and `PUT
    /:ws/:id/review/languages`. No saved list means every language the
    membership allows; a saved list is kept to what the membership allows, and
    `PUT` refuses anything outside it. The list only filters: the review inbox
    and the review plan count the followed languages, and the queues stay
    shared. `GET /auth/me/review` also returns `projects`: each project with
    work waiting or a saved list, with its followed languages and, for a
    project with work waiting, the plan in brief (`plan.steps`: a count,
    progress and done per level, `changed` from the coordinates' change count,
    and the policy's settle state). The plan in brief is computed with the
    plan route's own code, once per project with work waiting, and not for the
    rest. A person with `manage_project` turns a target language on or off
    through `POST /:ws/:id/review/languages/project`, which files a pending
    recipe change on `defaults.target_languages` and a `recipe.changed` feed
    entry; the project's languages change when the next pull applies it. On
    the phone the Review tab shows, per project, the step it is on ("step 2
    of 5"), one Next card with the step's title, one fact and one button, the
    later steps as a short list with a count or a state word, and one line per
    language whose translations are held. A header chip opens the language
    settings, and a project's first plan visit opens a setup (managers:
    coordinates, project languages, involvement; reviewers: their languages).
    Involvement is set on that languages screen, by a manager. Still to build:
    notifications filtered by the followed list, and the setting on the web
    Overview.
11. The plan gates the language decks, and each step reports its state (see
    "The plan is the queue"). Built. The phone's plan screen is one row per
    step (state, title, one fact) with the action on the next step only; the
    context row reads the term scan's state, or what the scan waits
    for. The
    web Overview marks a step not started and lists the held translations.
