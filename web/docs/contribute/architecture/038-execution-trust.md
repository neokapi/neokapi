---
id: 038-execution-trust
sidebar_position: 38
title: "AD-038: Execution trust"
---

# AD-038: Execution trust

## Summary

Two built-in tools exist to run code a *file* chooses: `external-command`
spawns a subprocess from its own config, and `script` evaluates
recipe-supplied JavaScript. So does one format: `exec` reads content by
shelling out. Together they are the **exec class**.

Every surface that can name an exec-class tool answers one question — *did a
person choose this?* — and the answer is a property of the surface, not of the
tool:

| Surface | Where the argv comes from | Answer |
| --- | --- | --- |
| `kapi exec <tool>` | the command line the user typed | runs; this **is** the choice |
| A `kapi.yaml` recipe | a file in the working directory | asks once, remembers, re-asks when the argv changes |
| A `.kpz` package | an archive from elsewhere | stripped on ingest |
| The engine gRPC API | the network | refused |
| The MCP agent surface | a model | refused, including under `--all-tools` |

Only the recipe row can ask, because it is the only one with a person present
and a legitimate reason to say yes. The rest have a fixed answer, so they do
not ask.

## Context

### Discovery makes a recipe an ambient input

`core/project.ResolveLayout` finds a project by walking up from the working
directory, the way `git` finds a repository. This is the right behaviour for a
tool people run inside checkouts, and it means **entering a directory is enough
to bind its recipe**. A recipe is therefore not something the user opened; it
is something they stood next to.

Most of what a recipe declares is inert — languages, content globs, gates,
per-tool settings. The exec class is not, and it was reachable with no gate at
all: `Validate()` on `external-command` checked that the command was non-empty
and nothing else, so a flow step named a program and the framework ran it, with
the user's privileges and the user's whole environment. That environment
includes provider API keys, which resolve from conventional variables by
design.

This is the `npm postinstall` threat model. The distinguishing feature of that
model is not that code runs — build tooling runs code constantly — but that
running it was never surfaced as a decision.

### The classification already existed; only one arm applied it

The judgement had been made before, twice, on the surfaces where it was
forced:

- **MCP** withholds both tools from the agent surface, and deliberately does
  not fold them into `--all-tools`, because *"show me every tool"* and *"let a
  caller execute arbitrary commands"* are different requests (C-06).
- **`.kpz` ingest** strips exec-class steps, their arming config, and
  exec-class format bindings from a package's recipe, because a package has
  crossed a trust boundary and a hostile packer simply will not sanitise on the
  way out.

Both could refuse outright because neither has a user to ask. The recipe arm
had no gate because it is the one case where the answer legitimately varies —
and "it depends" had been resolved as "yes, always".

## Decision

### A sticky, per-project decision, keyed to what was approved

The gate lives in `host.LoadProjectInteractive`, the wrapper every
project-aware command already routes through, so every command inherits it at
once. `core/project.ExecSurface` enumerates the exec-class sites in a loaded
recipe — flow steps including nested `parallel` branches and the retired
`source_transforms` stage, `defaults.tools` and `defaults.locales.*.tools`
presets, and format bindings on content collections and their items. An empty
surface, which is every recipe in this repository and the overwhelming majority
of real ones, means there is nothing to decide and nothing is shown.

The stored record answers *what was approved*, not *which project was trusted*:

- **It is keyed by absolute recipe path**, so a second checkout of the same
  project is a second decision. Approval does not travel with a copy.
- **It carries a digest of the exec surface alone** — each site's location,
  kind, name and canonical config — not of the whole file. Adding a target
  language keeps the approval; adding an argument to the approved command does
  not. This is what makes a sticky decision safe: it attaches to the argv a
  person read, so a recipe cannot be approved once and rewritten afterwards.
- **It lives under `ConfigDir()`**, not in the project's `.kapi/`. The state
  directory is documented as safe to delete and regenerate, so a record there
  would evaporate on a routine clean — and, worse, could ship inside the
  project for the next person to inherit. `ConfigDir()` honours
  `KAPI_CONFIG_DIR`, so a run isolated per the in-repo dogfood contract cannot
  read or write the developer's real decisions.

Declines are remembered too. Re-asking a question already answered no is how a
person is trained to answer yes.

### `--yes` does not grant execution trust

`--yes` means *"do not stop for prompts I would obviously accept"*, and it is
already passed by every unattended script that wants plugin auto-install.
Reusing it as the key for this decision would mean that on the day the gate
shipped, every existing automation silently acquired the right to run whatever
a checked-out recipe named — and automation over untrusted checkouts is
precisely the population the gate exists for. The flag would have granted the
thing it was introduced to gate.

`KAPI_TRUST_EXEC=1` is the separate opt-in, named for what it grants. It is
process-scoped and never written to the record: a container's config directory
is not a place to persist a decision. Unlike `KAPI_NO_PROJECT` and
`KAPI_PLUGINS_DIR_ONLY`, which treat any non-empty value as set, it requires an
affirmative value — reading `KAPI_TRUST_EXEC=0` as "yes" is the wrong way for a
switch like this to be wrong.

With no terminal and no opt-in, kapi **refuses**. Assuming yes when there is
nobody to ask would make the gate a formality.

### Refusal is enforced under tool construction, not only at load

The prompt is the user experience; it is not the enforcement point. A handful
of internal paths load a recipe directly rather than through
`LoadProjectInteractive`, and whether a subprocess spawns should not depend on
which of them got there first. `App.checkExecToolAllowed` therefore sits on
`toolFromStep` and `buildToolByName` — the two chokepoints through which
recipe-driven configuration becomes a tool — and refuses to build an
exec-class tool with no decision behind it. It never prompts: by the time a
flow is assembling steps there is no sensible place to stop and ask. It
re-reads the record instead, so a project already approved keeps working on
those paths.

`kapi exec <tool>` is untouched. It builds from the registry with an argv the
user typed, which is the user's own intent rather than a file's.

## Consequences

- **Nothing in this repository prompts.** No recipe here, no sample and no
  example names an exec-class tool, which is also why the gate could be added
  without a migration.
- **The tools are not removed, deprecated, or hidden.** `external-command` and
  `script` keep their config factories, their schemas, their CLI commands and
  their reference pages. The change is that a recipe cannot arm them silently.
- **A single classification, applied everywhere.** `IsExecClassTool` and
  `IsExecClassFormat` live in `core/project` so the recipe arm, the gRPC arm
  and the enforcement backstop cannot drift from one another.
- **Recipe text reaching a terminal is sanitised.** The prompt shows the
  command it is asking about, and that text is attacker-authored. Control
  characters are stripped and the summary is clipped, so the question cannot be
  repainted or scrolled away by the thing it is asking about.
- **Losing the record costs one prompt.** It is not authoritative state; an
  unreadable or corrupt file is treated as "nothing decided yet" rather than as
  an error, and the file is plain JSON so a decision can be withdrawn by
  deleting its entry.
