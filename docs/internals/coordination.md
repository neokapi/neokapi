# Coordination notes

Checks for whoever coordinates work on this repository, agent or person.

## At the start of a session

- List the failing scheduled workflows: `gh issue list --label scheduled-failure --state open`. Each open issue is one workflow whose last scheduled run failed, with the failing jobs named; `.github/workflows/scheduled-failure-issue.yml` closes it when the workflow next passes.
- List the overdue gardening: `gh issue list --label gardening --state open`, or `make gardening-due` locally. Each open issue is one item from `docs/internals/gardening.yaml`, with the command to run; `.github/workflows/gardening.yml` closes it once the item reads as done.
