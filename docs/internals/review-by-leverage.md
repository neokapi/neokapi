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
interfaces and the docs share the key language. Deciding it once, at the
coordinates it spans, is what makes both right.

## The pyramid

| Level | What is decided | Unit | Reach of one decision |
| --- | --- | --- | --- |
| 1. Coordinates | products, channels, brand, languages; which voice governs each point; what each language receives | one decision per axis or binding | every block at the point |
| 2. Key language | terms (the source term, its forms, its translation per language, do-not-translate), names, voice rules per product | one card per concept or rule | every block that uses it |
| 3. Exceptions | blocks that fail a check against the key language, grouped by the rule they break | one decision per rule, then per block | the blocks the rule names |
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

### Level 2: key language

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

## How key language enters: term extraction today

The key-language cards are only as good as the candidates behind them, and
on a large project the candidates are the weakest link. What exists:

| Path | Reads | Proposes | Lands as |
| --- | --- | --- | --- |
| `term-extract` (AI, `core/ai/tools/terminology.go`) | one block | term, definition, domain | a block property; the overlay the schema promises is never written |
| DeeperLook (`kapi context suggest --ai`, `host/contextdeeper.go`) | the first 12 chunks of 2,000 characters, in file order | the 10 terms used in the most blocks | a term rule when the term appears in two spellings, otherwise a note |
| Context scan (`bowrain/jobs/contextscan_worker.go`) | the first 6 chunks of 12,000 characters, English hard-coded | terms plus the drafted voice's preferred forms | a scan result; approving one creates a concept with status proposed |
| `entity-extract` (AI) | one block | text, category, translatability, confidence | term-candidate overlay spans no queue reads |
| Spelling variants (no AI, `host/contextsuggest.go`) | the whole corpus | words and two-word phrases written two or more ways | ledger suggestions with a block count and the shared coordinates |

What this means for a project the size of neokapi:

- **The sample is the front of the corpus.** Both AI paths read the first 24k
  or 72k characters. On neokapi that is a slice of whichever collection sorts
  first, so the key language of the CLI, the apps and most of the docs is
  never seen.
- **Candidates live in four places**, each with its own shape: ledger
  suggestions, proposed concepts, the scan's result, overlay spans.
- **A key term with no variants becomes a note.** Keeping a note creates no
  concept, so the term never governs anything.
- **Nothing ranks by reach or by coordinate.** Scan candidates sit at the
  default point; the context deck orders by kind and time and does not show
  a concept's coordinates.
- **Nothing proposes a translation.** Target terms arrive by hand or by
  import; `kapi terms expand` adds forms to terms that already exist.

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

Involvement can be narrowed per coordinate, with the `when:` selector the ship
gates already use: a product's public channels at `interfaces`, its reference
docs at `context`. It is a property of a point in the context graph, not of a
file.

Before a gate is set, the review plan shows what each involvement would ask of
the reviewer at the project's current size: how many context cards, how many
exception groups, how many pages. A founder chooses with the numbers in front
of them.

## Source first

The source is reviewed through the same pyramid and ahead of the targets:

- the key language is decided in the source language first;
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

## What the founder reviews on neokapi

In order, with the counts from 2026-11-19:

1. **Coordinates (about ten decisions).** The products `neokapi`, `bowrain` and
   `source`; their channels (`cli`, `engine`, `desktop`, `docs`, `demos`;
   `app`, `ctrl`, `pulse`, `docs`, `email`, `landing`, `mobile`; `comments`);
   the voice bound to each (`neokapi-documentation`, `bowrain-platform`,
   `source-comments`); the target language `nb` (with `qps` as a demo locale,
   not a translation); and that `source/comments` is source only.
2. **Key language (tens of cards).** The project's concepts (57 in the last
   local snapshot), ranked by reach, each with its Norwegian term; the voice
   rules of the three profiles; the pending terms change set.
3. **Exceptions (593 blocks, expected to collapse to a few dozen rules).** The
   blocks that fail the bound checks, grouped by rule.
4. **Interfaces.** Under `context`: a sample of pages from each product's
   high-priority channels (CLI, desktop, app, mobile, landing). Under
   `interfaces`: every page of those channels.
5. **Settled by policy.** The documentation (21,996 blocks) and every other
   block that clears the bars.

## Decisions for the founder

1. The default involvement for a new project is `context`. Agree?
2. Policy establishment is attributed to the person who set the policy, with
   the reason `review policy`. Or should it carry an actor of its own
   (`policy`) in the ledger?
3. A block settles by policy only when the context it was produced under is
   approved. That holds back every block on neokapi until level 2 is decided.
   Agree, or allow settling on checks alone for a project's first pass?
4. Sample size for interfaces under `context`: one page in ten per channel,
   with every page that has a finding. Agree?
5. Does `priority:` stay as the ordering within a level, or is it replaced by
   involvement per coordinate?

## Build order

1. The review plan endpoint (server), with the levels and counts. Built.
2. The plan on mobile. Built; the web Overview next.
3. Term candidates: sampling across coordinates, one candidate shape, keep
   creates a scoped concept, translations proposed (the section above).
4. Context deck ordered by reach, source term and translations together.
5. Exceptions grouped by rule (the check tally gains a count per rule).
6. Involvement in the recipe (`defaults.review.involvement`, per coordinate
   under `when:`) and policy establishment on the server, through the approve
   passing path, with the ledger reason. Until then the plan's last step
   applies approve passing on request.
7. User guide: reviewing a large project.
