---
title: Automation
sidebar_position: 11
description: Bowrain's two layers of automation, server-side rules that respond to events across a workspace, and local rules declared on a project recipe for kapi-driven CLI and CI workflows.
---

# Automation

Bowrain provides two complementary layers of automation: **server-side rules**,
configured in the web app, that respond to events across the whole workspace,
and **local rules** declared on a project recipe for kapi-driven (CLI/CI)
workflows.

## Server-side automation

Server-side automations are event-driven rules configured in the Bowrain web
app under **Project > Automations**. They respond to events on the server and
trigger actions such as running flows or sending notifications. Because they
react to events on the server, they apply to content from **any connector**, not
just a local checkout.

The Automations surface has three tabs: **Runs** (run history), **Rules**
(trigger + conditions + actions), and **Flows** (the flow canvas; see
[Server-side flows](/server/flows)). The Flows tab and the Rules tab are one
editor, so a `run_flow` action and the flow it runs are authored in one place.

### Triggers

A rule fires on one of these events:

| Trigger | Fires when |
| --- | --- |
| `connector.push.completed` | Content is pushed, from a checkout or a repository |
| `connector.pull.completed` | Content is pulled |
| `project.updated` | Project settings change |
| `quality.gate.fail` | A quality gate fails |
| `push.automations.completed` | Every automation for a push has completed |
| `source.review.completed` | A source review task is completed |

A connector fetch emits no event, so a rule cannot react to content arriving
from a content platform; catch that content up by starting a run. No event
fires when a flow finishes, so a rule cannot chain on a flow's completion.

### Creating rules

1. Navigate to **Project > Automations > Rules**
2. Click **New Rule**
3. Select a trigger event
4. Add optional conditions (filter by project, locale, or event data)
5. Add one or more actions. A `run_flow` action picks a flow from the project's
   flow registry (the built-in catalog plus any project flows authored on the
   Flows tab); the same flow applies to content from any connector
6. Save and enable the rule

Rules and the flow definitions they reference are persisted through the
project-scoped REST API: `/api/v1/:ws/:id/automations` for rules and
`/api/v1/:ws/:id/flows` for flow definitions. Rules can be reordered, disabled,
and duplicated from the editor. A rule is checked when it is saved: every
action must be one the server runs, and a `run_flow` action must name a flow
the project can see, so a rule that saves is a rule that fires.

### Actions

| Action                 | What it does                                                                 |
| ---------------------- | ---------------------------------------------------------------------------- |
| `run_flow`             | Runs a flow over the project's stored content and writes the results back    |
| `auto_translate`       | Starts translation jobs for the pushed items in every target language        |
| `create_review_tasks`  | Opens a review task per target language for the project's reviewers          |
| `create_source_review` | Opens a source review task                                                    |
| `notify`               | Sends an in-app notification                                                 |

A `run_flow` action takes these parameters:

| Parameter        | Meaning                                                                                                   |
| ---------------- | --------------------------------------------------------------------------------------------------------- |
| `flow`           | The flow to run: a built-in flow or one authored on the project's Flows tab. Required.                    |
| `stream`         | The stream to read and write. Defaults to the stream the event names, or `main`.                          |
| `items`          | Comma-separated item names. Defaults to the items the event names (a push's files), or every item.        |
| `target_locales` | Comma-separated target locales, one pass each. Defaults to the project's target languages.                |

The flow runs in the background over one item at a time, and the run's step
records what it wrote. A flow the project cannot see, a flow whose tool this
server lacks, or a tool that fails part way marks the step failed with the
reason, in the run history and in the execution history.

### Run history

Every server-side automation run is recorded under **Project > Automations >
Runs**, with its trigger, its steps, the outcome of each step, and per-step
logs. A run in progress updates in place as each step starts and finishes.
This is the record to consult when a rule did not do what you expected.

## Local automation (CLI)

Local automations run in kapi (with the bowrain plugin) and are declared at the
top level of your project's `kapi.yaml` recipe. They hook into CLI commands and
execute actions before or after push, pull, and flow runs: the developer
workflow and CI layer that complements the server rules above.

### Configuration

Add an `automations:` section to your `kapi.yaml` recipe:

```yaml
automations:
  - name: checks-before-push
    trigger: pre-push
    actions:
      - type: run_flow
        config:
          flow: qa

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
| `run_flow`       | Run a flow by name over the project's collections, as `kapi run <flow>` does with no `--input` |
| `wait_translate` | Wait for the server-side run to complete (with configurable timeout)     |
| `pull`           | Pull results from the server                                              |
| `push`           | Push local content to the server                                          |

A local `run_flow` action takes these parameters:

| Parameter       | Meaning                                                                                              |
| --------------- | ---------------------------------------------------------------------------------------------------- |
| `flow`          | The flow to run: one declared under `flows:` on the recipe, or a built-in flow such as `qa`. Required. |
| `fail_on_error` | `true` aborts the command when the flow's check steps report findings. Default `false`: report and continue. |

The flow runs the way `kapi run <flow>` runs it inside the project: over every
file the recipe's collections match, one pass per target language the flow
applies to, with the recipe's voice profile and terms bound, and with the
results committed to the project store rather than written to target files.
Its output, including the findings table its check steps produce, prints in
the output of the command that triggered it (on stderr under `--json`, so the
command's document stays intact). A flow that cannot run, because the name is
unknown or a tool fails, aborts the command whatever `fail_on_error` says.

### Example: a check gate before push

Prevent pushing content that fails the checks:

```yaml
automations:
  - name: checks-gate
    trigger: pre-push
    actions:
      - type: run_flow
        config:
          flow: qa
          fail_on_error: true
```

If `qa` reports findings and `fail_on_error` is `true`, the push is aborted
before anything is sent, with the findings summary and the exit code
`kapi check` uses for a failed gate. Without `fail_on_error` the findings are
reported and the push goes ahead.

### Catching up on every push

Catching the project up on push is the project's `bowrain.converge` policy
rather than an automation. With the default `on-push`, the server runs a full
pass (reuse, drafting, checks, terms, gates, parking) after every push, recorded
as a run anyone can watch:

```yaml
# kapi.yaml
bowrain:
  url: https://bowrain.example.com/my-team/abc123
  converge: on-push        # on-push (default) | manual
```

`kapi push` from CI just pushes; the server catches the project up on its own
clock; `kapi up` is push, then *watch the run*, then pull. See
[Keeping content caught up](/the-loop) for the model.

Every run, started by a push, by `kapi up`, or manually, appears in the
project's **Runs** view in the web and desktop app, with its state (*Running*,
*Up to date*, *Parked*), its trigger, the pass count, and a per-locale summary
such as "3 shippable · 1 parked". A **Run now** button starts a run from the
app, and an in-flight run can be canceled. Parked units land in the
[review session](/server/review).

## Quality gates

Quality gates evaluate content against thresholds before allowing operations to
proceed. A blocking gate aborts the operation if the check fails; an advisory
gate logs a warning but continues. Gates integrate with both server-side and
local automation, so you can enforce standards at the server level (on push)
and at the CLI level (pre-push).

## Loop prevention

Automation chains are tracked through a causation chain. If a chain of rules
exceeds five levels deep, it stops automatically and a warning is logged. This
prevents circular rules from running indefinitely.
