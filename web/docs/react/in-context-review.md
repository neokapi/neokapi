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

## Head and SEO strings

The page title, meta description, and Open Graph / Twitter card strings carry no rendered text node, so there is nothing on the page to hold ALT and click — yet they are often the strings a search result or a shared link shows first. Translate them through the head hooks and they become reviewable in a panel of their own.

```tsx
import {
  useTranslatedTitle,
  useTranslatedDescription,
  useTranslatedMeta,
} from "@neokapi/i18n-react/head";

function Head() {
  useTranslatedTitle("Dashboard · Acme");
  useTranslatedDescription("Manage your team, projects, and billing.");
  useTranslatedMeta("Dashboard · Acme", { property: "og:title" });
  return null;
}
```

Each hook translates its source through the runtime dictionary — the same lookup `t()` uses — sets the real head element, and registers the string for review. Nothing scrapes arbitrary head markup; only strings the pipeline translated appear.

A **head/SEO** button appears in the review toolbar whenever a page has such strings. It opens a panel listing each one — its slot (`title`, `description`, `og:title`, …), the source, and the current target — with the same editable field and save the inline panel uses. An edit writes back through the same `.klf` file and repaints the head in place. When a page registers no head strings, the button stays hidden.

Head translation resolves through the runtime dictionary, so it applies in OTA / [runtime mode](./modes). Identically-worded slots — a `<title>` and its `og:title` — share one translation.

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
