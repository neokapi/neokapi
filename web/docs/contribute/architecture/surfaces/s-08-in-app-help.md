---
id: s-08-in-app-help
sidebar_position: 8
title: "S-08: In-app help"
description: "Kapi Desktop's help is the docs, read in the app. A help topic is a section of a docs page, cut from the page's source at build time, opened from an info button beside the title of the page it explains, and linked to the same section on the website. Tours are pulled, never pushed."
keywords: [neokapi, architecture decision, help, help panel, guided tour, contextual help, progressive disclosure, single source, docs, Kapi Desktop]
---

# S-08: In-app help

## Summary

Kapi Desktop explains itself from one source: the docs. A help topic in the
app is a section of a docs page, read from that page's MDX when the app is
built. The Help panel and the website show the same text, a docs change
reaches both, and a topic's last line opens the same section on the website,
in the page around it.

Help reaches the reader where the question comes up. An info button beside a
page's title opens the panel on the topic for that page, a link inside a topic
opens the next topic in the panel, and tours light the controls a task uses.
Nothing starts on its own.

The design follows two published bodies of practice. The help system pattern
of AWS's Cloudscape design system describes a ramp of content from the words
on the page, to a help panel opened by an info link beside a heading, to the
full documentation. Nielsen Norman Group's research on onboarding separates
*push* revelations, shown whatever the user is doing and widely skipped, from
*pull* revelations, shown when the user asks or the task calls for them, and
recommends the second. The [references](#references) list both.

## The content ramp

Each step down the ramp holds more and asks more of the reader. A reader
takes the next one only when the one before was not enough.

| Step | What it holds | Where it sits | Source |
| --- | --- | --- | --- |
| Words on the page | Labels, counts, an empty state's one line, a setting's tooltip | On the control | The app's string catalog |
| Info button | Opens the Help panel on the topic for the page | Beside the page title, through `PageHeader`'s `help` | The topic id |
| Help panel topic | What the page is for, what each part does, the decision it asks for | A sheet at the right of the window | A docs section |
| Guided tour | The controls one task uses, lit in turn, with a short note | Over the window | The app's string catalog |
| The website | The whole page, the recipes and the reference | The system browser | The docs |

The page patterns guide (`packages/ui/docs/page-patterns.md`) holds the rule
for the first step: a page explains itself through structure and its empty
states, and a sentence under a heading moves into the topic.

## A topic is a docs section

The app names its topics; the docs hold their text.

- **The registry.** `apps/kapi-desktop/frontend/src/guide/topics.ts` lists
  each topic as an id, the heading id of its section, and the tours that walk
  through it. The topics are sections of one page,
  `web/docs/kapi/desktop/overview.mdx`, the page that says where each part of
  the app is. A section the app embeds carries an explicit heading id
  (`## Decide {#decide}`), so rewording a heading changes neither the
  website's anchor nor the app's topic. A comment at the top of the page says
  which headings the app reads.
- **The import.** The app imports the page with Vite's `?raw` query, so the
  source is in the bundle and no request leaves the machine to show a topic.
  The development server, the test runner and Storybook resolve the import the
  same way the release build does.
- **The cut.** `topicsFromDocs` in `@neokapi/ui-primitives`
  (`packages/ui/src/components/guide/docs-topics.ts`) reads a section from its
  heading to the next heading at the same level or above, and keeps its
  subsections. The heading is the topic's title. The first sentence of the
  first paragraph, without its markup, is the line the topic list shows under
  the title, so an embedded section opens with a sentence that says what the
  part is for.
- **What it leaves out.** MDX comments and the import lines. Screenshots and
  videos (`ThemedImage`, `ThemedVideo`) are dropped on purpose: the reader has
  the screen they show in front of them. Any other component is reported, and
  the test suite fails on a report, because the panel renders Markdown and a
  component it cannot draw would vanish without a trace.
- **Links.** A link to another embedded section of the same page becomes a
  link to that topic and opens it in the panel. A root-relative link becomes
  an absolute link to the site. The panel hands every link to the app's
  `onOpenUrl`, which opens the system browser, so no link navigates the
  webview away from the app.
- **The way out.** The topic's last line, **Open on the website**, opens the
  section's own anchor on the site. The reader lands on the text they just
  read, with the rest of the page around it.

The app is built from a release tag, and the stable docs are built from the
same tag, so the section the app shows and the section the link opens are the
same text.

### Translation

A topic is translated with its page. The docs translation writes the
translated page to `web/i18n/<locale>/docusaurus-plugin-content-docs/current/`
and keeps the explicit heading ids. The app picks up each translated copy of
the page present when it is built and reads the same sections from it. A
topic the translated page does not have yet shows the English section, and a
translated topic's links stay on that language's site. The translated pages
are build output, so a build without them shows every topic in English, and
nothing fails: target-language drift never blocks a build.

### What stays in the app

Two kinds of text stay in the app's string catalog, because each is bound to a
control rather than to a subject:

- **Microcopy.** A label, a tooltip or an empty state's line describes the
  control it sits on, and changes when the control does.
- **Tour steps.** A step names a control by its `data-slot` and says what it
  does in a sentence or two. A docs page has no reason to know which element
  draws the control. Each tour names the topic that explains its subject, and
  the topic lists the tours that walk through it, so the two meet in the
  panel.

## Pull, not push

A tour starts only when the reader starts one: from **Start** in the Help
panel, from a step of **Get started** on the home, or from **Guide** on a
project's first steps. No tour opens when the app launches, when a project
opens, or after an update. **Get started** is the one card that offers help
unasked, and **Hide** removes it for good, with **Show it again** in App
Settings to bring it back.

A tour's note sits beside the control it explains and leaves the control in
view. **Next**, **Back**, Esc and the × move through or end it. A step whose
control the page does not draw, such as a line that appears only for some
suggestions, is passed over rather than shown with nothing lit. A finished
tour is marked in the panel and on **Get started**; a skipped one is not.

## Help for a downstream product

These docs describe kapi and the framework, and nothing built on top of them;
the repo's docs guards hold them to that. The Help panel reads these docs, so
it has no topic about a product built on kapi. Where the app offers one, as
the Share page offers a managed workspace, the action opens that product's
own docs in the browser. Each product's docs are the single source for it,
and an import from a downstream docs tree into the Apache app would also
cross the licence boundary the module audit enforces.

## Guards

- `apps/kapi-desktop/frontend/src/__tests__/guides.test.ts` fails when a
  topic names a section the page does not have, when a section holds a
  component the panel cannot draw, when a link in a topic names a topic or a
  docs page that does not exist, when a page title names a topic the registry
  lacks, and when a tour step points at a `data-slot` the frontend never
  draws.
- `packages/ui/src/__tests__/docs-topics.test.ts` covers the cut: section
  bounds, subsections, fenced code, dropped and reported components, link
  rewriting and the summary line.
- `packages/ui/src/__tests__/Guide.test.tsx` covers the panel and the tours,
  including that a link in a topic opens another topic or goes out through
  `onOpenUrl`, and that an info button for a topic the provider lacks draws
  nothing.

A heading id renamed in a docs-only change fails the desktop's tests on the
next change that runs them, so the comment at the top of the page asks the
writer to keep each id.

## Writing a section the app embeds

- Open with one sentence that says what the part is for; the topic list
  shows it.
- Name controls in bold, as they read on screen.
- Keep the section readable on its own. Link to a sibling section by its
  anchor, which opens it in the panel, rather than writing "above" or
  "below".
- Use Markdown, a table, a list or a code block. A screenshot is fine and
  shows only on the website. Any other component belongs outside the
  embedded sections.
- Keep the heading id when you reword the heading.

## References

- Cloudscape Design System, [Help system](https://cloudscape.design/patterns/general/help-system/):
  info links beside headings, the help panel, learn-more links, and the
  progressive ramp from page text to documentation.
- Nielsen Norman Group, [Onboarding tutorials vs. contextual
  help](https://www.nngroup.com/articles/onboarding-tutorials/): push and
  pull revelations, and why tutorials shown up front are skipped.
- Nielsen Norman Group, [Help and
  documentation](https://www.nngroup.com/articles/help-and-documentation/):
  the tenth usability heuristic, and help that is searchable, task-focused and
  shown in context.
- [S-02: Kapi Desktop](s-02-kapi-desktop.md) for the desktop app the help
  sits in.
