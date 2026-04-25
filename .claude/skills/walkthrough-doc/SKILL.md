---
name: walkthrough-doc
description: Regenerate the published MDX page for a walkthrough by combining the walkthrough prompt with the recorded scene artifacts. Outputs {site}/docs/walkthroughs/{id}.mdx with prose interleaved with ThemedVideo/Screenshot embeds. Never touches recorder files.
---

# Skill: walkthrough-doc

Regenerates the published MDX for a walkthrough from its prompt and its existing scene artifacts.

## Inputs

- **walkthrough id** (required): basename of `{site}/walkthroughs/{id}.md`.
- **site** (inferred): `website` or `bowrain/website`.

## What this skill does

1. Reads `{site}/walkthroughs/{id}.md`.
2. Lists the recorded scene assets at `{site}/scenes/{id}/`.
3. Writes `{site}/docs/walkthroughs/{id}.mdx`:
   - Frontmatter with `title`, `sidebar_label`, `description` derived from the prompt's "Story" section.
   - A header note: "Generated from `walkthroughs/{id}.md`. Do not edit by hand — change the prompt and regenerate." with a link back to the prompt on GitHub.
   - The Story section as the lead paragraph.
   - For each scene block in the prompt:
     - A `## Scene N — {scene-id}` heading.
     - The embed: `<ThemedVideo />` for video, `<img />` for stills, with paths under `/video/{scope}/...` or `/img/{scope}/...`.
     - The prose from that scene's section in the prompt.
   - The Closing section as the wrap-up.
4. Imports `ThemedVideo` from `@neokapi/docs-shared` (and `<KapiLink>`/`<BowrainLink>` if the prompt references the other site).

## Constraints

- **Don't touch recorder files** — that's `walkthrough-scenes`'s job. If the assets are missing, surface it; don't regenerate them.
- **Asset path convention**: `/video/{site-scope}/{scene-filename}.webm` where site-scope is `kapi` for the kapi site and `bowrain` for bowrain. The pages-deploy.yml workflow places asset artifacts under these paths.
- **Cross-site links**: if the prompt references the other site (e.g. bowrain prose mentioning `kapi pseudo-translate`), use `<KapiLink to="..."/>` from `@neokapi/docs-shared` instead of relative paths.

## Output verification

The kapi site (or bowrain site) should still build with `onBrokenLinks: throw` after this skill runs. If the generated MDX introduces broken links, the skill should warn and not commit them.
