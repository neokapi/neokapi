---
id: bowrain-automation
audience: developer
target_doc: docs/walkthroughs/bowrain-automation.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: local-rules
    kind: terminal
    binary: bowrain
    duration_budget_seconds: 30
    fixtures: []
    smoke_contract:
      - kapi push
  - id: gh-actions
    kind: terminal
    binary: bowrain
    duration_budget_seconds: 25
    fixtures: []
    smoke_contract:
      - cat .github/workflows/bowrain-sync.yml
---

## Story

Bowrain has two layers of automation: **local rules** declared on the
`.kapi` recipe (top-level `hooks:` and `automations:`) that run on the
developer machine (e.g. pre-push QA), and **GitHub Actions workflows**
that run on the CI/CD side. Together they make translation sync fully
automated — code lands, translations follow.

## Scene 1 — local-rules (terminal)

Inspect the project's `<dir-name>.kapi` recipe for top-level `hooks:`
and `automations:` blocks. Run `kapi push` and watch the pre-push
QA hook fire automatically before the actual push happens.

## Scene 2 — gh-actions (terminal)

Show the `.github/workflows/bowrain-sync.yml` that runs `kapi sync`
on every push to main. This is the CI/CD half of the automation.

## Closing

The full automation rule schema and event types are documented in
[Automation engine](/architecture-decisions/013-automation-engine).
