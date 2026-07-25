# Onboard a brand: starter pack → Bowrain

Turn the user's site, repo, and materials into a working **brand starter
pack** — a voice profile, a terminology seed, and the checks that enforce
both — then connect the project to a Bowrain server. **You** do the reading and
the drafting; kapi is the schema, the validator, and the transport. The second
half of this file is the **refresh** flow: diffing new material against a pack
that already exists.

## 1. Gather

Collect signal before drafting anything:

- **The website.** Fetch the key pages yourself (your web tool, or `curl`) —
  home, product, about, one blog post — and read the live copy.
- **The repo.** README, docs, marketing copy, UI strings, existing catalogs.
- **Materials in opaque formats.** `kcat brochure.docx` prints the prose of a
  Word file or a deck ([toolbox.md](toolbox.md)); `kapi stats <file> --json`
  sizes what's there.
- **Existing translations.** Note target-language files (`fr.json`,
  `docs/de/…`) — they become `target:` mappings, not waste.
- **Names.** Product names, feature names, competitor names.

When the signal is thin, ask — don't pad the profile with generic filler:

- Who is the audience, and how formal should the voice be?
- Which competitors should never be named, and what to say instead?
- Which terms are house style ("sign in", not "log in")? Which are banned?
- Is there a paragraph they consider perfectly on-brand? (It becomes an
  `examples` entry.)
- Which target languages, if any?

## 2. Draft the pack

Three artifacts, all plain files the user can review before anything binds:

- **Voice profile** — scaffold, fill, validate:

  ```bash
  kapi brand new -o brand.yaml                       # commented template
  kapi brand new --pack friendly-dtc -o brand.yaml   # or seed from the closest pack
  kapi brand validate brand.yaml                     # exit 0 = schema-valid
  ```

  The drafting craft — what to infer from which signal, how concrete to be,
  weak→strong `examples` — is [brand.md → Create a profile](brand.md); follow
  it, don't improvise a schema.

- **Terminology seed** — draft the term list as a table you can show the user:
  term, status (`preferred`, `admitted`, `deprecated`, `forbidden`, or
  `proposed`), replacement for retired terms, and known translations. It
  materializes in step 4; competitor names and banned phrasing belong in
  `brand.yaml`'s vocabulary lists instead (see [brand.md](brand.md)).

- **Content mapping** — which files the gates will watch: the `content:` paths
  and formats, with `target:` patterns where translations already exist. Those
  existing translations need no import step — the loop recycles them as TM
  leverage, and `kapi push` carries them as block targets.

## 3. Review with the user

Render, score, iterate — the loop is [brand.md](brand.md)'s:

```bash
kapi brand guide --profile-file brand.yaml                    # the rendered guide
kapi brand check README.md --profile-file brand.yaml --json   # score one of their own files
```

Show the guide, the score and findings on their own text, and the term list.
Get explicit sign-off on the forbidden/competitor lists and every
`deprecated`/`forbidden` term — these will gate their builds. Fold feedback
into `brand.yaml` and re-render until the user agrees. Never invent competitors
or bans the user didn't confirm.

## 4. Bind

Create the project (or adopt the existing recipe — `kapi init` is idempotent):

```bash
kapi init --name my-app                                        # content project: brand + termbase + check flow
kapi init --name my-app --target-locale fr --target-locale de  # translation project
```

Bind the pack in the recipe:

```yaml
defaults:
  brand_voice:
    profile_file: brand.yaml   # file binding, so `kapi apply` brand rules land in it
  termbase: .kapi/termbase.db
content:
  - path: "docs/**/*.md"
    format: markdown
```

Materialize the terminology seed, now that the project exists:

```bash
# preferred: term entries — maintains the committed l10n/termbase.ktb source
# and compiles the .kapi/termbase.db cache in one verb
kapi apply terms.jsonl
# bulk path for a handed-over glossary file (csv, tsv, json, tbx, ktb):
kapi termbase import glossary.csv -s en -t fr --header
kapi termbase import vocab.csv -s en --monolingual --header
```

```jsonl
{"kind":"term","op":"upsert","term":"dashboard","locale":"en","status":"preferred"}
{"kind":"term","op":"upsert","term":"control panel","locale":"en","status":"deprecated","replacement":"dashboard"}
```

The `apply` route keeps a committed source of truth; a bulk `termbase import`
writes only the compiled store, so commit the glossary file itself. Then verify
the whole pack locally before pushing:

```bash
kapi check --ship --json     # brand + terminology (+ QA) gates — all green
```

The recipe carries the bindings; the thresholds ride on the check itself: the
brand gate's score bar is `--min-score` (default 80), not a recipe field, and a
translation-coverage bar is an optional top-level `ship_gate:` (see
[localize.md](localize.md)). Commit the pack: the recipe, `brand.yaml`, the
`l10n/` sources, any glossary file. `.kapi/` state stays gitignored.

## 5. Push to Bowrain

Bowrain commands come from the `kapi-bowrain` plugin. Once the recipe declares
a `server:` block, running one of its verbs without the plugin installed offers
the install rather than failing — on a terminal it prompts, and `--yes` accepts
without asking. To install it up front instead:

```bash
kapi plugins list
kapi plugin install bowrain             # from the plugin registry
brew install neokapi/tap/bowrain-cli    # or via Homebrew
```

Connect — `kapi init --server` adds the `server:` block to the existing recipe
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

On a workspace project, push prints the project and review URLs and — with the
default `converge: on-push` — the server picks up translation, checks, and
review queueing from there. An anonymous project pushes too (the machine that
ran init holds the claim token), but prints no workspace URLs until claimed.

**Terminology and the voice profile reach the server after the claim.** Both
are workspace-scoped, so an anonymous project's first push carries content
only. Once the user has claimed — they open the claim URL, or you run
`kapi auth login` then `kapi auth claim` (which also rebinds the recipe to the
workspace URL):

```bash
kapi pull               # establish the concept baseline
kapi push --concepts    # reconcile the local termbase into the server's terminology hub
```

**The bound voice profile travels with the push.** On a workspace project,
`kapi push` upserts it into the workspace brand hub, matched by profile name:
created on first push, a no-op when unchanged, a new server-side version when
it changed — server-side edits are archived in the version history, never
overwritten, and rules the server promoted from corrections are kept.
`--no-brand` skips it. `brand.yaml` still travels in git, and
`kapi check --ship` enforces it wherever the repo is checked out (dev
machines, CI).

## 6. Hand back a loop

End by telling the user, concretely:

- **What exists** — `kapi.yaml`, `brand.yaml`, the committed termbase source,
  the content mapping, and (if pushed) the server project.
- **The URLs** — the claim URL while unclaimed; the project and review URLs
  once claimed.
- **The standing instruction** — run `kapi check --ship` before shipping
  content and fix what it flags ([project.md](project.md)); in a translation
  project, `kapi up` catches locales up ([localize.md](localize.md)).

---

# Refresh an existing pack

When new material lands — a site relaunch, product renames, a batch of new
pages — refresh the pack against it. Never silently overwrite: every change is
a proposal the user approves.

## 1. Re-gather, read the baseline

Fetch the new material as in step 1 above, then read what the pack currently
says:

```bash
kapi brand guide                            # the bound profile, rendered (no flag in a project)
kapi termbase export --format json          # every concept; or --format csv -s en -t fr for one pair
```

`brand.yaml` itself is the profile baseline — read it directly for the exact
vocabulary lists.

## 2. Propose a change-set

Diff the new signal against the baseline and draft the delta as typed
entries — additions, retirements, replacements in one reviewable set:

```jsonl
{"kind":"term","op":"upsert","term":"workspace","locale":"en","status":"preferred"}
{"kind":"term","op":"upsert","term":"team space","locale":"en","status":"deprecated","replacement":"workspace"}
{"kind":"brand","op":"add-rule","list":"competitor","term":"Globex","replacement":"our platform","severity":"major"}
```

These are the same `term` and `brand` entry kinds `kapi apply` always takes —
shapes in [edit.md → mixed change-sets](edit.md) and [brand.md](brand.md).
Tone, style, and `examples` changes have no entry kind: propose them as an edit
to `brand.yaml` and show the diff.

Present the change-set as adds / retires / replaces, each with its evidence
(where the new term appeared, what the old one conflicts with). Apply only what
the user approves; drop the rest.

## 3. Apply, verify, push

```bash
kapi apply changeset.jsonl       # terms + brand rules land atomically
kapi brand validate brand.yaml   # if you edited the profile directly
kapi check --ship --json         # the refreshed gates — green
kapi push --concepts             # reconcile terminology with the server hub
```

A newly retired term starts flagging old usage on the next check — sweep for it
while you're here (`kgrep`, [toolbox.md](toolbox.md)) and fix the hits through
`kapi apply` content entries ([edit.md](edit.md)).
