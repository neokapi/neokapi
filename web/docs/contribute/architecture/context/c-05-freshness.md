---
id: c-05-freshness
sidebar_position: 5
title: "C-05: Freshness and the composite ref"
description: "Architecture decision: one composite ref per stream, a monotonic content position plus three governance identity hashes, answers how far what is held here sits from what is held there, on every axis at once, with compare-and-swap per component."
keywords: [freshness, composite ref, divergence, compare-and-swap, staleness, governance, neokapi, architecture decision]
---

# C-05: Freshness and the composite ref

## Summary

For one synchronized stream, a **ref** answers how far what is held here sits
from what is held there, on every axis at once, in one place:

```
ref(stream) = { content: position, context: hash, terms: hash, decisions: hash }
```

The four components are not the same kind of value. **Content** is a
monotonic position: ahead and behind are meaningful, and an interrupted transfer
resumes from it. **Context**, **terms** and **decisions** are identity hashes,
compared for equality and for nothing else.

Two unequal hashes carry no ordering, so a ref never claims one side is newer. It
reports that governance moved and leaves the reconciliation to a caller who can
ask a person: cheap observation, explicit resolution.

## Context

Governance and content move for different reasons and at different rates.
Content moves continuously as work lands; the governing context, the terminology
and the committed decisions move when someone decides something. Carrying both on
one monotonic cursor conflates them: ordinary content traffic then looks like a
governance change, and a writer nowhere near the governance gets a conflict it
cannot act on.

Separately, an answer that reads a project's context is a snapshot of a graph
other processes move, and a caller holds one for the length of a task. Without a
freshness value there is no way for that caller to learn that what it read has
since changed. The failure is silent, and the wording it settles on is only as
good as the context it read at the start.

## Decision

### One ref, four components, one comparison

```go
type Ref struct {
    Content   int64  `json:"content"`
    Context   string `json:"context,omitempty"`
    Terms     string `json:"terms,omitempty"`
    Decisions string `json:"decisions,omitempty"`
}
```

- **`content`** is the position in the stream's ordered change feed. `Advance`
  moves it forward and does nothing when the offered cursor does not lead the one
  held. Monotonicity is a property the transfer depends on rather than a
  defensive check, because a position is what a resumable transfer resumes
  from, and a caller taking its position from a response that carries none hands
  over a nought meaning *no position in this answer*, never *rewind to the
  beginning*.
- **`context`** identifies the governing context: which collections exist, the
  point each occupies, and the voice profile governing it.
- **`terms`** identifies the governed terminology in force.
- **`decisions`** identifies the committed decision record.

Comparison is `ref.Compare(local, remote) → ref.Divergence`, one
`ComponentDiff` per component:

| Status | Meaning |
| --- | --- |
| `unknown` | one side makes no claim; no comparison is possible |
| `current` | both sides hold the same value |
| `behind` / `ahead` | the position trails or leads |
| `moved` | two identities differ, and that is all a hash can say |

`Divergence.Moved()` names the governance components that differ and never
includes the position: the position moves in the ordinary course of content
traffic, and calling that *moved governance* is exactly the conflation this
design ends. `Divergence.Current()` reports that nothing diverged, treating
`unknown` as *a question neither side asked* rather than as a difference.

The **zero ref makes no claim about anything** (position nought and three empty
identities), which is what a side that has never synchronized holds. An empty
identity compares as `unknown` rather than as different, so a missing ref costs
one round trip and never a wrong answer.

### There is one definition of each hash

Two functions, and only two:

- **`ref.Fold(map[string]string)`** folds a set of named identity hashes into one
  component hash: the names in sorted order, each written with its hash, through
  SHA-256.
- **`ref.Identity(fields ...string)`** hashes an ordered list of fields into the
  identity of one member of such a set: the fields in the given order,
  NUL-separated, through SHA-256. The separator is what stops a member whose
  fields are `("ab", "c")` from hashing identically to one whose fields are
  `("a", "bc")`, which would let a rename in one field pass as no change at all.

Both sides of a protocol fold with these, over their own view of the same set,
and compare the results. A second fold with a different byte layout would
silently make two correct sides disagree forever, so there is not one.

**Voice has no component of its own.** A voice profile is part of what governs a
point, so it folds into the `context` identity along with the collections and
their coordinates. Where a surface needs to attribute a moved fingerprint, it
does so by looking at what changed: a moved profile identity or revision points
at `context`, otherwise at `terms`.

### Compare-and-swap, per component

A writer asserts only the component it owns:

```go
func Assert(component Component, expected, actual string) error // → *ref.Conflict
```

Ordinary content traffic, which moves nothing but the position, therefore cannot
manufacture a governance conflict for a writer that is nowhere near it. The
conflict renders as the instruction it implies: the component that moved, the
value expected, the value found, and the fact that no retry of the same write
will resolve it.

An **empty expectation asserts nothing and always succeeds**. That is what keeps
the mechanism additive: a client that predates the ref sends no assertion and
writes exactly as it always did, while a client that sends one is protected
against the writer it could not see. The framework supplies the primitive; the
protocol that carries refs between two sides is where the assertion is enforced.

### The observed ref lives on disk, not on the wire

`core/ref/refcache` holds a project's refs at `.kapi/work/cache/refs.json`: for
each stream, the position this project has consumed and the governance identities
its last contact established.

The file is **destination-keyed and disposable**. Everything in it is true of one
destination and of no other, so a cache belonging to a different one is discarded
rather than reconciled. A position is a place in one change feed and a
governance identity is one scope's state, so carrying either across a re-point
would answer a question about the new destination with a fact about the old one.
And everything in it is re-derivable, so deleting it costs exactly one
negotiation round trip and can never cost a wrong answer. It is therefore not
migrated, versioned or repaired: a file this side cannot read is a file this side
re-fetches.

Two readers, kept apart. `Load` is destination-keyed and is what everything that
**writes** goes through. `LoadObserved` reads the cache for **reporting**,
whatever destination wrote it, for a caller that wants to say what the project
last observed but does not know, and should not need to know, which destination
the recipe binds. Nothing decides from what `LoadObserved` returns: a report that
names the wrong destination is a report to correct, while a write against the
wrong destination is a position in one change feed applied to another.

Putting the ref on disk rather than fetching it per question has two
consequences, both intended:

- **A retrieval costs no round trip.** It is a read path a caller hits repeatedly
  inside a single thought; phoning a service on each one would trade a note
  nobody waits for against latency on every question asked. Refreshing the cache
  belongs to the transport, at the cadence it already runs at.
- **The staleness baseline is per process and advances on every read.** A first
  answer reports nothing (nothing has moved since a read that had not happened
  yet), and a change is reported once, to the answer that first spans it, rather
  than on every answer from then on. A note repeated forever is a note a caller
  learns to skip.

### Three consumers, one comparison

The comparison is made once, in `core/ref`, and rendered by each surface:

| Surface | What it does with the ref |
| --- | --- |
| `kapi status` | the **governance axis** of the report: the ref this project observed against the one its venue publishes now, rendered through `StatusGovernance.Divergence()`. The content axis is the coverage grid beside it. |
| Retrieval answers | a **staleness note** on the answer, naming the governance components that moved since this process last read (`host/freshness.go`). Silent for a project that has never observed governance: taking a position with no identities as a baseline would be taking silence for a fact. |
| `kapi check --ship` | the **staleness gate** (`host/verify_staleness.go`). Each produced target carries the `ContextFingerprint` of the context that produced it; the gate compares that against the context in force at the file's governance point. |

Two properties of the gate matter. It runs at the file's governance point, so a
per-file `channel:` override ([C-02](c-02-coordinates-and-governance.md)) is
honoured rather than averaged away. And an **unstamped target is reported as
information and never failed**: a target produced before anything stamped
fingerprints is not evidence of staleness, and failing on it would make the gate
useless on exactly the projects that most need to adopt it.

Reporting and enforcing are separate roles. The status report and the retrieval
note **report and never resolve**: what a moved context means for work already
written is a judgement, and neither is in a position to make it. The gate is the
enforcing half, and it fails a target rather than guessing what should replace
it.

A plugin that shipped its own verdict would be a second implementation of *moved*,
and the two would answer differently the day one of them learned about a new
component. So a venue contributes the ref it publishes, and the comparison stays
here.

### Streams

A ref belongs to a stream, and `ref.DefaultStream` is `"main"`, spelled in the
framework because the framework cannot reach the recipe vocabulary that also
spells it, with the two asserted equal where both are in scope. A project that
binds no venue has one stream and no ref, which is the ordinary local case: there
is nothing to be behind.

## Consequences

- Governance movement is observable without a round trip, and without a
  monotonic cursor pretending to order it.
- A writer can protect a governance write without blocking on content traffic,
  and a client that does not know about refs is not broken by them.
- A caller holding a context answer for an hour learns that the ground moved,
  once, at the moment it matters.
- Adding a component is a change in exactly two places, the struct and the
  comparison, because nothing else re-implements either.

## See also

- [C-02: Coordinates and governance](c-02-coordinates-and-governance.md): the
  governance point the staleness gate resolves at.
- [C-04: Unit state and the decision record](c-04-unit-state-and-decisions.md):
  the committed record the `decisions` component identifies.
- [C-06: Context retrieval](c-06-retrieval.md): the answers that carry a
  staleness note.
- [C-08: Terms](c-08-terms.md): the terminology the `terms` component
  identifies.
