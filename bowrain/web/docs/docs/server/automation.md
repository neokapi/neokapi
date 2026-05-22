---
title: Automation
sidebar_position: 11
---

# Automation

Bowrain provides two complementary automation systems: **local automation rules** declared on the project recipe (`<dir-name>.kapi`) for CLI-driven workflows, and **server-side automation** configured via the web UI or REST API for event-driven processing.

## Local Automation (CLI)

Local automations run in the Bowrain CLI and are declared at the top level of your project's `.kapi` recipe. They hook into CLI commands and execute actions before or after operations like push, pull, and flow runs.

### Configuration

Add an `automations:` section to your `<dir-name>.kapi` recipe:

```yaml
automations:
  - name: qa-before-push
    trigger: pre-push
    actions:
      - type: run_flow
        config:
          flow: qa-check

  - name: sync-on-push
    trigger: post-push
    actions:
      - type: wait_translate
        config:
          timeout: 5m
      - type: pull
```

### Local Trigger Types

| Trigger     | Fires When                                           |
| ----------- | ---------------------------------------------------- |
| `pre-push`  | Before `kapi push` sends blocks to the server     |
| `post-push` | After `kapi push` completes successfully          |
| `pre-pull`  | Before `kapi pull` fetches blocks from the server |
| `post-pull` | After `kapi pull` writes files locally            |
| `pre-flow`  | Before `kapi run` executes a flow                 |
| `post-flow` | After `kapi run` completes                        |

### Local Action Types

| Action           | Description                                                               |
| ---------------- | ------------------------------------------------------------------------- |
| `run_flow`       | Execute a flow by name (inline on the recipe, from `.kapi/flows/`, or built-in) |
| `wait_translate` | Wait for server-side translations to complete (with configurable timeout) |
| `pull`           | Pull translated content from the server                                   |
| `push`           | Push local content to the server                                          |

### Example: QA Gate Before Push

Prevent pushing content that fails quality checks:

```yaml
automations:
  - name: qa-gate
    trigger: pre-push
    actions:
      - type: run_flow
        config:
          flow: qa-check
          fail_on_error: true
```

If `qa-check` finds issues and `fail_on_error` is `true`, the push is aborted.

### Example: Full Sync After Push

Automatically wait for translations and pull them back after every push:

```yaml
automations:
  - name: auto-sync
    trigger: post-push
    actions:
      - type: wait_translate
        config:
          timeout: 10m
      - type: pull
```

This effectively makes `kapi push` behave like `kapi sync`.

## Server-Side Automation

Server-side automations are event-driven rules that run on Bowrain Server. They respond to system events and trigger actions like running flows, sending webhooks, or evaluating quality gates.

### Event Types

The server emits events for all major operations:

| Event                      | Description                                          |
| -------------------------- | ---------------------------------------------------- |
| `connector.push.completed` | Content pushed to the server (from CLI or connector) |
| `connector.pull.completed` | Content pulled from an external source               |
| `connector.sync.completed` | Bidirectional sync completed                         |
| `project.created`          | New project created                                  |
| `project.updated`          | Project configuration changed                        |
| `project.deleted`          | Project deleted                                      |
| `block.created`            | New translatable block added                         |
| `block.updated`            | Existing block content changed                       |
| `block.deleted`            | Block removed                                        |
| `version.created`          | New content version created                          |
| `flow.started`             | Flow execution began                                 |
| `flow.completed`           | Flow execution finished successfully                 |
| `flow.failed`              | Flow execution failed                                |
| `quality.gate.pass`        | Quality gate evaluation passed                       |
| `quality.gate.fail`        | Quality gate evaluation failed                       |

### Creating Rules via Web UI

The Bowrain web UI includes a visual rule editor for creating and managing automation rules:

1. Navigate to **Project Settings > Automation**
2. Click **New Rule**
3. Select a trigger event (e.g., `connector.push.completed`)
4. Add optional conditions (filter by project, locale, or event data)
5. Choose an action (run flow, send webhook, evaluate quality gate)
6. Save and enable the rule

Rules can be reordered, disabled, duplicated, and tested from the editor.

### Creating Rules via API

```bash
# Create an automation rule
curl -X POST https://bowrain.example.com/api/v1/automations \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "auto-translate-on-push",
    "project_id": "my-project",
    "trigger": "connector.push.completed",
    "conditions": [
      {"field": "project_id", "operator": "equals", "value": "my-project"}
    ],
    "action": {
      "type": "run_flow",
      "config": {"flow": "ai-translate"}
    },
    "enabled": true
  }'

# List automation rules
curl https://bowrain.example.com/api/v1/automations \
  -H "Authorization: Bearer $TOKEN"

# Update a rule
curl -X PUT https://bowrain.example.com/api/v1/automations/:id \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"enabled": false}'

# Delete a rule
curl -X DELETE https://bowrain.example.com/api/v1/automations/:id \
  -H "Authorization: Bearer $TOKEN"
```

### Conditions

Conditions filter which events trigger the rule:

| Operator   | Description                          |
| ---------- | ------------------------------------ |
| `equals`   | Exact match on event field value     |
| `contains` | Substring match on event field value |
| `exists`   | Field is present in event data       |

Multiple conditions are combined with AND logic — all must match for the rule to fire.

## Quality Gates

Quality gates evaluate content quality and can block or warn about issues. They integrate with both local and server-side automation.

### Server-Side Quality Gates

Configure quality gates in the web UI or via API:

```json
{
  "name": "translation-coverage",
  "type": "blocking",
  "threshold": 0.9,
  "check": "coverage"
}
```

```json
{
  "name": "terminology-compliance",
  "type": "advisory",
  "threshold": 0.8,
  "check": "terminology"
}
```

### Gate Types

| Type         | Behavior                                                                                 |
| ------------ | ---------------------------------------------------------------------------------------- |
| **Blocking** | The operation is prevented if the gate fails. Emits `quality.gate.fail`.                 |
| **Advisory** | A warning is logged but the operation proceeds. Emits `quality.gate.pass` with warnings. |

### Gate Events

Quality gates emit events that other automations can react to:

- `quality.gate.pass` — all checks passed (or advisory warnings only)
- `quality.gate.fail` — a blocking gate failed

This enables chains like: push content, run QA, and only proceed with translation if quality gates pass.

## Webhooks

Webhooks notify external systems when events occur on the server:

```bash
curl -X POST https://bowrain.example.com/api/v1/webhooks \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com/webhook",
    "secret": "shared-secret",
    "events": ["flow.completed", "quality.gate.fail"]
  }'
```

### Security

Webhook payloads are signed with HMAC-SHA256. The signature is sent in the `X-Bowrain-Signature` header:

```
X-Bowrain-Signature: sha256=<hex-encoded-hmac>
```

Verify the signature by computing `HMAC-SHA256(secret, request_body)` and comparing.

### Delivery

Webhooks use retry with exponential backoff:

- Up to 3 delivery attempts
- Backoff: 1s, 5s, 25s
- Delivery status is visible in the web UI under **Project Settings > Webhooks > Delivery History**

## Loop Prevention

Automation chains are tracked via a causation chain. Each event carries a `causation_id` linking it to the event that triggered it. If a chain of automation triggers exceeds **5 levels deep**, it is automatically stopped and a warning is logged.

For example, this chain would be stopped at level 5:

```
Push → flow.completed → run QA → quality.gate.pass → run export → ... (stopped)
```

This prevents infinite loops caused by circular automation rules (e.g., a push that triggers a flow whose completion triggers another push).

## Execution History

Both local and server-side automations maintain execution history:

- **Local**: `kapi status --automations` shows recent local automation runs
- **Server**: The web UI provides an execution log under **Project Settings > Automation > History**
- **API**: `GET /api/v1/automations/:id/executions` returns execution records with status, duration, and error details
