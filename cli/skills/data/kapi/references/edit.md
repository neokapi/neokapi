# Edit content in any format

Edit the text inside a file an editor can't open directly (a Word document, a
PowerPoint deck, a JSON catalog, an XLIFF file, Markdown) and write it back in
the same format, byte-for-byte except for the text you changed. You do the
editing; kapi parses the format, enforces a faithful round-trip, and is the
checker. No model provider is involved.

This is the **read → edit → write → verify** loop. It is the deliberate,
reviewed counterpart to a `ksed` find-and-replace (see [toolbox.md](toolbox.md)):
reach for this loop when you are rewriting block text by hand (an on-brand fix, a
clarity pass, a terminology correction), and for `ksed` when a regex
substitution expresses the change.

## 1. Read the blocks

`kapi inspect` reads any format into one record per content block:

```bash
kapi inspect report.docx --jsonl
```

```json
{"ref":{"doc":"report.docx","block":"word/document.xml/p"},"rev":"r:3f9a1c0e7b2d4a55","text":"Quarterly summary","ops":["set_content","replace_text"],"role":"heading","level":1}
{"ref":{"doc":"report.docx","block":"word/document.xml/p#2"},"rev":"r:0d71f30c75e4a087","text":"Revenue rose, see the <x id=\"1\"/>dashboard<x id=\"/1\"/>.","codes":{"1":{"kind":"paired","type":"link:hyperlink","attrs":{"href":"https://example.com/dash"}}},"ops":["set_content","replace_text"]}
```

Three fields anchor an edit:

- **`ref`** names the block: its document, its key, and, for a translation, its
  edition. Copy it into the operation's `at`.
- **`rev`** is the revision of the text you read, covering its inline codes and
  their attributes. Send it back as `if_match` so kapi can tell the block is
  still what you read.
- **`text`** renders inline codes (links, bold spans, placeholders) as
  `<x id="…"/>` tokens. **Keep every token, unchanged, in your edited text.**
  They are the markup the round-trip reconstructs. A placeholder is
  `<x id="1/"/>`; a paired span opens with `<x id="1"/>` and closes with
  `<x id="/1"/>`. `codes` lists each one with its type and attributes. A
  character reference the source spells out (`&amp;`, `&rsquo;` in HTML) shows
  as its character; keep each one where it is, and it keeps its spelling in the
  file.

`ops` lists the operations the block accepts; a block with none, such as a
Markdown code block (`role` is `code`), keeps its text. `structures` lists each
plural or select with the path to each of its branches. Inside a project,
`editions` lists each translation with its own `rev`.

Everything else in `text` is text, and kapi encodes it for the file's format.
In HTML a `<` or `&` you type is written as a character reference. Markdown,
MDX and AsciiDoc keep some of their own syntax in `text`: a backslash escape, a
bracket that opens no link, an MDX `{expression}`, an AsciiDoc `+++`
passthrough. Syntax the block already holds stays as you keep it, and syntax
you add (a tag, a link, an expression, an `import` line, a macro) is escaped so
it reads as the characters you typed.

`inspect` reads a file with the same reader `apply` writes it back through, so
every `ref` and `rev` it prints is one `apply` resolves. Inside a project a
document is named by its project-relative path, so run both from the same
project. An HTML image's `alt` or a link's `title` is a block of its own.

## 2. Write the edits

Write a change set (kapi.change/v1) with one operation per block you changed.
`set_content` gives the block new text; `replace_text` changes text inside it
and keeps everything around it:

```json
{"note": "Tighten the summary",
 "ops": [
  {"op": "set_content", "at": {"doc": "report.docx", "block": "word/document.xml/p#2"}, "if_match": "r:0d71f30c75e4a087",
   "text": "Revenue climbed; see the <x id=\"1\"/>dashboard<x id=\"/1\"/>."},
  {"op": "replace_text", "at": {"doc": "report.docx", "block": "word/document.xml/p"}, "if_match": "r:3f9a1c0e7b2d4a55",
   "edits": [{"find": "Quarterly", "text": "Third-quarter"}]}
 ]}
```

A `replace_text` edit names its text by `find` (with `occurrence` when it
matches more than once), by code-point `start` and `end`, or by run positions in
`range`. To edit one branch of a plural, give the edit the branch's `path` from
`structures`, such as `[1, {"plural": "one"}]`. A change set can also be JSONL
(the envelope fields on the first line, one operation per line) or a JSON array
of operations. `kapi apply --schema` prints the whole contract.

Then apply it. `kapi apply` reads the change set from a file or from stdin:

```bash
kapi inspect report.docx --jsonl > blocks.jsonl
# You write the change set from the blocks you rewrite; there is no command
# for it, you are the writer. Then:
kapi apply edits.json --dry-run          # check it and print a diff per document, write nothing
kapi apply edits.json                    # apply it
kapi apply edits.json --in-place=.bak    # apply, keeping a .bak of each file it replaces
kapi apply edits.json --json             # print the result (kapi.change-result/v1)
```

kapi never sends content to a model to rewrite it; you write the new text and
`kapi apply` round-trips it back.

## 3. What refuses an edit

`apply` writes a change set only when every operation in it holds. When one is
refused, nothing in the change set is written, the refused operation carries an
`error` with a `code`, and every other operation reports `not_applied`:

- **`stale`**: the block's revision is no longer the `if_match` you sent; the
  file changed since you read it. The result carries the block's `current`
  revision and text, so rebase your edit on it and resend.
- **`guard`**: your edited `text` drops, invents, duplicates or unbalances an
  `<x id="…"/>` token (`codes_changed`), or replaces a block holding a plural or
  select with flat text (`structure_lost`). Edit a branch with `path` instead.
- **`not_found`**: the `ref` names no document or block, or a `find` matches
  nothing. Re-read the file.
- **`ambiguous`**: a `find` matches more than once. Add `occurrence`.
- **`unsupported`**: the block or format takes no such operation, such as a
  Markdown code block, which `inspect` lists with no `ops`.

A refusal exits **3**, distinct from an operational error: re-read the affected
blocks and resend with fresh revisions, the same loop a failing check drives. A
change set that does not decode exits **2**. An operation whose text the block
already holds reports `unchanged`, so resending a change set that landed is
safe.

### Refused for its wording or its sender

Before an edit is written, kapi holds the edited text to the voice and terms in
force where the file sits, with the deterministic rules `kapi check` runs. An
edit is refused as `gate_failed`, with the findings, only for a failing finding
it introduces. A finding the block already had is reported and does not block
the edit, and an advisory or suggested rule only reports. Rewrite the wording a
finding names and send the edit again. Only a person can land an edit over its
findings, so do not try to override one.

An agent's change is refused as `not_permitted` when it writes a term, a
content-memory pair or the recipe (record a suggestion instead), records a
review decision other than a pre-review, or sends `if_match: "*"` in place of
the revision it read. The refusal names what to do instead. Both refusals exit
on the gate code (3).

## 4. Verify

Check the file you edited. In a project, its applicable voice and terms resolve
from the file's path:

```bash
kapi check report.docx --json
```

Read the findings and analyzer coverage, fix relevant flagged blocks through
another `apply` pass, and re-check. Review meaning and any unsupported guidance
against the retrieved context. A passing report does not establish those
judgments. Use `kapi check --ship --json` when the task also requires checking
project release gates.

## Repair a comment finding

A code comment, such as `func/Parse` in a Go file, is a block of its source
file. `kapi inspect` lists each comment with its prose as `text` and its `rev`:

```bash
kapi inspect internal/parse/parse.go --jsonl
```

Repair it with `set_content` on that block, the new prose in `text` without
comment markers. Do not edit the file around the comment:

```json
{"ops": [{"op": "set_content", "at": {"doc": "internal/parse/parse.go", "block": "func/Parse"},
          "if_match": "r:8d65ce744d4fe688", "text": "Parse reads the input.\n\nIt stops at the end."}]}
```

A comment's revision is `r:` and the first sixteen hex digits of the
`location.comment_sha256` that `kapi check --json` reports for it, so you can
also build it from the finding you are fixing.

- Keep the comment's code blocks, `[references]`, list items and any
  `Deprecated:` paragraph. Dropping or adding one refuses the edit.
- A comment someone changed after you read it is refused as `stale`, with its
  current revision and prose: write the edit against what it now says. A
  comment that only moved keeps its revision and is still written.
- A change set edits code comments or documents, never both: send each in a
  change set of its own.
- A refused edit writes nothing and exits 3. Directives, generated files, the
  cgo preamble and example output are refused.
- For a `/* */` comment, leave out `/*`, `*/` and the ` * ` that opens each
  line. kapi writes the text back in the comment's own layout. Text holding
  `*/` is refused, because the comment would end there: reword it.
- A TypeScript, TSX or JavaScript comment is written when the sourcecode plugin
  is installed, the project configures oxfmt or prettier, and the user trusts
  the project's formatter, which runs code the project controls. Without the
  plugin, without a formatter that runs on the file, or without that trust, the
  edit is refused as `unsupported` and nothing is written. The trust is
  execution trust: the user answers the prompt `kapi apply` shows in a
  terminal, once per formatter configuration. MCP `apply_edits` never runs a
  project's formatter: give the change set to the user to apply with
  `kapi apply`. Report such a refusal to the user rather than setting
  `KAPI_TRUST_EXEC` or answering the prompt yourself. Keep JSDoc tags such as
  `@param` and every `{@link}`: dropping one refuses the edit.
- The result carries the findings of a check scoped to what was written. If it
  reports one, send another edit for it. Then check the whole change
  (`kapi check --diff-against <base>` or `--staged`) before you report done.

## Which formats can I edit?

`kapi formats` reports an **Edit** column and the JSON adds `editable` and
`round_trip`:

```bash
kapi formats --json | jq -r '.formats[] | select(.editable) | .name'
```

A format is **editable** when it has both a reader and a writer and is not a
bilingual interchange format. That **includes binary office formats** (`.docx`,
`.pptx`, `.xlsx`): the faithful round-trip is exactly what makes editing a binary
container safe. `round_trip` (shown as `faithful` in the table) means the writer
reconstructs from a skeleton, so only your edited text changes and the rest is
byte-for-byte preserved. A read-only format (PDF is extraction-only) is not
editable. To translate it, extract to a bilingual format and merge (see
[translate.md](translate.md)).

Binary formats can be **edited in place but not authored from scratch**; to
create new content, author in a generative format (see [create.md](create.md)).

## Mixed change sets

A content edit and the term that justifies it can travel in **one change set**.
The content lands first, the term after it, and when any operation is refused
neither is written:

```json
{"ops": [
  {"op": "replace_text", "at": {"doc": "draft.md", "block": "intro/p"}, "if_match": "r:5e10a2b3c4d5e6f7",
   "edits": [{"find": "control panel", "text": "dashboard"}]},
  {"op": "term", "action": "upsert", "term": "dashboard", "locale": "en", "status": "preferred", "replaces": "control panel"}
]}
```

A term operation can also carry `"do_not_translate": true` to keep a name
verbatim in every language, or `false` to clear that. One that omits it leaves
the flag as it is.

A `term`, `memory` or `recipe` operation writes the project's context or its
recipe directly, which is a person's decision, and so is a `decide` other than
`advise`. kapi records each operation as whoever runs the command, so run from
your shell such an operation is refused, and the change set with it. Send your
content edits on their own, record the rule as a suggestion instead
([create.md → close the loop](create.md)), and leave the term for the person to
apply. For a word rule specifically, see [voice.md](voice.md).
