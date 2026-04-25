---
name: walkthrough-verify
description: Run a walkthrough's scenes locally against the real backend, capture asset sizes and durations, and validate the MDX renders. Gate before merging walkthrough PRs.
---

# Skill: walkthrough-verify

Runs every scene in a walkthrough end-to-end and validates the published MDX.

## Inputs

- **walkthrough id** (required): basename of `{site}/walkthroughs/{id}.md`.

## What this skill does

1. **Build the binaries** the scenes need (`make build` for kapi-only walkthroughs; `make build build-bowrain-cli` or `wails3 build` for bowrain ones).
2. **Resolve `BOWRAIN_BACKEND_URL`** — defaults to `https://dev.bowrain.cloud`, overridable via env. Fail loudly if unreachable; never fall back to a mock.
3. **Run each scene's recorder**:
   - Terminal: `vhs {site}/scenes/{id}/0N-*.tape` from inside the scene dir (so `fixtures/` resolves).
   - Desktop: `playwright test {site}/scenes/{id}/0N-*.spec.ts` with Wails dev mode running.
   - Web: `playwright test ...` against the configured backend.
4. **Capture asset metadata** — duration of each `.webm`, byte sizes — and write to `{site}/scenes/{id}/manifest.json`. Compare against `duration_budget_seconds` in the prompt and warn if exceeded.
5. **Build the Docusaurus site** for the affected scope (kapi or bowrain) and confirm zero broken links (the doc references its own scene assets).
6. **Smoke contract validation** — re-run every command in each scene's `smoke_contract:` and assert exit 0 + expected stdout pattern. This is what was lost in the legacy `test-cli.sh` — re-introduce it as a per-walkthrough invariant.

## Cleanup

For UI scenes that created a workspace via `BowrainAPI`, delete it in teardown. For terminal scenes, no cleanup beyond what VHS does on its own.

## Constraints

- **No mocks anywhere**. If the backend is unreachable, fail; don't silently skip or simulate.
- **Workspace churn**: each invocation creates and deletes its own workspace; never leave stragglers on `dev.bowrain.cloud`.
- **Exit codes are not enough**: a tape can succeed (exit 0) but produce a wrong-looking recording. Asset duration and the smoke-contract grep are what catch silent corruption.
