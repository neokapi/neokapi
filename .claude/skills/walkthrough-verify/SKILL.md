---
name: walkthrough-verify
description: Run a walkthrough's scenes locally against the real backend, capture asset sizes and durations, and validate the MDX renders. Gate before merging walkthrough PRs.
---

# Skill: walkthrough-verify

Runs every scene in a walkthrough end-to-end and validates the published MDX.

## When to use

The user types `/walkthrough-verify <id>` (or asks to "verify the X walkthrough"). Run before merging a walkthrough PR. Run after `/walkthrough-scenes` regenerates artifacts to confirm they actually record.

## Inputs

- **walkthrough id** (required): basename of `{site}/walkthroughs/{id}.md`.

## Steps

1. **Locate the prompt + assets.** Read `{site}/walkthroughs/{id}.md`. List `{site}/scenes/{id}/0N-*.{tape,spec.ts}` and confirm the count matches the prompt's `scenes:` list.
2. **Build the binaries** the scenes need:
   - kapi-only walkthroughs: `make build`
   - bowrain CLI walkthroughs: `make build build-bowrain-cli`
3. **Resolve `BOWRAIN_BACKEND_URL`** — defaults to `https://dev.bowrain.cloud`, overridable via env. For web scenes, also resolve `BOWRAIN_SESSION_TOKEN` via `bash bowrain/scripts/device-auth.sh "$BOWRAIN_BACKEND_URL"`. Fail loudly if unreachable; never fall back to a mock.
4. **Run each scene's recorder**:
   - `kind: terminal` → `cd {site}/scenes/{id} && timeout 180 vhs 0N-{scene}.tape` (so `fixtures/` resolves relative).
   - `kind: web` → `cd {site} && BOWRAIN_SESSION_TOKEN=... npx playwright test scenes/{id}/0N-{scene}.spec.ts --reporter=line`.
5. **Capture asset metadata.** For each produced `.webm`:
   - `ffprobe -v error -show_entries format=duration -of default=nw=1:nk=1 PATH` → seconds
   - `ls -la PATH` → bytes
   Compare duration against the prompt's `duration_budget_seconds`. Warn if a scene exceeded its budget by >20%.
6. **Stage assets.** Run `bash bowrain/website/scripts/stage-scenes.sh` (for bowrain web) so the generated `.webm` files end up at `bowrain/website/static/video/bowrain/{id}/0N-*.webm` where the MDX `<ThemedVideo>` references resolve. For the kapi site, the staging is inline in `docs-kapi.yml`'s recorder step.
7. **Build the Docusaurus site.** `cd {site} && npm run build`. Look for "Generated static files in 'build'." with no errors. If the build flags a broken video link, the MDX path doesn't match the produced asset filename — recheck step 4.
8. **Run the smoke contract.** For each scene with `smoke_contract:` in the prompt, re-run every listed command and assert exit 0. This is what was lost in the legacy `test-cli.sh` — re-introduce it as a per-walkthrough invariant.

## Cleanup

- **Web scenes that seeded a workspace via API:** delete the workspace in teardown. The current `_helpers.ts` doesn't auto-clean; if the spec did `getOrCreateProject` on the personal workspace, leave it (idempotent reuse).
- **Tape recordings**: VHS produces files in the scene dir; staging copies them. Don't delete originals — the recorder commits them as fresh artifacts on the next run.

## Output

Print a per-scene summary:

```
✓ {walkthrough-id}/0N-{scene}.webm — 4.8s / 248K (budget 5s)
✗ {walkthrough-id}/0N-{other}.webm — RECORDING FAILED: {error}
```

Exit non-zero if any scene failed, exceeded its budget by >20%, or the Docusaurus build broke.

## Constraints

- **No mocks anywhere**. If the backend is unreachable, fail; don't silently skip or simulate.
- **Workspace churn**: if a scene creates a workspace, it cleans it up. Never leave stragglers on `dev.bowrain.cloud`.
- **Exit codes are not enough**: a tape can succeed (exit 0) but produce a wrong-looking recording. Asset duration and the smoke-contract grep are what catch silent corruption.
- **Don't regenerate assets you can't verify.** If the binary needed for a scene's smoke contract isn't built, build it first; don't write a "skipped" status.
