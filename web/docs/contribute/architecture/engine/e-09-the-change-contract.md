---
id: e-09-the-change-contract
sidebar_position: 9
title: "E-09: The change contract"
description: "Every change to content, to a review decision or to a context asset is an operation in one change set, kapi.change/v1, applied by one service in core/change through the home that holds the document's text."
keywords: [neokapi, architecture decision, core/change, change set, kapi.change/v1, if_match, edition revision, change service, file home, commit check]
---

# E-09: The change contract

## Summary

Content changes in one way. A caller describes the change as a **change set**
(`kapi.change/v1`): an envelope of ordered operations, each addressed to one
edition of one block of one document and each naming the revision of that
edition its sender read. One service in the framework, `core/change`, applies
it. The service reads each document through the **home** that holds its text,
applies the operations in memory with `change.ApplyBlock`, checks what changed,
and commits every document or none. A file in a working tree is one home; the
file home commits by renaming a staged file onto the document under an advisory
lock, and applies the change again when the file moved since it was read. A
flow commits each document it writes through the same home and records it as
the flow's edit.

The service lives below every surface. It imports `core/model` and
`core/safeio` and nothing above them, so the CLI, the agent tools, Kapi Desktop
and an application that embeds the engine all build the same service and differ
only in the home and the hooks they give it.

## Context

A change to content has to answer the same questions wherever it comes from:
which block it means, whether that block still says what the sender read,
whether the result keeps the inline codes and structure the format needs, what
the change does to the edition's status, and whether governance allows it. A
document that changes in two places at once has to end with both changes or
with a clear refusal, and never with one change silently replaced by the other.

The engine reads documents into blocks whose editions hold runs
([F-02](../foundations/f-02-content-model.md)) and writes them back through
format writers that replay a skeleton ([E-02](e-02-format-system.md)). Tools in
flows change blocks through their views ([E-03](e-03-tool-system.md)). The
change contract puts both on one footing: a tool's write and a person's edit
are the same operations, applied by the same function, under the same rules.

## Decision

### The change set

A change set has an envelope and operations.

| Field | Meaning |
| --- | --- |
| `schema` | `kapi.change/v1` |
| `mode` | `apply` (the default) or `preview`, which computes and checks everything, writes nothing, and returns a diff per document |
| `gate` | `enforce` (the default) or `report`: what a failing governance finding the change introduces does |
| `require_basis` | refuse a write to a derived edition whose authoritative edition moved since the sender read it |
| `note`, `evidence` | what a person reads in history, and where the wording behind the change was seen |
| `ops` | the operations, applied in order; a change set with none changes nothing |

The content operations are `set_content`, `replace_text`, `set_attribute`,
`mark`, `remove_edition`, `annotate`, `unannotate`, `insert_block`,
`delete_block` and `native`. `decide` records a review decision, and `term`,
`memory` and `recipe` change the project's terms store, content memory and
recipe. The envelope carries no actor: the transport that delivers a change set
says who sent it. A `set_content` or `replace_text` may state how a tool
produced its content (`origin`: the tool, and the kind and engine it drew on),
as a run printed with `--print-ops` states each translation it wrote. A tool in
a flow keeps that origin on the translation; an edit a person or an agent sends
is theirs, and the origin it states is not kept.

`change.Decode` reads a change set strictly: an unknown field or operation is
refused with the JSON pointer of what was wrong. `changeschema.Schema` is the
JSON Schema generated from the same Go types. TypeScript clients use
`@neokapi/contract-types`, whose change set and operations are generated from
that schema and whose result, read and description types are generated from the
structs the service marshals; a drift gate regenerates them on every change. The
note [The change applier](../../implementation/engine/change-applier.md) covers
the types, the decoder, the schema and the TypeScript types.

### Addressing and revisions

An operation addresses `{doc, block, edition}`. `doc` is a project-relative
path, `container!entry` for an archive member, or another home's document key.
`block` is the key a read reports: the durable key where reconciliation
assigned one, else the name the format gives the block, else the reader's id
(`change.BlockKey`). `edition` is an edition key such as `fr` or
`en;channel=short`; empty means the document's own edition.

An operation that changes existing content carries `if_match`, the revision of
the edition its sender read. `model.EditionRevision` hashes the edition key and
its runs, codes and their attributes included, and nothing else, so the same
token holds in every home. `absent` creates an edition, and `*` writes whatever
is there. Every `if_match` is checked against the content as it stood when the
change set began, so a sender never computes an intermediate revision.

A position names that content too. An edit's `start` and `end`, a run `range`,
and the run index a `path` walks through all refer to the edition at the
revision the sender read, and the applier moves each through the text the
operations before it changed in the same edition, in the order they were sent:
two fixes `kapi check` prints for different words of one block, each guarded by
the revision the check read, both land where their sender meant. A position
inside text an earlier operation replaced, or after a `set_content` of the
sequence it lies in, has no place in the edition as it stands, and the
operation is refused as `guard` (`overlap`) with a message naming both
operations by their place in the change set. A `find` matches the text as the
earlier operations left it, and an annotation's anchor marks that text. An
in-process caller that builds each operation on the result of the one before
it, as a transform's passes do, applies them with `BlockEnv.Chained`, under
which every position reads the edition as the operations before it left it.

### The applier

`change.ApplyBlock` applies content operations to one block in memory and
returns one result per operation. It enforces the rules every content operation
obeys: placeholder text is parsed against a reference code set, inline codes
keep their editing constraints, plural and select structure is kept, run flags
survive, and overlays follow the edit on every edition. `change.Consequences`
says what an applied edit does to an edition's status and origin, and
`change.Diff` turns two blocks into the operations between them. The applier
note describes each rule.

### The service

`change.NewService(formats, homes, options…)` builds the service.

- **`Read`** reads a page of a document's blocks. Each block carries the
  reference to copy into an operation, the revision to send as `if_match`, the
  content as placeholder text, its inline codes with their attributes and the
  attributes `set_attribute` can write, its plurals and selects with the path to
  each branch, its other editions with their status and staleness, each one's
  own plurals and selects and its codes where they differ from the block's, and
  the operations it accepts. A read of the file one edition lives in, such as the
  German file of an English page, shows that edition as each block's own, with
  the document's own edition among the others, so a reference copied from it
  edits the German. A page ends with a cursor; a cursor into a document that
  changed since is refused as `stale`. `ReadEach` reads every block of a
  document in one pass and hands each to a callback beside the block it was
  read from, for a command line that streams a whole document.
- **`Apply`** applies a change set in two phases. It checks each operation's
  body as `Decode` does, since a sender in the process never passed the
  decoder, asks the policy about every operation, groups the operations by
  document, and opens each document in its home. It then prepares every document: the home reads it, the
  service's editor applies the operations addressed to each block, and the
  home stages the result. The service refuses an edition the staged file does
  not change, because the format has no place for it there, and runs the
  commit check over every changed edition. If any operation of any document is
  refused, nothing is written: the refused operations say why and every other
  one is `not_applied`. Otherwise the service takes the commit locks, settles
  each document, and checks again any document its home applied a second
  time, since that pass is the one that lands. It then commits each document
  and, with the locks still held, applies the decisions and asset operations,
  which bind to the content that landed, and records the change.
- **`Describe`** says what a format supports. `DescribeFormat` hands
  `FormatOps` the operations the format's round trip carries (`set_content` in
  either form, `replace_text`, and `remove_edition` where the format holds its
  editions in one file) and what its writer declares
  ([E-02](e-02-format-system.md#edits-a-writer-can-write)), `insert_block` and
  `delete_block` among it where the writer can add and remove blocks. For a
  document the declaration is the one its home reports for the document's
  writer (`DocInfo.Capabilities`), which `ApplyBlock` applies every operation
  with, less the structural operations its home cannot write there.
  `WithDescriber` replaces `DescribeFormat`, and that one function is what
  `Describe` reports, what a read lists per block, and what `Apply` refuses
  outside of.
- **`History`** lists the recorded changes to one edition, most recent first,
  beside the revision the edition holds now. It reads the block from its home,
  so a reference resolves as a read resolves it, and asks the
  `EditionHistories` hook for the changes: each with its record, the revisions
  around it, the basis a derived edition was made from, who made it (null when
  nobody knows, as for an edit made outside kapi), through which surface, and
  when.

A document's edition lives in the document (its own edition, or one a bilingual
file holds), in a file of its own (a project's target file), or nowhere (a
monolingual document outside a project, or an edition with a tone or a channel
in a bilingual file, which keeps one translation per language). An operation
on an edition with no home is refused as `unsupported`.

### Adding and removing blocks

`insert_block` and `delete_block` change which blocks a document holds. A
writer replays a skeleton with a slot for every block its reader read and none
for a new one, so these two are written by the format: the service hands them
to the home, and the format's writer writes the shell of each block added or
removed. A writer declares the operations it writes and writes them through
`format.StructureEditor`.
`Describe` reports them where a writer declares them, and every other format
refuses them as `unsupported`. The JSON, YAML and ARB writers declare both: in
a key-value catalog a block's shell is its key and the value beside it. A
writer's declaration can depend on its configuration, so describing a document
reports the operations its home can write in that document
(`StructuralSession`), and a read's per-block operations and `Apply` follow
the same list.

- `delete_block` addresses a block and carries `if_match` as a map from
  edition to revision. The map names the block's own edition, and may name any
  other edition its sender read. A map without the own edition is `invalid`;
  an edition named at another revision is `stale`, with the edition as it
  stands, and so is an edition the map names and the block lacks. The block
  goes with every edition it holds, its own and each translation in a file of
  its own, whether the map names it or not.
- `insert_block` names the document, the new block's key (`name`), the key it
  goes `after` or `before`, and the content of each edition. The new block goes
  in the object or mapping its key path names, beside its anchor or, with
  neither, last there. The document's own edition is required, and any other
  must live in a file of its own. A key a block already answers to is `stale`,
  a neighbour no block answers to is `not_found`, and the content obeys the
  rules of every content operation: a new block has no inline codes to name,
  so text with a code placeholder is a `guard` refusal.

Operations apply in order, and each sees what the ones before it left: a block
can go beside one an earlier operation added, and a block is replaced in its
place by adding the new one beside it and then removing the old. An operation
that changes the content of a block the change set adds or removes is refused
as `invalid`; a new block's content travels in its `editions`.

A home writes the structural edits first and then applies the rest of the
change set to what the writer wrote. A new block's content is therefore what
the format reads back (an ICU argument in an ARB message reads as the
protected code it is), and the result's `after` is the revision a later read
reports. A new block the format reads differently from its content, or does
not read at all, and a removed block it still reads, are refused as
`unsupported`. The commit check sees each new edition as created, and the
record holds every edition added or removed.

### Homes

A home holds the text of documents and decides when two writers conflict.

```go
type Home interface {
	Name() string
	Open(ctx context.Context, doc string) (Session, error)
}
```

A session reads the document at its head, stages a change by running the
service's editor over its blocks, and hands back a staged change that names the
commit locks it needs, settles (makes sure the change still applies with those
locks held), commits and releases. A home that reads a document whole streams
every block past the editor; one that keeps rows looks up only the blocks the
change names. The service takes the locks of every document of a change set in
one order, by their keys, so two change sets take the locks they share in that
order and wait for each other without deadlock. A change set that names one
file through two documents, such as two members of one archive or a file and a
link to it, is refused as `invalid`, because each document would stage the
whole file from what it read.

`changetest.Run` is the conformance suite every home passes: an edit lands and
reads back, a replayed change set is stale and carries the current content
while writing nothing, edits to different blocks commute, also when one lands
between the other's stage and commit, a refusal in one document leaves every
document as it was, whether it is found at the stage or at the commit, a
preview writes nothing, a missing block is not found, the same content said
again is unchanged, a file keeps its mode, a removed translation reads back
absent while a replay of its removal is stale and writes nothing, and a change
set with no operation applies and writes nothing. Each home runs the removal on
a translation it holds: the stream's own rows, a PO catalog for the file home,
and for the project homes the catalog a PO source's target template names
(`po/fr.po` beside `po/en.po`), where the suite also checks that the removal is
written there and the source catalog keeps its bytes.

**The file home** (`core/change/filehome`) keeps each document as a file. A
stage reads the document through its format's reader with the writer's
skeleton store wired, and writes the result through the same format's writer
into a temporary file beside the document with the document's mode. Every file
a stage reads is hashed before it is read. A commit takes the advisory lock
(`core/storage/filelock`) of each file, hashes the files again, and renames the
staged files onto them when each is still what the stage read. When one moved,
the home reads them again under the locks, applies the change once more and
renames that result; an operation whose `if_match` the new content breaks is
refused as `stale`, and a file that moves during that second pass as well is
`doc_changed`. Two processes that edit different blocks of one file both land.
An edition kept in a file of its own is written through that file's skeleton,
and an edit that needs a block the file does not hold is refused, so the file
is never rewritten from the document; a file that does not exist yet is
written from the document's skeleton. A change set that adds or removes
blocks has the format's writer write those edits into the document's file and
into the file of each edition the blocks hold or name, keeping them in memory
until the commit, and the stage's pass reads the result. A stage that removes a
translation a bilingual file holds reads its write back, and refuses the removal
as `unsupported` when the translation is still there: the XLIFF and TMX writers
write a translation of every unit, from the source where a block holds none,
so a removal there would put the source in the translation's place. Where the reader and
the writer both stream and no block is added or removed, the document is never
held whole. The home reports what the document's writer declares: an
in-process writer's declaration, with the writer spelling a changed attribute
or a new code itself (`change.WriterCapabilities`), or a plugin format's
manifest declaration, whose writer spells them when it writes
(`change.DeclaredCapabilities`). A home given a backup suffix copies each file
a commit replaces beside it, under the lock, from the bytes the change was
applied to. The implementation note
[The file home](../../implementation/engine/file-home.md) has the details.

### Flows

A flow writes a document whole: its writer renders every block the run passed
through, into the file it reads or into a target-language file built from the
source's skeleton. The file home commits that document too
(`filehome.Home.Produce`): the run digests the destination before it reads,
the writer's output is staged beside the destination, and the commit renames it
under the destination's lock only while the destination still holds what the
run began from ([E-01](e-01-processing-engine.md#the-write-stage)). Every flow
the kapi host runs commits this way: `kapi translate`, `pseudo-translate`,
`run`, `exec` and `up`, the same runs in Kapi Desktop and over MCP, a pull's
target files, and the delivery of a convergence pass's drafts. The host's flow
home takes its locks where the project's change service takes them, so a flow
and a change set on one file take turns.

Inside a project the host records each document a flow wrote as one
`content.edit`, through the recorder a change set's commit records through,
with the actor `tool:<flow>` and the origin `flow:<flow>`. `kapi exec` takes
the project the command resolves, as `kapi apply` does, for a file the project
holds. The record keeps revisions and hashes, no text. Its transitions are the
run's effect on the file as the service reads it: the document is read through
the service before the run and again after the commit (a commit that finds the
file holding the run's bytes already, as it did when the run read it, reuses
the first read), and each edition whose revision moved is a transition, with
the stamp the producing tool left as its producer. A translation's basis is the
source the run read before it ran, with that source's content hash: the commit
guards the file the run writes, so a source edited while the run worked is
drift against the basis rather than the basis. A translation the run reproduced
unchanged is recorded too, once, when the block history does not already say a
flow wrote it from the source the block holds, and not over a person's or an
agent's write of the same wording, which stays theirs. So the history keeps the
basis of every translation the loop made. That basis is what coverage grades an
undecided translation by, what a decision on it starts from, and where the
staleness gate finds what governed it. An undecided record in the decision
ledger (a basis an earlier release recorded there for a Kapi Desktop edit or a
loop pass) yields to the flow's last write when that write is the translation
the file holds.

A destination that moved while the run worked is applied again through the
service from the run's operations (`set_content` on each edition the run
changed, guarded by the revision read before the run, followed by the
provenance the producing tool left). The service reads the file under its lock,
and the document lands when no edition the run changed has moved too, and is
refused as a whole otherwise: when one has moved (`stale`), or when the run
wrote a block the file does not hold yet. The run then reports the document as
moved, and the next run writes it. A target-language edition the run left
without a translation is rendered from the source by the writer, and is left as
the file holds it when the run's changes are applied again.

Under `--print-ops` the run commits through a home that writes nothing
(`filehome.Options.WriteNothing`) and records nothing. What it changed in each
document becomes operations, the difference between each block as the reader
gave it and as the writer received it (`change.Diff`), each guarded by the
revision the service read before the run and each `set_content` of a
translation carrying its basis and, as each operation on a translation does,
the `origin` the producing tool left. Before a document's operations join the change
set, they are applied to a private copy of the file through the service `kapi
apply` reaches, and they are printed only when the copy then holds the bytes
the run would have written. A run writes a target-language file whole from its
source, while the service edits the blocks a file holds as it stands, so the
two differ when the source gained a block the file does not hold or the file
holds an entry or an order of its own; such a file is named on standard error
and left out. So is a file the run would write where the service does not keep
the edition (an output path given on the command line in place of the recipe's
target), a conversion, an export and an archive. `kapi apply` of the change set
writes the bytes the run would have written, and records the edit as its
applier's: a person's or an agent's edit is theirs, so the `origin` the
operations state is not kept on the translation, and `kapi apply` prints one
line naming the tools and saying so. A run with nothing to change prints a
change set with no operation, which applies and writes nothing.

A printing `kapi up` runs one pass. A gated pass drafts into its private tree
as any pass does (`Options.WriteUnder` lets the printing home write there),
the gate decides from the drafts, and delivery prints what it would commit for
each locale that clears its gate; a parked locale is named and prints nothing.
A printing run records no change, absorbs nothing into the content memory and
stamps nothing. As any pass does, it extracts the source into the derived store
and caches under `.kapi/work`, and records the identity of a document it reads
for the first time (`document.adopt`). In a
project a `kapi translate`, `pseudo-translate` or `run` without `-o` writes no
file, and prints nothing with a note.

### Hooks

The service calls six hooks a host supplies. Each is optional.

| Hook | Called | Without one |
| --- | --- | --- |
| `Policy` | for every operation, before anything is read | every operation is permitted |
| `CommitCheck` | over the changed editions, before anything is written; it returns findings before and after, and the governance fingerprint it used | nothing is checked |
| `Assets` | to prepare `decide`, `term`, `memory` and `recipe` before anything is written, and to apply them after the content landed | those operations are refused as `unsupported` |
| `Recorder` | after the homes committed, with the transitions and the fingerprint | nothing is recorded |
| `EditionStates` | by a read, for the status and basis of a derived edition | a read shows the status the document holds and no basis |
| `EditionHistories` | by `History`, for the recorded changes to an edition | a history lists nothing |

The service refuses a change only for a failing finding it introduces
(`change.Introduced`). Under `report` the change lands with its findings, and a
person's overridden findings go to the record.

The kapi host builds the service for a surface with `App.Changes` and
`App.ChangeService`. Inside a project the file home's layout resolves a
reference the way the recipe does: a source file is read with the format and
configuration its content item binds, the file of a translation is that edition
of its source, joined by key, then by translation-invariant address, then by
position, and a translation with no file yet is written from the source's
skeleton. Outside a project a reference is a path under the working
directory, read with the format detection finds by name, then by content. A
file of a translation interchange format, or of a multilingual string catalog
(an Xcode `.xcstrings` file), holds its editions in the file, unless the recipe
writes the source's translations to files of their own: with a target
`po/{lang}.po`, the French of `po/en.po` or of `po/messages.pot` is read from and
written to `po/fr.po`, in French, and the source catalog holds no edition, for
every surface. Decisions and asset operations
land through the host's review-queue and asset functions, a decision bound to
the wording the change set landed rather than to a later read of the file; on
an edition with no content in its home, such as a parked locale's draft, the
decision binds to the draft the project store holds. Such a decision names the
edition it read as `absent`, and is refused as stale once the home holds the
edition. The hooks each plug in at
one function of the host: the commit check is `App.CommitCheck`, which holds a
service outside a project to hygiene alone; the policy is `ChangePolicy`; the
recorder is `App.EditRecorder`, inside a project; a read takes a derived
edition's basis from the project's block history, where the most recent
recorded change to the edition left the content it holds; and a history lists
the edition's rows of that block history.

On the command line, `kapi apply` hands a decoded change set to the service,
`kapi inspect` prints the service's read records, and `ksed` compiles its
substitutions into `replace_text` operations and applies them through it
([S-01](../surfaces/s-01-kapi-cli.md)). All three build the service once per
run: for the project the command names or discovery finds, for the files under
its root, and over the working directory otherwise, where a reference may lead
out of it as a path on a command line does. Inside a project a file outside the
root is named by its absolute path, and `kapi apply` edits a change set of such
files over the working directory. The hooks of a service outside a project are
given a command that resolves no project, so discovery never puts an edit made
outside one under a project's governance. Only `ksed` reads a file no format
claims as plain text. A command that prints an edited document (`ksed` without
`-i`) applies the change to a private copy, located and read exactly as the
file itself, and writes the copy out. A read takes no lock and leaves a project
as it was: the lock directory, and the ignore rule that keeps it out of a
commit, are written when a change first commits, and the recorder opens the
project store then, before anything is written. A read takes a derived
edition's basis from a project store that exists and creates none. A preview
takes no lock and records nothing, and its commit check reads the project
store when the project has one and creates none. When the project has no store
yet, a preview checks an edit without the voice and the terms a store binds
from the workspace.

A `set_content` on a code comment, in a source file only the comment layer
reads, goes through the comment write path, which keeps its own guarantees: the
file is read again before it is written, the result must parse, and the
language's formatter must agree. A file is read for its comments when no format
reads it: none that `--format` names, the recipe binds, or detection finds by
extension or content, as a Qt Linguist catalog is found in a `.ts` file. What
was written is checked, scoped to the change, and under the enforcing gate a
failing finding, or a check that read nothing, exits 3. The comment path runs
neither the commit check nor the recorder: the check runs after the write, and
nothing records a comment edit.

The MCP tools `read_blocks`, `apply_edits` and `describe_format` build the
service for each call's project and send every change set as the calling agent
([S-03](../surfaces/s-03-agent-surfaces.md)). The browser engine's `kapiRead`,
`kapiApply` and `kapiDescribe` build it through the same function and carry
the contract as JSON in and out: a refusal, a change set that does not decode
included, is answered as a result. A call of either surface reads a bilingual
file that holds its translation, such as a PO catalog, in the one language
other than the source that its read's editions or its operations name, as
`kapi apply` does. The page names
the sender, a person unless it says an agent, and in a project an applied
change is recorded in the browser's workspace log
([WASM Engine ABI](../../implementation/surfaces/wasm-engine-abi.md#the-change-contract)).

The verbs that write whole translations build the service with
`Materialize` set, so the file home writes each translation's file from its
source's skeleton, keeping what each block's partner in the file held where the
change set leaves it, and a translation follows the source's structure, even
where the change set changes no content in a file whose blocks no longer pair
with the source's. A bilingual translation file that still holds every unit
of its source keeps its own skeleton, header included.
`kapi merge -i` compiles a returned XLIFF, PO or `.kpz` into `set_content`
operations carrying the `if_match` and `basis` each unit was extracted against,
under `require_basis` and the enforce gate, and sends them as a person;
`kapi extract` stamps each unit with the revision of the source it carries and
the translation's revision a read through the service gives
([M-01](../multilingual/m-01-bilingual-interop.md)). `kapi merge` with no `-i`
sends the targets the block store holds for a source and language as one
change set from the tool `merge`, and `kapi pull` sends the runs the server
holds for each pulled translation as the tool `pull`, each `if_match` the
revision the read before it found. A pull hands the writer of a document with
locale-variant media through `ChangeServiceOptions.WriterHook`, and writes a
target in another format than its source, which no edition reaches, with the
target's own writer. The record names the surface as the origin: `merge` or
`pull`.

Kapi Desktop reaches the service through four bindings, `Read`, `Apply`,
`Describe` and `History`, each taking and returning the contract's JSON as a
string. It builds the service for the tab's project with `desktop` as the
origin and sends every change set as the person at the keyboard: an edit in the
review pane or the document view, a check finding's fix, and Approve and Reject
as `decide` ([S-02](../surfaces/s-02-kapi-desktop.md)).

`kapi check` gives a finding whose rule names a replacement the operation that
applies it (`check.Fix`): a `replace_text` of the words the finding objects to,
by the run range the checker reported, under the revision of the block's own
edition the check read. Sent as it is, it lands while the block still says what
the check read, and is refused as stale once it does not. The fixes of one block
name places in the same text, so they compose as the edits of one operation;
as separate operations of one change set, each would see the text the one
before it left. A replacement is plain text, and `ApplyTextEdits` keeps a code
only at either end of the text it replaces, so a finding whose words have an
inline code among them gets no fix. The fix names its document as the project's
change service resolves one, by its path from the project root, and the file of
a translation gets no fix: the service reads that file as the translation's
edition, while the check read it as source and held it to the source
language's rules.

### Results and errors

A result has a status (`applied`, `refused`, `previewed`, or `partial` when an
I/O error stopped the final renames after some files landed), a record id, one
entry per file written or read with its digests before and after and whether it
was written, and one result per operation with its revisions, the positions it
resolved, the derived editions it left on an older basis (`invalidates`), and
on a refusal an error from a closed set of codes, each mapped once to an exit
code and an HTTP status. In a `partial` result the operations on the files that
were not written are `not_applied`, and so are the decisions and asset
operations, which wait for content that all landed; the record holds what did.
A `stale` refusal carries the edition as it stands, so the sender can rebase
without another read. A resource bound `core/safeio` reports is
`budget_exceeded`. A change set refused before any operation is considered,
because it does not decode or the request carrying it is refused, is answered
with the same result shape (`change.ErrorResult`): status `refused`, no record,
no files or operations, and the error. `kapi apply --json` prints that result
for a change set that does not decode, and exits 2, as MCP and the browser
answer one.

## Consequences

- A sender always learns whether its change landed and, when it did not, which
  of a few things to do next: retarget, re-read and resend, fix the content, or
  ask a person.
- Concurrent kapi writers of one file lose nothing: each file a change reads
  is hashed before the read and again under the lock. The lock orders kapi's
  own processes, and in the browser, which runs one, the calls of the page;
  an editor that saves between the re-hash and the rename remains a conflict
  the next read sees.
- An operation reported `applied` reached the file. One the format has no
  place for in that file is refused.
- Bytes kapi could not read as UTF-8, in a file in another encoding, are never
  overwritten with the U+FFFD a read shows for them: an edit that would write it
  there is refused as `unsupported`, and edits elsewhere in the file keep them.
- A change set is all or nothing across documents up to the final renames.
  Records follow the commit, so a record that fails to write leaves content
  that the next read finds and records as observed.
- A surface that adds a write path adds a home or a hook, and the contract, the
  applier and the rules stay one.

## Related

- [F-02: The content model](../foundations/f-02-content-model.md)
- [E-02: Format system](e-02-format-system.md)
- [E-03: Tool system](e-03-tool-system.md)
- [The change applier](../../implementation/engine/change-applier.md)
- [The file home](../../implementation/engine/file-home.md)
- [S-03: Agent surfaces](../surfaces/s-03-agent-surfaces.md)
