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
`App.ResolveMCPProject` runs the same git-style upward walk the CLI uses, so a
server started inside a project scopes itself to it.

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

## The surface is a contract

`kapi/cmd/kapi/mcp_snapshot_test.go` snapshots every tool name and input schema
to `testdata/mcp_tools.golden.json`, and every resource address and mime type to
`testdata/mcp_resources.golden.json`. Descriptions may evolve freely; names,
input schemas and addresses may only be extended. Renaming a tool, removing one,
changing a field's type, or moving an address breaks agent integrations already
in the field and needs an explicit decision: regenerate with
`KAPI_UPDATE_GOLDEN=1` and record it in
[the CLI contract](/reference/cli-contract).
