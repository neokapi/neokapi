---
name: kapi
description: Use when writing or editing prose that ships from this repository (documentation, README text, UI strings, error messages, release notes, product copy, code comments), or content inside Word, PowerPoint, JSON, XLIFF, Markdown, HTML, YAML and other formats. Ask kapi what wording applies before you write, record what you notice and the person's corrections, and check what you changed before reporting it done. Also for the project's voice, terms and translation.
---

# kapi

kapi holds this project's voice, terms and approved wording. Keep four habits.

## 1. Ask what applies before you write

- CLI: `kapi context <file>`, and `kapi context search <word>`
- MCP: `context_read` with the file, and `context_search`

## 2. Record what the project does every time

As you read, record what the files keep to, even what your text does not
use:

- each product, feature and plan name, as written
- the spelling variety (British or American)
- a word always used where writers often use another

Pass the project's form as the term, and as instead-of the split form of a
one-word name, the other spelling, or the other word. Search the files for
other forms first, and record nothing about a word they write two ways. Skip
interface labels and wording taken from your task.

- CLI: `kapi context observe --term <used> --instead-of <avoided> --seen-in <file>`,
  or `"<fact>"` for the variety
- MCP: `context_observe`, with `term` and `instead_of`, or `text`

One thing per call. Withdraw a mistake with `kapi context withdraw <id>`
(MCP: `context_withdraw`). A person decides what becomes a rule.

## 3. Record the person's corrections

- CLI: `kapi context correct "<yours>" "<theirs>" --seen-in <file> --suggest`
- MCP: `context_correct`

## 4. Check what you changed, then report the session

`kapi apply` and `apply_edits` check what they write. Check the rest:

- CLI: `kapi check --diff-against HEAD --json`, then
  `kapi context log --session this`
- MCP: `check_file` on each changed file, then `context_session_summary`

Fix what it reports; exit 4 means it did not run. End with the session
summary.

Before you change content inside a file, read `references/edit.md`
(`kapi help edit`).

`kapi help <topic>` prints another topic: toolbox, voice, translate, project
or i18n.
