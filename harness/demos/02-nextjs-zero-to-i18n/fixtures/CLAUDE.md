# Lumen Notes

A Next.js (App Router, TypeScript) notes app. UI strings are plain JSX — the
English text in the markup is the source (no message IDs). The base dependencies
are installed (`npm install` has been run).

## Internationalization

We want to internationalize Lumen Notes with **kapi-react** (the zero-wrapper
`@neokapi/kapi-react` stack — see the kapi skill for the workflow) and add a
**Japanese** translation. kapi-react isn't installed yet — install it
(`npm install -D @neokapi/kapi-react`), then wire it up.

Convention for this app: make a locale previewable via the **`?lang` query
parameter** — `/?lang=ja` should render the app in Japanese, and `/` (no `?lang`)
stays English. Use a client `I18nProvider` that reads `?lang` and calls
`loadTranslations(lang, "/translations/<lang>.json")`.

The compiled translation dictionary is the deliverable — you don't need to run
the dev server, build, or visually verify the app to finish.
