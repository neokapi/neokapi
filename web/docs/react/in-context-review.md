---
sidebar_position: 10
title: In-Context Review
description: Review and fix translations on the running React app — ALT+click any string to see its source and edit its target, with terminology and QA findings marked on the live text.
keywords: [in-context review, translation review, live editing, QA, terminology, neokapi-i18n]
---

# In-context review

A translator working from a file cannot see that "Save" landed on a button 40 pixels wide, that the German for "Cancel" now wraps to two lines, or that the tone is wrong for a dialog that deletes something. Review mode puts the review back on the app: run it, hold ALT, and every translatable string on the page becomes something you can inspect and fix in place.

## Turn it on

```ts title="vite.config.ts"
neokapi({
  review: true, // or set KAPI_REVIEW=1
});
```

Start the dev server. There is nothing to import and no component to add.

:::warning Dev and staging only
Review mode adds review data to your markup and lets the browser write to your translation files. Never enable it for a production build you ship.
:::

## Review a string

Hold **ALT** — hovering now outlines every translatable element. **ALT+click** one to open its panel:

- **Source** — the original text, with its links, bold spans, and placeholders shown the way a translator sees them.
- **Target** — the current translation, editable.
- **Note** — the context the developer left, if any (`data-i18n-note`, or `t(text, context)`).
- **Findings** — terminology and QA issues against this string.

Type a fix, save, and the app repaints immediately. You are not previewing the change; you are looking at it.

The fix is written into the `.klf` file the string came from — so it shows up as a line in `git diff`, travels through review in a pull request like any other change, and is picked up by the next `neokapi-i18n compile` with no extra step. A reviewer's edit takes exactly the same path as a translator's, because it is the same file.

## See terminology and QA on the page

Run kapi's checks against the same `i18n/` directory and their findings appear on the rendered text:

```bash
kapi exec qa i18n/ --target-lang de
kapi exec term-check i18n/ --target-lang de --termbase de-termbase.csv
```

A glossary term that wasn't used, a placeholder that went missing in translation, a string that runs 60% longer than its source and is about to break the button it lives in — each is marked on the words it's about, in the place it happens. Re-run a check while the app is open and the marks update; you don't need to restart anything.

## Reviewers without a checkout

Review talks to an endpoint, and the local one — reading and writing the `.klf` files in your repository — is simply the endpoint that needs no infrastructure. Point it at a different endpoint on a staging deployment and a reviewer with no repository and no toolchain can review the app in place:

```ts
initKapiReview({ endpoint: "https://staging.example.com/review" });
```

## Next

- [Translating with kapi](./translating-with-kapi) — where the terminology and QA findings come from.
- [Configuration](./configuration#review--reviewklfdir) — the `review` and `reviewKlfDir` options.
- [AD-035](/contribute/architecture/035-in-context-review) — how it works underneath, and why.
