# Type scale

Kapi Desktop and the Bowrain platform had grown a text size for every occasion,
from `text-[8px]` badges to `text-xl` titles, with no system holding them
together. A page read as busy because its labels, captions and values each sat
at a slightly different size. This file names the small set of steps a surface
draws from, so text of the same rank reads at the same size wherever it appears.

## The four steps

| Rank | Size | Where it belongs |
| --- | --- | --- |
| Page title | `text-xl` | The one heading a page carries, through `PageHeader`. |
| Section eyebrow | `text-xs` uppercase | The label above a group of settings or a block, through `SectionHeading`. |
| Body | `text-sm` | The content a reader reads: descriptions, values, list rows. |
| Caption | `text-[11px]` | Secondary and dense text: help lines, meta, badges. |

The two headings are components, not loose classes: reach for `PageHeader` and
`SectionHeading` rather than restating their sizes, so a page cannot invent a
fifth heading size by accident.

## Sub-11px is for monospace only

`text-[10px]` and below are reserved for monospace: short code, identifiers and
raw values inside a `font-mono` span, where a smaller glyph still reads and the
density earns its place. Everywhere else the floor is the caption step,
`text-[11px]`. Prose, labels and badges never go below it.
