---
sidebar_position: 3
title: Hooks
---

# Flow hooks

:::warning Not yet executed
Hooks are **parsed and validated** in the recipe, but the kapi-bowrain plugin
**does not yet run them**. Declaring a `hooks:` block has no runtime effect on
`kapi push` / `kapi pull` today, and there are no `--no-hooks` / hook-bypass
flags. This page describes the intended design; until execution ships, use
[automations](#what-runs-today) for the lifecycle behavior that does run.
:::

Hooks are intended to be flows that run automatically around sync operations, so
a project can enforce quality gates before content leaves the machine and
post-process content after it arrives.

## Intended design

Hooks would be declared at the top level of the `<dir-name>.kapi` recipe, mapping
a lifecycle trigger to a list of flow names:

```yaml
hooks:
  pre-push:
    - qa # block the push if QA fails
  post-pull:
    - segmentation # post-process freshly pulled source
```

- **pre-push** would run before `kapi push` uploads, as a quality gate.
- **post-pull** would run after `kapi pull` fetches, as post-processing.

The recipe schema already accepts and validates these triggers; the executor
that runs the referenced flows is not implemented.

## What runs today {#what-runs-today}

The lifecycle behavior that **does** run on `kapi push` / `kapi pull` is the
recipe's `automations:` block — trigger-based rules whose actions (such as
`pull`, `push`, and `wait_translate`) the plugin executes. See
[Automation](/server/automation) for that model.

For local content processing you can drive a flow explicitly with
[`kapi run <flow>`](/cli/flows/custom-flows) instead of relying on hooks.

## Next steps

- [Custom flows](/cli/flows/custom-flows)
- [Automation](/server/automation)
- [Push](/cli/commands/push) · [Pull](/cli/commands/pull)
