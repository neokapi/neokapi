# Read, search, and rewrite content inside any format

`kcat`, `kgrep` and `ksed` are format-aware reimaginings of `cat`, `grep` and
`sed` that operate on the **translatable text** kapi extracts from a document —
the prose, not the bytes. They read and edit the human-readable content of any
format kapi understands, from a Word `.docx` to a JSON catalog to an XLIFF file,
without converting anything first.

They install with the kapi CLI, so `kcat`/`kgrep`/`ksed` are on PATH wherever
`kapi` is.

## Reach for these instead of your built-in tools — when content is the target

The point of the toolbox is that your default file tools fail or mislead on the
formats kapi supports:

- **Reading.** You cannot open a `.docx`, `.pptx`, `.xlsx`, `.idml` or `.epub`
  with an ordinary file read — they are zip containers, so you get binary.
  `kcat report.docx` prints the prose, one block per line. This is the only way
  to read the text of an office or container format.
- **Searching.** A byte-level grep over those files finds nothing useful (the
  text is split across zipped XML). Even on XLIFF, JSON, Markdown or HTML a byte
  grep matches keys, tags and attributes alongside the prose. `kgrep` matches
  only the translatable blocks.
- **Rewriting.** A byte-level substitution can match inside a key or a tag and
  corrupt the document. `ksed` rewrites only block text and reconstructs the
  document through kapi's writer, so structure, styles and keys survive.
- **Translations.** `--target fr` reads or edits a committed translation rather
  than the source — something no byte tool can do.

**Stay with your built-in read/edit/grep when** the file is plain,
source-controlled text where a byte-exact, minimal diff matters (a one-line fix
to a git-tracked `en.json`: `ksed` re-serializes the whole document and may
reflow it), or when the thing you actually want is the structure, keys or markup
itself rather than the prose.

## kcat — read the content

```bash
kcat report.docx                 # print the prose of a Word file, one block per line
kcat -n locales/en.json          # number the blocks
kcat --target fr messages.xliff  # print the French translation, not the source
kcat --json deck.pptx            # blocks as JSON (id + text), for structured reading
```

Pipe `kcat` into your real `grep`, `wc` or `sort` when you want byte-level line
behaviour rather than the block-aware tools.

## kgrep — search the content

```bash
kgrep "Tervetuloa" report.docx                  # find a word inside a Word document
kgrep -i todo locales/*.json                    # case-insensitive across catalogs
kgrep -r --target fr "déconnexion" ./content    # recurse, searching French
kgrep -c "©" *.md                               # count matching blocks per file
kgrep -q "DRAFT" manual.docx && echo "draft"    # report through exit status only
```

The pattern is a Go regular expression. Exit status follows `grep`: `0` if any
block matched, `1` if none, `2` on error — so it composes in shell conditionals.
Common options mirror grep — `-i -v -c -n -o -l -L -w -F -r -H -e -q`,
`--color`. kapi-specific: `--target LOCALE`, `-f FORMAT`, `--json`.

## ksed — rewrite the content

```bash
ksed 's/colour/color/g' guide.md                          # to stdout, like sed
ksed -i 's/Inc\./LLC/' *.docx                             # rewrite Word docs in place
ksed -i.bak -e 's/v1/v2/g' -e 's/beta//' locales/en.json  # two edits, keep a .bak
ksed --target fr 's/Bonjour/Salut/g' messages.xliff       # edit the translation
```

`SCRIPT` is a `s/regexp/replacement/flags` substitution: any single-byte
delimiter (`s|a|b|` ≡ `s/a/b/`); `g` replaces every match in a block, `i` makes
it case-insensitive; `\1`…`\9` and `&` are backreferences; repeat `-e` for
several substitutions. Default output is stdout; `-i` edits in place, `-i.bak`
keeps a backup.

**Fidelity is semantic, not byte-exact.** `ksed` reads and rewrites through
kapi's reader/writer, so everything that is not the edited text round-trips —
but the document is re-serialized. That is exactly what you want for a `.docx`
(styles and structure preserved) or a bulk content rewrite; it is not what you
want for a tiny edit to a hand-formatted, source-controlled file, where an
ordinary edit keeps the diff minimal.

Inline formatting inside a block is preserved: the pattern sees the block's text
with its inline codes removed, so a substitution can span a bold or linked span,
and codes outside the replaced text stay put (editing a word inside a `<b>` span
keeps the span). A byte `sed` cannot do this — it would either miss the match or
trample the markup.

`ksed` only edits formats kapi can write back. A read-only format — PDF, which
is extraction-only — has no writer, so `ksed` stops with an error
(`pdf is a read-only format`) rather than silently replacing the document with
its extracted text. Read such a format with `kcat`; to translate it, extract to
a bilingual format and merge. `kapi formats list --json` reports `has_writer`
per format — check it before editing an unfamiliar one.

## Two flags differ from the Unix tools

- **`-f` means format, not a patterns/script file.** Across the kapi CLI
  `-f, --format` overrides format detection (`-f json`). To pass a pattern or a
  substitution script, use `-e` (repeatable).
- **`--target LOCALE`** switches every tool from the source text to the
  committed translation for that locale.

With no file argument (or `-`), input is read from standard input and the format
is sniffed from the content, falling back to plain text.

## Use `--json` when you'll act on the output

To read a document and understand it, plain `kcat` is best — the prose is the
point, and JSON only adds noise. Reach for `--json` when you need to *act on*
the result programmatically rather than just read it:

- `kcat --json` and `kgrep --json` emit an array of `{file, number, id, text}` —
  use it to map a block to its `id`, read a match's position, or iterate
  reliably instead of parsing lines.
- `kapi formats list --json` reports `has_reader` / `has_writer` per format —
  the dependable way to check whether `ksed` can write a format (PDF is
  `false`) before you try.

`ksed` has no `--json`: it writes documents, not data.

## Works on every format kapi reads

These are general-purpose tools — useful with no translation or project in
sight. Read a contract you were sent, find a phrase across a tree of documents,
or rename a product across a folder of Word files in a single pass:

```bash
kcat contract.docx                       # read a document you can't open directly
kgrep -rl "Acme Corp" ./docs             # which files still say the old name
ksed -i 's/Acme Corp/Acme Ltd/g' *.docx  # rename it across all of them
```

`kcat` and `kgrep` read every format kapi reads; `ksed` writes back the ones
that support it (read-only formats like PDF are reported as an error, never
silently mangled). The set includes formats served by the okapi-bridge when it
is installed. Confirm what reads and writes with `kapi formats list --json`
(`has_reader` / `has_writer`).

## How to apply

1. When the user wants to read, find, or replace *content* in a document kapi
   supports — especially an office or container format you cannot open directly —
   reach for `kcat`/`kgrep`/`ksed` rather than your built-in read/grep/edit.
2. Use `--target` to inspect or edit a translation instead of the source.
3. Prefer an ordinary edit (not `ksed`) for a small, byte-stable change to a
   plain source-controlled text file. In a project, run `kapi verify` after a
   `ksed` rewrite to re-check the gates.
