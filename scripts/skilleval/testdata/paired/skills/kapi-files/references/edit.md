# Edit content in any format

Edit the text inside a file an editor can't open directly (a Word document, a
PowerPoint deck, a JSON catalog, an XLIFF file, Markdown) and write it back in
the same format, byte-for-byte except for the text you changed. You do the
editing; kapi-files parses the format and enforces a faithful round-trip. No
model provider is involved.

This is the **read → edit → write** loop. It is the deliberate, reviewed
counterpart to a `ksed` find-and-replace (see [toolbox.md](toolbox.md)): reach
for this loop when you are rewriting block text by hand (a clarity pass, a
terminology correction), and for `ksed` when a regex substitution expresses the
change.

## 1. Read the blocks

`kapi-files inspect` reads any format into one record per content block:

```bash
kapi-files inspect report.docx --jsonl
```

```json
{"ref":{"doc":"report.docx","block":"word/document.xml/p"},"rev":"r:3f9a1c0e7b2d4a55","text":"Quarterly summary","ops":["set_content","replace_text"],"role":"heading","level":1}
{"ref":{"doc":"report.docx","block":"word/document.xml/p#2"},"rev":"r:0d71f30c75e4a087","text":"Revenue rose, see the <x id=\"1\"/>dashboard<x id=\"/1\"/>.","codes":{"1":{"kind":"paired","type":"link:hyperlink","attrs":{"href":"https://example.com/dash"}}},"ops":["set_content","replace_text"]}
```

Three fields anchor an edit:

- **`ref`** names the block: its document, its key, and, for a translation, its
  edition. Copy it into the operation's `at`.
- **`rev`** is the revision of the text you read, covering its inline codes and
  their attributes. Send it back as `if_match` so kapi-files can tell the block
  is still what you read.
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
plural or select with the path to each of its branches. A bilingual file lists
the translation it holds, and a PO catalog lists it once you name its language
(`kapi-files inspect fr.po --target-lang fr`). An operation on a translation
names its language in `at.edition`.

Everything else in `text` is text, and kapi-files encodes it for the file's
format. In HTML a `<` or `&` you type is written as a character reference.
Markdown, MDX and AsciiDoc keep some of their own syntax in `text`: a backslash
escape, a bracket that opens no link, an MDX `{expression}`, an AsciiDoc `+++`
passthrough. Syntax the block already holds stays as you keep it, and syntax
you add (a tag, a link, an expression, an `import` line, a macro) is escaped so
it reads as the characters you typed.

`inspect` reads a file with the same reader `apply` writes it back through, so
every `ref` and `rev` it prints is one `apply` resolves. A document is named as
you name it on the command line, so run both from the same directory. An HTML
image's `alt` or a link's `title` is a block of its own.

## 2. Write the edits

Write a change set (kapi.change/v1). `set_content` gives the block new text;
`replace_text` changes text inside it and keeps everything around it. Several
operations may name one block; each sends the `rev` you read, because every
`if_match` is checked against the content the change set began with:

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
`range`. `find` reads as `text` does: a `<x id="…"/>` token in it matches that
code, and the replacement then keeps each token you write back. To add a
sentence at the end of a block, send `replace_text` with `find` set to its last
words and `text` set to them with the sentence after, rather than the whole
text again. Positions name the block as you read it, even when an earlier
operation of the same change set changed that block: kapi-files moves them past
that change, so several fixes of one block, each computed from the one read,
apply together. A position inside text an earlier operation changed is refused
as `guard` (`overlap`); a `find` matches the text as the earlier operations left
it.

To edit one branch of a plural or select, give the operation the branch's
`path` from `structures`, such as `[0, {"plural": "one"}]`: `set_content`
replaces the branch's text, and `replace_text` edits inside it (an edit's own
`path` overrides the operation's). The branch is never part of `at`:

```json
{"ops": [
  {"op": "set_content", "at": {"doc": "lib/intl_en.arb", "block": "cartCount"}, "if_match": "r:77c0a1d2e3f40516",
   "path": [0, {"plural": "one"}], "text": "<x id=\"p1/\"/> item in your cart"},
  {"op": "replace_text", "at": {"doc": "lib/intl_en.arb", "block": "cartCount"}, "if_match": "r:77c0a1d2e3f40516",
   "path": [0, {"plural": "other"}], "edits": [{"find": "items", "text": "articles"}]}
]}
```

`set_attribute` changes one attribute of an inline code, such as a link's
`href`: the code's id in `code` (`1`, as its `<x id="1"/>` token shows), the
attribute in `name` and its new value in `value`. `mark` wraps the text
`range` names, as a `replace_text` edit names it, in a new code of a `type`
the format writes, such as `fmt:bold`. A code lists the attributes
`set_attribute` can change under `writable`, and a block's `ops` list both
where they apply:

```json
{"ops": [
  {"op": "set_attribute", "at": {"doc": "docs/guide.html", "block": "p"}, "if_match": "r:3f9a1c0e7b2d4a55",
   "code": "1", "name": "href", "value": "https://example.com/handbook"},
  {"op": "mark", "at": {"doc": "docs/guide.html", "block": "p"}, "if_match": "r:3f9a1c0e7b2d4a55",
   "range": {"find": "before you"}, "type": "fmt:bold"}
]}
```

A change set can also be JSONL (the envelope fields on the first line, one
operation per line) or a JSON array of operations. `kapi-files apply --schema`
prints the whole contract, and `kapi-files apply --schema set_attribute` one
operation's fields.

Then apply it. `kapi-files apply` reads the change set from stdin, so pipe it
in; you write it from the blocks you read, and there is no command that writes
it for you:

```bash
printf '%s' '<change set>' | kapi-files apply - --dry-run   # check it and print a diff per document, write nothing
printf '%s' '<change set>' | kapi-files apply -             # apply it
printf '%s' '<change set>' | kapi-files apply - --json      # print the result (kapi.change-result/v1)
```

For a change set in a file, write the file under `$TMPDIR`, never in the
repository you are editing:

```bash
kapi-files apply "$TMPDIR/edits.json" --dry-run          # check it, write nothing
kapi-files apply "$TMPDIR/edits.json" --in-place=.bak    # apply, keeping a .bak of each file it replaces
```

kapi-files never sends content to a model to rewrite it; you write the new text
and `kapi-files apply` round-trips it back.

## 3. What refuses an edit

`apply` writes a change set only when every operation in it holds. When one is
refused, nothing in the change set is written, the refused operation carries an
`error` with a `code`, and every other operation reports `not_applied`:

- **`stale`**: the block changed after you read it, so the `if_match` you sent
  no longer holds. The result's `current` is the re-read: resend against its
  `rev`. The difference is another editor's change; keep it, and tell the
  user.
- **`guard`**: your edited `text` drops, invents, duplicates or unbalances an
  `<x id="…"/>` token (`codes_changed`), or replaces a block holding a plural or
  select with flat text (`structure_lost`). Edit a branch with `path` instead.
- **`not_found`**: the `ref` names no document or block, or a `find` matches
  nothing; the refusal quotes the text it searched, and a `find` that lies in
  a plural's branch names the branch's `path`. Re-read the file.
- **`ambiguous`**: a `find` matches more than once. Add `occurrence`.
- **`unsupported`**: the block or format takes no such operation, such as a
  Markdown code block, which `inspect` lists with no `ops`. A block of a file
  in another encoding than UTF-8 shows `�` in its text and lists no
  `replace_text`. An edit that would write `�` over the bytes you cannot see,
  a `replace_text` or a `set_content` whose text still holds `�`, is refused
  with capability `encoding`. Send the block's whole text with `set_content`,
  every character stated, or ask the user to convert the file to UTF-8.

A refusal exits **3**, distinct from an operational error: fix the operation
and resend, against the `current` of a `stale` one. A change set that does not
decode exits
**2**. Resending a change set that landed writes nothing: its revisions no
longer hold, so it is refused `stale` with each block's current text, which
already reads as you wrote it. An operation reports `unchanged` when its
`if_match` still holds and the block already says what it sends.

An agent's change is refused as `not_permitted` when it writes a term, a
content-memory pair or the recipe, records a review decision other than a
pre-review, or sends `if_match: "*"` in place of the revision it read. The
refusal names what to do instead, and exits 3.

## Add and remove a key in a catalog

A JSON, YAML or ARB message catalog takes `insert_block` and `delete_block`.
Every other format refuses both as `unsupported`, and so does a JSON catalog
whose configuration reads a note or an id from the members beside a message.
The `ops` a block lists in `kapi-files inspect` show where they apply:

```json
{"ops": [
  {"op": "insert_block", "doc": "locales/en.json", "after": "nav.cart", "name": "nav.checkout",
   "editions": {"en": {"text": "Checkout"}}},
  {"op": "delete_block", "at": {"doc": "locales/en.json", "block": "nav.legacy"},
   "if_match": {"en": "r:5e10a2b3c4d5e6f7"}}
]}
```

- `insert_block` takes the new key in `name` and its text in `editions`, under
  the catalog's own language. The key goes in the object its key path names
  (`nav.checkout` goes in `nav`), after the key `after` names or before the one
  `before` names, and last in that object with neither; an object missing on
  the path is created. A key the catalog already holds is refused as `stale`
  with that block as `current`.
- `delete_block` names the block in `at`, with no edition, and the revision
  of the catalog's own language in `if_match`.
- A content operation on a block the same change set adds or removes is
  `invalid`. Put a new key's text in `editions`.

## Which formats can I edit?

`kapi-files formats` reports an **Edit** column and the JSON adds `editable`
and `round_trip`:

```bash
kapi-files formats --json | jq -r '.formats[] | select(.editable) | .name'
```

A format is **editable** when it has both a reader and a writer and is not a
bilingual interchange format. That **includes binary office formats** (`.docx`,
`.pptx`, `.xlsx`): the faithful round-trip is exactly what makes editing a binary
container safe. `round_trip` (shown as `faithful` in the table) means the writer
reconstructs from a skeleton, so only your edited text changes and the rest is
byte-for-byte preserved. A read-only format (PDF is extraction-only) is not
editable.

Binary formats can be **edited in place but not authored from scratch**.
