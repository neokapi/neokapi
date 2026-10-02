---
sidebar_position: 7
id: change-applier
title: "Note: The change applier (core/change)"
description: Implementation note on core/change, the package that holds the kapi.change/v1 contract types, strict decoding, the generated schema, ApplyBlock, the status consequences of an edit, and Diff, and on how tools write content through it.
keywords: [core/change, ApplyBlock, change set, edition revision, if_match, Diff, implementation note]
---

# The change applier: `core/change`

Every change to a block's content goes through one function, `change.ApplyBlock`. This note covers the package that holds it: the change-set contract, the rules an operation obeys, and how tools in a flow reach it. The package imports `core/model` and nothing from the host, the CLI or any surface, so every surface can build on it.

## The contract types

A change set (`change.Set`) is an envelope of ordered operations (`change.Op`) under the schema `kapi.change/v1`. The envelope carries `mode` (`apply` or `preview`), `gate` (`enforce` or `report`), `require_basis`, a `note` and `evidence`. It has no actor field: the transport that carries a change set says who sent it, and `ApplyBlock` takes the sender in its `BlockEnv`.

An operation is flat on the wire, `{"op": kind, "at": …, "if_match": …, …}`. `Op` holds the kind, the address (`change.Ref`: document, block, edition), the precondition, the basis of a derived edition, and a body whose type matches the kind (`*change.SetContent`, `*change.ReplaceText`, and so on). The edition in a reference is parsed with `model.ParseEditionKey`, which keeps it canonical and refuses a language that is not a locale or a dimension other than tone and channel.

`change.Decode` reads a change set as one object, as JSONL with the envelope on its first line, or as an array of operations. Decoding is strict: an unknown field, an unknown operation, a missing required field, a `null`, a value of the wrong type and a run payload that carries native `data` are refused as `invalid`, and the error's `Pointer` is the JSON pointer of the value at fault (`/ops/2/edits/0/find`).

`change.Schema` is the JSON Schema of a change set, generated from the same Go types with `github.com/google/jsonschema-go`. Each operation is a `oneOf` member whose `op` is a `const` and which allows no other properties. `core/change/testdata/schema.golden.json` pins it; regenerate it with `go test ./core/change -run TestSchemaGolden -update`. A test validates the contract's examples against the schema and the decoder alike, so the two cannot drift apart.

Refusals use a closed set of codes (`change.Code`), each mapped once to an exit code (`ExitCode`: 2 for `invalid`, 5 for `unreachable`, 3 for the rest) and an HTTP status (`HTTPStatus`).

## Revisions

An operation names the revision of the edition its sender read. `model.EditionRevision(block, key)` is `r:` and 16 hex digits of the SHA-256 of the edition key, a zero byte and the edition's runs as canonical JSON (`model.CanonicalRunsJSON`, the form the TypeScript mirror writes). Inline codes count, with their data and attributes. Status, origin and every other edition are left out, so a review decision does not move a revision and a source edit leaves a translation's revision where it was. An edition the block does not hold reads as `absent`.

`if_match` is a revision, `absent` to create an edition, or `*` to write whatever is there. A derived edition's write also records its `basis`, the authoritative edition's revision it was made from; with `require_basis` set, a basis that no longer holds is refused as `stale`.

## ApplyBlock

`ApplyBlock(block, ops, env)` applies `set_content`, `replace_text`, `remove_edition`, `annotate`, `unannotate` and the in-process provenance operation to one block in memory, and returns one result per operation. It checks every precondition against the block as it stood when it was called, applies the operations in order, and writes nothing unless all of them apply. A refusal names the first refused operation in every other result's `blocked_by`, and a `stale` refusal carries the edition as it stands. `set_attribute`, `mark`, `insert_block`, `delete_block` and `native` are refused as `unsupported` until a format declares the capability.

The rules each content operation obeys:

- **Text is a form of runs.** Placeholder text (`<x id="1"/>`) is parsed against a reference code set: the content being replaced, or the authoritative edition's codes when the operation creates a derived edition. A derived edition can also take back a code the authoritative edition holds.
- **Codes keep their constraints.** Dropping a code needs `deletable`, repeating one needs `cloneable`, moving one past another needs `reorderable`, resolved from the run or the vocabulary. Paired codes stay as well nested as the reference's. A violation is `guard` with subcode `codes_changed`.
- **Structure is kept.** Text over an edition that holds a plural or select is refused as `structure_lost`; `path` names the branch to replace or edit, and a path to a missing plural form adds it. Runs replace a whole structure.
- **Run flags survive.** Rebuilt text runs split where `TextRun.NoTranslate` changes. Text the edit leaves alone keeps its flag, and replacement text is flagged when everything it replaces was.
- **Native data stays with the format.** A wire payload carries no `data`. A code named by its id takes its data from the reference, and a payload that changes a held code's attributes is refused, since `set_attribute` is what changes them.
- **Overlays follow the edit on every edition.** Spans on the edited edition are remapped across the edit, dropped where they overlapped it, and checked to resolve in the new runs.

Positions in `replace_text` are code points of the text of the sequence a `path` reaches, in which every code, plural and select has zero width. An edit names its text by `find` (with `occurrence` among several matches), by `start` and `end`, or by a `range` of run positions. The result's `resolved` echoes each one as run positions under `model.RangeAnchor`'s attribution, where the end of a text run is the start of the run after it.

`model.TextEdit` counts code points too. `model.RangeAnchorForBytes` converts byte offsets a detector reports.

## Status consequences and Diff

`change.Consequences` says what an applied edit does to an edition's status and origin. An edit to the authoritative edition drops an established edition to written and lists the derived editions in `invalidates`. A person's edit of a derived edition makes it translated with a human origin, and an agent's makes it translated with an agent origin. A tool's edit leaves both as they were: the tool records its draft with the provenance operation.

`change.Diff(before, after)` returns the operations that turn one block's editions into another's, each guarded by the revision it changes. It emits a `replace_text` when one text edit with every code kept describes the change exactly, and a `set_content` otherwise. A property test applies the diff of every fixture and variant, codes, plurals, selects and translations included, and asserts the result equals the changed block, then sends the same operations through JSON and `Decode` and asserts the same.

## Tools write through it

A tool's view (`core/tool/view.go`) applies each target write, provenance stamp and overlay write as an operation at once, through `ApplyBlock`, as the tool, with guard violations landing as findings. Applying at once keeps read-your-writes: a handler that sets a new target and stamps it finds the target it set. A write the applier refuses becomes the handler's error. A tool that overrides `Process` writes through `tool.WriteAs`, which returns the same refusal, and a `Transform` returns an `EditPlan` whose `Ops` compile to `set_content` operations on the source and each replaced target.

## The edit guard

`scripts/editguard` type-checks every module and reports each call that points a format writer at a file (`SetOutput(path string) error`) outside the format packages and the places it lists, each with what it writes. `make check-edit-writes` runs it in `make lint`, `make pre-push` and CI.
