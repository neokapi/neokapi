---
id: f-03-identity
sidebar_position: 3
title: "F-03: Identity"
description: "Entity IDs are 8-character base62 strings from crypto/rand; a block's identity is the graded output of matching a fresh read against the previous one, on a content hash and a document-scoped context hash; and the store key namespaces that identity by source file."
keywords: [entity ID, base62, crypto/rand, block identity, content hash, context hash, reconcile, store key, architecture decision, neokapi]
---

# F-03: Identity

## Summary

The framework has three identity primitives, each answering a different question.
**Entity IDs** (`core/id`) are 8-character base62 strings from `crypto/rand`
(short, URL-safe, dependency-free), used wherever something needs an allocated,
opaque handle. **Block identity** (`core/model.BlockIdentity`) is a pair of
content-addressable hashes that answer "is this the same content?". **Reconciled
identity** (`core/reconcile`) answers the harder question, "is this the same
*unit* as last time?", by grading the two hashes against the previous read
rather than by picking a naming scheme. The block-addressed store
(`core/blockstore`) keys on a derived value that namespaces the block by its
source file, so blocks from different files never collide.

## Context

IDs appear everywhere: command output, log lines, API responses, internal
references. UUID v4 is globally unique and collision-proof, but 36 characters in
canonical form are excessive where IDs appear in paths and terminal output, and
hyphens make them awkward to read back.

A second problem is specific to content. The framework processes the same content
from many sources, so an identifier derived from the content itself, rather than
an allocated number, lets identical blocks deduplicate and lets edits be detected
by hash comparison. But format readers also assign their own IDs from the source
format (XLIFF `tu1`, `tu2`, and so on), and those are unique only within one file.

A third problem only appears once a project is iterative. A decision, a
translation, and a content-memory entry are all recorded against a unit, and the
source file is re-read after every edit. Something has to say which unit in the
new read is which unit from the old one, and neither hash can answer that alone.

## Decision

### Short base62 IDs

All entity IDs come from `core/id.New()`:

```go
const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// rejectThreshold is the largest byte value that divides evenly into the
// alphabet, so each symbol is equally likely: 256 - (256 % 62) = 248.
const rejectThreshold = 256 - (256 % len(base62))

func New() string {
    out := make([]byte, 8)
    var buf [1]byte
    for i := range out {
        for {
            if _, err := rand.Read(buf[:]); err != nil {
                panic("crypto/rand failed: " + err.Error())
            }
            if int(buf[0]) < rejectThreshold {
                out[i] = base62[int(buf[0])%len(base62)]
                break
            }
        }
    }
    return string(out)
}
```

Properties:

- **8 characters**, base62 alphabet, roughly 47.6 bits of entropy per ID: enough
  for millions of entities per scope before collision becomes a concern.
- **`crypto/rand`** for cryptographic randomness, with **rejection sampling**
  rather than a bare modulo, so no symbol is more likely than another. A modulo
  over 256 would favour the first eight characters of the alphabet.
- **Zero external dependencies.** The implementation does not justify importing a
  UUID or nanoid library.
- **URL-safe.** No special characters, no percent-encoding.

Failing to read `crypto/rand` panics rather than degrading. A read failure means
an OS-level problem, and continuing with weak randomness would silently produce
predictable IDs, which is worse than crashing. This matches the standard library's own
convention.

### Content-addressable block identity

Blocks carry no allocated entity ID at the framework level. `BlockIdentity`
derives identity from the block itself:

```go
type BlockIdentity struct {
    ContentHash string // SHA-256 of the normalized source text
    ContextHash string // SHA-256 of name, type, and identifying properties
}

func ComputeIdentity(b *Block) *BlockIdentity
```

`ComputeContentHash` normalizes by trimming leading and trailing whitespace and
nothing else: case and interior spacing are content-significant. Identical
strings therefore collapse to one identity and deduplicate across sources. That
normalization is part of a wire contract: changing it (adding case folding, say,
or Unicode normalization) alters every emitted hash, so a golden-hash test pins
the exact output for a known input.

`ComputeContextHash` folds in the block's name, type, and deterministically-sorted
properties, so a structural change is detectable as a context-hash change even
when the source text is untouched. Properties whose key begins with `@` are
**advisory** and excluded: they are derived locators (a line number, a byte
offset), and a locator moves whenever anything above it moves, so hashing one
would report an untouched block as changed every time a blank line was added
earlier in the file.

### The record hash: what a transfer compares

A third value is derived from the pair rather than stored beside it.
`BlockIdentity.RecordHash` folds both halves, and it is what decides **transfer**:
whether a far side already holds what a block currently is.

Neither half can decide that alone. The content hash must not move for a reason
other than the text moving: it is the identity a decision, a memory entry and a
store key are all filed under. But a block is persisted with more than its text:
its name, its type, and the properties a reader recorded about it. A reader that
starts recording something new leaves the text untouched, so a comparison made on
the content hash reports the block unchanged and the new field never arrives. Not
on the next transfer: never, because the text is the same forever.

Folding the context half in is what makes an ordinary transfer deliver it, per
block, with nothing declared and no version bumped. The cost that would otherwise
make this unaffordable (a locator shifting and re-sending a file's whole tail)
is already paid for by the advisory prefix above, which keeps derived locators
out of the context hash in the first place.

The consequence for readers: a field this hash cannot see is a field that never
reaches content already stored elsewhere. Anything a block is persisted with
belongs in one half or the other, and anything computed rather than read belongs
behind the advisory prefix.

### Two producers, two vintages

Delivering a field to content already stored raises the mirror question: what
stops an *older* producer from taking it away again? A fleet is not one version:
a CI runner pinned to an older release, or a laptop that has not upgraded, reads
the same files with a reader that records less.

Two mechanisms answer it, and they divide by what can be merged.

**Fields are declared, and silence is not deletion.** A push declares the
property keys its readers emit, computed over every block it read rather than the
ones it sent. The far side is authoritative about those keys and leaves the rest
of a stored block's properties alone, so a note deleted in the source is deleted
here, while a key this producer has never heard of survives. A producer that
declares nothing knows nothing: it adds and updates, and removes nothing. No
version is involved: a reader that learns to record something new needs no
coordination at all.

**Structure is versioned, and a downgrade is refused.** Segmentation, the run
model, overlays, how blocks are named: there is no merging half of one. So the
framework states a **content-model epoch**, the stream records the highest it has
received, and a push from a lower one is refused at init, before it uploads
anything, rather than applied. `kapi push --force` carries past it, because a
deliberate downgrade is a legitimate thing to want and an accidental one is not.
The epoch moves only when produced content gains fidelity an older kapi cannot
reproduce from the same file; a new block property is never a reason to move it.

### Identity across revisions

The two hashes answer "is this the same content?" at a point in time. An iterative
project needs the second answer, and neither hash can give it alone, nor can a
name:

- **Position.** A reader without a natural key numbers blocks as it goes
  (`para3`, `line5`, `tu2`). Delete an earlier block and everything below is
  renamed, so a decision recorded about one paragraph silently names another.
- **Content.** Deriving the name from the text breaks the opposite case: fixing a
  typo renames the block, so it reads as a deletion plus an addition and loses the
  history it should have kept. It also collapses the context hash into a
  restatement of the content hash, destroying the independence the pair exists to
  provide.

Choosing between them only chooses which way identity breaks. So identity is not a
naming scheme. It is the **output of matching** a read against the previous one,
the way rename detection works on files: `core/reconcile` keeps both signals and
grades the pair.

| content | context | meaning | carries over |
| --- | --- | --- | --- |
| match | match | untouched | everything |
| match | differ | moved, or the same text reused elsewhere | identity and history |
| differ | match | edited in place | identity and history; the target is stale |
| differ | differ | genuinely new | nothing |

The third row is the one no single key can express, and the reason the package
exists. The fourth carries nothing over: when both signals change, nothing links
the block to its predecessor, and a guess would attach a real decision to the
wrong words.

`reconcile.Blocks(scope, current, prior)` runs three passes (both hashes, then
content alone, then context alone), and each pass consumes the priors it claims,
so a prior unit is claimed at most once. Two blocks can never resolve to one key,
which is what stops approving one from approving another. Callers may pass the
whole project's prior units while reconciling one document at a time, which is what
lets content moved between files keep its identity, and lets a removed block return
to its own history later.

**Content is graded before context**, and that settles the ambiguous case. Delete a
paragraph and the one below slides into the vacated slot: its words match the old
lower block, its position matches the deleted one. Content-first reads that as a
move, which is what happened; context-first would call it a rewrite and then hand
the neighbour's history to whatever arrived next. An edit in place loses nothing to
this ordering: its words have changed, so it has no content match to lose, and the
context pass still claims it.

Whatever nothing claims is minted from both hashes, so a project with no history
reconciles to the same keys on a second run.

#### Scope, and why documents have identity too

A block's name is unique only inside its own file: every Markdown document has a
`para1`. Matching on context alone across a project would let a paragraph in one
file claim the history of an unrelated paragraph in another, which in a project of
a few hundred documents is the common case rather than an edge case. So the context
*match* is scoped to the document: `reconcile.Identify(scope, block)` records the
scope beside the block's own two hashes, unchanged from `model.ComputeIdentity`,
and the pool keys its context lookup on the pair (scope, context hash). The
hashes stay the ones the sync wire sends and a venue stores, so a venue's stored
hashes serve as a prior set. Content is left unscoped, so a sentence moved between
files keeps its translation.

That scope is the document's **key rather than its path**; otherwise renaming a file
would rewrite the context of every block inside it at once, and a file that was
only renamed would report every block in it as moved. Documents are therefore
matched by the same grading one level up (`reconcile.Documents`), and their
resolved key is what blocks reconcile within.

The pass order there is **inverted**: same path with the same contents, then same
path with changed contents, then a new path with recognisable contents. Path is
graded first because a path genuinely identifies a document, whereas `para1` does
not: two files do not share a path, and a file staying put while its contents
change is far more ordinary than a rename. Renames are recognised by content
similarity against the prior document at a `RenameThreshold` of half: enough that a
file moved and partly rewritten is still itself, not so little that unrelated files
sharing boilerplate merge.

Readers are unaffected by all of this. Their positional names remain exactly what
they were and are consumed as the context signal. Reconciliation sits beside the
readers rather than inside them so that readers keep reporting what the format
actually says.

#### Where the key lands

`host.ResolveIdentity(byPath, priors)` runs the two gradings for a tree of
documents: documents first, then each document's blocks against the whole prior
set, in a deterministic path order so two runs over the same tree mint the same
keys. The resolved key is written onto the block as `Block.Unit`
([F-02](f-02-content-model.md)), and `convergence.BlockKey` reads it before
falling back to the name. The priors are the venue's own tree
(`venue.Tree.Priors()`), fetched before a push declares what it carries, rather
than a local store, so a fresh clone resolves exactly as a warm checkout does. A
block that matches a prior takes that prior's key: resolving against keys a
venue already holds returns them unchanged, only new content is minted, and
nothing is ever re-keyed.

#### Identity across languages

Everything above answers "is this the same unit as before?" within one language.
Pairing a source file with its translation asks a different question, and a
structural name cannot answer it. Ancestors' text is a legitimate naming input
(it is what makes `getting-started/install/p` readable to the translator who opens
the extracted file), but it is written in the document's own language, so the
translation names the same paragraph `kom-i-gang/installer/p`. Matching on the
name alone reached only the blocks above the first heading.

Each name therefore has a **translation-invariant twin**: the same structural
path with every segment that came from another block's text replaced by that
block's own structural identity. A heading is already addressed by its parent
trail plus its ordinal among siblings (precisely so that rewording it moves one
signal rather than two), so writing sections that way gives `h/h#2/p`, which
reads the same in every language. Readers compose it beside the name and carry
it on the block as `model.StructureAnnotation.Address`
(`convergence.BlockAddress`). Only formats whose names embed ancestor text
compose one; a key path, an element path or a catalog id is already invariant and
composes none.

The address is consulted for the one question the name cannot answer, and
nothing keyed by the name moves: the context hash, the store key and the XLIFF
`name` attribute all stay on the name. The file-scan
pairing (`host.OverlayTargets`) therefore matches on the name, then on the
address, and only then falls back to document position under an equal-block-count
guard, which is the last resort for formats that compose no address.

### The store key

Reconciled identity says which unit is which. The block-addressed store
(`core/blockstore`) needs a second thing: one string that keys a block and its
overlays within a project, stable across re-reads.

That key is neither the bare content hash nor the reader's block ID. Readers
assign file-local IDs that restart in every source file, so keying
on the raw ID lets blocks and target overlays from different files collide, with the
last writer winning. `blockstore.StoreKey` namespaces the key by the source file's
project-relative path:

```go
func StoreKey(sourceRel, blockID, sourceText string) string
```

The seed is `sourceRel` plus the block ID, or `sourceRel` plus the source text when
the reader assigned no ID, hashed with the same content-hash function. Every
`(file, block)` pair stays distinct while staying stable across re-reads, so
overlays written by a run are found again at merge time.

Both ends of a split run must spell the path identically, which is why
`blockstore.ProjectRel` exists and why it returns an error rather than falling back:
the file's path relative to the project root, in exactly the spelling the store's
keys use. A run that cannot name its own file the way merge will has nowhere to put
its work. The failure mode if the two sides disagree is not an error: merge finds
no translation for any block, reads that as pending work, and writes source text
into every target file with a zero exit code.

A `Store` is opened once per process and hands out `Session` transactions. Blocks
are written content-addressed and, once written, are immutable: tools append
`Overlay` layers (targets, annotations, skeletons) keyed by `(kind, blockHash)`
rather than rewriting blocks. This is the substrate flows run against; see
[C-01: The project model](../context/c-01-project-model.md) and
[C-03: The context store and graph](../context/c-03-context-store-and-graph.md).

## Consequences

- Paths and command output stay short and readable (`aB3xK9mL` rather than a
  36-character canonical UUID).
- ID generation needs no external dependency (`crypto/rand` is the only
  requirement), and rejection sampling keeps the distribution uniform.
- Collision probability at roughly 47.6 bits is negligible for the per-scope
  populations these IDs address, and random IDs avoid the enumeration leaks that
  sequential IDs produce.
- Content-addressable `BlockIdentity` deduplicates identical content across sources
  and detects edits by hash comparison, with no allocated block ID in the model.
- Because identity is the *output* of matching rather than a naming scheme, a
  reader can keep emitting the positional names its format actually supports, and a
  paragraph edited in place keeps the decisions recorded against it.
- Because the store key namespaces by source file, a project can ingest twenty
  files that each call their first unit `tu1` without a three-part composite key
  appearing anywhere else in the stack.

## See also

- [F-01: The framework and its modules](f-01-framework-and-modules.md)
- [F-02: The content model](f-02-content-model.md): where `BlockIdentity` sits on a Block
- [E-02: The format system](../engine/e-02-format-system.md): where reader-assigned IDs originate
- [C-01: The project model](../context/c-01-project-model.md): the store the keys address
- [C-03: The context store and graph](../context/c-03-context-store-and-graph.md): the durable content key in the graph
- [C-04: Unit state and the decision record](../context/c-04-unit-state-and-decisions.md): what is recorded against a reconciled unit
