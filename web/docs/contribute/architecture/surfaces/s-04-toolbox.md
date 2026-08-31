---
id: s-04-toolbox
sidebar_position: 4
title: "S-04: Toolbox utilities"
description: "kcat, kgrep, ksed and kdiff are format-aware reimaginings of cat, grep, sed and diff, and kconv converts between formats. They ship as busybox-style names on the kapi binary, operate over one block-text projection, and follow the grep-style exit-code convention."
keywords: [neokapi, architecture decision, toolbox, kcat, kgrep, ksed, kdiff, kconv, busybox, multi-call, block projection, exit codes]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# S-04: Toolbox utilities

## Summary

`kcat`, `kgrep`, `ksed`, and `kdiff` are format-aware reimaginings of `cat`,
`grep`, `sed`, and `diff` that operate on the **text inside** any format kapi
understands (a Word document, a JSON catalog, XLIFF, Markdown) rather than on
raw bytes. `kconv` has no classic analogue: it converts a document into another
format by handing the blocks, and the role each carries, to a different format's
writer. All five are the **same `kapi` binary under different names**, dispatched
on `argv[0]` busybox-style. Each operates over one shared block-text projection
and follows the grep-style exit-code convention.

## Context

The recurring need is to read, search, and edit the human-readable content of
files that neither an editor nor a classic Unix tool can meaningfully touch. A
Word document is a zipped XML container. An XLIFF file interleaves source and
target inside markup. A JSON catalog buries strings among keys and structure.
Running `grep` on those byte streams matches markup and misses content split
across runs; running `sed` on them corrupts structure.

kapi already has the machinery to do this correctly: format detection, readers
that yield blocks, writers that reconstruct structure faithfully through the
skeleton store ([E-02](../engine/e-02-format-system.md)). The toolbox exposes
that machinery behind an interface engineers already have in muscle memory.

Two goals shaped it. **Zero marginal footprint**: these must not be more binaries
to build, sign, and distribute. **Fidelity to the classics**: the option surface
and the exit-code behaviour should be what a `grep` or `sed` user expects,
including the shorthand letters that a CLI's global flags would otherwise shadow.

## Decision

### Multi-call dispatch

The commands live in the shared CLI base (`cli/toolbox*.go`) and are built into
`kapi`. They are reachable two ways.

**As multi-call names.** The build and the Homebrew formula create `kgrep`,
`ksed`, `kcat`, `kconv`, and `kdiff` as symlinks to `kapi`. At startup
`cli.BusyboxRoot(app, os.Args[0])` normalises the program name, stripping any
`.exe`, and on a match returns a standalone root for that utility instead of
the full kapi command tree. The standalone root owns the application lifecycle
(config load, init, shutdown) so the utility behaves identically however it was
launched.

**As hidden kapi subcommands.** `kapi kgrep`, `kapi ksed`, `kapi kcat`,
`kapi kconv`, and `kapi kdiff` are thin proxies with `DisableFlagParsing` set, so
kapi's persistent flags are *not* merged into them. Each proxy hands its raw
argument list to the very same standalone command the symlink runs. They carry
the k-names verbatim, one spelling everywhere, which also keeps every bare verb
free for something else.

`DisableFlagParsing` is what lets the utilities keep the classic option surface.
Without it, kapi's global `-v` / `-c` / `-q` would shadow the toolbox
shorthands. In standalone form the busybox root never inherits those flags at
all, so the same letters are free to define. The proxies are hidden from
`kapi --help`, which steers users to the dedicated names.

### One block-text projection

Every utility works over the same projection: stream the document through its
format reader, take each block part in document order, act on its text. This is
the single place the toolbox decides what "the text" of a file is.

<PipelineDiagram
  stages={[
    { label: "Input", sub: "file or stdin", role: "io" },
    { label: "Resolve format", sub: "-f, else detection", note: "binary guard on the fallback" },
    { label: "Reader", sub: "DataFormatReader" },
    { label: "Block text", sub: "in document order", role: "annotate", note: "markup never reaches it" },
    { label: "Utility", sub: "print · match · edit · convert · compare", role: "tool" },
  ]}
  channelLabel=""
  caption="The projection is defined once and shared, so what counts as the text of a file is the same for every utility and matches the rest of the pipeline."
/>

**Format resolution.** An explicit `--format` / `-f` wins; otherwise the
framework's canonical detection cascade runs (extension, then container-aware
content sniffing), falling back to plaintext. Standard input carries no usable
path, so its detection is purely content-based through the same detector: a piped
Word document or JSON catalog is still recognised.

**The binary guard.** The plaintext fallback is what makes extensionless prose
work, and it is also what would make `kcat ~/Downloads/*` print a disk image. So
the fallback is gated: input that no format claimed *and* whose bytes are binary
fails with a dedicated error, which each utility reports per file and then
carries on from. The rule is git's (a NUL byte in the first 8000 bytes),
exempting the Unicode byte-order marks, since UTF-16 and UTF-32 text is full of
NUL bytes and announces itself. The guard sits in the one format-resolution
helper, so every utility inherits it. It cannot fire for a format kapi
understands, because detection already claimed those, and `-f` is the escape
hatch because it short-circuits detection entirely.

**Read path** (`kcat`, `kgrep`). The stream helper opens the input, detects the
format, and calls back for each block in order. `kcat` prints each block's source
text, or a `--target LOCALE` translation, one block per line; `kgrep` matches
each block's text against the pattern. Markup and non-translatable structure
never reach the projection.

**Edit path** (`ksed`). The document is read, the substitution tool is applied to
every part, and the reconstructed document is written back in the same format.
The skeleton store is wired between reader and writer when both support it, so a
faithful format round-trips its structure while only the edited text changes.
Edits target the source unless `--target LOCALE` selects a translation. In-place
editing requires a file argument and refuses stdin. A read-only format, one with
no writer, returns an actionable error pointing at `kcat`.

**Convert path** (`kconv`). The input is read and written through a *different*
format's writer, chosen from `--to` (a format id or an extension) or inferred
from the `-o` extension. The skeleton store and the source bytes are wired to the
writer **only when the reader and writer are the same format**: a same-format
conversion round-trips faithfully through the skeleton, while a cross-format one
reconstructs from the content model and the block roles, so the source's foreign
byte skeleton is never emitted. This is the same format-match guard the file
runner applies.

**Compare path** (`kdiff`). Two inputs are projected to block text and the
**blocks are aligned**, not the lines, so structural noise (a re-zipped
document, a reflowed container, a reordered catalog) never registers as a diff.
Alignment is chosen per pair. **Keyed** sides, whose blocks carry stable semantic
keys such as a JSON key path or an XLIFF unit id, align by key, so added,
removed, changed, and reordered keys fall out directly (a reorder is reported as
*moved*, via a longest-common-subsequence pass over the shared keys' order).
**Positional** sides, whose ids merely encode document order, align by an LCS
over the block text, so an inserted paragraph is one added block rather than a
cascade of changes. `--by id|content` overrides the heuristic; the LCS table is
capped, with a positional fallback that is logged rather than silent. A single
input plus `--target LOCALE` switches to **coverage mode**, source against
translation within one file, reporting untranslated and source-identical blocks.
`kdiff` reads only; it never writes a document back.

Because the projection is the translatable blocks, the utilities inherit the
content model's notion of what is translatable
([F-02](../foundations/f-02-content-model.md)) rather than re-deriving one.

### Which valid conversion targets exist

A cross-format conversion reconstructs the target from the content model, never
from a foreign skeleton. So the valid `--to` targets are exactly the
**generative, non-interchange** writers, the declared writer capabilities in
[E-02](../engine/e-02-format-system.md). Skeleton-bound formats (Word,
PowerPoint, ODF, IDML, EPUB) can be converted *from* but not *to*. Bilingual
interchange formats are reached through `kapi extract` and `kapi merge`
([M-01](../multilingual/m-01-bilingual-interop.md)), not through conversion.
[`kapi formats`](/reference/commands/formats) reports both capabilities per
format, resolved declaratively without loading a plugin.

### Flag surface

Each utility carries its classic option surface plus a few kapi-aware additions.
Common to all: `--target LOCALE` (operate on a translation instead of the
source) and `--format` / `-f`.

- **`kgrep`**: `-i`, `-v`, `-c`, `-n`, `-o`, `-l` / `-L`, `-w`, `-F`, `-r`,
  `-H` / `--no-filename`, repeatable `-e`, `-q`, plus `--color` and `--json`.
- **`ksed`**: repeatable `-e` (`s/regexp/replacement/flags`), `-i` with an
  optional attached backup suffix, and `-R` to recurse. The script supports
  backreferences, the `g` and `i` flags, and any single-byte delimiter. sed's
  attached-suffix form (`-i.bak`) is normalised before dispatch.
- **`kcat`**: `-n`, `--id` (prefix each block with its source id), `-r`, and
  `--json`.
- **`kdiff`**: `--by auto|id|content`, `-q` / `--brief`, `--stat`, `--color`,
  and `--json`. It takes one or two `FILE` arguments rather than a glob, and
  never edits in place.
- **`kconv`**: `-t` / `--to FORMAT`, `-o` / `--output PATH`, `-r`, and
  `--timing`.

Recursion is `-R` on `ksed` and `-r` everywhere else. That is fidelity to the
classics winning over internal uniformity: sed's `-r` is `--regexp-extended`, and
taking that letter would silently rewrite a whole tree for someone who asked for
a regexp dialect. `-r` is left unbound on `ksed`, so typing it is an error rather
than a surprise.

`kconv`'s `-o` becomes a **batch** when it names a directory: a trailing
separator, or a path that already is one. Each input produces one output file
named after its stem with the target extension, and the input sub-tree is
mirrored relative to the deepest directory every input shares. That root is
computed from the *resolved* inputs rather than the arguments, so a
shell-expanded glob and a kapi-expanded one behave alike, and same-named files in
sibling directories cannot collide. Two inputs resolving to one output, and an
output that would land on its own input, are reported and skipped, never
silently overwritten.

With no `FILE`, or when `FILE` is `-`, standard input is read. A terminal stdin
read is raced against the command context, so an interrupt (which the CLI traps
as cancellation rather than letting the signal kill the process) returns cleanly
rather than hanging.

### Exit codes follow grep

The utilities report a *result* as a status, not as an error. `kgrep` exits `0`
when any block matched, `1` when none did, and `2` on operational trouble. To
express "no match" as a status without printing an error line, a no-match returns
the `ErrSilentExit` sentinel: the CLI runner maps it to a non-zero exit but
suppresses the message, because the command has already written, or deliberately
withheld, its own output.

`kdiff` reuses the same spine for the classic `diff` convention: `0` when the
inputs are equivalent, `1` when they differ (or, in coverage mode, when
translation work is pending), `2` on trouble. It prints the diff first, then
returns the sentinel, so there is no spurious error line after a legitimate
result.

This is the same exit-code spine used across the CLI
([S-01](s-01-kapi-cli.md)): success, error, usage, gate, and cancellation
mapped to the signal code, so scripts and agent skills can branch on toolbox
results reliably.

## Consequences

- Engineers grep and sed the *content* of formats their classic tools can only
  see as bytes, with no new binary to install.
- Reusing the readers, writers, and skeleton store means a faithful format
  round-trips its structure on edit and only the prose changes.
- `kconv` reuses that same machinery to convert between formats: a same-format
  conversion round-trips faithfully, while a cross-format one projects the
  document's structure through block roles into the target, so a Word document
  becomes clean Markdown or HTML without its source packaging.
- `kdiff` compares content rather than bytes: re-saving a document or reordering
  a catalog produces no diff, only genuine prose changes do, and the
  block-shaped change set is exactly what a re-translation pass consumes.
- The projection is defined once, so what counts as "the text" is consistent
  across the utilities and matches the rest of the pipeline.
- The grep-style exit codes let scripts distinguish "no match" from "error"
  without parsing output, the same property an agent skill relies on
  ([S-03](s-03-agent-surfaces.md)).

## Related

- [F-02: The content model](../foundations/f-02-content-model.md): blocks, the unit the toolbox projects to
- [E-02: The format system](../engine/e-02-format-system.md): readers, writers, writer output modes, and the skeleton store
- [S-01: The kapi CLI](s-01-kapi-cli.md): the CLI base the utilities live in and the exit-code contract they extend
- [S-03: Agent surfaces](s-03-agent-surfaces.md): the skill that drives the toolbox, and `kapi apply` as the reviewed-edit sibling of `ksed`
- [M-01: Bilingual interop](../multilingual/m-01-bilingual-interop.md): where interchange formats are reached instead
- [CLI tools](/toolbox/overview): the user-facing guide to each utility
