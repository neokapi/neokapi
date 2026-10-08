---
name: kapi
description: Use when writing or editing prose that ships from this repository (documentation, README text, UI strings, error messages, release notes, product copy, code comments), or content inside Word, PowerPoint, JSON, XLIFF, Markdown, HTML, YAML and other formats. Follow the writing rules kapi keeps in AGENTS.md and CLAUDE.md, record what you notice and the person's corrections, and check what you changed before reporting it done. Also for the project's voice, terms and translation.
---

# kapi

kapi holds this project's voice, terms and approved wording. Keep four habits.

## 1. Follow the rules files

kapi writes each folder's rules into AGENTS.md and CLAUDE.md. Follow them
without asking first. A "Keep as it is" line means the old name is correct
there, even when the task asks for the rename.

Only for a file no section covers, or for the full answer:

- CLI: `kapi context <file>`, or `kapi context search <word>`
- MCP: `context_read` with the file, or `context_search`

## 2. Record what the project does every time

Record what the files keep to and the rules do not list, even what your
text does not use: a name, the spelling variety, a word used where writers
often use another. Pass the project's form as the term, and the split form,
other spelling or other word as instead-of. Record nothing about a word the
files write two ways.

- CLI: `kapi context note --term <used> --instead-of <avoided> --seen-in <file>`,
  or `"<fact>"` for the variety
- MCP: `context_note` (`term` and `instead_of`, or `text`)

`--withdraw <id>` takes a note back. A person decides what becomes a rule.

## 3. Record the person's corrections

- CLI: `kapi context note --from "<yours>" --to "<theirs>" --seen-in <file> --suggest`
- MCP: `context_note` with `from`, `to`, `suggest`

## 4. Check what you changed

- CLI: `kapi check <file>...`
- MCP: `check_file` on each changed file

Fix what it reports; exit 4 means it did not run. After a note,
end with `kapi context log --session this` (MCP: `context_session_summary`).

Before you edit Word, XLIFF or other files your tools cannot edit safely,
run `kapi help edit`.

`kapi help <topic>` prints another topic: toolbox, voice, translate, project
or i18n.
