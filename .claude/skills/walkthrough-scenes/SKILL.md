---
name: walkthrough-scenes
description: Regenerate the recorder artifacts (VHS .tape files for terminal scenes, Playwright .spec.ts for UI scenes) for a walkthrough. Inputs the walkthrough prompt + per-scene context (CLI --help for terminal, testid registry for UI); outputs/updates files under {site}/scenes/{walkthrough-id}/.
---

# Skill: walkthrough-scenes

Regenerates the recorder artifacts for a walkthrough from its authored prompt. Operates per-walkthrough, not per-scene, so a multi-modal walkthrough (terminal + desktop + web) regenerates atomically.

## Inputs

- **walkthrough id** (required): the basename of `{site}/walkthroughs/{id}.md`. Example: `kapi-pseudo-translate`.
- **site** (inferred): `website` or `bowrain/website` based on where the prompt lives.

## What this skill does

For each scene in the prompt's `scenes:` frontmatter:

1. **Terminal scenes** (`kind: terminal`):
   - Reads `binary --help` for every command referenced in the smoke contract.
   - Reads the existing `.tape` if present (for diff-minimal regeneration).
   - Writes `{site}/scenes/{id}/0N-{scene-id}.tape` with VHS commands.
   - Copies referenced fixtures into `{site}/scenes/{id}/fixtures/`.
   - Honors `duration_budget_seconds` — total `Sleep` time + interaction time should fit.

2. **Desktop scenes** (`kind: desktop`):
   - Reads `bowrain/packages/ui/src/test-ids.ts` (typed testid registry).
   - Writes a Playwright spec that runs Wails dev mode against `BOWRAIN_BACKEND_URL`.
   - Each scene seeds its own workspace via `BowrainAPI` (or reuses a previous scene's via `seed: from-{scene-id}`).
   - Cleanup in `afterAll`.

3. **Web scenes** (`kind: web`):
   - Same testid registry; specs target the bowrain web app at `BOWRAIN_BACKEND_URL`.

## Constraints

- **Reproducibility**: same prompt → byte-identical scene files. No timestamps, no random IDs, no machine-specific paths.
- **Real backend only**: never emit code that mocks Wails RPC, intercepts fetch, or fakes auth. UI scenes hit a real bowrain-server.
- **Hermetic per scene**: no `test.describe.serial`, no shared state between scenes unless explicitly via `seed: from-...`.
- **No editing the MDX**: the doc is `walkthrough-doc`'s job; don't touch `{site}/docs/walkthroughs/{id}.mdx` here.

## Output verification

After writing, the user should be able to run `walkthrough-verify {id}` to confirm scenes record successfully against the real backend.

## Failure modes to call out

- **Smoke contract command fails its --help check**: don't write a tape that will deterministically fail; surface the issue and suggest fixing the binary first.
- **Testid not in registry**: if a UI scene references a name not in `test-ids.ts`, add it to the registry first via a separate edit, then regenerate.
