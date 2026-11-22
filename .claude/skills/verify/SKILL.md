---
name: verify
description: Verify a change by running it, before committing. Finds the surfaces a diff touches (kapi CLI, formats, Bowrain server and stores, web app, Kapi Desktop, mobile app, CI and release workflows), runs the real thing for each, and records the evidence. Use before every commit of product code, and whenever asked to verify, prove or check that a change works.
---

# Verify a change by running it

A passing unit test shows that the code does what its author expected. It does
not show that the feature works. Verification here means running the changed
behaviour on the real binary, server or app and looking at what it does.

Work through the steps in order. Stop at the first one that fails and fix the
cause before going on.

## 1. Find what the diff touches

```bash
git diff --name-only "$(git merge-base origin/main HEAD)"
```

Match the paths to a recipe. A diff can need more than one.

| Paths | Recipe |
| --- | --- |
| `cli/`, `host/`, `kapi/`, `core/` (not `core/formats/`), `memory/`, `terms/`, `providers/` | [cli.md](cli.md) |
| `core/formats/`, `plugins/` | [formats.md](formats.md) |
| `bowrain/` Go, `.sql`, migrations, `terms/`, `memory/`, `core/storage/` | [server.md](server.md) |
| `bowrain/apps/web/`, `bowrain/packages/`, `packages/ui/`, `packages/bowrain-client/` | [web.md](web.md) |
| `apps/kapi-desktop/`, `bowrain/apps/bowrain/`, `packages/` used by a desktop app | [desktop.md](desktop.md) |
| `bowrain/mobile/` | [mobile.md](mobile.md) |
| `.github/`, `Makefile`, `scripts/` | [ci.md](ci.md) |

Documentation, comments and test-only changes need steps 2 and 3 and no recipe.

## 2. Read the checklist for this diff

```bash
make reliability-checklist
```

It prints the items of `docs/internals/reliability-checklist.yaml` whose
trigger matches the diff, usually three to ten. Each names a check and the
incident behind it. Do each check. An item that does not apply after you have
looked is fine; say so in the report.

## 3. Run the cheap checks

```bash
make pre-push
```

It runs the guards, lint and tests relevant to the changed paths. Read its exit
code, not a filtered tail (see "A piped command hides its exit code" in
`CLAUDE.md`).

## 4. Run the change

Follow the recipe. Each one says how to build, start and drive the surface, and
what counts as evidence. Exercise the behaviour the diff changes, on real
input, and one case next to it that should be unaffected.

Pull request CI skips some suites, so a green pull request has not run them.
The recipes say when you must run them yourself:

- The integration lane and the suites that start their own containers
  (skipped by `-short`): [server.md](server.md).
- The race detector: add the `race` label, or run `make test-race-changed`.
- Release workflows: a manual run on the branch, [ci.md](ci.md).

## 5. When a check fails

Fix the cause and run it again. After three rounds on the same check, stop and
bring in the person: say what fails, what you tried and what you suspect.

Never make a check pass by weakening it. That means no skipped or deleted test,
no added sleep or retry, no loosened assertion, no new allowlist or exemption
entry, and no narrowed path filter, unless the change is the point of the work
and the person has agreed to it.

A test that fails some of the time has a cause. Find it: the two flaky tests
fixed in November 2026 were a real clock-skew defect and a real ordering
defect. If you cannot find it, say so. Do not rerun until green.

## 6. Record what you ran

End the commit message with a trailer that names the recipes you ran:

```text
Verified: cli, server
```

When you ran none, say why:

```text
Verified: none (comment-only change)
```

In the pull request, a `## Verification` section lists each thing you ran with
its result, and each thing you did not or could not run. A reviewer reads that
section as a claim and checks it.

## A change to a check

A diff that edits a guard, an allowlist, a workflow, `REVIEW.md` or the
checklist changes what future changes are held to. Say so at the top of the
pull request description, with what the change lets through that was caught
before, and do not merge it until the person has seen it.
