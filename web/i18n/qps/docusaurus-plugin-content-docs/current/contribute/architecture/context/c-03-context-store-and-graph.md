---
id: c-03-context-store-and-graph
sidebar_position: 3
title: "C-03: The context store and graph"
description: "Architecture decision: a kapi project keeps one local database, .kapi/work/store.db — the projection of its committed context and the substrate of its context graph. Every subsystem's tables share the file, a property graph relates them, and the same query shapes answer at any scope."
keywords: [store.db, context graph, graph_nodes, graph_edges, project store, projection, coordinates, durable identity, neokapi, architecture decision]
---

# ▒ Ç-03: Ţĥé çöñţéẋţ šţöŕé àñđ ĝŕàþĥ ▒

## ▒ Šüḿḿàŕý ▒

▒ À ķàþî þŕöĵéçţ ķééþš **öñé** ļöçàļ đàţàƃàšé: `.ķàþî/ŵöŕķ/šţöŕé.đƃ`. Îţ îš ţĥé
ļöçàļ **þŕöĵéçţîöñ** öƒ ţĥé þŕöĵéçţ'š çöḿḿîţţéđ çöñţéẋţ — ñéṽéŕ îţš ţŕüţĥ — àñđ
ţĥé šüƃšţŕàţé öƒ ţĥé þŕöĵéçţ'š **çöñţéẋţ ĝŕàþĥ**. ▒

▒ Éṽéŕý šüƃšýšţéḿ'š ţàƃļéš šĥàŕé ţĥàţ öñé ƒîļé: ţĥé ƃļöçķ çàçĥé, ţĥé ţéŕḿš šţöŕé,
ţĥé çöñţéñţ ḿéḿöŕý, ţĥé üñîţ-šţàţé ŵöŕķîñĝ šéţ, àñđ à þŕöþéŕţý ĝŕàþĥ
(`ĝŕàþĥ_ñöđéš` / `ĝŕàþĥ_éđĝéš`) ŕéļàţîñĝ ţĥéḿ. Öñé ƒîļé, öñé çöññéçţîöñ þööļ, öñé
ḿîĝŕàţîöñ ļéđĝéŕ þéŕ šüƃšýšţéḿ. ▒

▒ Ţĥé þöîñţ öƒ ţĥé šîñĝļé ƒîļé îš ţĥé ĵöîñ. *Ŵĥîçĥ ƃļöçķš üšé ţĥîš ţéŕḿ, îñ ŵĥîçĥ
çöļļéçţîöñ, àţ ŵĥîçĥ çööŕđîñàţé, àñđ ŵĥîçĥ öƒ ţĥéḿ àŕé šîĝñéđ öƒƒ?* îš öñé ǫüéŕý
ĥéŕé àñđ îš üñàñšŵéŕàƃļé àçŕöšš šéþàŕàţé đàţàƃàšé ƒîļéš. ▒

## ▒ Çöñţéẋţ ▒

▒ Çöñţéẋţ îš ŕéļàţîöñàļ. À ţéŕḿ öççüŕš îñ ƃļöçķš; ƃļöçķš ƃéļöñĝ ţö çöļļéçţîöñš àñđ
šîţ àţ à þöîñţ îñ ţĥé çöñţéẋţ šþàçé ([Ç-02](c-02-coordinates-and-governance.md));
à šţàţé ŕéçöŕđ ƃļéššéš à üñîţ àţ à çöñţéñţ ĥàšĥ; à ḿéḿöŕý éñţŕý ŕéçýçļéš îñţö à
ƃļöçķ. Ŕéţŕîéṽàļ ([Ç-06](c-06-retrieval.md)) àñđ ĝöṽéŕñàñçé
([Ç-02](c-02-coordinates-and-governance.md)) ƃöţĥ ţŕàṽéŕšé ţĥöšé ŕéļàţîöñš ŕàţĥéŕ
ţĥàñ ŕéàđîñĝ öñé šţöŕé îñ îšöļàţîöñ. ▒

▒ À ĥöšţéđ ļàýéŕ àñšŵéŕš šüçĥ ǫüéšţîöñš öṽéŕ öñé đàţàƃàšé šþàññîñĝ ḿàñý þŕöĵéçţš.
Îƒ à ļöçàļ þŕöĵéçţ çöüļđ ñöţ àñšŵéŕ ţĥéḿ àţ àļļ, ţĥé ţŵö ĥàļṽéš ŵöüļđ đîƒƒéŕ îñ
ŵĥàţ ţĥéý çàñ ƃé *àšķéđ*, ñöţ ḿéŕéļý îñ šçàļé — àñđ ţĥé ƒŕàḿéŵöŕķ'š šţàñđîñĝ
çöñšţŕàîñţ îš ţĥàţ ķàþî ŕüñš öñ îţš öŵñ. ▒

## ▒ Đéçîšîöñ ▒

### ▒ Öñé ƒîļé, ḿàñý šüƃšýšţéḿš ▒

▒ `.ķàþî/ŵöŕķ/šţöŕé.đƃ` ĥöļđš: ▒

| Tables | Subsystem | Derived from |
| --- | --- | --- |
| block cache | `core/blockstore` ([C-01](c-01-project-model.md)) | the content files |
| terms | `terms/` ([C-08](c-08-terms.md)) | the committed terms source |
| content memory | `memory/` ([C-09](c-09-content-memory.md)) | the committed targets plus the `.memory.json` seeds |
| unit-state working set | `core/state` ([C-04](c-04-unit-state-and-decisions.md)) | the committed `.kapi/state/*.jsonl` shards |
| `graph_nodes`, `graph_edges` | `host/storage/graph`, vocabulary in `core/contextgraph` | the four above, plus the recipe |

▒ Éàçĥ šüƃšýšţéḿ öŵñš îţš öŵñ šçĥéḿà àñđ îţš öŵñ ḿîĝŕàţîöñ ļéđĝéŕ
(`šţöŕàĝé.Ḿîĝŕàţé(đƃ, "<šüƃšýšţéḿ>", …)`), šö à šüƃšýšţéḿ éṽöļṽéš ŵîţĥöüţ
ŕéþļàýîñĝ àñýöñé éļšé'š ḿîĝŕàţîöñš. Šĥàŕîñĝ ţĥé ƒîļé îš à šţöŕàĝé đéçîšîöñ, ñöţ à
çöüþļîñĝ: à šüƃšýšţéḿ šţîļļ ŕéàçĥéš îţš ţàƃļéš öñļý ţĥŕöüĝĥ îţš öŵñ îñţéŕƒàçé. ▒

▒ Šĥàŕîñĝ à ƒîļé ḿéàñš šĥàŕîñĝ à ŵŕîţéŕ. `çöŕé/šţöŕàĝé` îñšţàļļš àñ îñ-þŕöçéšš
**ŵŕîţé ĝàţé** öñ ţĥé ĥàñđļé: öñé ƑÎƑÖ þéŕḿîţ ţĥàţ éṽéŕý ŵŕîţé ţŕàñšàçţîöñ
þàššéš ţĥŕöüĝĥ, šö ŵŕîţéŕš ǫüéüé îñ àŕŕîṽàļ öŕđéŕ îñšţéàđ öƒ çöñţéñđîñĝ öñ
ŠǪĻîţé'š `ƃüšý_ţîḿéöüţ`, ŵĥéŕé à šàţüŕàţîñĝ ŵŕîţéŕ çàñ šţàŕṽé à šḿàļļ öñé
îñđéƒîñîţéļý. Ţĥé ĝàţé îš ñöţ ŕééñţŕàñţ, àñđ à ŵŕîţé ţŕàñšàçţîöñ ţĥàţ ţŕîéš ţö
öþéñ àñöţĥéŕ ŕéþöŕţš `ÉŕŕŴŕîţéĜàţéŔééñţŕàñţ` ŕàţĥéŕ ţĥàñ đéàđļöçķîñĝ. ▒

### ▒ Ţĥé çöḿḿîţţéđ šöüŕçéš àŕé ţĥé ţŕüţĥ ▒

▒ `šţöŕé.đƃ` îš àñ **îñđéẋ**, àñđ éṽéŕý ŕöŵ îñ îţ îš ŕéçöñšţŕüçţîƃļé ƒŕöḿ: ▒

- ▒ `.ķàþî/ţéŕḿš.ĵšöñ` — ţĥé ţéŕḿš šöüŕçé, ƃöüñđ ƃý `đéƒàüļţš.ţéŕḿš_šöüŕçé`; ▒
- ▒ `.ķàþî/ḿéḿöŕý/*.ḿéḿöŕý.ĵšöñ` — ţĥé çöñţéñţ-ḿéḿöŕý šééđš; ▒
- ▒ `.ķàþî/šţàţé/*.ĵšöñļ` — ţĥé çöḿḿîţţéđ üñîţ-šţàţé ŕéçöŕđ, öñé šĥàŕđ þéŕ
  đöçüḿéñţ; ▒
- ▒ ţĥé çöñţéñţ ƒîļéš ţĥéḿšéļṽéš, šöüŕçé àñđ ţàŕĝéţ. ▒

▒ Đéļéţé `šţöŕé.đƃ` àñđ à ŕé-ŕüñ ŕéƃüîļđš îţ ƒŕöḿ ţĥöšé. Ñöţĥîñĝ àüţĥöŕîţàţîṽé
ļîṽéš öñļý îñ ţĥé đàţàƃàšé, ŵĥîçĥ îš ŵĥý îţ šîţš üñđéŕ `.ķàþî/ŵöŕķ/` — ţĥé öñé
îĝñöŕéđ þàţĥ ([Ç-01](c-01-project-model.md)) — ŵĥîļé ţĥé šöüŕçéš îţ îš ƃüîļţ ƒŕöḿ
àŕé çöḿḿîţţéđ öñé đîŕéçţöŕý öṽéŕ. ▒

### ▒ Îţ šîţš àţ ţĥé ţöþ öƒ `ŵöŕķ/`, ñöţ üñđéŕ `çàçĥé/` ▒

▒ `.ķàþî/ŵöŕķ/çàçĥé/` ḿéàñš *ƒŕéé ţö đéļéţé*: ţĥé þàŕšé çàçĥé, éẋţŕàçţîöñ ƃàţçĥéš,
çöļļéçţîöñ öṽéŕļàýš. `šţöŕé.đƃ` đöéš ñöţ ǫüàļîƒý, àñđ ƃý éẋàçţļý öñé ḿàŕĝîñ.
Ƃéţŵééñ à đéçîšîöñ ļàñđîñĝ àñđ `ķàþî çöḿḿîţ` ḿàţéŕîàļîžîñĝ îţ ţö `.ķàþî/šţàţé/`,
ţĥé ŵöŕķîñĝ šéţ îñšîđé `šţöŕé.đƃ` ĥöļđš ţĥé **öñļý** çöþý öƒ ţĥàţ šţàĝéđ šţàţé. ▒

▒ Šö ţĥé çöšţ öƒ ļöšîñĝ ţĥé ƒîļé îš ƃöüñđéđ àñđ šţàţéđ þŕéçîšéļý: àţ ḿöšţ ţĥé üñîţ
šţàţé šţàĝéđ šîñçé ţĥé ļàšţ çöḿḿîţ. Éṽéŕýţĥîñĝ éļšé ŕéƃüîļđš. `ŕḿ -ŕƒ
.ķàþî/ŵöŕķ/çàçĥé` ŕéḿàîñš çöḿþļéţéļý ƒŕéé, àñđ ķééþîñĝ ţĥé ţŵö àþàŕţ îš ŵĥàţ ļéţš
ţĥàţ šéñţéñçé šţàý ţŕüé. ▒

▒ Đéļéţîñĝ `.ķàþî/ŵöŕķ` öüţŕîĝĥţ îš à ŵîđéŕ çļàîḿ, ƃéçàüšé ţĥé ŕéđàçţîöñ ṽàüļţ
([Ç-10](c-10-redaction.md)) ļîṽéš ƃéšîđé ţĥé đàţàƃàšé àţ `.ķàþî/ŵöŕķ/ṽàüļţ/` àñđ
ĥöļđš ŵîţĥĥéļđ öŕîĝîñàļš. Ţĥöšé àŕé ļöçàļ-öñļý ƃý đéšîĝñ — ñéṽéŕ çöḿḿîţţéđ, ñéṽéŕ
šéñţ àñýŵĥéŕé — šö ñöţĥîñĝ éļšé ĥàš à çöþý, àñđ ļöšîñĝ ţĥéḿ îš ñöţ à ŕéƃüîļđ ƃüţ
à ļöšš. ▒

### ▒ Þŕéšéñçé îš ţàƃļé-ļéṽéļ ▒

▒ Àñ éḿþţý šüƃšýšţéḿ îñšîđé àñ éẋîšţîñĝ `šţöŕé.đƃ` ƃéĥàṽéš éẋàçţļý àš àñ àƃšéñţ
šţöŕé đöéš. Ţĥé đàţàƃàšé'š éẋîšţéñçé îš ñöţ à šîĝñàļ, šö ñöţĥîñĝ ĥàš ţö ĝüàŕđ
àĝàîñšţ ţĥé ƒîļé ƃéîñĝ ţĥéŕé: ţĥé ţéŕḿîñöļöĝý ĝàţé ŕéàđš ţĥé çöḿḿîţţéđ ţéŕḿš
šöüŕçé đîŕéçţļý öñ à ƒŕéšĥ çĥéçķöüţ ŵĥéţĥéŕ öŕ ñöţ à đàţàƃàšé éẋîšţš
([Ç-08](c-08-terms.md)), `ķàþî üþ --þļàñ` ñéṽéŕ çŕéàţéš šţàţé, àñđ `ķàþî þàçķ`
çàŕŕîéš öñļý ñöñ-éḿþţý þàŕţš
([Ḿ-06](../multilingual/m-06-content-packages.md)). `ĐéţéçţŠţöŕéĐŕîƒţ` ŕéàđš
"šţöŕé ḿîššîñĝ" àš *ţĥé ƃļöçķ çàçĥé ĥöļđš ñö ƃļöçķš*, ñöţ àš *ţĥé ƒîļé îš
àƃšéñţ*, þŕéçîšéļý ƃéçàüšé ţĥé ƒîļé éẋîšţš ƒŕöḿ ţĥé ƒîŕšţ öþéñ öƒ àñý šüƃšýšţéḿ. ▒

### ▒ Ţĥé ĝŕàþĥ ŕéļàţéš ţĥé šüƃšýšţéḿš ▒

▒ `ĝŕàþĥ_ñöđéš` àñđ `ĝŕàþĥ_éđĝéš` àŕé à þŕöþéŕţý ĝŕàþĥ — ļàƃéļļéđ ñöđéš àñđ
ļàƃéļļéđ, öþţîöñàļļý ţîḿé-ƃöüñđéđ éđĝéš, ƃöţĥ çàŕŕýîñĝ ĴŠÖÑ þŕöþéŕţîéš.
`çöŕé/çöñţéẋţĝŕàþĥ` öŵñš ţĥé ṽöçàƃüļàŕý: ţĥé ļàƃéļš, ţĥé šçöþé ţüþļé, ţĥé îđ
šçĥéḿé, àñđ ţĥé ñöđé àñđ éđĝé çöñšţŕüçţöŕš éṽéŕý ŵŕîţéŕ çàļļš. ▒

▒ Ñöđé ļàƃéļš: ▒

| Label | What it is | Scope |
| --- | --- | --- |
| `block` | a unit of source content, keyed by its content key | instance |
| `collection` | a content collection, keyed by its label | instance |
| `unit_state` | one unit's state in one document, in one locale variant | instance |
| `concept` | a terminology concept | vocabulary |
| `coordinate` | a point in the context space — a `(profile, channel)` pair | vocabulary |

▒ Éđĝé ļàƃéļš: ▒

| Label | Relates | Carries |
| --- | --- | --- |
| `uses_term` | block → concept | the term used, its status, the locale, the document, a use count — and the term's own validity window |
| `in_collection` | block → collection | membership |
| `governed_by` | collection → coordinate | the governing profile's validity window |
| `blesses` | unit state → block | the pairing the decision was written against: the target hash, and the source basis |

`host.MaterializeContextGraph` writes all four on the convergence path, after
extraction commits its block-write transaction. The subgraph is a pure
projection: each pass clears what it is entitled to and rebuilds it, so a delete
of the store loses nothing the recipe, the terms, the blocks and the committed
state cannot rebuild. Occurrence edges come from a term search over the block
cache (`core/occurrence`), where repeated uses of one term in one block fold into
a `count` property rather than into separate edges, and the term and the locale
are the edge discriminators; `governed_by` comes from resolving each named
collection's governance; `blesses` joins the unit-state working set against the
block cache, so a record whose block no longer exists keeps its node and loses
its edge.

The term is a discriminator because a concept holds several spellings and a
project reaches for one of them: a block still saying the deprecated word is a
different finding from one saying the preferred word, and folding both onto one
edge would leave the graph with a status to choose between. So the edge records
the status of the term it names, and carries that term's own window — which is
what makes *is this discouraged* answerable as *is it discouraged here*. The
standing is a property of the concept at a coordinate, never of the word: the
same block answers differently before and after a deprecation date, and inside
the market a deprecation reaches versus outside it.

The `governed_by` edge is where governance stops being re-derived. It carries the
same half-open validity window the recipe declares
([C-02](c-02-coordinates-and-governance.md)), so *what governed here on that
date* is answered by the graph under the same temporal model, not by a second
implementation of the ladder.

### Node identity carries the scope tuple

An id is `<label>:<workspace>/<project>/<stream>:<local>`, with the separators
percent-escaped inside each component so two different tuples can never render to
one id. The scope segment is always three fields, so it says which dimensions the
node is qualified by rather than leaving that to be inferred from the label.

Two kinds of node, and the split decides what can be asked:

- **Vocabulary nodes** — concepts, coordinates — drop the instance dimensions.
  One concept is one node however many projects use it, which is what makes
  *which projects use this concept* a two-hop traversal instead of a join across
  project boundaries. A coordinate is vocabulary for the same reason: a set of
  projects binds to one coordinate vocabulary rather than each inventing its own.
- **Instance nodes** — blocks, collections, unit states — carry `(project,
  stream)`. Two projects' `docs` collections are two nodes. Two projects holding
  identical wording hold two block nodes carrying the same content key, and *same
  wording* is a content-key equality query: one shared node would say the two
  instances are governed together, and an instance sits somewhere.

**Dimensions are fields, not containment edges.** Every node carries the
non-empty components of its scope tuple as properties as well as in its id, so
slicing a view by project or stream is a filter rather than a traversal. An empty
dimension is written as an *absent* key rather than an empty string, because a
property filter compares against a value and absence is not the empty string —
locally that means the scope properties are not there, which is correct: a
project on its own has no workspace to name. There is no project→project edge and
there will not be one: projects relate by co-occurrence through the vocabulary
they share.

Within a scope, identity is **durable** — a block is its content key
([F-03](../foundations/f-03-identity.md)), a unit state is its document, unit and
variant — not a reader's positional id, so a re-parse that renumbers a document
rewrites the same rows rather than orphaning them. The document is part of a unit
state's identity for the reason [C-04](c-04-unit-state-and-decisions.md) gives:
a unit id is unique inside its document and nowhere wider, so without it two
pages of one collection are one node and the decision written last answers for
both.

Changing a scope value changes the id, so **a rename is a deterministic
re-key**. That is safe because a writer clears the scope it is about to write in
the same pass: no row survives under the old key. The project dimension is the
recipe's project name, the only identity a project on its own has, so a rename
re-keys the whole projection — which is the rebuild each pass performs anyway.
Where a project dimension is issued rather than authored, a rename does not touch
it: the display name rides as the `project_name` property and the rule costs
nothing.

### Finding a term inside blocks

Term occurrence has to search text that lives inside a JSON payload no SQL can
read, so the block cache keeps `block_texts`: a row per block per locale — the
source under the empty locale, each target under its own — with a contentless
FTS5 trigram index over it.

The index is built **before a search, not during a write**. Maintaining it inside
every block write was measured first and cost roughly seven times the write:
extraction writes every block in the project, which is too much to levy for a
query that may never be asked. A write now only marks the block stale, and the
first search reconciles exactly what changed. The index narrows and never
decides — a trigram match is necessary, not sufficient — so matching is done in
Go, which is also how the browser build answers the same question by scanning.

`kapi terms occurrences` is the surface, and `kapi context search` carries a
usage count on each term it reports.

### The query shapes are written once

A store spanning many projects answers the same questions with the dimensions
free; a local project answers them with the dimensions pinned to one value. That
is enforced rather than described. The queries live in `core/contextgraph/query.go`
against two narrow read interfaces — `EdgeReader` and `NodeFinder` — that both
backing stores satisfy, so there is one implementation rather than two agreeing
by convention:

| Query | Question |
| --- | --- |
| `Uses` | term → blocks → collection, by traversal |
| `ProjectsUsingConcept` | which projects use this concept |
| `UsesByProject` | how much of it sits in each, in which words, and how it stands there at this point |
| `CollectionsAtCoordinate` | what is governed at this point, at this instant |
| `BlessingsOfBlock` | which decision covers this unit, at which basis |
| `BlocksWithContentKey` | who else holds this same wording |

▒ `çöŕé/çöñţéẋţĝŕàþĥ/ĝŕàþĥţéšţ` îš ţĥé šĥàŕéđ ǫüéŕý-šĥàþé šüîţé: öñé ƒîẋţüŕé öƒ ţŵö
þŕöĵéçţš šĥàŕîñĝ à çöñçéþţ, öñé ţàƃļé öƒ éẋþéçţéđ àñšŵéŕš, ŕüñ àĝàîñšţ éṽéŕý
šţöŕé ţĥàţ çļàîḿš ţö ĥöļđ ţĥîš ṽöçàƃüļàŕý. À ǫüéŕý àđđéđ ƒöŕ à ŵîđéŕ šçöþé îš
éẋþŕéššîƃļé àţ à ñàŕŕöŵéŕ öñé, àñđ à ļöçàļ ǫüéŕý đöéš ñöţ ĥàṽé ţö ƃé ŕéîñṽéñţéđ
ŵĥéñ ţĥé šçöþé ŵîđéñš. ▒

▒ Šçöþé ƒîļţéŕîñĝ îš öñé þŕéđîçàţé, `Šçöþé.Çöñţàîñš`, ţŕéàţîñĝ àñ éḿþţý đîḿéñšîöñ
àš *àñý* — ŵĥîçĥ îš ĥöŵ ţĥé šàḿé çàļļ šéŕṽéš ƃöţĥ à þŕöĵéçţ-šçöþéđ šüŕƒàçé àñđ à
ŕöļļüþ àçŕöšš þŕöĵéçţš. ▒

### ▒ Ƃŕöŵšéŕ àñđ ŵàšḿ ▒

▒ Ţĥéŕé îš ñö ŠǪĻîţé îñ ţĥé ƃŕöŵšéŕ ƃüîļđ. Ţĥé ḿöđéļ îš üñçĥàñĝéđ àñđ ţĥé ƃàçķéñđš
đîƒƒéŕ: îñ-ḿéḿöŕý çöñţéñţ ḿéḿöŕý àñđ ţéŕḿš, à þàţĥ-ķéýéđ îñ-ḿéḿöŕý ƃļöçķ šţöŕé,
àñđ à ŵöŕķîñĝ šéţ ţĥàţ þéŕšîšţš ţö à ĴŠÖÑ šîđéçàŕ, `.ķàþî/ŵöŕķ/šţöŕé.ĵšöñ`.
Öþéŕàţîöñš ţĥàţ ĝéñüîñéļý ñééđ ţĥé đàţàƃàšé ŕéþöŕţ `þŕöĵéçţđƃ.ÉŕŕÑöŠţöŕé`, ŵĥîçĥ
çàļļéŕš ŵĥöšé ƒéàţüŕé îš öþţîöñàļ ţĥéŕé ḿàţçĥ àñđ đéĝŕàđé öñ. Ţĥé šàḿé šöüŕçéš
ŕéƃüîļđ îţ, àñđ ţĥé šàḿé ĝŕàþĥ ŕéļàţîöñš ĥöļđ. ▒

## ▒ Çöñšéǫüéñçéš ▒

- ▒ **Öñé çöññéçţîöñ þéŕ þŕöĵéçţ.** Šüƃšýšţéḿš öþéñ ţĥé šĥàŕéđ `šţöŕàĝé.ĐƂ` ŕàţĥéŕ
  ţĥàñ à ƒîļé éàçĥ, šö öñé ţŕàñšàçţîöñ çàñ šþàñ à šţàţé ŕéçöŕđ àñđ ţĥé ĝŕàþĥ
  éđĝéš îţ îḿþļîéš. ▒
- ▒ **Çŕöšš-šüƃšýšţéḿ ǫüéšţîöñš àŕé öŕđîñàŕý ŠǪĻ.** Ţéŕḿ çöṽéŕàĝé þéŕ çöļļéçţîöñ,
  ţĥé ƃļöçķš ƃéĥîñđ à çööŕđîñàţé, ţĥé üñîţš à ţéŕḿ çĥàñĝé þüţš àţ ŕîšķ — ĵöîñš,
  ñöţ àþþļîçàţîöñ-ļéṽéļ ḿéŕĝéš. ▒
- ▒ **Šţöŕé þàţĥš àŕé ñöţ à üšéŕ šüŕƒàçé.** Ţĥé ŕéçîþé ƃîñđš *šöüŕçéš*
  (`đéƒàüļţš.ţéŕḿš_šöüŕçé`, `đéƒàüļţš.ḿéḿöŕý_šöüŕçé`); îţ ñéṽéŕ ñàḿéš ţĥé đéŕîṽéđ
  đàţàƃàšé. Šţàñđàļöñé šţöŕéš öüţšîđé à þŕöĵéçţ ķééþ ţĥéîŕ öŵñ šéļéçţöŕš
  (`--ţéŕḿšţöŕé`, `--ḿéḿöŕý`) — ţĥöšé àđđŕéšš à ƒîļé ţĥé üšéŕ öŵñš, ŵĥîçĥ îš à
  đîƒƒéŕéñţ ţĥîñĝ. ▒
- ▒ **ÇÎ çàçĥéš `.ķàþî/ŵöŕķ/çàçĥé/`, ñöţ `šţöŕé.đƃ`.** Ţĥé đàţàƃàšé çàŕŕîéš šţàĝéđ
  üñîţ šţàţé, àñđ à çàçĥé ķéý ţĥàţ ŕéšţöŕéš šţàļé šţàţé îš ŵöŕšé ţĥàñ à çöļđ
  ŕéƃüîļđ ([Çöñṽéŕĝéñçé îñ ÇÎ](/kapi/convergence-in-ci)). ▒

## ▒ Šéé àļšö ▒

- ▒ [Ç-01: Ţĥé þŕöĵéçţ ḿöđéļ](c-01-project-model.md) — ţĥé ļàýöüţ ţĥé šţöŕé šîţš îñ. ▒
- ▒ [Ç-04: Üñîţ šţàţé àñđ ţĥé đéçîšîöñ ŕéçöŕđ](c-04-unit-state-and-decisions.md) —
  ţĥé ŵöŕķîñĝ šéţ îñšîđé `šţöŕé.đƃ`. ▒
- ▒ [Ç-08: Ţéŕḿš](c-08-terms.md) àñđ [Ç-09: Çöñţéñţ ḿéḿöŕý](c-09-content-memory.md)
  — ţĥé ţŵö šüƃšýšţéḿš ŵĥöšé šöüŕçé-ṽéŕšüš-þŕöĵéçţîöñ šþļîţ ţĥîš šţöŕé
  îḿþļéḿéñţš. ▒
- ▒ [Ç-06: Çöñţéẋţ ŕéţŕîéṽàļ](c-06-retrieval.md) — ţĥé þŕîḿîţîṽéš ţĥéšé ţàƃļéš
  àñšŵéŕ. ▒
- ▒ [Ţĥé þŕöĵéçţ šţöŕé](/kapi/project-store) — ţĥé éñđ-üšéŕ ṽîéŵ. ▒
