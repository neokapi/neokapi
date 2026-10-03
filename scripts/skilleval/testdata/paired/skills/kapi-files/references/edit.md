# Edit content in any format

Edit the text inside a file and write it back in the same format, byte for byte
except for the text you changed. You do the editing; kapi-files parses the
format, guards the round trip and writes the file. No model provider is
involved.

## 1. Read the blocks

```bash
kapi-files inspect site/page.html --jsonl
```

```json
{"ref":{"doc":"site/page.html","block":"p#2"},"rev":"r:0d71f30c75e4a087","text":"Revenue rose, see the <x id=\"1\"/>dashboard<x id=\"/1\"/>.","codes":{"1":{"kind":"paired","type":"link:hyperlink","attrs":{"href":"https://example.com/dash"},"writable":["href"]}},"ops":["set_content","replace_text","set_attribute","mark"]}
```

- **`ref`** names the block: its document and its key. Copy it into the
  operation's `at`.
- **`rev`** is the revision of the text you read. Send it back as `if_match`,
  so kapi-files can tell the block is still what you read.
- **`text`** renders inline codes (links, bold spans, placeholders) as
  `<x id="…"/>` tokens. **Keep every token, unchanged, in your edited text.** A
  placeholder is `<x id="1/"/>`; a paired span opens with `<x id="1"/>` and
  closes with `<x id="/1"/>`. `codes` lists each one with its type and
  attributes, and `writable` names the attributes `set_attribute` can change.

`ops` lists the operations the block accepts. A block with none, such as a
Markdown code block, keeps its text. Each form of a plural is a block of its
own. A file is read with the format its name and content suggest; name one
with `-f` (`kapi-files inspect -f androidxml res/values/strings.xml`) when the
detected one is too general. A bilingual file holds a translation: a PO
catalog lists it once you name its language
(`kapi-files inspect --target-lang nb messages.po`), and an operation on it
names that language in `at.edition`.

## 2. Write the edits

Write a change set with one operation per block you change:

```json
{"note": "Point the report link at the new page",
 "ops": [
  {"op": "set_attribute", "at": {"doc": "site/page.html", "block": "p#2"}, "if_match": "r:0d71f30c75e4a087",
   "code": "1", "name": "href", "value": "https://example.com/reports"},
  {"op": "replace_text", "at": {"doc": "site/page.html", "block": "p#2"}, "if_match": "r:0d71f30c75e4a087",
   "edits": [{"find": "dashboard", "text": "report page"}]}
 ]}
```

`set_content` gives a block new text. `replace_text` changes text inside it by
`find` (with `occurrence` when it matches more than once), by code-point
`start` and `end`, or by run positions in `range`, and keeps the codes around
it. A change set can also be JSONL (the envelope on the first line, one
operation per line) or a JSON array of operations.

```bash
kapi-files apply edits.json --dry-run   # check it and print a diff per document, write nothing
kapi-files apply edits.json             # apply it
kapi-files apply edits.json --json      # print the result (kapi.change-result/v1)
```

## 3. What refuses an edit

`apply` writes a change set only when every operation in it holds. When one is
refused, nothing is written, the refused operation carries an `error` with a
`code`, and every other operation reports `not_applied`:

- **`stale`**: the block's revision is no longer the `if_match` you sent; the
  file changed since you read it. The result carries the block's `current`
  revision and text: rebase your edit on it and send it again.
- **`guard`**: your text drops, invents or unbalances an `<x id="…"/>` token.
- **`not_found`**: the `ref` names no block, or a `find` matches nothing.
- **`ambiguous`**: a `find` matches more than once. Add `occurrence`.
- **`unsupported`**: the block or format takes no such operation.

A refusal exits 3; a change set that does not decode exits 2.

## Add a key to a catalog

A JSON, YAML or ARB catalog takes `insert_block`, addressed to the document:

```json
{"ops": [{"op": "insert_block", "doc": "locales/en.json", "after": "nav.cart", "name": "nav.checkout",
          "editions": {"en": {"text": "Checkout"}}}]}
```

The key goes in the object its path names (`nav.checkout` goes in `nav`),
after the key `after` names or before the one `before` names.

## Write a translation into a file of its own

Each file is a document in its own language. To write a translation that lives
in a file of its own, copy the source file to the translation's path, read the
copy with `inspect`, and replace each block's text with `set_content`, keeping
every `<x id="…"/>` token. The markup, links and code spans come from the copy.
