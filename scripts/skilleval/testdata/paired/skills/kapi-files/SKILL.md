---
name: kapi-files
description: Use when reading, editing, searching or translating the text inside a file of any format (HTML, Markdown, JSON, YAML, ARB, PO, XLIFF, Android and Apple string resources, Word, PowerPoint and others). kapi-files reads a file into content blocks, applies the edits you write as a change set, and writes the file back in its own format with only your text changed.
---

# kapi-files

`kapi-files` edits the content inside files. It works on the files you name:
there is no project, so no recipe, terms, voice or check applies to an edit.

## The loop

1. Read the blocks: `kapi-files inspect <file> --jsonl`. Each block carries
   `ref` (copy it into an operation's `at`), `rev` (send it as `if_match`) and
   `text`, with inline codes as `<x id="…"/>` tokens that an edit keeps.
2. Write a change set (kapi.change/v1) with one operation per block you change:
   `set_content` replaces a block's text, `replace_text` changes part of it,
   `set_attribute` changes a link's `href`, and in a JSON, YAML or ARB catalog
   `insert_block` adds a key.
3. Apply it: `kapi-files apply edits.json`, or pipe it on standard input.
   `--dry-run` prints a diff and writes nothing.
4. A refusal exits 3 and writes nothing. Read its `code`, fix the operation
   and send it again.

`kapi-files apply --schema` prints the whole contract. `kapi-files formats`
lists the formats and what each can do. `kcat`, `kgrep`, `ksed`, `kdiff` and
`kconv` run as `kapi-files kcat …` and so on.

Read [references/edit.md](references/edit.md) before your first change set.
