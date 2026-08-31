---
id: s-01-kapi-cli
sidebar_position: 1
title: "S-01: The kapi CLI"
description: "The kapi binary is a thin Cobra shell over the cobra-free host runtime: verbs grouped by intent, ad-hoc and project modes resolved by a git-style upward walk, one output-format contract, and a stable exit-code contract that scripts and agents branch on."
keywords: [neokapi, architecture decision, kapi CLI, command surface, project resolution, exit codes, output format, credential store, MCP]
---

import { PhaseFlow } from "@neokapi/docs-shared";

# S-01: The kapi CLI

## Summary

`kapi` is the binary a person types. It is a thin [Cobra](https://github.com/spf13/cobra)
shell (`cli/`) over the cobra-free host runtime (`host/`), assembled in
`kapi/cmd/kapi`. Verbs are grouped by intent rather than by subsystem (*Work*,
*Translate*, *Assets*, *Advanced*), and every one of them runs either ad hoc on
files you name or inside a project, which the CLI finds by a git-style upward
walk for a `kapi.yaml` recipe. Three contracts make the surface scriptable: one
output-format resolution shared by every command, one exit-code table, and one
JSON error envelope. Configuration lives in the user config directory; provider
API keys live in the OS keychain. The same binary also answers to the toolbox
names ([S-04](s-04-toolbox.md)) and starts an MCP server for agents
([S-03](s-03-agent-surfaces.md)).

## Context

The framework has to reach three callers that share every capability and share
almost no invocation style.

An engineer processing a file wants one command, no state, and a usable exit
code. A team running a repeatable workflow wants the same capability bound to a
recipe under version control, so that "run it again" means the same thing on
another machine. An AI assistant wants typed input and output and a signal it can
branch on without reading prose.

Splitting these into separate binaries would fork the behaviour three ways. A
single binary with progressive complexity does not: the ad-hoc invocation is the
project invocation with the project left out, and the agent surfaces are the same
host functions under a different transport.

The module boundary follows from that. `host/` holds the runtime (registries,
services, project resolution, the storage layer) and knows nothing about Cobra.
`cli/` is the Cobra shell that binds flags to it. `kapi/` is the binary that wires
the shell up, adds the release-channel and self-update behaviour, and embeds
nothing else. See [F-01](../foundations/f-01-framework-and-modules.md) for the
full dependency direction.

## Decision

### Binary and module layout

`kapi` is a Go binary at `kapi/cmd/kapi/`, in the `kapi` module. It depends on
the framework, `host`, and `cli`, and on no plugin's code: a plugin is
discovered at runtime through the manifest model and dispatched as a subprocess
([E-05](../engine/e-05-plugin-system.md)), so the binary's dependency set is the
three modules above and nothing a plugin brings.

```
kapi/
├── go.mod                   # module github.com/neokapi/neokapi/kapi
├── cmd/kapi/                # root command wiring + telemetry reporter
├── cmd/kapi-wasm-cli/       # the browser build; registers cli.BrowserCommandSet
├── mcptools/                # the hand-authored MCP porcelain
├── preset/                  # built-in preset definitions
└── e2e/                     # end-to-end suite over the built binary
```

`cli.KapiCommandSet(app)` is the single source of truth for what the binary
exposes. The root command adds those, then attaches plugin commands and
contributions on top. Built-ins always register first, so installing a plugin can
never change what an existing verb means; a plugin command that collides with a
built-in attaches under its plugin group instead. The browser build registers
`cli.BrowserCommandSet`, which mirrors the native set verb for verb and records
the verbs a browser cannot run ([WASM Engine ABI](/contribute/implementation/surfaces/wasm-engine-abi)).

### Verbs are grouped by intent

`kapi --help` renders four groups (`cli.AddCommandGroups`), and the group a verb
lands in is the decision about who it is for.

| Group | What lives there |
| --- | --- |
| **Work** | the loop over a project's own content: bringing it up to date, checking it, reading its status, recording decisions, applying an edit, asking what context applies, and the project-composition verbs |
| **Translate** | the guardrailed built-in flows over files you name: a real translation pass and its pseudo-translation pre-flight |
| **Assets** | the standing resources a project draws on: content memory, terms, voice profiles, models, credentials |
| **Advanced** | the plumbing the porcelain composes, and the machinery around it: one named flow, one registry tool, the bilingual hand-off, the package verbs, block-level inspection and measurement, the registry listings, plugin management, configuration, the assistant hooks, and the MCP server |

Version, update, and shell-completion stay ungrouped, and registry tools render
no root group at all. The [command reference](/reference/commands/up) is
generated from the binary, so it is the current list rather than this prose.

Two verbs carry the layering explicitly. `kapi run <flow>` executes one named
flow; `kapi exec <tool>` executes exactly one registry tool with nothing around
it. The porcelain verbs are compositions over that layer: a project catch-up
runs many tools across many files, and a translation pass is a flow with recycling
and checks around it. Reach for the plumbing when you want precisely one tool's
behaviour.

Registry tools do not appear as top-level verbs; they are reached through
`kapi exec`. The generated [command reference](/reference/commands/exec) lists
each one with its schema, so the set stays derived from the registry rather than
transcribed into prose.

### Ad-hoc and project modes

Most verbs work on files you name. Where a project would supply defaults, the
verb takes `-p` / `--project` (registered by `host.AddProjectFlag`) and resolves
it through one shared helper, `host.ResolveProjectPath`, re-exported as
`cli.ResolveProjectPath`.

<PhaseFlow
  nodes={[
    { label: "Explicit -p <path>", sub: "wins outright", role: "io" },
    { label: "KAPI_NO_PROJECT set", sub: "stop: no implicit project", edge: "not given", role: "qa" },
    { label: "KAPI_PROJECT", sub: "environment override", edge: "not set" },
    { label: "Upward walk", sub: "project.ResolveLayout(cwd)", edge: "empty", role: "annotate" },
    { label: "Ad-hoc mode", sub: "or \"not a kapi project\"", edge: "nothing found" },
  ]}
  caption="Project resolution: an explicit flag wins, an opt-out stops discovery, and otherwise the walk finds the nearest kapi.yaml the way git finds .git."
/>

The upward walk is what makes a project feel like a repository: run a verb
anywhere inside the tree and it binds to the recipe above it. `KAPI_NO_PROJECT`
is the opt-out, and anything that runs *inside* a project tree without wanting
to act on it (a test harness, a nested build) depends on it. Setting
`KAPI_PROJECT` to the empty string does not disable discovery; only a non-empty
`KAPI_NO_PROJECT` does.

Once a project resolves, it supplies the defaults a flag would otherwise have to
carry: source and target locales, concurrency and encoding, the bound stores, and
the plugin scoping that narrows format detection to what the recipe declares
([C-01](../context/c-01-project-model.md)).

Every locale that enters through a flag, an environment variable, a recipe or a
file is canonicalized on the way in. `core/locale.Canonical` accepts what people
and file formats write (`nb_NO`, `en_US.UTF-8`, `NB-no`) and returns the BCP-47
form the rest of the system holds, and it rejects a value that is not a locale,
so a typo cannot become an identity nothing checks. The recipe's `locale_format`
(`bcp-47`, the default, or `posix`) governs only how a locale is spelled in the
paths kapi writes; internally there is one spelling.

:::warning `-p` is `--progress` under `kapi exec`
On `kapi exec <tool>` the `-p` shorthand is bound to `--progress`, the progress
bar; those commands take no `--project` flag. So `kapi exec translate -p
kapi.yaml` parses as a progress request with a stray positional argument, not as
a load-project request. Give `kapi exec` its project context through the upward
walk instead: run it from inside the tree, or point `KAPI_PROJECT` at the recipe.
:::

### One output contract

Every command that produces output resolves its format through `host/output`,
which registers `--json`, `--text`, `--output-format`, and `--jq` once. Resolution
is by precedence, highest first:

1. `--jq <expr>`: a filter implies JSON.
2. `--json`.
3. `--text`.
4. `--output-format=<text|table|json|yaml>`.
5. Default: text.

YAML emits the same structured record as JSON. Text is the human rendering:
tables, aligned columns, colour when the terminal supports it and `--color` /
`NO_COLOR` permit it.

Each command declares its result as a concrete Go type with `json` tags, and the
type is expected to be complete rather than a transcription of what text mode
shows. Types that render themselves implement one interface:

```go
type TextFormatter interface {
    FormatText(w io.Writer) error
}
```

A type without one falls back to formatted JSON, so a new command is never
unreadable, only unpolished.

### Exit codes and the error envelope

| Code | Symbol | Meaning |
| --- | --- | --- |
| 0 | `ExitOK` | success |
| 1 | `ExitError` | operational error, the default for a failed command |
| 2 | `ExitUsage` | usage error, and the toolbox's grep-style "trouble" status |
| 3 | `ExitGate` | a quality or voice gate was not met |
| 130 | `ExitSignal` | interrupted (128 + SIGINT) |

A draft that scores below its threshold is not a crash, and a script that
cannot tell the two apart has to choose between ignoring real failures and
treating every low score as one. `host.ErrQualityGate` maps to `ExitGate`; a
command can request any other code by tagging its error with
`host.WithExitCode`. `host.ErrSilentExit` requests a non-zero exit with the
message suppressed, which is how the toolbox reports "no match" as a status
rather than as an error line ([S-04](s-04-toolbox.md)).

Under `--json`, a failure is a structured envelope rather than a prose line, with
the code symbol mirroring the exit code:

```json
{
  "error": "failed to connect to server",
  "code": "gate"
}
```

Long-running verbs can also stream progress to stderr as NDJSON with
`--progress=jsonl`, keeping stdout reserved for the result. The full machine
contract is documented in [Scripting & JSON contract](/reference/cli-contract).

### Credentials and configuration

The credential store lives in `host/credentials/` and is shared with Kapi Desktop
([S-02](s-02-kapi-desktop.md)). Non-secret provider configuration is JSON at
`providers.json` under the kapi config directory; API keys go to the OS keychain
under the service name `kapi`: macOS Keychain, Windows Credential Manager, or
libsecret on Linux. Where no keychain exists, a provider key is read from its
conventional environment variable, the list `core/credentials/providerenv`
names, and the host strips those variables from every plugin subprocess
environment so a key kapi holds never reaches a plugin by accident.

Application configuration sits in the same directory: `~/.config/kapi` on Linux,
`~/Library/Application Support/kapi` on macOS, overridable with
`KAPI_CONFIG_DIR`, and printed by `kapi config path`:

```
kapi.yaml                # global settings
providers.json           # provider configs (keys in the keychain)
plugins/                 # installed plugins
```

`kapi.yaml` here holds global defaults: output format, log level, plugin
directory, UI language, update channel, telemetry opt-in, default provider. This
is the *global* config file and is not the project recipe, which is a
`kapi.yaml` at a project root ([C-01](../context/c-01-project-model.md)). CLI
flags override configuration values; `KAPI_`-prefixed environment variables
override the file. `KAPI_PLUGINS_DIR` names an alternative plugin root.

### Agent and toolbox surfaces on the same binary

`kapi mcp` starts an MCP server over stdio, exposing a curated tool set plus the
`context://` resources; `--all-tools` and `--all-flows` widen it for debugging,
and `--all` is the shorthand for both. `kapi hook` provides the
assistant-integration hooks. Both are covered in
[S-03](s-03-agent-surfaces.md).

The binary also answers to `kcat`, `kgrep`, `ksed`, `kconv`, and `kdiff` when
invoked under those names, busybox-style, and carries them as hidden subcommands
so `kapi kgrep` and `kgrep` behave identically ([S-04](s-04-toolbox.md)).

## Consequences

- One binary covers ad-hoc processing, project workflows, agent integration, and
  the toolbox, with no build flags and no separate distributions.
- Because the CLI is a shell over `host`, every capability it exposes is
  reachable without Cobra, which is what lets Kapi Desktop and embedded runs
  share the implementation instead of reimplementing it.
- Grouping by intent means the help output answers "what am I trying to do"
  rather than "what subsystem is this", and it gives the porcelain/plumbing split
  a place to live in the interface itself.
- The shared output contract makes `jq` and language-specific parsers work
  uniformly, and the JSON error envelope means a scripted caller never parses
  prose.
- The distinct gate exit code lets CI and agents branch on "below threshold"
  versus "crashed" without reading output, the property the assistant loops in
  [S-03](s-03-agent-surfaces.md) depend on.
- Keychain storage keeps API keys out of shell history and anything committed;
  a key supplied through the environment is accepted and scrubbed from every
  subprocess.
- One locale spelling inside the system means a target language written three
  ways in three places still resolves to one translation.
- Ad-hoc mode has no state beyond the input file, so a CI job needs nothing
  provisioned to run one.

## Related

- [F-01: The framework and its modules](../foundations/f-01-framework-and-modules.md): the module boundary the CLI sits on
- [E-01: The processing engine](../engine/e-01-processing-engine.md): flow execution behind `kapi run`
- [E-03: The tool system](../engine/e-03-tool-system.md): the registry behind `kapi exec`
- [E-05: The plugin system](../engine/e-05-plugin-system.md): runtime plugin discovery and `kapi plugin`
- [E-07: Model providers](../engine/e-07-model-providers.md): the credentials the store holds
- [C-01: The project model](../context/c-01-project-model.md): the `kapi.yaml` recipe that project mode loads
- [S-02: Kapi Desktop](s-02-kapi-desktop.md): the visual companion sharing this credential store
- [S-03: Agent surfaces](s-03-agent-surfaces.md): MCP, skills, and the hooks
- [S-04: Toolbox utilities](s-04-toolbox.md): the multi-call names on this binary
- [WASM Engine ABI](/contribute/implementation/surfaces/wasm-engine-abi): the browser build's command set and its recorded gaps
- [Command reference](/reference/commands/up): the generated, per-command surface
- [Scripting & JSON contract](/reference/cli-contract): structured results, progress events, exit codes
