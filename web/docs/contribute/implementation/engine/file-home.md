---
sidebar_position: 11
id: file-home
title: "Note: The file home (core/change/filehome)"
description: Implementation note on the change service's file home, which keeps each document as a file, stages an edit beside it, and commits by renaming under an advisory lock, applying the edit again when the file moved.
keywords: [core/change/filehome, file home, advisory lock, staged write, rename, change service, edition file, implementation note]
---

# The file home: `core/change/filehome`

The file home is the home of the change service ([E-09](../../architecture/engine/e-09-the-change-contract.md)) for documents kept as files in a working tree. This note covers how it locates a document, how a stage reads and writes it, how a commit settles against other writers, how editions in files of their own are joined and written, and the tests that hold it to the contract.

## Locating a document

`filehome.New(layout, options)` builds the home over a `Layout`, which resolves a reference into a `filehome.Doc`: the canonical reference, the file on disk (or the archive and its member for `container!entry`), a `Binding` that opens the format's reader and writer each configured for the document, the source locale and encoding, whether the format holds editions in the file, and a function that names the file of each edition kept in a file of its own.

`filehome.DirLayout` serves a directory with no recipe: references resolve under its root with `filehome.ResolvePath`, which refuses a path that leaves the root and confines an archive member to an archive under it, and the format is the one detection finds. The kapi host supplies a project layout (`host/changes.go`) that reads each source file with the format and configuration its content item declares, maps each target file back to the source and locale it holds, and names a source's target files through the recipe's target template.

## A stage

`Session.Stage` makes one pass over the document:

1. It hashes the file (`sha256:` over the bytes on disk; for an archive member, over the whole archive). That digest is the head the stage is applied against.
2. It joins the editions the change addresses that live in files of their own (see below).
3. It reads the document through the binding's reader with the writer's skeleton store wired, the read every read path of the service makes, so the blocks an edit is addressed from are the blocks the write sees.
4. It passes every block to the service's editor, which applies the operations addressed to it and reports the editions it changed.
5. When the change writes the document's own file, it writes the result through the same format's writer into a temporary file beside the document (`atomicfile.Stage`), with the mode the document has and the symlink resolved, hashing the bytes as they are written. A writer that re-reads the original gets the document's path; one that needs its bytes gets them.

When the reader and the writer both stream (`format.StreamingReader` and `format.StreamingWriter`) and the writer needs no copy of the original, the read and the write run concurrently through a streaming skeleton store, so neither the input nor the block stream is held whole. Any other pair reads every part and then writes them. The editor keeps only the blocks it changed. A pass that changes nothing in the document's own edition discards the temporary file and reports the file as read.

An archive member is read from its bytes, written to a buffer, and spliced into a copy of the archive (`container.Transform`) staged beside it, every other member copied as it was.

## A commit

`Staged.Settle` takes the advisory lock of every file the change touches, in the order of their lock files, then hashes each file again.

- Every file still has the digest the stage read: the staged files are renamed as they are.
- A file moved: another kapi process committed first, or a person saved. The home makes the stage's pass again under the lock, over the files as they now stand, with the editor starting afresh. An operation whose `if_match` the new content breaks is refused as `stale` with the content it found, and the change set is refused. Otherwise the new result is staged, and the files are hashed once more; a file that moved during that pass as well is refused as `doc_changed`.

`Staged.Commit` renames each staged file onto its target, and `Staged.Release` removes what was staged and drops the locks. The service settles every document of a change set before it commits any, so a refusal found at settle time leaves every document as it was.

The lock is `core/storage/filelock`: `flock(2)` on Unix and `LockFileEx` on Windows, on a lock file of its own named for the document's resolved path, so two links to one file share it. Inside a project the lock files sit under `.kapi/work/locks/`; outside one, under `kapi-locks-<uid>` in the temporary directory. The lock orders kapi's processes on one machine. An editor saving between the second hash and the rename is a conflict the lock cannot see; the next read finds the change.

`Options.BeforeSettle` is called after a stage and before the lock is taken. A test sets it to hold two writers at a barrier once both have staged against the same content.

## Editions in files of their own

In a project, the German edition of `docs/guide.md` is a file the recipe's target template names, read monolingually: its blocks hold German as their own content. When a change addresses an edition kept that way, the stage first reads the document once for each block's key and translation-invariant address, then reads the edition's file and pairs its blocks with the document's: by key, then by address (a heading written as its own identity, so a section pairs across languages), then by position when both files hold the same number of blocks. Each document block is given the edition from its partner while the editor sees it, and has it removed again before the document's own writer sees the block.

An edition the editor changed is written to its file:

- When the file exists and holds a partner for every changed block, the file is read through its own skeleton and each partner's content replaced, so every other byte of the file stays.
- Otherwise the file is written from the document's skeleton with the writer set to the edition's locale, every edition the file held kept as content and the blocks with no translation falling back to the document's own text, as `kapi merge` writes a target file. The directory is created when it does not exist.

A reference to the edition's file itself (`{"doc": "i18n/de/guide.md", "block": "installieren/p"}`) resolves to the document and the edition, and the block key the German file reads with is translated to the document's through the same join (`change.EditionKeyResolver`). Results echo the canonical reference.

## Preview

A preview stages each document and settles none: the service releases every stage after the commit check, so the temporary files are removed and the documents keep their bytes. `Staged.Diff` renders a unified diff of each file it would write, from the staged bytes against the file, when both are text of at most 1 MiB; an archive member is diffed as the member.

## Tests

- `changetest.Run` is the conformance suite, run by `TestFileHome_Conformance` on two JSON documents with mode `0640`.
- `TestFileHome_TwoProcessesEditingDifferentBlocksLoseNoEdit` starts the test binary as two writer processes over 50 rounds; each stages an edit of a different paragraph of one HTML file and waits at a barrier until both have staged. A third of the rounds let the first writer finish before the second settles, a third the reverse, and a third let the lock decide. Both edits are in the file after every round. Settling without the second pass loses an edit in the first round.
- `TestFileHome_TwoWritersOfOneBlockConflict` stages two edits of one block against one content: one lands and the other is refused as `stale` with what the first wrote.
- `TestFileHome_ALargeEditStaysInBoundedMemory` edits one message of a 100,000-message JSON catalog of about 8 MiB and asserts the heap the edit takes stays under 24 MiB; the streaming path takes under 2 MiB, and a buffered round trip of the same catalog takes about 80 MiB.
- `TestFileHome_ADocumentThatMovedOnceIsAppliedAgain` and `TestFileHome_ADocumentThatKeepsMovingIsDocChanged` move the file between the stage and the commit.
- The edition tests write a translation through its own file, materialize a missing one, name the editions a source edit leaves stale, and edit an archive member in place.
