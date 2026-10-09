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
*Languages*, *Assets*, *Advanced*), and every one of them runs either ad hoc on
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
| **Languages** | the guardrailed built-in flows over files you name: a real translation pass and its pseudo-translation pre-flight |
| **Assets** | the standing resources a project draws on: content memory, terms, voice profiles, models, credentials |
| **Advanced** | the plumbing the porcelain composes, and the machinery around it: one named flow, one registry tool, the bilingual hand-off, the package verbs, block-level inspection and measurement, the registry listings, plugin management, configuration, and the MCP server |

A command family divides the same way when its verbs serve different
readers. `kapi context` (Work) lists a person's verbs (`review`, `reset`,
`search`, `sync`) under *Commands* and the two an assistant uses (`note`,
`log`) under *For assistants*, and the maintenance of the store those verbs
read is a separate command, `kapi store` (Advanced: `import`, `export`,
`rebuild`, `locales`) ([C-11](../context/c-11-context-operations.md#surfaces)).

Version, update, telemetry and shell-completion stay ungrouped, registry tools render
no root group at all, and `kapi hook` is hidden. The [command reference](/reference/commands/up) is
generated from the binary, so it is the current list rather than this prose.

Two verbs carry the layering explicitly. `kapi run <flow>` executes one named
flow; `kapi exec <tool>` executes exactly one registry tool with nothing around
it. The porcelain verbs are compositions over that layer: a project catch-up
runs many tools across many files, and a translation pass is a flow with recycling
and checks around it. Reach for the plumbing when you want precisely one tool's
behaviour.

### Verbs for people, change sets for scripts

An edit reaches a file through the change service
([E-09](../engine/e-09-the-change-contract.md)), and the command line offers it
two ways. People keep verbs that say what they do: `ksed` compiles a
substitution into `replace_text` operations, each guarded by the revision `ksed`
read, and applies them through the service. Scripts and agents send a
kapi.change/v1 change set to `kapi apply`, which reads one JSON object, JSONL or
an array of operations from a file or standard input, applies it whole or not at
all, previews it with `--dry-run`, prints the kapi.change-result/v1 result with
`--json`, and prints the contract's schema with `--schema`. The command line
stamps the actor the environment names; a change set names none.

`kapi inspect` is the read that pairs with it. It reads through the same
service, with the format and configuration the recipe binds inside a project
(the one `-p` names, or the one discovery finds), and prints each block's
reference and revision, so an operation copies what the read printed instead of
constructing an address. A file of the project is named by its
project-relative path, and a file outside it by its absolute path, which
`kapi apply` in the project resolves the same way. `ksed` resolves the files it
is given the same way, so `--print-ops` prints the change set `ksed` would
apply and `kapi apply` applies it as printed, from any directory of the
project; `kapi apply --print-ops` prints the set as decoded. The format a
person can read is the format an agent sends. The flow verbs (`kapi translate`,
`pseudo-translate`, `run`, `up`, and `kapi exec` for a tool that writes) take
`--print-ops` as well: the run writes no file and records no change, and prints
the change set it would apply, which `kapi apply` applies to the same bytes;
each file it leaves out is named on stderr
([E-09](../engine/e-09-the-change-contract.md#flows)).

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

`kapi exec <tool>` takes `-p` / `--project` like every other project-aware
command, and its progress bar is `--progress` with no shorthand. Naming a
project changes what governs the run (the recipe's tool presets, term rules
and stores); the files named on the command line stay the files named, each
read by the format its extension calls for.

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
| 3 | `ExitGate` | a quality or voice gate was not met, or `kapi apply` refused a change set (nothing was written) |
| 4 | `ExitNotRun` | a check reached no verdict: it checked no content, or an analyzer missed its canary |
| 5 | `ExitUnreachable` | the context backend `kapi context sync` shares through, or a change set's backend, could not be reached; nothing changed on either side |
| 130 | `ExitSignal` | interrupted (128 + SIGINT) |

A draft that scores below its threshold is not a crash, and a script that
cannot tell the two apart has to choose between ignoring real failures and
treating every low score as one. A check over nothing is neither: it is not a
pass, and it has no findings to fix. `host.ErrQualityGate` maps to `ExitGate`
and `host.ErrCheckNotRun` to `ExitNotRun`; a pull or push that cannot reach its
backend tags its error with `ExitUnreachable`, so a CI job can retry it; a
command can request any other code by tagging its error with
`host.WithExitCode`. `host.ErrSilentExit` requests a non-zero exit with the
message suppressed, which is how the toolbox reports "no match" as a status
rather than as an error line ([S-04](s-04-toolbox.md)).

Under `--json`, a failure is a structured envelope rather than a prose line, with
the code symbol mirroring the exit code:

```json
{
  "error": "failed to connect to server",
  "code": "unreachable"
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

### `kapi init` proposes, and wires the agents

`kapi init` creates a project without interactive prompts. It proposes content
collections from existing files and records the matches in recipe comments
([C-01](../context/c-01-project-model.md)). It also writes agent configuration:

- an MCP server entry that starts `kapi mcp --project kapi.yaml`, with
  `--tools writing,translation` when the recipe declares target languages, in
  each host's own configuration file (`.mcp.json`, `.cursor/mcp.json`,
  `.vscode/mcp.json`, `.codex/config.toml`);
- a short `SKILL.md` describing context retrieval, notes about what the
  project does and what a person changed, and checks, with CLI and MCP equivalents and session-reporting guidance.

Rerunning initialization preserves existing collections and reports uncovered
content. It updates generated MCP entries to match the recipe while preserving
custom entries. Obsolete generated skill files are identified by content and
removed; user-authored files remain intact.

### `kapi help <topic>` serves the guidance

The skill stays short because the binary carries the rest. `kapi help` prints
the command overview and then the topics; `kapi help <topic>` prints one,
served from the embedded skill references (`cli/skills`), with links between
references rewritten as the `kapi help` command that serves each. An agent
therefore reads the guidance of the binary it is running. A topic that shares a
name with a command (`check`, `context`, `translate`, `voice`) prints the guide
and names the command's own `--help`.

### Agent and toolbox surfaces on the same binary

`kapi mcp` starts an MCP server over stdio, serving the tool sets `--tools`
names (`writing`, with the `context://` resources, when it names none);
`--all-tools` and `--all-flows` add tools no set holds, for debugging, and
`--all` serves every set and both. `kapi hook` provides the
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
- [S-07: The review model](s-07-context-centric-review.md): what `kapi status --review` renders
- [WASM Engine ABI](/contribute/implementation/surfaces/wasm-engine-abi): the browser build's command set and its recorded gaps
- [Command reference](/reference/commands/up): the generated, per-command surface
- [Scripting & JSON contract](/reference/cli-contract): structured results, progress events, exit codes
