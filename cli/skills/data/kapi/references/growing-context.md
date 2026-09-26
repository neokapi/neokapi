# Growing a project's context

Project context records writing guidance, terms and approved wording. Build it
from observations during ordinary work or through a dedicated discovery session.
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
kapi context observe "the docs address the reader as you" --seen-in docs/guide.md
kapi context observe --term Quickcast --instead-of "Quick cast" --seen-in README.md --quote "Quickcast forecasts the next hour"
kapi context correct "sign in" "log in" --seen-in web/src/auth.tsx --suggest
```

Over MCP the same two are `context_observe` (with `term` and `instead_of`) and
`context_correct`.

- **Observe** what you notice. A fact in prose, such as who the text addresses
  or the register it keeps, states no rule. A name or spelling the project keeps
  to is a rule: pass `--term` with the form the project uses and `--instead-of`
  with a form it avoids, and kapi adds the spacing, hyphen and case variants
  (`Quickcast` avoids `Quick cast`, `Quick-cast`, `QuickCast` and `quickcast`).
  Everything recorded is a **suggestion**: every check reports it, and no check
  can fail on it until a person's signal backs it. Never tell the user a rule
  is in force because you observed one.
- **Correct** when the user changes your wording, and only then: a correction
  is the person's signal, so a correction toward an existing suggestion
  establishes it. `--suggest` also records the implied rule for review. A
  correction that reverses an established rule contests that rule, which then
  reports instead of failing.

Attach evidence with `--seen-in` for the file and `--quote` for the wording.
Term rules and corrections require evidence so reviewers can compare the
proposal with its source.

Two suggestions that name different forms for one word are **contested**: both
advise, each names the other, and a person chooses by dropping one.

One call records one thing. Do not batch a session's worth of observations into
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

## Reviewing and undoing

```bash
kapi context log --status suggested       # what is waiting for a decision
kapi context log --status contested       # what disagrees with another rule
kapi context log --session this --json    # what this run recorded
kapi context log --session s4f1c2 --json  # what one agent run recorded
kapi context keep 0n794e2gk7 0n79gkq853   # establish rules, and write them
kapi context keep --session s4f1c2        # keep everything one run suggested
kapi context drop 0n794e2gk7              # set a suggestion aside
kapi context revert --session s4f1c2      # undo everything one run recorded
kapi context widen 0n794e2gk7 --to workspace # put an established rule in force everywhere
```

Keeping, dropping, reverting and widening belong to a person. An agent that
tries is refused, and told so. You may still withdraw a suggestion you recorded
yourself when it turns out wrong: over the CLI with `kapi context withdraw
<id>`, and over MCP with `context_withdraw`. That is how a
run cleans up after itself. For everything else, end your task by reporting what
you recorded and the command above for reviewing it, and let the user decide.

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
  kapi context observe --term dashboard --instead-of "control panel" --seen-in docs/guide.md
  kapi context observe --term "our platform" --instead-of Globex --seen-in web/src/pricing.tsx
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
kapi voice guide --profile-file voice.yaml                    # the rendered guide
kapi voice check README.md --profile-file voice.yaml --json   # score one of their own files
```

Show the guide, the score and findings on their own text, and the term list.
Get explicit sign-off on every forbidden and competitor term and every
`deprecated` one, these will gate their builds. Fold feedback
into `voice.yaml` and re-render until the user agrees. Never invent competitors
or bans the user didn't confirm.

Where you recorded suggestions rather than drafting a file, the review list is
the log, and the decisions are the log's verbs:

```bash
kapi context log --status suggested    # each entry with its evidence
kapi context keep 0n794e2gk7 --use dashboard    # keep, editing the rule as you go
kapi context drop 0n79gkq853
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

Then point the next assistant at the voice you bound:

```bash
kapi voice pointer     # a section in CLAUDE.md (or an AGENTS.md already there) naming the voice
```

It writes a few sentences: the project's voice is held by kapi, it applies to
prose written here, and `kapi voice guide` retrieves it. A recipe that places
comments at a point of their own also gets a sentence naming
`kapi voice guide --comments <path>`, the command for a comment's voice. Nothing
about how to write; the guide stays one command away. `kapi init` writes the same section
when the scaffold it creates binds a voice, and the recipe edit above is what it
names, so run it after the edit. The section sits between `<!-- kapi:voice -->`
markers and is replaced in place; hand-written content around it is kept. Tell
the user which file it went into: they commit it with the recipe, and a section
that landed in `AGENTS.md` reaches an assistant limited to `CLAUDE.md` through
an `@AGENTS.md` line.

Materialize the terminology seed, now that the project exists:

```bash
# preferred: term entries write the project's terms store and record a
# context operation for each, so `kapi context log` carries the decision
kapi apply terms.jsonl
# bulk path for a handed-over term list (csv, tsv, json, tbx, bundle):
kapi terms import terms.csv -s en -t fr --header
kapi terms import vocab.csv -s en --monolingual --header
```

```jsonl
{"kind":"term","op":"upsert","term":"dashboard","locale":"en","status":"preferred"}
{"kind":"term","op":"upsert","term":"control panel","locale":"en","status":"deprecated","replacement":"dashboard"}
```

The `apply` route records each term as a decision the user can read back and
undo; a bulk `terms import` writes the store without recording one. Then verify
the whole thing locally:

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
the assistant file. `.kapi/` is this checkout's cache and stays out of the
commit. The context itself stays in the project's store, where every checkout
reads it; `kapi context log` is where the user reads what was decided, and
`kapi context export -o backup.kpz` is the backup. If the user works on more than
one machine, or with other people, tell them about a context backend
(`context: {backend: git}` in kapi.yaml, then `kapi context pull` and
`kapi context push`) and let them decide.

## 5. Document the workflow

End by telling the user, concretely:

- **What exists**: `kapi.yaml`, the voice profile and terms in the project's
  store, the content mapping, and the assistant file (`CLAUDE.md`, or an
  `AGENTS.md` already at the root) that points the next assistant at the
  voice.
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
kapi voice guide                    # the bound profile, rendered
kapi terms export --format json     # every concept; --format csv -s en -t fr for one pair
```

`kapi ls --untracked` is the surfaces half of the diff: what is on disk, minus
what the recipe declares. It reports and never adopts: a file appearing there
is a candidate, not a decision. Expect true positives the user will decline (a
README, a fixture, a vendored page); that is the report working.

The bound voice profile is the register baseline, and the project's terms are
the word list: `kapi voice guide <file>` prints both for a file.

## 2. Draft the change-set

Use the route appropriate to each proposed change. Each context operation
records the affected rule for review. Never hand-edit a project's context: the routes
below are how it changes, and `kapi context log` is how the user reads it back.

| What moved | Route | What the user reviews |
| --- | --- | --- |
| A surface appeared | `kapi add <pattern> --name <collection> --channel <profile/channel>` | the `kapi.yaml` diff |
| A term, a name, a rename, a word to avoid | `kapi context observe --term`, or a `kapi apply` entry, `kind:"term"` | the suggestion with its evidence, in `kapi context log` |
| A brand or mode axis moved | `kapi apply` entry, `kind:"recipe"`, `path` `defaults.coordinates.<axis>` (or a collection's `coordinates`) and `value` | the `kapi.yaml` diff |
| Tone, style, `examples` | an edit to the profile YAML | the file diff |

Two routes for the same two kinds, and the difference is who decides.
`kapi context observe --term` records a suggestion the user keeps, each carrying
the file it came from; a `kapi apply` change-set lands what the user has already
approved, atomically. Reach for the first when you are reading material and
suggesting what it implies, and for the second when the decisions are already
made. Every word rule is a term, so they go in one change-set file:

```jsonl
{"kind":"term","op":"upsert","term":"workspace","locale":"en","status":"preferred"}
{"kind":"term","op":"upsert","term":"team space","locale":"en","status":"deprecated","replacement":"workspace"}
{"kind":"term","op":"upsert","term":"Globex","locale":"en","status":"forbidden","replacement":"our platform","competitor":true}
```

These are the `term` entries `kapi apply` takes (`"advisory":true` makes a
use report without failing); shapes in [edit.md → mixed change-sets](edit.md)
and [voice.md](voice.md). A
declared axis moves through the `recipe` kind, one axis per entry
(`{"kind":"recipe","path":"defaults.coordinates.brand","value":"acme"}`); an
empty `value` withdraws the axis, and `product` or `channel` are refused there
because both derive from a collection's `channel:`.
Tone, style, and `examples` changes have no entry kind: propose them as an edit
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

Present the change-set as **adds / retires / replaces**, each with its evidence:
where the new term appeared in the material, what the old one conflicts with,
which file the new surface came from.

Before approval, show where each term proposed for retirement appears.
These occurrences may produce findings on the next check:

```bash
kapi terms occurrences "team space"      # where the word is used today
kgrep -r "team space" docs/              # the same question over files not yet extracted
```

Then apply only what the user approved, and drop the rest. Do not fold a
declined item into a later change-set "for consistency".

## 4. Apply, converge, verify

```bash
kapi apply refresh.jsonl         # terms + voice rules land atomically
kapi up                          # reconcile the graph and re-extract the sources
kapi check --ship --json         # the refreshed gates
```

Read the result as three separate facts: what the change-set applied, what the
new gates now flag, and what the user has to do about it. A refresh that ends on
a red gate is normal: the findings are the work the decision created.

## 5. Sweep what the retirement now flags

A newly retired term flags old usage everywhere it survives. Sweep for it while
you are here (`kgrep`, [toolbox.md](toolbox.md)) and fix the hits through
`kapi apply` content entries ([edit.md](edit.md)), one change-set, reviewed the
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
