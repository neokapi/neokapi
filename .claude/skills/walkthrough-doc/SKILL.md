---
name: walkthrough-doc
description: Regenerate the published MDX page for a walkthrough by combining the walkthrough prompt with the recorded scene artifacts. Outputs {site}/docs/walkthroughs/{id}.mdx with prose interleaved with ThemedVideo embeds. Never touches recorder files. Idempotent — running twice produces the same MDX byte-for-byte.
---

# Skill: walkthrough-doc

Regenerates the published MDX for a walkthrough from its prompt and its existing scene artifacts.

## When to use

The user types `/walkthrough-doc <id>` (or asks to "regenerate the doc for X walkthrough"). Run after a prompt has been edited. Do **not** run if the prompt hasn't changed and the existing MDX already matches.

## Inputs

- **walkthrough id** (required): basename of `{site}/walkthroughs/{id}.md`. Example: `bowrain-web-settings`.
- **site** (inferred): if the prompt lives at `bowrain/website/walkthroughs/{id}.md`, write MDX to `bowrain/website/docs/walkthroughs/{id}.mdx`. If at `website/walkthroughs/{id}.md`, write to `website/docs/walkthroughs/{id}.mdx`.

## Steps

1. **Locate the prompt.** Read `{site}/walkthroughs/{id}.md`. If missing, fail — don't guess.
2. **Parse the YAML frontmatter** for `id`, `audience`, `target_doc`, `backend_url`, and the `scenes:` list. Each scene has `id`, `kind`, optionally `duration_budget_seconds`, `seed`, `smoke_contract`.
3. **Parse the prose sections** by `##` heading: `## Story`, `## Scene N — {scene-id}` (one per scene in order), `## Closing`.
4. **List the scene assets.** Look in `{site}/scenes/{id}/` for `0N-{scene-id}.{webm,mp4,png}` files and the corresponding `0N-{scene-id}.{tape,spec.ts,applescript}`. The numeric prefix `0N` reflects scene ordering — match it to the scenes block.
5. **Compute MDX frontmatter values:**
   - `id`: from prompt frontmatter `id`
   - `title`: human-readable. Pull the first `# {title}` heading from the prompt's body if present; otherwise derive from id.
   - `sidebar_label`: short form (often parenthetical "(web)"/"(desktop)"/"(cli)" suffix based on the dominant scene kind)
   - `description`: a one-sentence summary derived from the Story section's first sentence.
6. **Write the MDX file** using the template below, exact whitespace and order.

## MDX template (the format every regenerated walkthrough must follow)

```mdx
---
id: {id}
title: "{title}"
sidebar_label: "{sidebar_label}"
description: "{description}"
---

import { ThemedVideo } from "@neokapi/docs-shared";

# {title}

> Generated from [`walkthroughs/{id}.md`](https://github.com/neokapi/neokapi/blob/main/{site}/walkthroughs/{id}.md). Do not edit by hand — change the prompt and regenerate.

{Story prose, verbatim from the prompt's `## Story` section. Strip the heading itself; the prose flows directly under the H1.}

## Scene N — {scene-id}

<ThemedVideo
  sources={{
    light: "/video/{scope}/{walkthrough-id}/0N-{scene-id}.webm",
    dark: "/video/{scope}/{walkthrough-id}/0N-{scene-id}.webm",
  }}
/>

{Scene N prose, verbatim from the prompt's matching scene section.}

{Repeat for each scene...}

## Next

{Closing prose, verbatim from the prompt's `## Closing` section. If the prompt's closing already starts with "See" or "For", leave the heading as `## Next`; otherwise use the prompt's section heading.}
```

## Key conventions

- **Asset path**: `/video/{scope}/{walkthrough-id}/0N-{scene-id}.webm` where `{scope}` is `bowrain` for the bowrain site and `kapi` for the kapi site. The `pages-deploy.yml` workflow puts assets at these paths under `static/video/{scope}/`.
- **Same source for both light/dark**: until per-theme assets exist, both `light` and `dark` point at the same `.webm`. Do not invent a `-dark.webm` filename if it doesn't exist on disk.
- **No `draft: true`** in regenerated MDX — that flag is for hand-authored stubs awaiting a recording. If a scene asset is missing, surface it; don't write a draft page.
- **Cross-site links**: if the prompt's prose mentions the other site (e.g. bowrain prose linking to `kapi pseudo-translate`), use `<KapiLink to="/walkthroughs/kapi-pseudo-translate" />` from `@neokapi/docs-shared` rather than a relative path. Keep imports tidy: `import { ThemedVideo, KapiLink } from "@neokapi/docs-shared"`.
- **Code spans in prose**: pass through unchanged. Don't reformat backticks.

## What NOT to do

- Don't touch any file under `{site}/scenes/` — that's `walkthrough-scenes`'s domain.
- Don't add prose that isn't in the prompt. The prompt is the source of truth for everything except the scaffolding template.
- Don't reorder scenes. Order matches the prompt's frontmatter order, and the file numbering already reflects it.
- Don't change a recording's filename to match what you'd prefer; read what's on disk and embed it as named.

## Verification

After writing the MDX, build the affected Docusaurus site once to check it compiles:

```bash
cd {site} && npm run build
```

Look for "Generated static files in 'build'." with no errors. If `onBrokenLinks: throw` flags a missing video, the asset path is wrong — recheck step 4.

## Idempotency check

Running this skill twice on the same prompt + scenes must produce a byte-identical MDX. If it doesn't, the template above isn't being followed deterministically. The diff is the bug, not a feature.
