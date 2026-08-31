# Context discovery: from existing material to a governed project

An empty context is worth nothing, and nobody will write one from a blank page.
So the first act is **discovery**: turn the user's repo, site, and materials into
a working content context, a voice profile, a terminology seed, and the checks
that enforce both, and bind it to the content it governs.

That is a complete journey in one language. Governing the source the user
already has needs no second language, no server, and no provider credential.
Target languages are one more axis on the same context, and connecting the
project to a Bowrain server is a later step for teams who want review and
approval off the machine, both are the last sections of this file, and neither
is a prerequisite for any of the ones before it.

The user corrects a first draft instead of authoring one. **You** do the reading
and the drafting; kapi is the schema, the validator, and the gate.

This is the same correction loop that keeps the context current afterwards, not
a separate onboarding mode, so the second half of this file is the **refresh**
flow: diffing new material against a context that already exists.

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
  materializes in step 4; competitor names and banned phrasing belong in
  `voice.yaml`'s vocabulary lists instead (see [voice.md](voice.md)).

- **Content mapping**: which files the gates will watch: the `collections:` paths
  and formats, with `target:` patterns where translations already exist. Those
  existing translations need no import step, the loop recycles them as content memory
  leverage, and `kapi push` carries them as block targets.

  Surfaces with genuinely different registers belong at different **points**: a
  named collection per surface, each bound to a `channel:` of the profile that
  governs it. One voice profile carries them all; a channel override bends tone
  and style without loosening the vocabulary.

## 3. Review with the user

Render, score, iterate, the loop is [voice.md](voice.md)'s:

```bash
kapi voice guide --profile-file voice.yaml                    # the rendered guide
kapi voice check README.md --profile-file voice.yaml --json   # score one of their own files
```

Show the guide, the score and findings on their own text, and the term list.
Get explicit sign-off on the forbidden/competitor lists and every
`deprecated`/`forbidden` term, these will gate their builds. Fold feedback
into `voice.yaml` and re-render until the user agrees. Never invent competitors
or bans the user didn't confirm.

## 4. Bind

Create the project (or adopt the existing recipe, `kapi init` is idempotent):

```bash
kapi init --name my-app                                        # content project: voice + terms + check flow
kapi init --name my-app --target-locale fr --target-locale de  # translation project
```

Bind the context in the recipe:

```yaml
defaults:
  voice:
    profile_file: voice.yaml   # file binding, so `kapi apply` voice rules land in it
  terms_source: .kapi/terms.json   # the committed terms source
collections:
  - path: "docs/**/*.md"
    format: markdown
```

Materialize the terminology seed, now that the project exists:

```bash
# preferred: term entries: maintains the committed .kapi/terms.json
# source and reindexes it into .kapi/work/store.db in one verb
kapi apply terms.jsonl
# bulk path for a handed-over term list (csv, tsv, json, tbx, bundle):
kapi terms import terms.csv -s en -t fr --header
kapi terms import vocab.csv -s en --monolingual --header
```

```jsonl
{"kind":"term","op":"upsert","term":"dashboard","locale":"en","status":"preferred"}
{"kind":"term","op":"upsert","term":"control panel","locale":"en","status":"deprecated","replacement":"dashboard"}
```

The `apply` route keeps a committed source of truth; a bulk `terms import`
writes only the derived index, so commit the imported term list itself. Then
compile the committed context and verify the whole thing locally:

```bash
kapi up                      # reconcile the graph and the sources
kapi check --ship --json     # voice + terminology (+ QA) gates: all green
```

The recipe carries the bindings; the thresholds ride on the check itself: the
voice gate's score bar is `--min-score` (default 80), not a recipe field, and a
translation-coverage bar is an optional top-level `ship_gate:` (see
[translate.md](translate.md)). Say which of these the project's CI should run,
and on what: a check nobody runs governs nothing.

Commit the context: the recipe and all of `.kapi/`, the voice profile, the
sources under `.kapi/`, any imported term list, and `.kapi/state/`. Only
`.kapi/work/` stays gitignored. Committing is what makes the graph reviewable:
it arrives as untracked files next to content that is already committed, and the
user reads `git status` and `git diff` to decide.

## 5. Hand back a loop

End by telling the user, concretely:

- **What exists**: `kapi.yaml`, the voice profile, the committed terms source,
  and the content mapping.
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
kapi push --concepts    # reconcile the local terms into the server's terminology hub
```

**The bound voice profile travels with the push.** On a workspace project,
`kapi push` upserts it into the workspace brand hub, matched by profile name:
created on first push, a no-op when unchanged, a new server-side version when
it changed, server-side edits are archived in the version history, never
overwritten, and rules the server promoted from corrections are kept.
`--no-brand` skips it. The profile still travels in git, and `kapi check --ship`
enforces it wherever the repo is checked out (dev machines, CI).

Tell the user the claim URL while unclaimed, and the project and review URLs
once claimed.

---

# Notice when the context moves under you

A context bound to a server is shared, and it moves while you work. Two surfaces
report it, and both stop at reporting:

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

A context is drafted once and corrected forever. Refresh is the second visit:
the material has moved, and the record has to catch up, **as a proposal the
user approves, never as a rewrite you perform**. Nothing under `.kapi/` and
nothing in `kapi.yaml` changes before the user has seen the delta.

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

Time alone is not a trigger. A context nobody has changed in a year is not
stale; it is settled. Refresh what moved, and leave the rest alone.

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

The bound voice profile is the register baseline, read the file itself for the
exact vocabulary lists.

## 2. Draft the change-set

Five kinds of delta, each with a route that writes only what the entry names.
Every route **edits** the committed file rather than re-emitting it (comments
and key order survive) so the user reviews a diff the size of the decision.
Never hand-write a governance file you could reach through one of these routes:
a rewritten file loses the user's comments and buries the change.

| What moved | Route | What the user reviews |
| --- | --- | --- |
| A surface appeared | `kapi add <pattern> --name <collection> --channel <profile/channel>` | the `kapi.yaml` diff |
| A term, a name, a rename | `kapi apply` entry, `kind:"term"` | the terms source diff |
| A word to forbid or prefer | `kapi apply` entry, `kind:"voice"` | the voice profile diff |
| A brand or mode axis moved | `kapi apply` entry, `kind:"recipe"`, `path` `defaults.coordinates.<axis>` (or a collection's `coordinates`) and `value` | the `kapi.yaml` diff |
| Tone, style, `examples` | an edit to the profile YAML | the file diff |

Terms and voice rules go in one change-set file, applied atomically:

```jsonl
{"kind":"term","op":"upsert","term":"workspace","locale":"en","status":"preferred"}
{"kind":"term","op":"upsert","term":"team space","locale":"en","status":"deprecated","replacement":"workspace"}
{"kind":"voice","op":"add-rule","list":"competitor","term":"Globex","replacement":"our platform","severity":"major"}
```

These are the same `term` and `voice` entry kinds `kapi apply` always takes;
shapes in [edit.md → mixed change-sets](edit.md) and [voice.md](voice.md). A
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

Before approval, show the **blast radius** of every retirement. A retired term
starts flagging existing content on the next check, and a user who learns that
from a red build did not consent to it:

```bash
kapi terms occurrences "team space"      # where the word is used today
kgrep -r "team space" docs/              # the same question over files not yet extracted
```

Then apply only what the user approved, and drop the rest. Do not fold a
declined item into a later change-set "for consistency".

## 4. Apply, converge, verify

```bash
kapi apply refresh.jsonl         # terms + voice rules land atomically
kapi voice validate voice.yaml   # if you edited the profile directly
kapi up                          # recompile the graph and re-extract the sources
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
was published with. Say so rather than rewriting history; a `deprecated` term is
an advisory finding for exactly this reason, so the record and the gate can
disagree in public without blocking anybody.

## 6. Carry it to the server (only if connected)

```bash
kapi push --concepts    # reconcile the local terms with the workspace hub
kapi push               # the corrected content
```

The voice profile travels with `kapi push`, versioned server-side rather than
overwritten (§6 above). Terminology reconciles into the workspace hub, where a
term the server already holds is matched rather than duplicated.
