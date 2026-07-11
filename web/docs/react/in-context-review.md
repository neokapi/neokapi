---
sidebar_position: 10
title: In-Context Review
description: Review and fix translations on the running React app — ALT+click any string to see its source and edit its target, with terms and QA findings painted onto the live text.
keywords: [in-context review, translation review, live editing, QA, terminology, CSS Custom Highlight API, kapi-react]
---

# In-context review

A translator reviewing a spreadsheet of strings cannot see that "Save" landed on a button 40 pixels wide, that the German for "Cancel" now wraps to two lines, or that the tone is wrong for a destructive-action dialog. Context is exactly what the file format threw away.

Review mode puts the review back on the app. Run the dev server, hold ALT, and every translatable string on the page is live: hover to see it outlined, click to open its source, its current target, and any terminology or QA findings against it. Edit the target, save, and the app repaints — and the change lands in the `.klf` file on disk, as a line in a diff you can read and commit.

## Turning it on

```ts title="vite.config.ts"
neokapi({
  review: true, // or set KAPI_REVIEW=1
  reviewKlfDir: "i18n", // the KLF tree to serve and write back to (default)
});
```

Three things switch on together:

1. **The transform stamps the DOM.** Every extracted element gets `data-kapi-id` (its block hash), `data-kapi-loc` (`file:line`), and `data-kapi-attr` for attribute blocks. This is what maps a pixel on screen back to a block on disk.
2. **The dev server mounts `/__kapi/review`.** It reads and writes your local KLF tree — `GET /{hash}` for a block's payload, `PUT /{hash}` to write a target, `GET /annotations` for stand-off findings, and an SSE stream so several open tabs stay in sync.
3. **The overlay is injected** into `index.html`, so there is nothing to import and nothing to remember to remove.

:::warning Dev and staging only
Review mode stamps hashes and source locations into your DOM and mounts a middleware that writes to disk. Never enable it for a production build you ship.
:::

## Using it

Hold **ALT** and the page becomes reviewable — hovering outlines any translatable element. **ALT+click** opens the panel:

- **Source** — the block's source runs, with inline elements and placeholders shown as the translator sees them.
- **Target** — the current translation, editable.
- **Note** — the `data-i18n-note` or `t()` context, if the developer left one.
- **Findings** — terminology and QA annotations attached to this block.

Saving writes the target into the `.klf` and repaints the running app immediately. You are not looking at a preview of what the change would do; you are looking at the change.

## Terms and QA, painted on the live text

Point kapi's checks at the same KLF tree and their findings become visible *on the rendered page*:

```bash
kapi exec qa i18n/ --target-lang de
kapi exec term-check i18n/ --target-lang de --termbase de-termbase.csv
```

Both write stand-off annotation files (`*.klfl`) next to the blocks they describe. The overlay reads them and highlights the exact character ranges — a glossary term that wasn't used, a placeholder that went missing, a target that runs 60% longer than the source and is about to break the button it lives in.

The highlighting uses the [CSS Custom Highlight API](https://developer.mozilla.org/en-US/docs/Web/API/CSS_Custom_Highlight_API): ranges are painted by the browser's text renderer without inserting a single node into the DOM. React's tree is untouched, so nothing you see is an artifact of being observed — no wrapper spans changing layout, no re-render loops, no hydration mismatches. Turn the overlay off and the DOM is byte-identical to what it was.

## Why write back to the `.klf`

The obvious design is a review database: comments, states, an approve button. Review mode deliberately does the boring thing instead and edits the file.

The `.klf` tree is already the contract between developers and translators — it's what `extract` produces, what `kapi translate` fills in, what QA reads. Writing a reviewed target back into it means the review is a **git diff**: reviewable in a PR, revertable, attributable, and picked up by the very next `kapi-react compile` with no sync step. A reviewer's fix travels the same path as a translator's, because it is the same artifact.

It also means review works with no server, no account, and no network — `git clone && vp dev` and you are reviewing.

## Reviewers without a checkout

The overlay talks to an endpoint, and the local KLF middleware is just the endpoint that needs no infrastructure. Point `initKapiReview({ endpoint })` at a different one — on a staging deployment, say — and the same stamping and the same block hashes let a reviewer with no repository, no toolchain, and no local server review the app in place. What changes is where the target is written; the client contract does not.

## Next

- [Translating with kapi](./translating-with-kapi) — where the terminology and QA findings come from.
- [Configuration](./configuration#review--reviewklfdir) — the `review` and `reviewKlfDir` options.
- [Pipeline](./pipeline) — how a reviewed `.klf` flows back into the app.
