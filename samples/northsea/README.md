# Northsea — the monolingual sample

Northsea is a small, complete company repository that carries a context graph
and passes its own gates in **one language**. It exists to show that the first
journey — govern the content you already have — stands on its own, with no
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

The domain vocabulary — berth, vessel, alert, arrival, draught, terminal — is
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
tone and style — the vocabulary stays in force everywhere.

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
The decision is in `.kapi/terms.json` as one concept carrying both terms —
`berth` preferred, `mooring` deprecated — plus a second concept for the API
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

**What the recipe cannot yet say.** The rename is not in the enforced
vocabulary, and that is deliberate: `.kapi/voice.yaml` is the only surface a
monolingual check enforces, its vocabulary is project-wide, and both the
changelog and the API reference carry the retired word legitimately. A
channel override cannot narrow vocabulary — `ChannelOverride` carries a tone and
a style and nothing else — so today the ban would be everywhere or nowhere. The
point beneath the file that would permit the retired name in exactly those two
places is the open case named at the end of
[C-02](../../web/docs/contribute/architecture/context/c-02-coordinates-and-governance.md).
Until it exists, the rename lives in the record, where retrieval answers it, and
the two historical surfaces are simply true.

## The enforced vocabulary

`.kapi/voice.yaml` bans two things a reader can check for, and the sample ships
one violation of each so the gate has something to find:

| Rule | Where the sample violates it | Point |
| --- | --- | --- |
| `seamless` → `unified` (major) | `landing/index.html`, in the Compass section | `northsea/landing` |
| `ship` → `vessel` (major) | `app/strings.en.json`, `fleet.search.placeholder` | `northsea/app` |

A third word, `dock`, appears in the landing page testimonial and is **not**
decided yet. It is the sample's correction: a reviewer decides it during the
walkthrough, and the next check enforces the decision.

## Running the journey

From a copy of this directory (the commands assume kapi on `PATH`):

```bash
kapi voice validate .kapi/voice.yaml      # the drafted profile is schema-valid
kapi context docs/berths.md               # where am I, and what governs here
kapi context search mooring               # what do we call this, and why
kapi check --strict                       # two findings, two points — exit 3
```

Correct the two violations, then teach the graph the third word:

```bash
ksed -i 's/our seamless integration/our unified integration/' landing/index.html
ksed -i 's/by ship name/by vessel name/' app/strings.en.json

echo '{"kind":"term","op":"upsert","term":"dock","locale":"en-GB","status":"deprecated","replacement":"berth"}'  > decisions.jsonl
echo '{"kind":"voice","op":"add-rule","list":"forbidden","term":"dock","replacement":"berth","severity":"major"}' >> decisions.jsonl
kapi apply decisions.jsonl                # the decision lands in both records
kapi check --strict                       # now it finds "dock" — exit 3
```

Fix the last one and the gate is green:

```bash
ksed -i 's/could not dock when the pilot called/could not reach its berth when the pilot called/' landing/index.html
kapi check --strict                        # 100/100, exit 0
kapi check --ship                          # voice, terminology and QA gates all pass
```

The change-set carries **both** a `term` entry and a `voice` entry for one
decision. That is not redundancy: the term entry is the vocabulary record that
retrieval answers from, and the voice entry is what a monolingual check
enforces. A single decision has to reach both until the source-side terms gate
exists ([#1904](https://github.com/neokapi/neokapi/issues/1904)).

## Every file round-trips

Editing any file in this sample through kapi rewrites it byte-identically apart
from the change itself, including `landing/index.html`. That is a property of
how the landing page is **authored**, not a free one: the HTML writer
re-serializes markup, so a page whose text blocks are wrapped across source
lines comes back reflowed. Each text block here is one source line, and no
inline element is the last child of a block container, which is the shape the
writer emits.

## Known gaps this sample exercises

Running the journey above is also how these were found. Each is filed, and none
of them is worked around in the sample beyond what is noted here.

| Gap | Issue |
| --- | --- |
| `kapi up` does not converge a project with no target languages; it seeds the context and then stops, and `kapi status` has nothing to report either | [#1900](https://github.com/neokapi/neokapi/issues/1900) |
| `kapi context <path>` answers from the committed terms document while `kapi context search` answers from the derived store, so the two disagree until something has seeded the store | [#1901](https://github.com/neokapi/neokapi/issues/1901) |
| The HTML writer reflows a document it did not change; this sample's landing page is authored in the shape the writer emits so that it does not | [#1902](https://github.com/neokapi/neokapi/issues/1902) |
| `kapi apply` strips the comments from `.kapi/voice.yaml` the first time a `voice` entry lands | [#1903](https://github.com/neokapi/neokapi/issues/1903) |
| No terms gate runs on a monolingual project, which is why one decision has to be written as two change-set entries | [#1904](https://github.com/neokapi/neokapi/issues/1904) |

## Where it is used

- `harness/demos/s0-northsea-governance/` records the journey above as a
  narrated walkthrough, seeded straight from this directory (`fixturesFrom`),
  so the recording and the sample cannot drift apart.
- `scripts/contexteval/corpus.go` uses the same fictional company, products and
  domain vocabulary.
