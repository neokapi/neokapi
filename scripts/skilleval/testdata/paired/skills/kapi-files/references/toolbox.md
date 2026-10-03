# Read, search, rewrite, compare and convert content inside any format

`kcat`, `kgrep`, `ksed`, `kdiff` and `kconv` are format-aware reimaginings of
`cat`, `grep`, `sed`, `diff` and a converter that operate on the **content**
kapi-files extracts from a document: the prose, not the bytes. They read, edit,
compare and convert the human-readable text inside any format kapi-files
understands, from a Word `.docx` to a JSON catalog to a Markdown page to an
XLIFF file, without converting anything first.

Each runs as a command of kapi-files: `kapi-files kcat`, `kapi-files kgrep`,
`kapi-files ksed`, `kapi-files kdiff` and `kapi-files kconv`.

## Reach for these instead of your built-in tools when content is the target

The point of the toolbox is that your default file tools fail or mislead on the
formats kapi-files supports:

- **Reading.** You cannot open a `.docx`, `.pptx`, `.xlsx`, `.idml` or `.epub`
  with an ordinary file read: they are zip containers, so you get binary.
  `kapi-files kcat report.docx` prints the prose, one block per line. This is
  the only way to read the text of an office or container format.
- **Searching.** A byte-level grep over those files finds nothing useful (the
  text is split across zipped XML). Even on XLIFF, JSON, Markdown or HTML a byte
  grep matches keys, tags and attributes alongside the prose. `kgrep` matches
  only the content blocks.
- **Rewriting.** A byte-level substitution can match inside a key or a tag and
  corrupt the document. `ksed` rewrites only block text and reconstructs the
  document through kapi-files' writer, so structure, styles and keys survive.
- **Comparing.** A byte `diff` of two `.docx` files (or a reordered catalog)
  reports noise: re-zipped XML, shuffled keys. `kdiff` compares the *blocks*, so
  only genuine prose changes show, and `kdiff --target fr file.xliff` reports
  which blocks are still untranslated.
- **Converting.** Turning a `.docx` into Markdown by hand means re-typing its
  headings, lists and tables. `kconv report.docx --to md` re-expresses the
  document's structure in the target format from the content model.
- **Translations.** `--target fr` reads, edits or converts a committed
  translation rather than the source, something no byte tool can do.

**Stay with your built-in read/edit/grep when** the file is plain,
source-controlled text where a byte-exact, minimal diff matters (a one-line fix
to a git-tracked `en.json`: `ksed` re-serializes the whole document and may
reflow it), or when the thing you actually want is the structure, keys or markup
itself rather than the prose.

**`ksed` is for a regex substitution; `kapi-files apply` is for block-by-block
edits you author.** When the change is "replace this pattern with that" across
blocks, `ksed` expresses it directly. When you are rewriting block text by hand
(a clarity pass, a per-block correction), read the blocks with
`kapi-files inspect` and write your edited text back through
`kapi-files apply`, which refuses an edit whose block changed since you read it
and preserves inline codes. See [edit.md](edit.md).

## kcat: read the content

```bash
kapi-files kcat report.docx                 # print the prose of a Word file, one block per line
kapi-files kcat -n locales/en.json          # number the blocks
kapi-files kcat --target fr messages.xliff  # print the French translation, not the source
kapi-files kcat --json deck.pptx            # blocks as JSON (id + text), for structured reading
```

Pipe `kcat` into your real `grep`, `wc` or `sort` when you want byte-level line
behaviour rather than the block-aware tools.

## kgrep: search the content

```bash
kapi-files kgrep "Tervetuloa" report.docx                  # find a word inside a Word document
kapi-files kgrep -i todo locales/*.json                    # case-insensitive across catalogs
kapi-files kgrep -r --target fr "déconnexion" ./content    # recurse, searching French
kapi-files kgrep -c "©" *.md                               # count matching blocks per file
kapi-files kgrep -q "DRAFT" manual.docx && echo "draft"    # report through exit status only
```

The pattern is a Go regular expression. Exit status follows `grep`: `0` if any
block matched, `1` if none, `2` on error, so it composes in shell conditionals.
Common options mirror grep (`-i -v -c -n -o -l -L -w -F -r -H -e -q`,
`--color`). kapi-files-specific: `--target LOCALE`, `-f FORMAT`, `--json`.

## ksed: rewrite the content

```bash
kapi-files ksed 's/colour/color/g' guide.md                          # to stdout, like sed
kapi-files ksed -i 's/Inc\./LLC/' *.docx                             # rewrite Word docs in place
kapi-files ksed -i.bak -e 's/v1/v2/g' -e 's/beta//' locales/en.json  # two edits, keep a .bak
kapi-files ksed --target fr 's/Bonjour/Salut/g' messages.xliff       # edit the translation
kapi-files ksed 's/shop/store/g' guide.html --print-ops > change.json  # print the change set, edit nothing
```

`SCRIPT` is a `s/regexp/replacement/flags` substitution: any single-byte
delimiter (`s|a|b|` ≡ `s/a/b/`); `g` replaces every match in a block, `i` makes
it case-insensitive; `\1`…`\9` and `&` are backreferences; repeat `-e` for
several substitutions. Default output is stdout and the file stays as it is;
`-i` edits in place, `-i.bak` keeps a backup. `--target LOCALE` edits the
translation a bilingual or multilingual file (XLIFF, PO, Qt Linguist, an Xcode
string catalog) holds. An archive is edited all or nothing: when one member's
edit is refused, no member is written.

**`ksed` writes through `kapi-files apply`'s contract.** Each substitution that
changes a block becomes a `replace_text` operation on it, guarded by the
revision `ksed` read. `--print-ops` prints that change set (kapi.change/v1)
instead of applying it, so you can read what a one-liner will do, and
`kapi-files apply change.json` applies exactly what was printed. A plural or
select keeps its structure: each branch is matched on its own, and a match that
would swallow one is left alone. When the file changes between the read and the
write, the edit is refused, nothing is written, and `ksed` exits 2.

A file is read with the format detection finds, and a file no format claims as
plain text.

**Editing a binary document at a terminal is refused.** A `.docx` edit writes a
zip, so `ksed 's/a/b/' report.docx` with stdout on a terminal stops with
`binary output not written to a terminal` and exit 2 rather than wedging it. Use
`-i`, redirect to a file, or pass `--force`. Piped and redirected output is
unaffected, which is what matters when you are driving it from a script.

**Fidelity is semantic, not byte-exact.** `ksed` reads and rewrites through
kapi-files' reader/writer, so everything that is not the edited text
round-trips, but the document is re-serialized. That is exactly what you want
for a `.docx` (styles and structure preserved) or a bulk content rewrite; it is
not what you want for a tiny edit to a hand-formatted, source-controlled file,
where an ordinary edit keeps the diff minimal.

Inline formatting around your edit is preserved: a substitution can span a bold
or linked span and the markup is kept intact (editing a word inside a `<b>` keeps
the bold; consuming the whole span leaves no broken tag). A byte `sed` cannot do
this; it would either miss the match or trample the markup.

`ksed` only edits formats kapi-files can write back. A read-only format (PDF,
which is extraction-only) takes no edit, so `ksed` stops with an `unsupported`
error naming the operation the format does not support, and the document stays
as it was. Read such a format with `kcat`. `kapi-files formats --json` reports
`has_writer` per format, and `kapi-files inspect` lists the operations each
block accepts; check them before editing an unfamiliar format.

## kdiff: compare the content

```bash
kapi-files kdiff old.json new.json                  # what content changed between two versions
kapi-files kdiff report.docx report-v2.docx         # prose changes only; ignores re-save noise
kapi-files kdiff --target fr old.xliff new.xliff    # what changed in the French specifically
kapi-files kdiff --target de -q messages.xliff      # coverage: exit 1 if German is incomplete
kapi-files kdiff --json a.json b.json               # structured changeset for a pipeline
```

`kdiff` aligns **blocks**, not lines. Keyed formats (JSON, XLIFF, PO) align by
key, so reordering keys is not a diff and a renamed value is a `changed`; prose
formats (Word, Markdown) align by content, so an inserted paragraph is one
`added` block, not a cascade. Force either with `--by id` / `--by content`.

Two modes: **two files** = revision diff (what changed); **one file +
`--target LOCALE`** = coverage report (which blocks are untranslated or a
verbatim copy of the source). Exit status follows `diff`: `0` equivalent, `1`
differ / pending, `2` trouble, so it composes in shell conditionals. `kdiff`
reads only; it never writes a document back.

## kconv: convert the content

```bash
kapi-files kconv proposal.docx --to md                  # a Word proposal as Markdown, to stdout
kapi-files kconv report.dclg.xml -o report.html         # target format inferred from the -o extension
kapi-files kconv messages.xliff --to md --target fr     # the French translation of an XLIFF as Markdown
kapi-files kconv ~/Downloads/* --to md -o converted/    # one file per input, in a directory
kapi-files kconv -r docs --to html -o site/             # a whole tree, sub-directories mirrored
```

`kconv` reads a document into the content model and re-expresses it in another
format from each block's structural role (heading, list item, table cell,
caption) and each run's inline type, so a `.docx` heading becomes `#` in
Markdown or `<h1>` in HTML, a bold span becomes `**…**` or `<strong>`. The
target comes from `--to` (a format id such as `markdown`, `html`, `doclang`, or
an extension such as `md`) or is inferred from the `-o` extension; with no `-o`
the result goes to stdout. `-o` naming a directory (trailing slash, or an
existing directory) writes one file per input, and `-r` walks directory
arguments. `--target LOCALE` converts a committed translation instead of the
source.

Any readable format converts **from**. Conversion **to** is limited to formats
that can be produced from content alone: documents (Markdown, HTML, DocLang,
AsciiDoc, plain text) and data/catalog formats (JSON, YAML, resource strings).
Bilingual interchange (XLIFF, PO, TMX, KBF) is not a conversion target.
Packaged formats (`.docx`, ODT, InDesign, EPUB) and read-only ones (PDF)
convert from, not to. Converting to a **different** format is a clean
projection (structure and prose, without the source's packaging); converting to
the **same** format is a faithful round-trip. A conversion that would produce a
binary document at a terminal is refused, as with `ksed`: pass `-o`, redirect,
or `-o -`. Two inputs that would write the same output file are reported (exit
2) rather than overwritten.

## Two flags differ from the Unix tools

- **`-f` means format, not a patterns/script file.** Across kapi-files
  `-f, --format` overrides format detection (`-f json`). To pass a pattern or a
  substitution script, use `-e` (repeatable).
- **`--target LOCALE`** switches every tool from the source text to the
  committed translation for that locale.

With no file argument (or `-`), input is read from standard input and the format
is sniffed from the content, falling back to plain text.

## Use `--json` when you'll act on the output

To read a document and understand it, plain `kcat` is best: the prose is the
point, and JSON only adds noise. Reach for `--json` when you need to *act on*
the result programmatically rather than just read it:

- `kcat --json` and `kgrep --json` emit an array of `{file, number, id, text}`;
  use it to map a block to its `id`, read a match's position, or iterate
  reliably instead of parsing lines.
- `kapi-files formats --json` reports `has_reader` / `has_writer` per format,
  the dependable way to check whether `ksed` can write a format (PDF is
  `false`) before you try.

`ksed` and `kconv` have no `--json`: they write documents, not data.

## Works on every format kapi-files reads

These are general-purpose tools, useful with no translation or project in
sight. Read a contract you were sent, find a phrase across a tree of documents,
rename a product across a folder of Word files in a single pass, or turn a
`.docx` into Markdown:

```bash
kapi-files kcat contract.docx                       # read a document you can't open directly
kapi-files kgrep -rl "Acme Corp" ./docs             # which files still say the old name
kapi-files ksed -i 's/Acme Corp/Acme Ltd/g' *.docx  # rename it across all of them
kapi-files kconv contract.docx --to md -o contract.md   # a Markdown copy to quote from
```

`kcat`, `kgrep` and `kconv` read every format kapi-files reads; `ksed` writes
back the ones that support it (read-only formats like PDF are reported as an
error, never silently mangled). The set includes formats served by the
okapi-bridge when it is installed. Confirm what reads and writes with
`kapi-files formats --json` (`has_reader` / `has_writer`).

## How to apply

1. When the user wants to read, find, replace, compare or convert *content* in a
   document kapi-files supports (especially an office or container format you
   cannot open directly), reach for `kcat`/`kgrep`/`ksed`/`kdiff`/`kconv` rather
   than your built-in read/grep/edit/diff.
2. Use `--target` to inspect, edit, convert, or measure coverage of a translation
   instead of the source.
3. Prefer an ordinary edit (not `ksed`) for a small, byte-stable change to a
   plain source-controlled text file.
