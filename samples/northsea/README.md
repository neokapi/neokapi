# Northsea: the monolingual sample

Northsea is a small, complete company repository that carries a context graph
and passes its own gates in **one language**. It exists to show that the first
journey, govern the content you already have, stands on its own, with no
second language, no server, and no provider credential anywhere in it.

Everything here runs offline from the committed files. `kapi check` reads
`kapi.yaml`, `.kapi/voice.yaml` and `.kapi/terms.json`, and nothing else.

This README follows the project's own register
([brand-communication.md](../../docs/internals/brand-communication.md)); the
content **inside** the sample is Northsea's own product copy and is written the
way a real company would write it. That difference is the point of the sample.

## The fiction

Northsea Maritime Systems sells software to port and fleet operators. It ships
two products and one command line, and the names are shared with the context
evaluation corpus (`scripts/contexteval/`) so one fictional company serves every
fixture, eval and recording:

| Name | What it is |
| --- | --- |
| **Compass** | The berth plan and fleet view a duty officer keeps open all day. |
| **Tidewatch** | The service that compares the forecast with each berth's constraints and raises an alert when a movement stops being safe. |
| **tidectl** | The command line that drives both. |

The domain vocabulary (berth, vessel, alert, arrival, draught, terminal) is
the vocabulary the eval corpus already tests against, so a term decision made
here means the same thing there.

## The shape

```
samples/northsea/
├── kapi.yaml                 # the recipe: one profile, four channels
├── .kapi/
│   ├── voice.yaml            # the house voice, with a per-channel register
│   ├── terms.json            # the committed vocabulary record
│   └── .gitignore            # work/ is derived; everything else is committed
├── docs/                     # northsea/docs — operator documentation
│   ├── index.md
│   ├── berths.md
│   ├── alerts.md
│   ├── api/reference.md      # northsea/reference — the published contract
│   └── changelog.md          # northsea/reference — the record, as announced
├── app/
│   └── strings.en.json       # northsea/app — Compass interface strings
└── landing/
    └── index.html            # northsea/landing — the marketing page
```

Three surfaces with genuinely different registers, in three formats. The
operator documentation instructs; the interface strings name controls and state
what happened; the landing page addresses somebody deciding whether to look
further. One voice profile carries all three, because a channel override changes
tone and style, and the vocabulary stays in force everywhere.

## The points

The product axis is the profile; the channel axis is the surface. Both are
slugs, and both come from the recipe rather than from a taxonomy beside it:

| Point | Content | Register |
| --- | --- | --- |
| `northsea/docs` | `docs/index.md`, `docs/berths.md`, `docs/alerts.md` | Instructional, formal, second person |
| `northsea/reference` | `docs/api/reference.md`, `docs/changelog.md` | Descriptive, technical, third person |
| `northsea/app` | `app/strings.en.json` | Microcopy, neutral, no terminal punctuation on controls |
| `northsea/landing` | `landing/index.html` | Warmer, first person plural, numbers that are real |

`northsea/reference` is bound per **item** rather than per collection: the two
historical files sit inside the documentation collection and take their own
channel, which is the finest point the recipe can declare.

## The rename, and where it stops

Northsea renamed a place alongside from **mooring** to **berth** on 2026-01-20.
The decision is in `.kapi/terms.json` as one concept carrying both terms,
`berth` preferred and `mooring` deprecated, plus a second concept for the API
field `mooring_id`, which is `admitted` because the wire contract keeps the name
the vocabulary retired.

Ask about it by content and the graph answers with both halves:

```
$ kapi context search mooring
Terms
  berth                    ok (en-GB, preferred)  [operations]
  mooring                  discouraged — say "berth" (en-GB, deprecated)  [operations]
  mooring_id               ok (en-GB, admitted)  [api]
```

**What the recipe says, and what it still cannot.** The rename is not in the
enforced vocabulary, because it does not need to be. A **deprecated** term is
enforced as a **minor** finding, because retiring a word never fails anybody's
build, so the changelog keeps the retired name, the gate says so, and nothing is
blocked. The record and the gate agree without an exception mechanism.

The API reference needs no exception either. `mooring_id` is its own concept,
**admitted**, and the gate matches whole words and prefers the longest term the
project declared, so the field name is not read as a use of the word it
contains.

What a recipe still cannot say is *permitted in this passage*: an exception
finer than a file. A file-scoped exception is declarable today, through a profile with
its own `terms:` (or the conventional
`.kapi/profiles/<name>/terms.json`) governs exactly the files its channels
carry, because `kapi check` resolves the vocabulary per file the way it resolves
the voice. Below the file there is no point to bind, which is the open case
named at the end of
[C-02](../../web/docs/contribute/architecture/context/c-02-coordinates-and-governance.md).
Until it exists, the advisory severity is what makes the arrangement liveable.

## The enforced vocabulary

Two rules, from two sources, and the sample ships one violation of each:

| Rule | Source | Severity | Where the sample violates it | Point |
| --- | --- | --- | --- | --- |
| `seamless` → `unified` | `.kapi/voice.yaml` | major | `landing/index.html`, Compass section | `northsea/landing` |
| `ship` → `vessel` | `.kapi/terms.json` (deprecated) | minor | `app/strings.en.json`, `fleet.search.placeholder` | `northsea/app` |

The severity difference is the point. A word the voice profile forbids is a
defect; a word the vocabulary retired is a migration, and a migration that fails
builds is a migration nobody finishes.

A third word, `dock`, appears in the landing page testimonial and is **not**
decided yet. It is the sample's correction: a reviewer decides it during the
walkthrough, and the next check enforces the decision.

## Running the journey

From a copy of this directory (the commands assume kapi on `PATH`):

```bash
kapi voice validate .kapi/voice.yaml      # the drafted profile is schema-valid
kapi up                                   # reconcile the graph and the sources
kapi context docs/berths.md               # where am I, and what governs here
kapi context search mooring               # what do we call this, and everywhere it lands
kapi check --strict                       # exit 3 — one major, the rest advisory
```

`kapi up` comes before the two questions on purpose: it is what compiles the
committed terms into the store and builds the occurrence graph, so
`kapi context search` can answer *where* a word is used and not only what it
means.

Correct the wording the source got wrong, then teach the graph a word it has
never been told about:

```bash
ksed -i 's/our seamless integration/our unified integration/' landing/index.html
ksed -i 's/Release a mooring earlier/Release a berth earlier/' docs/berths.md
ksed -i 's/by ship name/by vessel name/' app/strings.en.json

echo '{"kind":"term","op":"upsert","term":"dock","locale":"en-GB","status":"forbidden","replacement":"berth"}' > decisions.jsonl
kapi apply decisions.jsonl                # one entry, reaching record and gate
kapi check --strict                       # now it finds "dock", major — exit 3
```

One entry is the whole decision. `forbidden` is chosen over `deprecated`
deliberately: `dock` is a word Northsea does not use, not a name it is migrating
away from, so it should stop a build.

Fix the last one and converge:

```bash
ksed -i 's/could not dock when the pilot called/could not reach its berth when the pilot called/' landing/index.html
kapi up                                    # 3 source files changed, re-extracted
kapi status                                # the source axis, on one line
kapi check --strict                        # exit 0, with two advisory findings left
```

The two that remain are the changelog's, and they are correct: the record says
what was announced, in the words it was announced in.

`kapi check --ship` runs the same gates plus the ship and source coverage gates,
also passes here, and names its findings the way `kapi check` does, by the
project-relative path and the block.

## The second visit

The journey above is the first visit: a context drafted from the material and
corrected once. The second visit is what keeps it true. Northsea launched a
support site and renamed Tidewatch to Tideguard, and neither fact is in the
record, so the material and the record disagree, which is the only reason to
refresh.

Seed that drift on a copy of this directory:

```bash
mkdir support
printf '# Support\n\nTideguard re-evaluates an approved movement whenever the\nforecast changes.\n' > support/faq.md
```

Read what moved before writing anything:

```bash
kapi ls --untracked                  # support/faq.md — and README.md, which you will decline
kapi context search Tideguard        # nothing: the record has never been told this name
kapi context search Tidewatch        # the name the record still prefers
```

`kapi ls --untracked` is the recipe subtracted from the disk. It reports and
never adopts, so what it finds is a list of candidates a reviewer works through.
This README appears there too, and declining it is the correct answer.

Then apply only what was approved. The new surface takes the point that suits
it, and the rename is two term decisions:

```bash
kapi add "support/**/*.md" --name northsea-support --channel northsea/docs

printf '%s\n%s\n' \
  '{"kind":"term","op":"upsert","term":"Tideguard","locale":"en-GB","status":"preferred"}' \
  '{"kind":"term","op":"upsert","term":"Tidewatch","locale":"en-GB","status":"deprecated","replacement":"Tideguard"}' \
  > refresh.jsonl
kapi apply refresh.jsonl

kapi up
kapi check --strict
```

Both writes edit the committed file rather than re-emitting it: `kapi.yaml` and
`.kapi/terms.json` keep their comments and their key order, so `git diff` is
small enough to read as a decision. The check that follows reports the retired
name wherever the documentation still carries it, each finding naming Tideguard
as the fix, advisory as a retirement always is, and exactly the sweep the
decision created.

`TestRefresh_NorthseaDrift` (`cli/refresh_northsea_test.go`) drives this same
sequence against this directory, so the sample and the flow cannot drift apart.

## Every file round-trips

Editing any file in this sample through kapi rewrites it byte-identically apart
from the change itself, including `landing/index.html`. The HTML writer holds
that for any pretty-printed page: whitespace the extraction trims off a text
block travels with the skeleton, and a block nothing edited writes back the
bytes the document held rather than the normalized join. Source line wrapping
inside a paragraph therefore survives a round-trip, and a block that *was*
edited joins its sibling tags directly, the way extraction parity requires.

## Known gaps this sample exercises

Running the journey is how these were found. The blockers are fixed; what is
left is filed and visible in the sample's own output rather than worked around.

| Gap | Issue |
| --- | --- |
| No point beneath the file, so a vocabulary decision cannot carry an exception for a passage. A whole surface can keep a retired name, since a profile with its own `terms:` does that, but two paragraphs of one file cannot. Advisory severity is what makes that liveable today | [C-02](../../web/docs/contribute/architecture/context/c-02-coordinates-and-governance.md) |

Fixed while this sample was being built, each found by running the journey on
it: [#1900](https://github.com/neokapi/neokapi/issues/1900) (monolingual
`kapi up` and `kapi status`),
[#1901](https://github.com/neokapi/neokapi/issues/1901) (the two retrieval
surfaces disagreeing), [#1902](https://github.com/neokapi/neokapi/issues/1902)
(the HTML writer reflowing an unedited document),
[#1903](https://github.com/neokapi/neokapi/issues/1903) (`kapi apply` losing a
governance file's comments),
[#1904](https://github.com/neokapi/neokapi/issues/1904) (no terms gate on a
monolingual project),
[#1906](https://github.com/neokapi/neokapi/issues/1906) (the monolingual CLI
surface, including the ship gate's absolute paths),
[#1914](https://github.com/neokapi/neokapi/issues/1914) (the terms matcher
reporting `mooring_id` as the retired `mooring` inside it), and
[#1915](https://github.com/neokapi/neokapi/issues/1915) (a replacement handed to
`kapi apply` never reaching the finding it answers).

## Where it is used

- `harness/demos/s0-northsea-governance/` records the journey above as a
  narrated walkthrough, seeded straight from this directory (`fixturesFrom`),
  so the recording and the sample cannot drift apart.
- `scripts/contexteval/corpus.go` uses the same fictional company, products and
  domain vocabulary.
