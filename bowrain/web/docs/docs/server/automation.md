---
title: Automation
sidebar_position: 11
---

# Automation

Bowrain provides two complementary layers of automation: **local rules** declared on the project recipe for CLI-driven workflows, and **server-side rules** configured in the web UI for event-driven processing across the whole workspace.

## Local automation (CLI)

Local automations run in kapi (with the bowrain plugin) and are declared at the top level of your project's `.kapi` recipe. They hook into CLI commands and execute actions before or after operations like push, pull, and flow runs.

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

### Trigger types

| Trigger     | Fires when                                           |
| ----------- | ---------------------------------------------------- |
| `pre-push`  | Before `kapi push` sends content to the server       |
| `post-push` | After `kapi push` completes successfully             |
| `pre-pull`  | Before `kapi pull` fetches content from the server   |
| `post-pull` | After `kapi pull` writes files locally               |
| `pre-flow`  | Before `kapi run` executes a flow                    |
| `post-flow` | After `kapi run` completes                           |

### Action types

| Action           | Description                                                               |
| ---------------- | ------------------------------------------------------------------------- |
| `run_flow`       | Execute a flow by name (inline on the recipe, from `.kapi/flows/`, or built-in) |
| `wait_translate` | Wait for server-side translations to complete (with configurable timeout) |
| `pull`           | Pull translated content from the server                                   |
| `push`           | Push local content to the server                                          |

### Example: QA gate before push

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

### Example: Full sync after push

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

This makes `kapi push` behave like `kapi sync`.

## Server-side automation

Server-side automations are event-driven rules configured in the Bowrain web
app under **Project > Automations**. They respond to events on the server —
content pushed, translations completed, quality gates evaluated — and trigger
actions like running flows or sending notifications.

The Automations surface is a **superset flow + automation editor** with three
tabs: **Runs** (run history), **Rules** (trigger + conditions + actions), and
**Flows** (the flow canvas — see [Server-Side Flows](/server/flows)). The Flows
tab embeds the same `@neokapi/flow-editor` component the desktop apps use, so a
`run_flow` action and the flow it runs are authored in one place.

### Creating rules

1. Navigate to **Project > Automations > Rules**
2. Click **New Rule**
3. Select a trigger event (for example, "When content is pushed")
4. Add optional conditions (filter by project, locale, or event data)
5. Add one or more actions. A `run_flow` action picks a flow from the project's
   flow registry (the built-in catalog plus any project flows authored on the
   Flows tab) — connector-agnostic, so the same flow applies to content from
   any connector
6. Save and enable the rule

Rules and the flow definitions they reference are persisted through the
project-scoped REST API: `/api/v1/:ws/:id/automations` for rules and
`/api/v1/:ws/:id/flows` for flow definitions. Rules can be reordered, disabled,
and duplicated from the editor.

## Quality gates

Quality gates evaluate content against thresholds before allowing operations to proceed. A blocking gate aborts the operation if the check fails; an advisory gate logs a warning but continues.

Gates integrate with both local and server-side automation, so you can enforce standards at the CLI level (pre-push) and at the server level (post-push).

## Webhooks

Webhooks notify external systems when events occur. Configure them in **Project Settings > Webhooks** with a destination URL and the events you want to receive.

Webhook payloads are signed with HMAC-SHA256 so the receiving system can verify authenticity. Delivery history is visible in the web UI.

## Loop prevention

Automation chains are tracked through a causation chain. If a chain of rules exceeds five levels deep, it stops automatically and a warning is logged. This prevents circular rules from running indefinitely.

## Execution history

Both local and server-side automations maintain execution history:

- **Local**: `kapi status --automations` shows recent local automation runs
- **Server**: **Project Settings > Automation > History** provides an execution log with status, duration, and error details
