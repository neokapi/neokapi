---
name: walkthrough-scenes
description: Regenerate the recorder artifacts (VHS .tape files for terminal scenes, Playwright .spec.ts for UI scenes) for a walkthrough. Reads the walkthrough prompt + per-scene context (CLI --help for terminal, testid registry for UI); writes/updates files under {site}/scenes/{walkthrough-id}/. Reproducible — same prompt produces the same files.
---

# Skill: walkthrough-scenes

Regenerates the recorder artifacts for a walkthrough from its authored prompt. Operates per-walkthrough, not per-scene, so a multi-modal walkthrough (terminal + web in one story) regenerates atomically.

## When to use

The user types `/walkthrough-scenes <id>` (or asks to "regenerate scenes for X"). Run after the prompt's frontmatter changed (different scenes block) or after a `data-testid` rename happened in the UI.

## Inputs

- **walkthrough id** (required): basename of `{site}/walkthroughs/{id}.md`. Example: `bowrain-web-settings`.
- **site** (inferred): `bowrain/website` or `website`.

## Steps

1. **Locate the prompt.** Read `{site}/walkthroughs/{id}.md`. Parse frontmatter `scenes:` list.
2. **For each scene** in the prompt's `scenes:` list, in declaration order, write `{site}/scenes/{id}/0N-{scene-id}.{ext}` where `N` is the 1-based index and `ext` depends on `kind`:
   - `kind: terminal` → `.tape`
   - `kind: web` → `.spec.ts`
   - `kind: desktop` → not supported (desktop scenes were dropped from scope; surface this if the prompt declares one)
3. **Stage fixtures**: any file referenced in `scene.fixtures:` should already exist under `{site}/scenes/{id}/fixtures/`. If it's missing, fail loudly — don't generate fixture content from thin air.

## Web scene template (`kind: web`)

The bowrain web specs all share helpers from `bowrain/website/scenes/_helpers.ts`. Match this exact structure:

```typescript
/**
 * Walkthrough: {walkthrough-id}
 * Scene N: {scene-id} (web)
 *
 * {one-line description from the scene's prose section in the prompt}
 */

import { test, expect } from "@playwright/test";
import { BACKEND_URL, injectAuthCookie, getMyWorkspaceSlug, saveSceneVideo } from "../_helpers";

test.describe("walkthrough: {walkthrough-id}", () => {
  test("{scene-id} [scene]", async ({ page }) => {
    await injectAuthCookie(page);
    const slug = await getMyWorkspaceSlug();
    await page.goto(`${BACKEND_URL}/{path-derived-from-prompt}`);
    await expect(page.getByTestId("{landing-testid}")).toBeVisible({ timeout: 15000 });
    await page.waitForTimeout(2000);
  });
  test.afterEach(async ({ page }, testInfo) => saveSceneVideo(page, testInfo, "0N-{scene-id}.webm"));
});
```

Key conventions:
- **Auth via cookie injection**, never PKCE. `injectAuthCookie` reads `BOWRAIN_SESSION_TOKEN` env.
- **Workspace from `getMyWorkspaceSlug()`** — picks the personal workspace by default. If the scene needs a specific slug, take it from `scene.seed.workspace`.
- **One assertion before the wait**: pick a deployed `data-testid` that's stable under the route. Look it up in `bowrain/packages/ui/src/test-ids.ts` or grep the deployed component for `data-testid=`.
- **`page.waitForTimeout(2000)` at the end** — holds the final frame so the recording's last 2s shows the loaded view.
- **Cookie injection + saveSceneVideo come from `../_helpers.ts`** — don't inline them.

If the prompt's scene block declares a richer interaction (uploads, multiple clicks, form fills), extend the body but keep the auth + workspace + saveSceneVideo wrapper.

## Terminal scene template (`kind: terminal`)

```tape
# VHS tape — generated from {site}/walkthroughs/{walkthrough-id}.md
# Scene N: {scene-id} (terminal). Do not edit by hand.

Output "0N-{scene-id}.webm"

Set FontSize 16
Set Width 900
Set Height 550
Set Theme "Dracula"
Set Padding 20

Type "# {short comment from the scene's prose}"
Enter
Sleep 500ms

# (Each command from scene.smoke_contract / prose, with appropriate sleeps)
Type "{binary} {args}"
Enter
Sleep {N}s

# (Final beat — hold for ~1s on the last visible frame)
Sleep 1s
```

Key conventions:
- **Terminal width 900x550, padding 20, Dracula theme** — matches the existing kapi tapes' visual style.
- **`Type "# ..."`** comments are visible in the recording; use them sparingly to set context.
- **`Sleep N s`** pacing: 500ms after a comment, 1-2s after a command, 1s at the end. Total should fit `duration_budget_seconds`.
- **`smoke_contract:` commands MUST appear in the tape** verbatim — they're what `walkthrough-verify` re-runs to detect breakage.

## Constraints

- **Reproducibility**: same prompt → byte-identical scene files. No timestamps, no random IDs, no machine-specific paths. If two regenerations diverge, the template above wasn't followed deterministically.
- **Real backend only**: never emit code that mocks Wails RPC, intercepts fetch, or fakes auth. UI scenes hit a real bowrain-server.
- **Hermetic per scene**: no `test.describe.serial`, no shared state between scenes unless explicitly declared via `seed: from-{scene-id}`.
- **No editing the MDX**: the doc is `walkthrough-doc`'s job; don't touch `{site}/docs/walkthroughs/{id}.mdx` here.
- **Don't fabricate testids**: if the prompt asks to assert a name not in the deployed component (grep `data-testid=` to verify), surface it and stop. Adding a missing testid is a UI change, not a scene-regen.

## Output verification

After writing, the user should be able to run `/walkthrough-verify {id}` to confirm scenes record successfully against the real backend.

## Failure modes

- **Smoke contract command fails its --help check**: don't write a tape that will deterministically fail; surface the issue.
- **Testid not in registry / not in deployed component**: stop and report; don't assume the registry is the source of truth — components sometimes drift.
- **Scene ordering mismatch**: if the existing `0N-` files don't match the prompt's scene order, renumber them as part of the regeneration. Note this in the diff.
