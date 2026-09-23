---
name: kapi
description: Use when writing or editing prose that ships from this repository (documentation, README text, UI strings, error messages, release notes, product copy, code comments), or content inside Word, PowerPoint, JSON, XLIFF, Markdown, HTML, YAML and other formats. Ask kapi what wording applies before you write, record what you notice and the person's corrections, and check what you changed before reporting it done. Also for the project's voice, terms and translation.
---

# kapi

kapi holds what this project has recorded about how it writes: the voice, the
terms and the wording already approved at each location. Keep four habits.

## 1. Ask what applies before you write

- CLI: `kapi context <file>`, and `kapi context search <word>`
- MCP: read `context://<file>`, and call `context_search`

Build on the answer. A proposed rule advises; only an established rule fails a
check.

## 2. Record what you notice

- CLI: `kapi context observe "<fact>" --seen-in <file>`, and
  `kapi context propose <term> --use <better> --seen-in <file> --quote "<text>"`
- MCP: `context_observe`, `context_propose`

Record one fact per call, as you read. Take back a proposal of yours that turns
out wrong with `kapi context discard <id>` (MCP: `context_withdraw`).

## 3. Record the person's corrections

- CLI: `kapi context correct "<yours>" "<theirs>" --seen-in <file> --propose`
- MCP: `context_correct`

## 4. Check what you changed, then report the session

- CLI: `kapi check --diff-against HEAD --json`, then
  `kapi context log --session this`
- MCP: `check_file` on each changed file, then `context_session_summary`

Repair what it reports and run it again. Exit 4 means the check did not run,
which is never a pass. End your report with the session summary.

`kapi help` lists the topics for everything else (editing any format, voice,
terms, translation, i18n, project setup); `kapi help <topic>` prints one.
