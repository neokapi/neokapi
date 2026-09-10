---
sidebar_position: 2
title: "MCP Tools Reference"
description: Implementation note for S-03. Where the kapi MCP server's tool handlers live, how a tool reaches the server, and what shape its result takes. The tool list itself is generated onto the MCP reference page.
keywords: [MCP tools, kapi mcp, tool handlers, JSON-RPC, MCP server, implementation note, neokapi]
---

# MCP Tools Reference

Implementation detail for
[S-03: Agent surfaces](/contribute/architecture/surfaces/s-03-agent-surfaces),
which decides that MCP is one of the two doors an assistant reaches kapi
through.

**The tool list is not here.** Tool names, descriptions, input schemas, the
surface each belongs to, and the resource addresses the server answers reads at
are generated from a running server onto
[the MCP reference page](/reference/mcp), with a CI drift gate. A second,
hand-maintained copy is how the published list came to document three tools
that no longer existed and omit the ones that did. This note covers what a
generated table cannot: where the handlers live and how a tool gets registered.

## How a tool reaches the server

`kapi mcp` (`cli/mcp.go`) starts an `mcp.Server` over stdio and calls
`host.ApplyMCPToolFactories`. Every factory registered through
`host.RegisterMCPToolFactory` (`host/mcpregistry.go`) is invoked with the
server and the `*host.App`, and adds its tools. Registration happens in
`init()`, so linking a package is what exposes its tools, which is why the
kapi porcelain lives in an importable package rather than in `package main`.

`App.MCPSurface` is set from the `--all-tools` / `--all-flows` / `--all` flags
and never from the environment, because the surface is a property of how the
server was started; the factories read it to decide what to add.
`App.ResolveMCPProject` accepts `-p` / `--project` through the CLI's project
flag convention and otherwise runs the shared upward walk. `KAPI_NO_PROJECT=1`
disables implicit discovery; an explicit recipe still wins. The resolver retains
the exact recipe path and source language before serving tools. Invalid project
loading returns an error. `check_file` and a scoped `check_text` thread that path
into their synthetic host commands, so profile and terms resolution uses the
bound project even when implicit discovery is disabled. Custom recipe filenames
remain intact.

The `context://` resources and `context_search` also use the server's bound
recipe. Resource locations resolve relative to that recipe's root, including
when the server runs from a subdirectory or outside the project. Explicit
standalone store inputs on `context_search` retain their override semantics.

`check_text.context_path` names a project-relative destination, including a file
that has not been written. It resolves the destination's voice channel and terms
without extracting a file. The path requires a bound project and cannot be
combined with `profile_file` or `profile_pack`. Unscoped snippets retain those
explicit profile options. Invalid paths and context-resolution errors fail the
operation instead of dropping the requested scope.

A scoped draft report keeps `target.kind: "text"` and records the destination in
`target.context_path`. Its findings and analyzer entries have no file location:
they describe the supplied snippet. `check_file` remains the post-save check for
document extraction, structure and block locations.

For project-scoped `check_file`, omit `profile_file` and `profile_pack` so the
file's voice channel resolves from the project. Either explicit option replaces
that voice selection; loading the project's profile YAML directly does not
select the file's channel. Project terms still resolve for the file. Record the
arguments when assessing check coverage, since a tool name alone does not
establish which voice governed the check.

The canonical CLI/MCP check report includes optional `execution.analyzers` and
phase timings. An absent inventory means unreported coverage. Applicable
semantic guidance has an explicit `unsupported` entry in a deterministic run;
its presence cannot be inferred from the score. Requested analysis or context
that fails returns the existing operation-error transport. See the
[checks documentation](/framework/checks)
for analyzer scope and timing boundaries.

| Handlers | Where |
| --- | --- |
| `context_search`, the `context://` resources | `host/mcp_context.go` |
| `up`, `up_plan` | `host/mcp_up.go` |
| `check_text`, `check_file` | `host/mcp_check.go` |
| `apply_edits` | `host/mcp_edit.go` (change-set kinds in `host/apply.go`) |
| `stats` | `host/mcp_stats.go` |
| `voice_check`, `voice_rewrite` | `host/mcp_voice.go` |
| Curated framework tools (`translate`, `term-check`, `redact`) | `host/mcp_tools.go` |
| `detect_format`, `extract_content`, the listing and flow verbs | `kapi/mcptools/tools.go` |
| The review verbs | `kapi/mcptools/review.go` |

## Curation

`host/mcp_tools.go` projects registry tools into MCP tools by rendering their
`ComponentSchema` as the input schema, plus a required `text` and an optional
`target_lang`. Only the tools in `agentFacingTools` are offered by default.

Most of the unfiltered surface was never authored for an assistant: it arrived
because someone added a pipeline step. Anything with a porcelain equivalent is
deliberately absent too, since two names for one job means the caller picks
wrong half the time, and `recycle` and `diff-leverage` are what `up` does for
you.

`neverAgentFacing` is stricter still: `external-command` and `script` execute
caller-supplied commands and JavaScript, and are withheld even under
`--all-tools`. "Show me every tool" and "let a caller run arbitrary code" are
different decisions, and bundling them would make the first silently grant the
second. `host/mcp_tools_curation_test.go` asserts this.

## Resources as well as tools

`registerContextResources` (`host/mcp_context.go`) registers the two
`context://` addresses as **resource templates**, both with one handler: the URI
itself says which address form was asked for, so dispatch never depends on which
template the SDK matched; both templates match `context://profile/x`, which
would otherwise be a coin toss.

The URI is split by hand rather than through `url.Parse`. Under `context://` the
first path segment would be read as an authority and lowercased, silently
renaming a location on a case-sensitive filesystem.

Surface widening does not apply here: `--all-tools` and `--all-flows` govern
which tools a caller may *run*, not what it may *read*.

## Result shapes

Handlers return typed Go structs, which the SDK serializes into the result's
`structuredContent` and mirrors as JSON text content. The struct is the schema:
read it rather than a prose copy.

| Result | Type |
| --- | --- |
| `extract_content` | `mcptools.ExtractContentOutput` |
| `detect_format` | `mcptools.DetectFormatOutput` |
| `run_flow`, `pseudo_translate` | `mcptools.RunFlowOutput` |
| `list_formats`, `list_flows`, `list_tools` | `mcptools.ListFormatsOutput`, `ListFlowsOutput`, `ListToolsOutput` |
| Review verbs | `mcptools.ReviewQueueOutput`, `ReviewUnitOutput`, `ReviewDecisionOutput` |
| `check_text`, `check_file` | a `kapi.check/v1` Report; see [the JSON contract](/reference/cli-contract) |
| `stats` | the same document `kapi stats --json` emits |
| A curated framework tool | `host.frameworkToolOutput`: target translations, rewritten source, properties, overlays, and annotations for the one processed block |

`apply_edits` reports `ok` alongside the per-block outcome (`applied`,
`skipped`, `stale`, `guard_failed`) and a per-entry asset result. `ok` is false
when an edit drifted or was rejected, which is the caller's signal to re-read
the block and retry rather than to force the write.

A content entry uses `kind: "content"`, `file`, the extracted block `id` and
`content_hash`, and `text` for its new wording. `replacement` belongs to voice
rules. Both the CLI and MCP reject content entries carrying a nonempty
`replacement` before applying any entry, with an error identifying the expected
`text` field.

## The surface is a contract

`kapi/cmd/kapi/mcp_snapshot_test.go` snapshots every tool name and input schema
to `testdata/mcp_tools.golden.json`, and every resource address and mime type to
`testdata/mcp_resources.golden.json`. Descriptions may evolve freely; names,
input schemas and addresses may only be extended. Renaming a tool, removing one,
changing a field's type, or moving an address breaks agent integrations already
in the field and needs an explicit decision: regenerate with
`KAPI_UPDATE_GOLDEN=1` and record it in
[the CLI contract](/reference/cli-contract).
