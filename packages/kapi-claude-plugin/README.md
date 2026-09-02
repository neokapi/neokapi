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
- **Voice profile** — load a voice guide, score a draft (0–100 + findings), fix what
  drifts; the project's vocabulary and terminology enforced at the gate.
- **Translate & publish** — translate, enforce terminology, and round-trip into
  other languages and formats.
- **i18n setup** — add i18n to a project.

The hooks (`plugins/kapi/hooks/hooks.json`), both scoped to the `.kapi` project in
the session's working directory and both **fail-open** (outside a project, or if
they cannot run, the assistant proceeds normally):

- **`Stop` → `kapi hook stop`.** When the assistant tries to finish, it runs the
  project's `kapi check --ship` gates — brand voice, terminology, translation checks —
  and, if a gate is failing, keeps the assistant working with the findings to fix.
  The skill makes the check loop the default; this hook makes it a guarantee.
- **`PreToolUse` (Edit/Write/MultiEdit) → `kapi hook pre-edit`.** Denies direct
  hand-edits to files the project *generates* as translation targets (a
  `kapi merge` output), steering the change through the source + round-trip
  instead. Source files and files the project does not generate are never blocked.

## How this repo is maintained

This marketplace is **generated** from the [neokapi
monorepo](https://github.com/neokapi/neokapi): the skill source of truth lives in
`cli/skills/data`, is embedded in the `kapi` binary, exported into the bundle
(`make plugin-bundle`), and published here (`make publish-plugin`). Edit the skill
in the monorepo, not here.
