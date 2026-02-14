---
title: Automation
sidebar_position: 11
---

# Automation

gokapi's event-driven automation system lets you create rules that trigger actions when content changes.

## Event System

The platform emits events for all major operations:

- Content pulled from a connector
- Blocks stored or updated
- Versions created
- Flows completed
- Quality gates evaluated

## Automation Rules

Rules consist of an event trigger, conditions, and an action:

```yaml
automations:
  - name: auto-translate-on-pull
    trigger: connector.pulled
    conditions:
      - field: project_id
        equals: "my-project"
    action: run-flow
    flow: ai-translate
```

### Conditions

| Operator | Description |
|----------|-------------|
| `equals` | Exact match |
| `contains` | Substring match |
| `exists` | Field is present |

## Quality Gates

Quality gates evaluate content quality before pushing translations:

```yaml
quality_gates:
  - name: translation-coverage
    type: blocking
    threshold: 0.9
    check: coverage

  - name: terminology-compliance
    type: advisory
    threshold: 0.8
    check: terminology
```

### Gate Types

- **Blocking**: Push is prevented if the gate fails
- **Advisory**: A warning is logged but push proceeds

## Webhooks

Configure webhooks to notify external systems of events:

```yaml
webhooks:
  - url: https://example.com/webhook
    secret: shared-secret
    events:
      - version.created
      - quality.failed
```

Webhooks are signed with HMAC-SHA256 and include retry with exponential backoff (up to 3 attempts).

## Loop Prevention

Automation chains are tracked to prevent infinite loops. If a chain of automation triggers exceeds 5 levels deep, it is automatically stopped.
