---
sidebar_position: 6
title: Scripting & JSON contract
description: "The machine-readable CLI contract: structured --json results for run, extract, and merge, the JSON error envelope, exit codes, NDJSON progress events, and the stable MCP tool surface."
keywords: [kapi json, scripting, jq, exit codes, NDJSON, progress events, automation, CI]
---

# Scripting & JSON contract

kapi's core verbs speak a documented, golden-tested machine contract so scripts, CI pipelines, and foreign-language callers never have to parse prose. This page covers the output flags, the structured results of `run`, `extract`, and `merge`, the JSON error envelope, exit codes, and streaming progress events. An application that drives kapi instead of a person at a terminal uses one of the channels in [Driving kapi from an application](#driving-kapi-from-an-application); for the AI-agent surface, see the [MCP server](/reference/mcp).

Compatibility: the JSON documents below are a stable contract. Fields may be added in a release; existing field names and types do not change. The shapes are locked by golden tests (`cli/contract_golden_test.go`).

One exception is made before 1.3: the content-memory counts in the `kapi merge` result are named `memory_new` and `memory_updated`, and the exact-match count in the `kapi up --plan` result is named `memoryExact`. Releases before 1.3 wrote them as `tm_new`, `tm_updated` and `tmExact`, and a script reading those names needs updating.

Human-readable text output is **not** a contract. It is presentation: column widths adapt to your terminal, values are truncated to fit, and it carries ANSI styling when stdout is a TTY. It is restyled whenever the CLI's presentation improves. Scripts must use `--json` (or `--jq`), which is stable, unstyled, and never truncated.

## Output flags

Every command accepts the persistent output flags:

| Flag | Effect |
| --- | --- |
| `--json` | Machine-readable JSON on stdout |
| `--text` | Human text (the default) |
| `--output-format <json\|text>` | Explicit format selection |
| `--jq <expr>` | Filter JSON output through a jq expression (implies `--json`) |
| `--color <auto\|always\|never>` | Colorize output. `auto` colorizes only when stdout is a terminal |

Precedence: `--jq` > `--json` > `--text` > `--output-format`.

### Color

Color is off whenever stdout is not a terminal, so piping or redirecting always yields plain text, with no ANSI stripping required. `NO_COLOR` disables color and `CLICOLOR_FORCE` forces it, both overridden by an explicit `--color`.

There is no light/dark setting, because kapi does not have a light and a dark theme. It has one palette, chosen so that every color clears the WCAG contrast bar for UI text on any terminal background: white, black, Solarized, One Dark, Dracula alike. Nothing to configure and nothing to get wrong.

That also means kapi never queries the terminal for its background color. Terminals answer such a query on **stdin**, which kapi's own commands read (`kcat -`), and a terminal that does not answer leaves the raw escape sequence in the output, corrupting piped output, CI logs, and recorded sessions.

## Result documents

With `--json`, the core verbs print one JSON document on stdout when they finish. Without it, they print a human report, which is presentation rather than a contract (see above).

### `kapi run`

A single-file run reports the flow and the paths involved; a batch run reports the file count. An in-project run with no `-o` is process-only ([projects](/kapi/projects)): it commits target overlays to the project store instead of writing a file, and reports `process_only`.

```json
{
  "flow_name": "pseudo-translate",
  "input_path": "src/messages.json",
  "output_path": "src/messages_qps.json"
}
```

```json
{
  "flow_name": "translate",
  "input_path": "src/messages.json",
  "process_only": true
}
```

### `kapi extract`

One document per batch: identity (`batch_id`, `manifest`), the extraction inputs (`format`, `targets`, `sources`), one entry per target-locale pass under `pairs` (file/block counts and content-memory leverage), the aggregate `leverage`, incremental `reused` count, and `failures` when source/target pairs failed (details stream to stderr as they happen).

```json
{
  "batch_id": "0b6be731-3a5c-4a02-9e04-2f79e4c2d1aa",
  "format": "xliff2",
  "targets": ["fr", "de"],
  "sources": 2,
  "manifest": ".kapi/work/cache/extractions/0b6be731/manifest.yaml",
  "pairs": [
    {
      "target_locale": "fr",
      "files": 2,
      "blocks": 10,
      "leverage": { "exact": 4, "fuzzy": 1, "new": 5 }
    }
  ],
  "leverage": { "exact": 4, "fuzzy": 1, "new": 15 }
}
```

### `kapi merge`

Applying returned bilingual files (`merge -i`) reports one entry per input plus totals and the resolved conflict policy; a failed input carries an `error` instead of counts.

```json
{
  "files": [
    {
      "input": "out/app.en-to-fr.xliff",
      "applied": 8,
      "stale": 1,
      "skipped": 0,
      "memory_new": 6,
      "memory_updated": 2
    }
  ],
  "applied": 8,
  "stale": 1,
  "skipped": 0,
  "memory_new": 6,
  "memory_updated": 2,
  "conflict_policy": "translator-wins"
}
```

Materializing from the project store (`kapi merge` with no `-i` in a project) reports the written-file count:

```json
{ "written": 4, "from_project_store": true }
```

## Error envelope

Under `--json` (or `--jq` / `--output-format=json`), a failing command prints a structured envelope on **stderr** instead of the plain `Error:` line:

```json
{ "error": "quality gate failed", "code": "gate" }
```

`code` is the symbolic form of the process exit code (below). Exit codes are unchanged by `--json`; in text mode the error is still reported as an `Error: <message>` line.

## Exit codes

| Code | Symbol | Meaning |
| --- | --- | --- |
| 0 | (none) | Success |
| 1 | `error` | Operational error |
| 2 | `usage` | Usage / invocation error (also grep-style "trouble" for the toolbox utilities) |
| 3 | `gate` | A check failed: at least one finding fails (`kapi check`, `kapi check --ship`, `kapi voice check`), distinct from an operational error so CI can tell "the content isn't good enough" from "the tool broke" |
| 4 | `did_not_run` | `kapi check` reached no verdict: it checked no content, or one of its checks reported nothing on the known-bad sample it runs beside the content. Never a pass |
| 130 | `signal` | Interrupted (SIGINT/SIGTERM); no error line is printed |

The toolbox utilities (`kgrep`) additionally use grep-parity semantics: exit 1 with no message when nothing matched.

## Streaming progress: `--progress jsonl`

`run`, `extract`, and `merge` accept `--progress jsonl`, which streams progress events to **stderr** as NDJSON (one JSON object per line) while stdout stays reserved for the final result. The events use the flow-run event vocabulary (the same shapes the Kapi Desktop run sink receives), with `flow` naming the verb or flow:

| `type` | Meaning | Key fields |
| --- | --- | --- |
| `state` | Run-state transition | `message` |
| `progress` | About to process one file (or source→locale pair) | `file_index`, `file_count`, `file_path`, `locale` |
| `file_done` | One unit completed | `file_path`, `output_path`, `locale` |
| `pipeline_metrics` | Per-step throughput snapshot (multi-locale project runs) | `steps` |
| `complete` | Run finished | `duration_ms`, `files_processed`, `message` |

```bash
kapi extract -p kapi.yaml --progress jsonl 2> >(jq -c 'select(.type=="file_done")')
```

```json
{"type":"progress","flow":"extract","locale":"fr","file_count":4,"file_path":"src/messages.json"}
{"type":"file_done","flow":"extract","locale":"fr","file_path":"src/messages.json","output_path":"out/src-messages.en-to-fr.xliff"}
{"type":"complete","flow":"extract","duration_ms":420,"files_processed":4}
```

## Stream integrity: a truncated NDJSON stream is never silent

Every NDJSON stream kapi writes (`kapi up --json`, `--progress jsonl`) is a
contract with a machine reader, so a stream that stops early must not look like one
that finished. The two ways a write can fail are treated differently, because they
mean opposite things:

| What happened | kapi's behaviour |
| --- | --- |
| **You stopped reading**: `kapi up --json \| head`, a `jq` filter that exits, a watching UI that disconnects | The stream stops, quietly, and the run is not failed. Failing a run because its reader walked away would be worse than the silence. On a shell pipe the process is normally terminated by `SIGPIPE` before the write even returns, giving the conventional `141` exit; either way nothing is printed. |
| **The write failed**: a full disk, a closed file, an unwritable destination | One message on stderr (or the JSON error envelope) naming the stream and counting what got through, and a non-zero exit. A consumer must never believe a truncated stream. |

```console
$ kapi up --json > /mnt/full/out.ndjson
Error: kapi up --json: the event stream truncated: 6 record(s) written, 12 lost: write /dev/stdout: no space left on device
$ echo $?
1
```

The distinction is drawn structurally (`errors.Is` against `syscall.EPIPE` and
`io.ErrClosedPipe`, plus the Windows broken-pipe errnos), never by matching the
message text. Under `--progress jsonl` the exit code is the signal to read:
when the failing writer *is* stderr, the message has nowhere to land.

## Streaming inspection: `kapi inspect --jsonl`

For block-level content streaming (rather than run progress), `kapi inspect --jsonl` emits one JSON object per block, without HTML escaping, so `<x id="…"/>` placeholders read as written. Each record is the read record of the change contract: `ref` (`doc`, `block`, and `edition` for a translation), `rev`, `text`, `codes`, `structures`, `editions` and `ops`, with the block's `role` and `level`; run `kapi inspect --help` for each field. Inside a project (`-p`, or the one discovery finds) `doc` is a file's project-relative path, or the absolute path of a file outside the project, and `kapi apply` in the same project resolves both.

## Change sets: `kapi apply`

`kapi apply` reads a `kapi.change/v1` change set: one JSON object with its operations under `ops`, JSONL with the envelope fields on the first line and one operation per line, or a JSON array of operations. `kapi apply --schema` prints its JSON Schema, generated from the Go types and pinned by a golden file (`core/change/testdata/schema.golden.json`). Decoding is strict: an unknown field or operation is refused with its JSON pointer. The entry shape `kapi apply` read before (`kind`, `file`, `id`, `content_hash`) is refused with a message naming the operation that takes its place.

`--json` prints the result as `kapi.change-result/v1`: the set's `status` (`applied`, `refused`, `previewed` or `partial`), each document's digests before and after, and one result per operation with its `status` (`applied`, `unchanged`, `refused`, `not_applied` or `previewed`), its revisions, and on a refusal an `error` whose `code` is one of a closed set. A `stale` refusal carries the edition's `current` revision and text. `--dry-run` writes nothing and gives each document its `diff`; `--print-ops` prints the change set as decoded and applies nothing.

The exit status follows the result: `0` when the change set applied or previewed; `2` when it does not decode, contradicts itself, or an operation is refused `invalid`; `5` when a backend did not answer (`unreachable`); `3` for every other refusal, for a change set that landed in part, and, under the enforcing gate, for a written code comment whose check fails. A refusal writes nothing. `ksed --print-ops` prints a change set in the same contract, which `kapi apply` applies as printed, from any directory of the project.

The flow commands take `--print-ops` too: `kapi translate`, `kapi pseudo-translate`, `kapi run`, `kapi up` (one pass, on this machine) and `kapi exec <tool>` for a tool that writes files. The run reads and runs its tools as it would, writes no file of the project, records no change and absorbs nothing into the content memory, and prints one change set: for each document, the difference between each block as it was read and as the run would write it, each operation guarded by the revision `kapi inspect` reads and each translation's `set_content` carrying its `basis`. `kapi apply` of that change set writes the bytes the run would have written, and records the edit as the applier's. A run that would change nothing prints a change set with no operation, which `kapi apply` refuses as `invalid`. Every file the change set leaves out is named on stderr: a target file `kapi apply` would write other bytes into than the run (the run writes it whole from its source, so a block the source gained or an entry only the target holds), a file the project does not keep the edition in (an `-o` path), a conversion, an export, an archive, and the files of a locale `kapi up` would park at its ship gate. In a project, `kapi translate`, `kapi pseudo-translate` and `kapi run` without `-o` write no file and print an empty change set. `kapi exec` over a file of the project the command resolves names it as `kapi apply` in that project does.

Every flow command writes a file only while it still holds what it held when the run began. A file that changed meanwhile keeps its bytes; inside a project the run applies its own changes to it again, and when an edition the run changed has moved too the file is left as it is and the command fails, naming it.

## Driving kapi from an application

An application reads and changes content through one of four channels. Each carries the change contract ([E-09](/contribute/architecture/engine/e-09-the-change-contract)): a read gives every block its `ref` and `rev`, and a write is a `kapi.change/v1` change set, held to the same rules whichever channel sends it.

| Channel | For | How |
| --- | --- | --- |
| The Go library | a Go program | `host.App.ChangeService` builds the change service for a project, with the recipe's formats, its governance and its history; `core/change` with `core/change/filehome` builds one over a directory. The service has `Read`, `Apply` and `Describe`. |
| `kapi apply -` | a program in any language that can start a process | write a change set to standard input; with `--json` the `kapi.change-result/v1` result arrives on standard output, and the exit status follows [Change sets](#change-sets-kapi-apply). `kapi inspect --jsonl` is the matching read. |
| MCP over standard input and output | an agent host, or any MCP client | `kapi mcp` serves `read_blocks`, `apply_edits`, `describe_format` and the rest of the [MCP tools](/reference/mcp). |
| The browser build | a web page | `@neokapi/engine` loads the WebAssembly build of kapi and calls the functions of its [engine ABI](/contribute/implementation/surfaces/wasm-engine-abi). |

## MCP surface stability

The [MCP server](/reference/mcp) (`kapi mcp`) is part of the same contract: its tool names and input schemas are a stable surface for agent integrations, locked by a snapshot test (`kapi/cmd/kapi/mcp_snapshot_test.go`). New tools and new optional fields may be added; existing tools are not renamed or removed, and existing fields do not change type, without an explicit, documented decision.

`check_text` accepts an optional `context_path`, a project-relative destination
whose voice and terms apply to the supplied draft. It requires a bound project
and cannot be combined with `profile_file` or `profile_pack`. The result keeps
`target.kind: "text"` and records the destination in `target.context_path`;
the destination need not exist. Unscoped snippet checks retain their explicit
profile options. See [Checks](/framework/checks) for report coverage.

The check report schema is `kapi.check/v2`. Its `summary` carries `findings`,
`failing`, `reporting` and `score`. Each finding carries `fails` (a boolean the
rule that raised it sets) and, when a suggested rule raised it,
`suggested: true`. Findings sort failing first, then by rule. The verdict fails
when at least one finding fails; the score is informational and does not determine the verdict.
`kapi check --ship` findings carry the same `fails`, and its summary counts
`failing` and `reporting`.

The optional `execution.contexts` array adds per-input guidance selection to
`kapi.check/v2` without changing the other report fields. Entries contain `file`
or `context_path`, `voice` (`selection`, `applied`, optional `name`, `source`,
`profile` and `channel`), `terms_applied`, and, when a project resolved the
guidance, `point` (`profile`, `channel`, and `comments` for the point a file's
comments sit at). Findings, including those of `kapi check --ship`, carry the
same optional `point`. `voice.selection` is `project`,
`override` or `none`. Missing context metadata means unreported selection.
CLI file checks, MCP file/draft checks and source-content release checks supply
it. Explicit voice overrides retain their behavior and are identified as
`override`; MCP field descriptions state that omitting overrides preserves
file-scoped project guidance. See [Checks](/framework/checks) for interpretation.

The optional `evaluation` object is additive to `kapi.check/v2` and says what
the run was evaluated against. It contains `at` (RFC 3339 in UTC), `tool`
(`name`, `version`, `commit`) and `analyzers`, an entry per analyzer with `id`,
`ran` and, for the inputs it produced no usable result for, `not_run` entries
carrying `status`, `reason` and `inputs`. A run inside a project also carries
`context` (`project`, `name`, `revision`, `stale`, `stale_reason`), the same
shape the MCP context replies carry as `provenance`, and a run a plugin served
carries `plugins` (`name`, `version`, `serves`). `Decide` never reads the
record, so it leaves `pass`, `verdict`, `summary` and the exit code as
they are, and the human output carries none of it. Every producer of a
`kapi.check/v2` Report supplies it: `kapi check` whole or scoped to a diff, and
the MCP `check_file` and `check_text` tools. `kapi check --ship` reports gates rather
than a Report and carries no evaluation record, and neither does the findings
roll-up of `kapi exec <check>`. See [Checks](/framework/checks) for the fields.

The optional `warnings` array is also additive to `kapi.check/v2`, and the
`kapi check --ship` result carries the same array at its top level. Each entry
contains `code`, `message`, `source` and, when the warning concerns one key,
`key`. The array is omitted when a run has none. Warnings describe the
configuration a check ran under, and they never change `pass`, `verdict`,
`summary` or the exit code. The MCP `check_text` and `check_file` tools
return them in the same report, and the `check_report_warnings` golden in
`cli/contract_golden_test.go` pins the shape. See [Checks](/framework/checks)
for the codes.

The edit tools carry the change contract
([E-09](/contribute/architecture/engine/e-09-the-change-contract)), and their
names and schemas are a documented break of this surface. `read_blocks` reads
a document's blocks with the `ref` and `rev` each operation names, and
`review_block` reads one block's review picture with the `ref` and `rev` of the
edition under review; together they cover what `extract_content` and
`review_unit` read. `apply_edits` takes a `kapi.change/v1` change set: its input
schema is the change-set schema, which the snapshot records in full, with the
per-call `project` added. Its result is a `kapi.change-result/v1` document, an
error result when the change set is refused or lands only in part.
`describe_format` reports what each operation supports in a format. A
pre-review is a `decide` operation with outcome `advise` sent through
`apply_edits`, where `pre_review_unit` recorded one. `review_queue` rows carry
the `ref` that `review_block` takes.

A person sends the same change sets with `kapi apply` (see
[Change sets](#change-sets-kapi-apply)). A change set takes an optional
`evidence` field. A term or a content-memory pair is a decision about the
project: applying one records one `edit` operation in that project's history,
established from the start, and `evidence` says where the wording behind it was
seen: a file (`path`, `unit`, `quote`) or a web page (`url`). A change set
takes no actor: `apply_edits` sends every change set as the calling agent in
the server's session, so the context policy refuses its term, content-memory
and recipe operations, and `kapi apply` records the person or agent its
environment names. See
[Growing context](/kapi/context-decisions).

Four tools record and read a project's context history: `context_observe`,
`context_correct`, `context_withdraw` and `context_session_summary`. Each wraps
one call in the host's context-operations API and takes the same optional
`project` as every other project-scoped tool. `context_read` returns exactly
the text the `context://<path>` resource returns, for a client that lists no
resources from a template.

None of them takes an actor. The kind is `agent`, the name is the client's own
`initialize` name, and the session is minted once per server process, so an
argument naming an actor or a session is refused by the schema. Evidence is
required where a rule is stated: `context_observe` refuses a term rule with no
`path`, and `context_correct` declares `path` as required and refuses a blank
one. A recorded operation is a suggestion, so a check reports it with
`"fails": false` and `"suggested": true`. The record carries
`contested_by` when it disagrees with another rule.

The MCP tool set excludes keeping, dropping, reverting and widening. The
context policy reserves those operations for people. `context_withdraw` takes back only
what the same session recorded. A by-location context answer and a
`context_search` answer carry a `suggestions` array, each entry with `status`
`suggested` or `contested`, `contested_by` and `suggested_by`. See
[Growing context](/kapi/context-decisions).

The registry tools on that surface are exactly the CLI-visible ones: a built-in tool appears under `kapi exec`, in `kapi tools list`, and as an MCP tool when it registers a config factory and does not declare itself internal (`registry.ToolRegistry.CLITools`). Wiring a factory for a tool that lacked one is therefore an additive surface change: it adds the tool to all three at once, and the snapshot moves. `whitespace-correct` gained one this way, and `dnt-check`, `placeholder-check`, `xml-validation`, `create-target`, `remove-target`, `inline-codes-remove` and `external-command` followed.

The `up` tool takes an optional `local` field, mirroring `kapi up --local`: in a project connected to a server the run happens at that venue by default (the same decision the command makes) and `local` keeps the loop on this machine, pushing the results afterwards.

`review_queue` takes an optional `language` field, mirroring `kapi status --review --lang`: the queue holds every language the project has work in, so `language` narrows it to one, the project's source language included. The rows carry `language`, `isSource`, `status` and `held`, and the result carries `languages` with the pending count per language.

## Running commands a recipe names

`external-command` and `script` run code the configuration chooses. They stay
available on every surface where the argv is the user's own: `kapi exec
external-command --command …` runs what it was told to, unchanged.

What a **recipe** does with them is gated. A project whose recipe names either
tool prompts once, showing the command it would run, and the answer is
remembered under the kapi config directory against a fingerprint of what was
approved, so an unrelated recipe edit keeps the approval and a changed command
asks again. With no terminal attached kapi refuses rather than assuming
consent; `KAPI_TRUST_EXEC=1` is the opt-in for automation, and the general
`--yes` flag deliberately does not grant it. The engine gRPC API and the MCP
tool surface refuse these tools outright. See
[E-06](/contribute/architecture/engine/e-06-execution-trust).

## Tool registration invariants

Four properties of a built-in tool's registration are asserted over the populated registry, in `core/tools/registration_invariants_test.go`, because each one fails silently when it is left to review:

- **A tool that declares settable schema fields registers a config factory.** Without one, `NewToolWithConfig` calls the zero-arg factory and discards the step's `config:` map without a word, and the tool's documented parameters do nothing.
- **A bilingual tool's target locale comes from the run.** `--target-lang` outranks any locale written into a step's config, so one flow serves every locale it is run for. A factory that pins a locale leaves the tool processing content the run never asked for, and reporting success.
- **A CLI-visible tool that rewrites content declares `writesOutput`.** `kapi exec` grows `-o` / `--output-dir` only for tools that do; without it an exec run rewrites the content in memory, exits 0, and writes nothing.
- **A monolingual tool declares the target language it reads, and only that.** `kapi exec` offers `--target-lang` to a bilingual tool and to a monolingual one whose `ToolMeta` accepts or requires `target-language`. A monolingual tool whose config factory reads the run's target language without declaring it has a target scope nobody can reach from the command line; one that declares it without reading it offers a flag that does nothing.

Withholding a tool from the CLI is a separate decision, declared with `internal: true` on its `ToolMeta` and never expressed by omitting a config factory, which would make a forgotten tool indistinguishable from one deliberately withheld. An internal tool is still configurable: a flow may name it as a step.

Step config keys are the config struct's JSON names, which are **camelCase** (`normalizeSpaces`, `flagExtra`, `textUnitIDs`). Application is a `json.Unmarshal`, so an unrecognized key (a snake_case spelling, or a field the tool does not have) is silently ignored.
