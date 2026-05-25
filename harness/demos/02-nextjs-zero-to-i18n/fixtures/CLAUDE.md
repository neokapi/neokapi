# Lumen Notes

A Next.js (App Router, TypeScript) notes app. UI strings are plain English text in
the JSX markup — no message IDs. The base dependencies are installed (`npm install`
has been run).

## Locale preview convention

A locale must be previewable via a `?lang` query parameter: `/?lang=ja` renders the
app in Japanese, and `/` (no `?lang`) stays English.
