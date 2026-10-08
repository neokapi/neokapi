# Growing a project's context

Project context records writing guidance, terms and approved wording. Build it
from notes during ordinary work or through a dedicated discovery session.
Both approaches record operations in the context log. A suggestion becomes an
established rule when a person's signal backs it and nothing contradicts it: a
person keeps it, a correction toward it is recorded, or a change writing it
reaches the default branch. kapi then writes it through the same appliers as
other context edits.

During ordinary work, record useful facts, consistent terminology and user
corrections as you encounter them. Use a discovery session for a first review
of the material or a substantial update to existing guidance.

## Everyday growth: two calls

```bash
kapi context note "the docs address the reader as you" --seen-in docs/guide.md
kapi context note --term Quickcast --instead-of "Quick cast" --seen-in README.md --quote "Quickcast forecasts the next hour"
kapi context note --from "sign in" --to "log in" --seen-in web/src/auth.tsx --suggest
```

Over MCP the same calls are `context_note`: with `term` and `instead_of`, or
`text`, for what you noticed, and with `from`, `to` and `suggest` for a change
the person made.

- **Note** what the project's files do every time, whether or not your own
  text needs it. Look for three kinds in every project:
  - each product, feature and plan name, as the files write it;
  - the spelling variety (British or American), as a fact in prose with two or
    three words that show it;
  - a word the files always use where writers often use another.

  A name or word is a rule: pass `--term` with the form the files use and
  `--instead-of` with the form they avoid, which is the split form of a
  one-word name, the other spelling, or the other word. kapi adds the spacing,
  hyphen and case variants (`Quickcast` avoids `Quick cast`, `Quick-cast`,
  `QuickCast` and `quickcast`). A fact in prose, such as who the text addresses
  or the register it keeps, states no rule.

  Before you record a word, search the files for its other forms (spaced,
  hyphenated, closed, the other spelling). A word they write more than one way
  is a choice the project has not made, so leave it for a person: record
  nothing about it, not even a note describing the forms. Leave alone
  interface labels, wording taken from your task, anything seen once, and a
  name you introduced yourself; `--seen-in` names a file that was there before
  you started.

  Everything recorded is a **suggestion**: every check reports it, and no check
  can fail on it until a person's signal backs it. Never tell the user a rule
  is in force because you noted one.
- **Record a person's change** (`--from`, `--to`) when the user changes your
  wording, and only then: their change is the person's signal, so a correction toward an existing suggestion
  establishes it. `--suggest` also records the implied rule for review. A
  correction that reverses an established rule contests that rule, which then
  reports instead of failing.

Attach evidence with `--seen-in` for the file and `--quote` for the wording.
Term rules and corrections require evidence so reviewers can compare the
proposal with its source.

Two suggestions that name different forms for one word are **contested**: both
advise, each names the other, and a person chooses one in `kapi context review`.

One call records one thing. Do not batch a session's worth of notes into
a single sentence, and do not wait until the end of the task to record them.

A `kapi context <path>` answer lists the suggestions at that point, each with
its standing as plain counts (`seen in 3 sessions · 14 of 15 uses in docs/ ·
merged in #412`), so a later session builds on what an earlier one recorded
instead of working it out again. Recording the same rule again, or applying an
edit that writes its preferred form, adds to its standing and establishes
nothing.

## What the entry says about you

Every operation carries who recorded it and, for an agent, a session that groups
one run. kapi works both out for itself: it recognises the common agent hosts
from the environment they put a command in, and over MCP the server knows. So
these calls take no flag naming you, and an entry you record reads back under
`kapi context log --actor agent`.

In a host kapi does not recognise, set `KAPI_AGENT_NAME` to what you are called
and `KAPI_AGENT_SESSION` to an id that holds still for this task and differs
from the last one.

Read your own run back with `kapi context log --session this`, which is the
session this command resolves for itself. End your task report with what it
says.

## Reading back, and what a person decides

```bash
kapi context log --status suggested       # what is waiting for a decision
kapi context log --status contested       # what disagrees with another rule
kapi context log --session this --json    # what this run recorded
kapi context log --session s4f1c2 --json  # what one agent run recorded
```

Withdraw a note of your own when it turns out wrong, in the session that
recorded it: `kapi context note --withdraw <id> --why "<what was wrong>"`, or
`context_note` with `withdraw` over MCP. That is how a run cleans up after
itself.

Everything else belongs to a person, and kapi refuses it from an agent:
keeping, dropping, choosing a side, applying a rule more widely, going back to
an earlier state (`kapi context reset`), and sharing (`kapi context sync`). End
your task by reporting what you recorded and telling the user how to decide
about it:

```bash
kapi context review                       # walk the suggestions one by one
kapi context review --session s4f1c2      # only what one run suggested
```

A person reviews whenever it suits them, in a terminal or in the Learned
section of Kapi Desktop. Nothing you record waits on that review, because a
suggestion advises from the moment it is recorded.

---

# Deliberate discovery: from existing material to a governed project

Turn the user's repo, site, and materials into a working content context: a
voice profile, a terminology seed, and the checks that enforce both, bound to
the content it governs.

Start with the user's source content. This workflow works in one language
without a server or provider credentials. Add target languages or a Bowrain
connection when the project needs them.

Read the material and prepare a draft for the user to review. Use kapi to
validate the profile and check its effect on the content. For an existing
profile, follow *Refresh an existing context* below.

## 1. Gather

Collect signal before drafting anything:

- **The repo.** README, docs, marketing copy, UI strings, existing catalogs.
- **The website.** Fetch the key pages yourself (your web tool, or `curl`),
  home, product, about, one blog post, and read the live copy.
- **Materials in opaque formats.** `kcat brochure.docx` prints the prose of a
  Word file or a deck ([toolbox.md](toolbox.md)); `kapi stats <file> --json`
  sizes what's there.
- **Existing translations.** Note target-language files (`fr.json`,
  `docs/de/…`), they become `target:` mappings, not waste.
- **Names.** Product names, feature names, competitor names.

When the signal is thin, ask, don't pad the profile with generic filler:

- Who is the audience, and how formal should the voice be?
- Which competitors should never be named, and what to say instead?
- Which terms are house style ("sign in", not "log in")? Which are banned?
- Is there a paragraph they consider perfectly on-brand? (It becomes an
  `examples` entry.)
- Which target languages, if any?

## 2. Draft the context

Three artifacts, all plain files the user can review before anything binds:

- **Voice profile**: scaffold, fill, validate:

  ```bash
  kapi voice new -o voice.yaml                       # commented template
  kapi voice new --pack friendly-dtc -o voice.yaml   # or seed from the closest built-in pack
  kapi voice validate voice.yaml                     # exit 0 = schema-valid
  ```

  The drafting craft, what to infer from which signal, how concrete to be,
  weak→strong `examples`, is [voice.md → Create a profile](voice.md); follow
  it, don't improvise a schema.

- **Terminology seed**: draft the term list as a table you can show the user:
  term, status (`preferred`, `admitted`, `deprecated`, `forbidden`, or
  `proposed`), replacement for retired terms, and known translations. It
  materializes in step 4. Competitor names and banned words are terms too:
  list them under `terms:` in `voice.yaml` (`competitor: true` for a rival's
  name), and importing the file moves them into the project's terms (see
  [voice.md](voice.md)).

  In a project that already exists, record each rule you read out of the
  material as a suggestion instead, one call per rule, with the file it came
  from:

  ```bash
  kapi context note --term dashboard --instead-of "control panel" --seen-in docs/guide.md
  kapi context note --term "our platform" --instead-of Globex --seen-in web/src/pricing.tsx
  ```

  The user then reviews a list where every entry carries its evidence, and
  keeping writes each one into the project's store. Tone, style and
  `examples` carry no rule, so they stay an edit the user makes with
  `kapi voice edit`.

- **Content mapping**: which files the gates will watch: the `collections:` paths
  and formats, with `target:` patterns where translations already exist. Those
  existing translations need no import step, the loop recycles them as content memory
  leverage, and `kapi push` carries them as block targets.

  Surfaces with genuinely different registers belong at different **points**: a
  named collection per surface, each bound to a `channel:` of the profile that
  governs it. One voice profile carries them all; a channel override bends tone
  and style. Terms apply on every channel.

## 3. Review with the user

Render, score, iterate, the loop is [voice.md](voice.md)'s:

```bash
kapi voice show --profile-file voice.yaml                     # the rendered guide
kapi voice check README.md --profile-file voice.yaml --json   # score one of their own files
```

Show the guide, the score and findings on their own text, and the term list.
Get a person to confirm every forbidden and competitor term and every
`deprecated` one, these will gate their builds. Fold feedback
into `voice.yaml` and re-render until the user agrees. Never invent competitors
or bans the user didn't confirm.

Where you recorded suggestions rather than drafting a file, the user decides
about them in review:

```bash
kapi context log --status suggested             # each entry with its evidence
kapi context review                             # the user keeps or drops each one
kapi context review --keep 0n794e2gk7 --use dashboard   # keep, editing the rule as you go
kapi context review --drop 0n79gkq853
```

## 4. Bind

Create the project (or adopt the existing recipe, `kapi init` is idempotent):

```bash
kapi init --name my-app                                        # collections proposed from the tree
kapi init --name my-app --target-locale fr --target-locale de  # and target languages to translate into
```

Bind the context in the recipe:

```yaml
defaults:
  voice:
    profile: my-app-docs   # the id `kapi voice import` printed
collections:
  - path: "docs/**/*.md"
    format: markdown
```

Then write the rules where the next assistant loads them:

```bash
kapi context sync --files-only   # the project's rules into AGENTS.md and CLAUDE.md
```

kapi writes the rules that hold in each folder into a section of the
`AGENTS.md` and `CLAUDE.md` there: the root's for the rules that hold
everywhere, and one in each folder whose rules differ. A rename held for
`help/` gives `help/AGENTS.md` the new name and `api/AGENTS.md` a line saying
the old name is correct there. Each section states the voice, what to write
and what not, the wording to keep, and the `kapi check` line, and stays short;
`kapi context <path>` gives the full answer for one file. An agent follows the
section without asking first, and `kapi check` flags the new name in a folder
whose section says to keep the old one. `kapi init` writes the same
files, and kapi rewrites them when the context changes: after
`kapi context review`, `kapi context sync` and `kapi up`. The section sits
between `<!-- kapi:rules -->` markers; hand-written content around it is kept.
Tell the user which files changed: they commit them with the recipe.

Materialize the terminology seed, now that the project exists:

```bash
# preferred: each term is a suggestion with its evidence, in `kapi context log`
kapi context note --term dashboard --instead-of "control panel" --seen-in docs/guide.md
# bulk path for a handed-over term list (csv, tsv, json, tbx, bundle):
kapi terms import terms.csv -s en -t fr --header
kapi terms import vocab.csv -s en --monolingual --header
```

The user keeps the suggestions they agree with in `kapi context review`
(`kapi context review --session <id> --keep all` keeps one run's). A person who has already decided can instead write the
terms as `term` operations and run `kapi apply` on them, which records each as
their decision; from your shell `kapi apply` refuses a `term` operation. A bulk
`terms import` writes the store without recording a decision. Then verify the
whole thing locally:

```bash
kapi up                      # reconcile the graph and the content
kapi check --ship --json     # voice + terminology (+ rule-based) gates: all green
```

The recipe carries the bindings. The voice gate fails on a failing finding and
reports the compliance score beside it; a translation-coverage bar is an
optional top-level `ship_gate:` (see
[translate.md](translate.md)). Without one, `kapi status` and `kapi up` report
each language as not gated rather than shippable. Say which of these the
project's CI should run and which files it should check.

Commit the configuration: `kapi.yaml`, the agent wiring `kapi init` wrote, and
the rules files. `.kapi/` is this checkout's cache and stays out of the
commit. The context itself stays in the project's store, where every checkout
reads it; `kapi context log` is where the user reads what was decided, and
`kapi store export -o backup.kpz` is the backup. If the user works on more than
one machine, or with other people, tell them about sharing the context
(`context: {backend: git}` in kapi.yaml, then `kapi context sync`) and let them
decide.

## 5. Document the workflow

End by telling the user, concretely:

- **What exists**: `kapi.yaml`, the voice profile and terms in the project's
  store, the content mapping, and the rules files (`AGENTS.md` and
  `CLAUDE.md`, at the root and in each folder with rules of its own) that
  state the rules for the next assistant.
- **The standing instruction**: run `kapi check --ship` before shipping content
  and fix what it flags ([project.md](project.md)); in a translation project,
  `kapi up` catches locales up ([translate.md](translate.md)).
- **When to come back**: the triggers in *Refresh an existing context*, below.

## 6. Connect to a Bowrain server (optional, last)

A project is complete without this. Connect it when the user wants review,
approval and terminology shared off the machine.

Bowrain commands come from the `kapi-bowrain` plugin. Once the recipe declares
a `bowrain:` block, running one of its verbs without the plugin installed offers
the install rather than failing, on a terminal it prompts, and `--yes` accepts
without asking. To install it up front instead:

```bash
kapi plugins list
kapi plugin install bowrain             # from the plugin registry
brew install neokapi/tap/bowrain-cli    # or via Homebrew
```

Connect, `kapi init --server` adds the `bowrain:` block to the existing recipe
(idempotent; an already-connected project is a no-op):

```bash
# no account yet: create the project anonymously, hand over a claim link
kapi init --server https://app.bowrain.cloud --anonymous
#   prints:  claim:   <server>/claim/<token>
#   --email <addr> emails the claim link instead

# signed in: create it in their workspace
kapi auth login --server https://app.bowrain.cloud   # device flow: URL + code
kapi init --server https://app.bowrain.cloud         # --workspace <slug> if they have several
```

Then push:

```bash
kapi push        # changed blocks, including any existing translations as targets
```

On a workspace project, push prints the project and review URLs and, with the
default `converge: on-push`, the server picks up translation, checks, and
review queueing from there. An anonymous project pushes too (the machine that
ran init holds the claim token), but prints no workspace URLs until claimed.

**Terminology and the voice profile reach the server after the claim.** Both
are workspace-scoped, so an anonymous project's first push carries content
only. Once the user has claimed, they open the claim URL, or you run
`kapi auth login` then `kapi auth claim` (which also rebinds the recipe to the
workspace URL):

```bash
kapi pull               # establish the concept baseline
kapi push               # synchronize content and declared context
```

**The bound voice profile travels with the push.** On a workspace project,
`kapi push` upserts it into the workspace brand hub, matched by profile name:
created on first push, a no-op when unchanged, a new server-side version when
it changed, server-side edits are archived in the version history, never
overwritten, and rules the server promoted from corrections are kept.
Configure the voice binding in the recipe. Use `kapi check --ship` to enforce
it in each environment where the project context is available.

Tell the user the claim URL while unclaimed, and the project and review URLs
once claimed.

---

# Respond to context changes

Shared context can change during a task. Use these reports to detect updates:

- `kapi status` prints a **governance** line: `in sync`, or which of the
  context, the terms and the decisions moved since this project last observed
  the server. Movement is not distance: two identities that differ carry no
  ordering, so the line names what moved and never how far behind you are.
- A `context_search` answer carries a **note** when any of them moved since you
  last read them in this session. Re-read the context before continuing; the
  answer you were working from describes a graph that has changed.

`kapi check` is the enforcing half: content produced under a context that has
since been superseded fails the staleness gate, naming what moved. Re-running
`kapi up` reproduces it under the context now in force.

---

# Refresh an existing context

When the material changes, compare it with the existing context and propose
updates for review. Show the proposed changes before editing `.kapi/` or
`kapi.yaml`.

## When to refresh

Refresh on a **change in the material**, not on a calendar:

- **A surface appeared.** A directory of content nobody declared, a support
  site, a new app, docs that moved. `kapi ls --untracked` is how you see it.
- **A name changed.** A product, a feature, a company. The record still says
  the old one, and the new material says the new one.
- **The register drifted.** New copy the user is happy with keeps scoring badly,
  or the guide describes a voice the material no longer has.
- **The same finding keeps coming back**, and the user keeps overriding it. A
  rule that is always wrong in one place is a rule that needs a decision.
- **The user asks.** "Our brand changed", "we renamed X", "refresh our context".

Base the refresh on changed material or requirements. Preserve guidance that
still applies.

## 1. Read the baseline, and write nothing

This whole step is read-only. Do not edit a governance file while you are still
working out what changed.

```bash
kapi ls --untracked                 # readable files no collection governs
kapi context docs/guide.md          # what governs one of the new files
kapi context search "Tidewatch"     # what the record says about a name
kapi terms export --format json     # every concept; --format csv -s en -t fr for one pair
```

`kapi ls --untracked` is the surfaces half of the diff: what is on disk, minus
what the recipe declares. It reports and never adopts: a file appearing there
is a candidate, not a decision. Expect true positives the user will decline (a
README, a fixture, a vendored page); that is the report working.

The bound voice profile is the register baseline, and the project's terms are
the word list: `kapi context <file>` prints both for a file.

## 2. Draft the change set

Use the route appropriate to each proposed change. Each context operation
records the affected rule for review. Never hand-edit a project's context: the routes
below are how it changes, and `kapi context log` is how the user reads it back.

| What moved | Route | What the user reviews |
| --- | --- | --- |
| A surface appeared | `kapi add <pattern> --name <collection> --channel <profile/channel>` | the `kapi.yaml` diff |
| A term, a name, a rename, a word to avoid | `kapi context note --term`; a `kapi apply` `term` operation, for the user to apply | the suggestion with its evidence, in `kapi context log` |
| A brand or mode axis moved | a `kapi apply` `recipe` operation, `path` `defaults.coordinates.<axis>` (or a collection's `coordinates`) and `value`, for the user to apply | the `kapi.yaml` diff |
| Tone, style, `examples` | an edit to the profile YAML | the file diff |

Two routes for the same two kinds, and the difference is who decides.
`kapi context note --term` records a suggestion the user keeps, each carrying
the file it came from. A `kapi apply` change set lands what the user has already
decided, recorded as theirs, so the user runs it: kapi records each operation
as whoever runs the command, and from your shell it refuses `term` and `recipe`
operations. Reach for the first when you are reading material and suggesting
what it implies, and draft the second for the user when the decisions are
already made. Every word rule is a term, so they go in one change set, one
operation per line:

```jsonl
{"op":"term","action":"upsert","term":"workspace","locale":"en","status":"preferred"}
{"op":"term","action":"upsert","term":"team space","locale":"en","status":"deprecated","replacement":"workspace"}
{"op":"term","action":"upsert","term":"Globex","locale":"en","status":"forbidden","replacement":"our platform","competitor":true}
```

These are the `term` operations `kapi apply` takes (`"advisory":true` makes a
use report without failing); shapes in [edit.md → mixed change sets](edit.md)
and [voice.md](voice.md). A declared axis moves through a `recipe` operation,
one axis per operation
(`{"op":"recipe","path":"defaults.coordinates.brand","value":"acme"}`); an
empty `value` withdraws the axis, and `product` or `channel` are refused there
because both derive from a collection's `channel:`.
Tone, style, and `examples` changes have no operation: propose them as an edit
to the profile YAML and show the diff.

A new surface takes the point that suits it. `--name` puts the pattern in a
named collection instead of a bare entry, and `--channel` binds that collection
to a point in the context space, so the new content is governed by the register
that fits it rather than by the project default:

```bash
kapi add "support/**/*.md" --name acme-support --channel acme/docs
```

If no declared channel fits, that is itself a proposal: a new channel on the
profile, which is a recipe edit the user reviews.

## 3. Review with the user

Present the change set as **adds / retires / replaces**, each with its evidence:
where the new term appeared in the material, what the old one conflicts with,
which file the new surface came from.

Before approval, show where each term proposed for retirement appears.
These occurrences may produce findings on the next check:

```bash
kapi terms occurrences "team space"      # where the word is used today
kgrep -r "team space" docs/              # the same question over files not yet extracted
```

Then apply only what the user approved, and drop the rest. Do not fold a
declined item into a later change set "for consistency".

## 4. Apply, converge, verify

```bash
kapi apply refresh.jsonl         # every term lands, or none does; fix a refused one and re-run
kapi up                          # reconcile the graph and re-extract the sources
kapi check --ship --json         # the refreshed gates
```

Read the result as three separate facts: what the change set applied, what the
new gates now flag, and what the user has to do about it. A refresh that ends on
a red gate is normal: the findings are the work the decision created.

## 5. Sweep what the retirement now flags

A newly retired term flags old usage everywhere it survives. Sweep for it while
you are here (`kgrep`, [toolbox.md](toolbox.md)) and fix the hits through
`kapi apply` content operations ([edit.md](edit.md)), one change set, reviewed the
same way.

Some hits are legitimate: a changelog entry or an API field keeps the name it
was published with. Say so rather than rewriting history; a `deprecated` term
raises a finding that reports and never fails for exactly this reason, so the
record and the gate can disagree in public without blocking anybody.

## 6. Carry it to the server (only if connected)

```bash
kapi push               # synchronize corrected content and declared context
```

The voice profile travels with `kapi push`, versioned server-side rather than
overwritten (§6 above). Terminology reconciles into the workspace hub, where a
term the server already holds is matched rather than duplicated.
