---
id: 023-toolbox-utilities
sidebar_position: 23
title: "AD-023: Toolbox Utilities"
description: "Architecture decision: kcat, kgrep, and ksed are format-aware reimaginings of cat/grep/sed, and kconv converts between formats. They operate on the translatable text of any supported format, ship as busybox-style multi-call symlinks to the kapi binary, project documents to block text, and follow a grep-style exit-code contract."
keywords: [toolbox, kcat, kgrep, ksed, kconv, busybox, multi-call, block projection, format conversion, exit codes, format-aware, architecture decision, neokapi]
---

# AD-023: Toolbox Utilities

## Summary

`kcat`, `kgrep`, and `ksed` are format-aware reimaginings of the classic Unix
text utilities — `cat`, `grep`, `sed` — that operate on the **translatable
text** of any format kapi understands (Word `.docx`, JSON catalogs, XLIFF,
Markdown, …) rather than on raw bytes. They reuse kapi's reader/writer pipeline,
so `kgrep` searches the prose inside a `.docx`, and `ksed` rewrites it and saves
the document back faithfully. A fourth utility, `kconv`, has no classic Unix
analog: it **converts** a document into another format — a `.docx` to Markdown,
a DocLang document to HTML, any supported format to DocLang — by handing the
blocks (and the role each carries) to a different format's writer. Because a
cross-format conversion reconstructs the target from the content model (never a
foreign skeleton), the valid `--to` targets are exactly the **generative,
non-interchange** writers — the declared writer capabilities defined in
[AD-005: Writer output modes](005-format-system.md). Skeleton-bound formats
(`.docx`, ODF, IDML, EPUB) can be converted *from* but not *to*; bilingual
interchange formats (XLIFF, PO, TMX, KLF) are reached via `kapi extract`/`merge`
([AD-017](017-bilingual-format-interop.md)), not `convert`. They ship as
**busybox-style multi-call binaries**: the names are symlinks to the single
`kapi` binary, which dispatches on `argv[0]`. One binary, the extra names, no
extra size. Each operates over a **block-text projection** of the document and
follows the grep-style exit-code contract ([AD-013](013-kapi-cli.md)).

## Context

A recurring need is to read, search, and edit the human-readable content of
files whose format an editor or a classic Unix tool cannot meaningfully touch: a
Word document is a zipped XML container, an XLIFF file interleaves source and
target inside markup, a JSON catalog buries strings among keys and structure.
Running `grep` or `sed` on these byte streams matches markup, misses content
split across runs, and corrupts structure on edit.

kapi already has exactly the machinery to do this correctly — format detection,
readers that yield translatable Blocks, writers that round-trip structure
faithfully via the skeleton store ([AD-005](005-format-system.md)). The toolbox
exposes that machinery through the muscle-memory interface engineers already
have. Two design goals shaped it:

- **Zero marginal footprint.** The utilities must not be three more binaries to
  build, sign, and distribute. They are the same `kapi` binary under different
  names.
- **Faithful to the classics.** `kgrep`/`ksed`/`kcat` should accept the option
  surface and exit-code behavior users expect from `grep`/`sed`/`cat`, including
  the shorthand flags (`-v`, `-c`, `-i`) that kapi's global flags would
  otherwise shadow. `kconv` has no classic analog, so it takes a small, kapi-
  native flag surface (`--to`, `-o`) instead.

## Decision

### Multi-call (busybox) dispatch

The toolbox commands live in the shared CLI base (`cli/toolbox*.go`) and are
built into the `kapi` binary. They are reachable two ways:

- **As multi-call symlinks.** The build (and the Homebrew formula) create
  `kgrep`, `ksed`, `kcat`, and `kconv` as symlinks to `kapi`. At startup `kapi`'s
  `main()` calls `cli.BusyboxRoot(app, os.Args[0])`: it normalizes the program
  name (stripping any `.exe` suffix) and, when it matches a toolbox name,
  returns a standalone root for that utility instead of the full kapi command
  tree. The standalone root owns the app lifecycle (config load, `Init`,
  `Shutdown`) so the utility behaves identically however it is launched.
- **As hidden kapi subcommands.** `kapi grep`, `kapi sed`, `kapi cat`, and
  `kapi convert` are thin proxies (`NewToolboxProxies`) with `DisableFlagParsing`
  set, so kapi's persistent flags are *not* merged into them. Each proxy
  delegates the raw argument list to the very same standalone command the symlink
  runs, so `kapi grep` and `kgrep` behave identically. They are hidden from
  `kapi --help` so the help steers users to the dedicated
  `kgrep`/`ksed`/`kcat`/`kconv` names.

`DisableFlagParsing` on the proxies is what lets the utilities keep their full
classic option surface — without it, kapi's global `-v`/`-c`/`-q` would shadow
the toolbox shorthands. In standalone form the busybox root never inherits
kapi's persistent flags, so the same shorthands are free to define.

### Block-text projection

All three utilities operate over the same projection of a document: stream it
through the format reader, take each Block part in document order, and act on its
text. This is the one place the toolbox decides what "the text" of a file is.

- **Format resolution.** A single helper picks the format: an explicit
  `--format`/`-f` wins, otherwise the framework's canonical detection cascade
  (extension → container-aware content sniffing) runs, falling back to
  `plaintext`. stdin carries no usable path, so its detection is purely
  content-based through the same detector — a piped `.docx` or JSON catalog is
  still recognized.
- **Read path (`kcat`, `kgrep`).** `streamBlocks` opens the input, detects the
  format, and calls back for each Block in order. `kcat` prints each block's
  source text (or a `--target LOCALE` translation) one block per line; `kgrep`
  matches each block's text against the pattern. Markup and non-translatable
  structure never reach the projection.
- **Edit path (`ksed`).** `editDocument` reads the input, applies the sed tool to
  every part, and writes the reconstructed document back in the same format. The
  skeleton store is wired between reader and writer when both support it, so a
  faithful format (a `.docx`) round-trips its structure while only the edited
  text changes. Edits target the source text unless `--target LOCALE` selects a
  translation; in-place editing (`-i`, optional `-i.bak` backup) requires a file
  argument and refuses stdin. A read-only format (one with no writer) returns an
  actionable error pointing at `kcat`.
- **Convert path (`kconv`).** `convertDocument` reads the input and writes it
  through a *different* format's writer, chosen from `--to` (a format id or
  extension) or the `-o` extension. With no target locale it projects the
  source; `--target LOCALE` projects a translation. The skeleton store and the
  source bytes are wired to the writer **only when `reader.Name() ==
  writer.Name()`** — a same-format conversion round-trips faithfully via the
  skeleton, while a cross-format one reconstructs from the content model and the
  block roles so the source's foreign byte skeleton is never emitted. This is
  the same format-match guard the file runner applies
  ([AD-005](005-format-system.md)).

Because the projection is "the translatable Blocks," the utilities inherit the
content model's notion of what is translatable — the same Blocks the rest of the
pipeline processes — rather than re-deriving it.

### Flag surface

Each utility carries the classic option surface plus a few kapi-aware additions.
Common to all three: `--target LOCALE` (operate on a translation instead of the
source), `--format`/`-f`, `--source-lang`, and `--encoding`.

- **`kgrep`** — `-i` (ignore case), `-v` (invert), `-c` (count), `-n` (block
  number), `-o` (only matching), `-l`/`-L` (files with/without matches), `-w`
  (word match), `-F` (fixed strings), `-r` (recurse directories), `-H`/`--no-filename`
  (filename prefix), `-e` (repeatable pattern), `-q` (quiet; status only),
  `--color`, and `--json`.
- **`ksed`** — `-e` (repeatable `s/regexp/replacement/flags` script), `-i`
  (in-place, optional attached backup suffix). The script supports
  backreferences (`\1`, `&`), the `g` and `i` flags, and any single-byte
  delimiter. sed's attached-suffix form (`-i.bak`) is normalized into the flag
  parser before dispatch.
- **`kcat`** — `-n` (number blocks), `--id` (prefix each block with its source
  ID), and `--json`.
- **`kconv`** — `-t`/`--to FORMAT` (target format id or extension) and
  `-o`/`--output PATH` (write to a file, format inferred from its extension;
  default stdout). `-o` takes a single input.

With no `FILE`, or when `FILE` is `-`, standard input is read. A terminal stdin
read is raced against the command context so Ctrl-C (which the CLI traps as
context cancellation rather than letting the signal kill the process) cleanly
returns rather than hanging.

### Exit-code contract

The utilities follow grep's status convention rather than reporting a result as
an error. `kgrep` exits `0` when any block matched, `1` when none did, and `2`
on an operational error. To express "no match" as a status without printing an
`Error:` line, a no-match returns the `ErrSilentExit` sentinel: the CLI runner
maps it to a non-zero exit (`ExitError`) but suppresses the message, since the
command has already written (or deliberately withheld) its own output. This is
the same exit-code spine used across the CLI ([AD-013](013-kapi-cli.md)): `0`
success, `1` error, `2` usage, and cancellation mapped to the signal code — so
shell scripts and skills can branch on toolbox results reliably.

## Consequences

- Engineers grep and sed the *content* of formats their classic tools can only
  see as bytes, with no new binary to install — the three names are symlinks to
  `kapi`.
- The utilities reuse the format readers/writers and skeleton store, so a
  faithful format round-trips structure on edit and only the prose changes.
- `kconv` reuses the same machinery to convert *between* formats: a same-format
  conversion round-trips faithfully, while a cross-format one projects the
  document's structure (via block roles) into the target, so a `.docx` becomes
  clean Markdown or HTML without its source packaging.
- The block-text projection is defined once and shared by all three, so what
  counts as "the text" is consistent and matches the rest of the pipeline.
- The grep-style exit-code contract, layered on the CLI's `ErrSilentExit`
  sentinel, lets scripts distinguish "no match" from "error" without parsing
  output.

## Related

- [AD-005: Format System](005-format-system.md) — readers/writers and the
  skeleton store that round-trips structure on edit
- [AD-002: Content Model](002-content-model.md) — Blocks, the unit the toolbox
  projects to
- [AD-013: Kapi CLI](013-kapi-cli.md) — the CLI base the utilities live in, and
  the exit-code contract they extend
- [AD-024: Agent Skills](024-agent-skills.md) — the skill that drives the
  toolbox for an AI assistant
