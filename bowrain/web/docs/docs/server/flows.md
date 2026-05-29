---
title: Server-Side Flows
sidebar_position: 12
---

# Server-Side Flows

Flows on Bowrain Server execute the same processing pipelines available in the CLI, but triggered by events, automations, or manual API calls. Server-side flows run in the background, process content from the ContentStore, and write results back — no local CLI needed.

## How Server-Side Flows Work

When a flow runs on the server, it follows this lifecycle:

```
Event (e.g., connector.push.completed)
  → Automation Rule matches
    → Flow Execution starts
      → Read blocks from ContentStore
      → Process through tool pipeline
      → Write results back to ContentStore
      → Emit flow.completed or flow.failed event
```

Unlike CLI flows that read/write local files, server-side flows operate on the ContentStore — the server's persistent block storage. This means flows can run without any CLI connected.

## Triggering Flows

### Via Automation Rules

The most common way to run server-side flows is through automation rules. See [Automation](/server/automation) for details on creating rules.

Example: automatically translate content when it arrives from a connector push:

```json
{
  "name": "auto-translate",
  "trigger": "connector.push.completed",
  "action": {
    "type": "run_flow",
    "config": {
      "flow": "ai-translate",
      "target_locales": ["fr-FR", "de-DE", "ja-JP"]
    }
  }
}
```

### Via API

Trigger a flow manually through the REST API:

```bash
# Start a flow execution
curl -X POST https://bowrain.example.com/api/v1/projects/:id/flows/run \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "flow": "ai-translate",
    "config": {
      "target_locales": ["fr-FR"],
      "provider": "anthropic",
      "model": "claude-sonnet-4.5"
    }
  }'

# Response:
# {
#   "execution_id": "exec-abc123",
#   "status": "running",
#   "flow": "ai-translate",
#   "started_at": "2026-03-09T14:30:00Z"
# }
```

### Via Web UI

The Bowrain web app exposes a **superset flow + automation editor** under
**Project > Automations**, with three tabs:

1. **Runs** — automation run history (see [Automation](/server/automation)).
2. **Rules** — trigger, CEL conditions, and ordered actions. A `run_flow`
   action selects a flow from the project's flow registry.
3. **Flows** — the flow canvas. This is the same `@neokapi/flow-editor`
   component the desktop apps render, wired to the flow-definition REST API.
   It lists the built-in flows alongside the project's stored flows and edits
   the latter on a reader → tool(s) → writer graph (with a source-transform
   stage for redaction/normalization tools).

Flows are **connector-agnostic**: a flow runs server-side on content from any
connector. The graph never names a connector — Kapi is one content source
among many.

## Monitoring Executions

### Polling Status

```bash
# Check execution status
curl https://bowrain.example.com/api/v1/projects/:id/flows/executions/:execId \
  -H "Authorization: Bearer $TOKEN"

# Response:
# {
#   "execution_id": "exec-abc123",
#   "status": "completed",
#   "flow": "ai-translate",
#   "started_at": "2026-03-09T14:30:00Z",
#   "completed_at": "2026-03-09T14:30:42Z",
#   "blocks_processed": 47,
#   "summary": {
#     "translated": 45,
#     "skipped": 2
#   }
# }
```

### Execution States

| Status      | Description                  |
| ----------- | ---------------------------- |
| `pending`   | Queued, waiting to start     |
| `running`   | Currently processing blocks  |
| `completed` | Finished successfully        |
| `failed`    | Stopped due to an error      |
| `cancelled` | Cancelled by user or timeout |

### Listing Executions

```bash
# List recent executions for a project
curl https://bowrain.example.com/api/v1/projects/:id/flows/executions \
  -H "Authorization: Bearer $TOKEN"

# Filter by flow name
curl "https://bowrain.example.com/api/v1/projects/:id/flows/executions?flow=ai-translate" \
  -H "Authorization: Bearer $TOKEN"

# Filter by status
curl "https://bowrain.example.com/api/v1/projects/:id/flows/executions?status=failed" \
  -H "Authorization: Bearer $TOKEN"
```

## Event Flow Diagram

A typical end-to-end flow involving CLI push, server-side translation, and CLI pull:

```
Developer                    Bowrain Server                      Translator
    |                              |                                 |
    |  kapi push                |                                 |
    |----------------------------->|                                 |
    |                              | Event: connector.push.completed |
    |                              |                                 |
    |                              | Automation: run ai-translate    |
    |                              |   → Read blocks from store      |
    |                              |   → AI translation pipeline     |
    |                              |   → Write translations to store |
    |                              |                                 |
    |                              | Event: flow.completed           |
    |                              |                                 |
    |                              | Automation: run qa-check        |
    |                              |   → QA validation pipeline      |
    |                              |   → Write QA annotations        |
    |                              |                                 |
    |                              | Event: flow.completed           |
    |                              |   → quality.gate.pass           |
    |                              |                                 |
    |  kapi pull                |                                 |
    |<-----------------------------|                                 |
    |                              |                                 |
    |                              |  Open editor, review, approve   |
    |                              |<--------------------------------|
```

## Managing Flow Definitions

Project flow definitions are stored server-side and exposed through a
project-scoped REST API. The flow editor in the web app (and the desktop apps)
drives these endpoints; automations reference a flow by its `id`.

| Method   | Path                            | Description                                  |
| -------- | ------------------------------- | -------------------------------------------- |
| `GET`    | `/api/v1/:ws/:id/flows`         | List flows (built-in catalog + project flows) |
| `GET`    | `/api/v1/:ws/:id/flows/:flowId` | Get one flow (built-in or project)           |
| `POST`   | `/api/v1/:ws/:id/flows`         | Create a project flow                        |
| `PUT`    | `/api/v1/:ws/:id/flows/:flowId` | Replace a project flow                       |
| `DELETE` | `/api/v1/:ws/:id/flows/:flowId` | Delete a project flow                        |

Built-in flows are read-only — the server rejects writes to a flow whose `id`
collides with a built-in. Mutations require the `manage_automation` permission.

A flow definition is a node/edge graph (reader → tool(s) → writer) rather than
a flat step list, so it round-trips losslessly through the visual editor:

```bash
curl -X POST https://bowrain.example.com/api/v1/acme/proj-1/flows \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "translate-with-qa",
    "description": "Translate then run quality checks",
    "nodes": [
      {"id": "reader", "type": "reader", "name": "auto", "position": {"x": 0, "y": 0}},
      {"id": "ai-translate", "type": "tool", "name": "ai-translate", "position": {"x": 250, "y": 0}},
      {"id": "qa-check", "type": "tool", "name": "qa-check", "position": {"x": 500, "y": 0}},
      {"id": "writer", "type": "writer", "name": "auto", "position": {"x": 750, "y": 0}}
    ],
    "edges": [
      {"id": "e1", "source": "reader", "target": "ai-translate"},
      {"id": "e2", "source": "ai-translate", "target": "qa-check"},
      {"id": "e3", "source": "qa-check", "target": "writer"}
    ]
  }'
```

Project flow definitions can also be pushed as part of a project's
`.kapi/flows/` directory and authored with the kapi CLI.

## Flow Chaining

Server-side flows can be chained through automation rules. When one flow completes, its `flow.completed` event can trigger another:

```
connector.push.completed → ai-translate → flow.completed → qa-check → flow.completed → quality.gate.pass
```

This is managed automatically by the automation system. Loop prevention ensures chains cannot exceed 5 levels deep (see [Automation > Loop Prevention](/server/automation#loop-prevention)).

## Related

- [Automation](/server/automation) — event-driven automation rules
- [Translation Flows (CLI)](/cli/flows/overview) — local flow execution
- [kapi sync](/cli/commands/sync) — push + wait + pull in one command
