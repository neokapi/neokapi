# Review in context

Working note, 2026-11-18. How kapi and Bowrain show the context a decision
needs, on every surface, and the plan to close the gaps. The architecture is
[S-07: The review model](../../web/docs/contribute/architecture/surfaces/s-07-context-centric-review.md)
(the container and the code-display rule); this note is the evaluation and the
roadmap behind it.

## The problem

The engine captures where every block lives and what it is, and most review
surfaces threw that away. A mobile card showed one string. Its details sheet
listed data (collection, path, key) and the neighbouring strings as a list,
and it drew a JSX element as `<=m0><serverURL/></=m0>`. A reviewer judging
"Can't reach your server" could not see that it is the heading of an offline
screen above a sentence and a retry button.

Two separate failures:

1. **Context missing.** The surfaces show a string where the reader meets a
   component, a section or a page.
2. **Coded text shown.** Chips and flattened text printed a code's native
   source or an extractor's internal identifier.

## What the engine already knows

| Fact | Where it is recorded | Reaches the server |
| --- | --- | --- |
| React string's component, file, line, element, JSX path | `model.BlockProperties` from the React extractor (`packages/i18n-react`) | yes, as block properties |
| Story id, snapshot path, sample values for a string | `model.BlockPreviewHints` | the type exists; nothing fills it |
| Heading and its level | markdown reader (`Type: "heading"`, `level`), structure annotation | yes |
| Role, page and bounding box | structure and geometry annotations (`core/model/structure.go`) | annotations, partly |
| Reading order | block `position`, set by a push | yes |
| Whole-document preview with each block marked | `format.PreviewBuilder`, `<kat-block>` | as `Item.PreviewHTML` when the reader sends it |
| A component in any locale | `@neokapi/i18n-react` Storybook decorator | in the customer's Storybook |
| Neighbours before and after | `core/review.Context` | yes, every client |

What does not exist: screenshots attached to blocks, a server-side render of a
page in the target language with one block highlighted, and the original file
bytes on the server (removed in #136).

## What each surface shows

| Surface | Before this work | Now |
| --- | --- | --- |
| Kapi Desktop review | queue, five shared cards, and the rendered document opening at the unit with findings marked | unchanged; codes now read by name |
| Bowrain web review page | document-first: the item drawn as a document, findings inline, decisions in an inspector | unchanged; codes now read by name |
| Bowrain web review session | queue with the focused block and the neighbourhood list, no document | the document opened at the block, and before and after |
| Bowrain web editor | visual editor, and a live Storybook preview for collections with a preview URL | unchanged |
| Bowrain mobile deck | one string per card; details sheet with data and a neighbour list | opens on pages (components and documents, reviewed in place); on cards, the context page: the block's component or section laid out by role, decidable in place; opens from the footer or a swipe down |

## The principle

**Decide at the unit a reader understands.** Two kinds of thing are reviewed,
and they want different shapes:

| What is decided | Shape | Why |
| --- | --- | --- |
| A block translation | inside its container: component, section, page | the meaning of a string depends on what surrounds it |
| A term rule, concept, voice rule, coordinate, channel binding | a card, in the digest | a statement about how the project writes, complete in itself |
| A short string whose container adds nothing (an approved label repeated elsewhere) | a card, as the quick path | speed; the container is one swipe away |

The swipe deck stays. It is the right shape for context items and for the
quick path, and the card now opens its container on request instead of
pretending a string is enough.

## The context ladder

A surface shows the richest rung it can reach and falls back a rung when it
cannot (S-07):

1. **Live**: the component in its own runtime with the translation in place
   (Storybook story with the pending catalog; the running app through the
   in-context runtime).
2. **Rendered document**: the format's preview opened at the block.
3. **Laid-out container**: the container's units drawn by role from block
   data. Needs only the store, so a phone can draw it.
4. **Neighbourhood**: before and after, which every client holds.

The mobile app draws rung 3. Rungs 1 and 2 need a WebView, which is a
native module and therefore a new build, not an over-the-air update.

## The code-display rule

A code reads as its human name (from its label, its identifier, or the name
inside `{{name}}`, `${name}`, `%(name)s`) or as the word for its kind, and a
paired code draws its content as marked text with no tags. One implementation
per language (`codeDisplays` in TypeScript, `HumanCodeName` and
`RunsDisplayText` in Go). The mobile chip and the shared web chip accept only
a branded `CodeDisplay`. `scripts/check-code-display.sh` fails when interface
code reads `data`, `equiv` or `disp` outside an allowlist.

## Roadmap

Done (PRs #3571 and the container PR):

- the code-display rule on mobile, web, desktop and the Go display text, with
  the guard;
- `GET …/blocks/:ref/:bid/container`: component, section or window, units with
  roles and revisions;
- the mobile context page and the swipe down;
- the web review session opening the document at the block;
- the mobile deck opening on pages (`GET …/review-deck/:ref/pages`), with a
  document walked section by section.

Next, in the order that pays most per unit of work:

1. **Live component previews on mobile.** Add `react-native-webview` in the
   next native build and load the collection's Storybook story for the
   block's component with the pending translation, as the web `StoryPreview`
   does. Needs the story id per string (item 2).
2. **Fill the preview hints.** Have the React extractor record a story id per
   component when the project has a Storybook (`BlockPreviewHints.StoryID`),
   so a string maps to the story that renders it without a lookup.
3. **Rendered documents on mobile.** With the WebView in place, show the
   item's preview HTML in the target language with the block marked, for
   markdown and HTML items (rung 2).
4. **Review by page on web.** The pages route serves Bowrain web as it serves
   the app.
5. **Screenshots.** Deferred by the founder (2026-11-18). The harness
   already drives real screens; capturing one
   per component and attaching it as `SnapshotPath` gives a still image where
   no live render is reachable (rung 1.5).

## Decisions

Taken by the founder on 2026-11-18:

- The mobile deck opens on pages for block translations; the swipe deck stays
  for context items and as the quick path.
- The WebView ships in the next TestFlight build, for live component previews
  and rendered documents on mobile.
- Screenshots per component wait; for a collection without a Storybook, the
  laid-out container is the ceiling on mobile.
