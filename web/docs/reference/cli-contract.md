---
sidebar_position: 6
title: Scripting & JSON contract
description: "The machine-readable CLI contract: structured --json results for run, extract, and merge, the JSON error envelope, exit codes, NDJSON progress events, and the stable MCP tool surface."
keywords: [kapi json, scripting, jq, exit codes, NDJSON, progress events, automation, CI]
---

# Scripting & JSON contract

kapi's core verbs speak a documented, golden-tested machine contract so scripts, CI pipelines, and foreign-language callers never have to parse prose. This page covers the output flags, the structured results of `run`, `extract`, and `merge`, the JSON error envelope, exit codes, and streaming progress events. For driving the engine over gRPC instead of the CLI, see the [Engine service](/reference/engine-service); for the AI-agent surface, see the [MCP server](/reference/mcp).

Compatibility: the JSON documents below are a stable contract. Fields may be added in a release; existing field names and types do not change. The shapes are locked by golden tests (`cli/contract_golden_test.go`).

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
      "tm_new": 6,
      "tm_updated": 2
    }
  ],
  "applied": 8,
  "stale": 1,
  "skipped": 0,
  "tm_new": 6,
  "tm_updated": 2,
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
| 3 | `gate` | A quality or voice gate failed (e.g. `kapi voice check --min-score`), distinct from an operational error so CI can tell "the content isn't good enough" from "the tool broke" |
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

For block-level content streaming (rather than run progress), `kapi inspect --jsonl` emits one JSON object per block; run `kapi inspect --help` for the block fields.

## MCP surface stability

The [MCP server](/reference/mcp) (`kapi mcp`) is part of the same contract: its tool names and input schemas are a stable surface for agent integrations, locked by a snapshot test (`kapi/cmd/kapi/mcp_snapshot_test.go`). New tools and new optional fields may be added; existing tools are not renamed or removed, and existing fields do not change type, without an explicit, documented decision.

The registry tools on that surface are exactly the CLI-visible ones: a built-in tool appears under `kapi exec`, in `kapi tools list`, and as an MCP tool when it registers a config factory and does not declare itself internal (`registry.ToolRegistry.CLITools`). Wiring a factory for a tool that lacked one is therefore an additive surface change: it adds the tool to all three at once, and the snapshot moves. `whitespace-correct` gained one this way, and `dnt-check`, `placeholder-check`, `xml-validation`, `create-target`, `remove-target`, `inline-codes-remove` and `external-command` followed.

The `up` tool takes an optional `local` field, mirroring `kapi up --local`: in a project connected to a server the run happens at that venue by default (the same decision the command makes) and `local` keeps the loop on this machine, pushing the results afterwards.

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

Three properties of a built-in tool's registration are asserted over the populated registry, in `core/tools/registration_invariants_test.go`, because each one fails silently when it is left to review:

- **A tool that declares settable schema fields registers a config factory.** Without one, `NewToolWithConfig` calls the zero-arg factory and discards the step's `config:` map without a word, and the tool's documented parameters do nothing.
- **A bilingual tool's target locale comes from the run.** `--target-lang` outranks any locale written into a step's config, so one flow serves every locale it is run for. A factory that pins a locale leaves the tool processing content the run never asked for, and reporting success.
- **A CLI-visible tool that rewrites content declares `writesOutput`.** `kapi exec` grows `-o` / `--output-dir` only for tools that do; without it an exec run rewrites the content in memory, exits 0, and writes nothing.

Withholding a tool from the CLI is a separate decision, declared with `internal: true` on its `ToolMeta` and never expressed by omitting a config factory, which would make a forgotten tool indistinguishable from one deliberately withheld. An internal tool is still configurable: a flow may name it as a step.

Step config keys are the config struct's JSON names, which are **camelCase** (`normalizeSpaces`, `flagExtra`, `textUnitIDs`). Application is a `json.Unmarshal`, so an unrecognized key (a snake_case spelling, or a field the tool does not have) is silently ignored.
