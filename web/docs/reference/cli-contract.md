---
sidebar_position: 6
title: Scripting & JSON contract
description: The machine-readable CLI contract — structured --json results for run, extract, and merge, the JSON error envelope, exit codes, NDJSON progress events, and the stable MCP tool surface.
keywords: [kapi json, scripting, jq, exit codes, NDJSON, progress events, automation, CI]
---

# Scripting & JSON contract

kapi's core verbs speak a documented, golden-tested machine contract so scripts, CI pipelines, and foreign-language callers never have to parse prose. This page covers the output flags, the structured results of `run`, `extract`, and `merge`, the JSON error envelope, exit codes, and streaming progress events. For driving the engine over gRPC instead of the CLI, see the [Engine service](/reference/engine-service); for the AI-agent surface, see the [MCP server](/reference/mcp).

Compatibility: the JSON documents below are a stable contract. Fields may be added in a release; existing field names and types do not change. The shapes are locked by golden tests (`cli/contract_golden_test.go`).

Human-readable text output is **not** a contract. It is presentation — column widths adapt to your terminal, values are truncated to fit, and it carries ANSI styling when stdout is a TTY. It is restyled whenever the CLI's presentation improves. Scripts must use `--json` (or `--jq`), which is stable, unstyled, and never truncated.

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

Color is off whenever stdout is not a terminal, so piping or redirecting always yields plain text — no ANSI stripping required. `NO_COLOR` disables color and `CLICOLOR_FORCE` forces it, both overridden by an explicit `--color`.

There is no light/dark setting, because kapi does not have a light and a dark theme. It has one palette, chosen so that every color clears the WCAG contrast bar for UI text on any terminal background — white, black, Solarized, One Dark, Dracula alike. Nothing to configure and nothing to get wrong.

That also means kapi never queries the terminal for its background color. Terminals answer such a query on **stdin**, which kapi's own commands read (`kcat -`), and a terminal that does not answer leaves the raw escape sequence in the output — corrupting piped output, CI logs, and recorded sessions.

## Result documents

With `--json`, the core verbs print one JSON document on stdout when they finish. Without it, they print a human report — presentation, not a contract (see above).

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

One document per batch: identity (`batch_id`, `manifest`), the extraction inputs (`format`, `targets`, `sources`), one entry per target-locale pass under `pairs` (file/block counts and TM leverage), the aggregate `leverage`, incremental `reused` count, and `failures` when source/target pairs failed (details stream to stderr as they happen).

```json
{
  "batch_id": "0b6be731-3a5c-4a02-9e04-2f79e4c2d1aa",
  "format": "xliff2",
  "targets": ["fr", "de"],
  "sources": 2,
  "manifest": ".kapi/cache/extractions/0b6be731/manifest.yaml",
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
  "conflict_policy": "prefer-incoming"
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
| 0 | — | Success |
| 1 | `error` | Operational error |
| 2 | `usage` | Usage / invocation error (also grep-style "trouble" for the toolbox utilities) |
| 3 | `gate` | A quality or brand gate failed (e.g. `kapi brand check --min-score`) — distinct from an operational error so CI can tell "the content isn't good enough" from "the tool broke" |
| 130 | `signal` | Interrupted (SIGINT/SIGTERM); no error line is printed |

The toolbox utilities (`kgrep`) additionally use grep-parity semantics: exit 1 with no message when nothing matched.

## Streaming progress: `--progress jsonl`

`run`, `extract`, and `merge` accept `--progress jsonl`, which streams progress events to **stderr** as NDJSON — one JSON object per line — while stdout stays reserved for the final result. The events use the flow-run event vocabulary (the same shapes the Kapi Desktop run sink receives), with `flow` naming the verb or flow:

| `type` | Meaning | Key fields |
| --- | --- | --- |
| `state` | Run-state transition | `message` |
| `progress` | About to process one file (or source→locale pair) | `file_index`, `file_count`, `file_path`, `locale` |
| `file_done` | One unit completed | `file_path`, `output_path`, `locale` |
| `pipeline_metrics` | Per-step throughput snapshot (multi-locale project runs) | `steps` |
| `complete` | Run finished | `duration_ms`, `files_processed`, `message` |

```bash
kapi extract -p app.kapi --progress jsonl 2> >(jq -c 'select(.type=="file_done")')
```

```json
{"type":"progress","flow":"extract","locale":"fr","file_count":4,"file_path":"src/messages.json"}
{"type":"file_done","flow":"extract","locale":"fr","file_path":"src/messages.json","output_path":"out/src-messages.en-to-fr.xliff"}
{"type":"complete","flow":"extract","duration_ms":420,"files_processed":4}
```

## Streaming inspection: `kapi inspect --jsonl`

For block-level content streaming (rather than run progress), `kapi inspect --jsonl` emits one JSON object per block — run `kapi inspect --help` for the block fields.

## MCP surface stability

The [MCP server](/reference/mcp) (`kapi mcp`) is part of the same contract: its tool names and input schemas are a stable surface for agent integrations, locked by a snapshot test (`kapi/cmd/kapi/mcp_snapshot_test.go`). New tools and new optional fields may be added; existing tools are not renamed or removed, and existing fields do not change type, without an explicit, documented decision.
