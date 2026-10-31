# neokapi-plugins — Claude Code marketplace

The Claude Code plugin marketplace for [neokapi](https://github.com/neokapi/neokapi):
give your AI coding assistant the terms, voice and rules your project goes by,
check every edit against them, and publish content in every language and format.

## Install

```text
/plugin marketplace add neokapi/claude-plugins
/plugin install kapi@neokapi-plugins
```

Then restart Claude Code. The plugin needs the `kapi` CLI on your `PATH`
(`kapi version`) — install it from
[neokapi releases](https://github.com/neokapi/neokapi) or
`brew install neokapi/tap/kapi-cli`. The skill drives the local CLI; no
AI-provider credential is required for the attended loop. Keep the CLI updated
alongside the plugin so the skill matches your installed command surface.

## Plugins

### `kapi`

One `kapi` Agent Skill — a router `SKILL.md` that loads the relevant reference on
demand (progressive disclosure) — plus two Claude Code hooks. It covers the loop
**know the context → author → check → fix → publish**:

- **Discover & refresh the context** — draft a project's voice profile and
  terminology from the repository it already has, then, when the material moves,
  propose the delta as a change-set the user approves before anything is written.
- **Author & edit** — read a document's blocks (`kapi inspect`), rewrite them, and
  write them back faithfully (`kapi apply`) in any format your editor can't open
  (Word, PowerPoint, JSON, XLIFF); no second model.
- **Voice profile** — follow the voice and the rules kapi writes into the
  project's `AGENTS.md` and `CLAUDE.md`, score a draft (0–100 + findings), fix
  what drifts; the project's terms enforced at the gate.
- **Grow the context** — record the names and corrections met while working
  (`kapi context note`) as suggestions a person reviews.
- **Translate & publish** — translate, enforce terminology, and round-trip into
  other languages and formats.
- **i18n setup** — add i18n to a project.

The hooks (`plugins/kapi/hooks/hooks.json`), all scoped to the `.kapi` project in
the session's working directory and all **fail-open** (outside a project, or if
they cannot run, the assistant proceeds normally):

- **`PostToolUse` (Edit/Write/MultiEdit) → `kapi hook post-edit`.** After each
  edit, it runs `kapi check` on the one file the edit wrote. A failing finding
  (a name, a spelling, a word rule) comes back to the assistant beside the
  edit's result, with the fix when the rule names one, so the assistant
  corrects the file before it reports done. Files outside the project, files
  the project does not read and generated targets are left alone. Each run is
  recorded in the project's log under the session.

- **`Stop` → `kapi hook stop`.** When the assistant tries to finish, it runs the
  project's `kapi check --ship` gates (voice, terminology, translation checks)
  and, if a gate is failing, keeps the assistant working with the findings to fix.
  The skill makes the check loop the default; this hook makes it a guarantee.
  Each run is recorded in the project's log (`kapi context log`) under the
  session, and a finding one run of either hook reports and the next run on
  the same file no longer finds counts as caught.
- **`PreToolUse` (Edit/Write/MultiEdit) → `kapi hook pre-edit`.** Denies direct
  hand-edits to files the project *generates* as translation targets (a
  `kapi merge` output), steering the change through the source + round-trip
  instead. Source files and files the project does not generate are never blocked.

## How this repo is maintained

This marketplace is **generated** from the [neokapi
monorepo](https://github.com/neokapi/neokapi): the skill source of truth lives in
`host/skills/data`, is embedded in the `kapi` binary, exported into the bundle
(`make plugin-bundle`), and published here (`make publish-plugin`). Edit the skill
in the monorepo, not here.
