---
sidebar_position: 3
title: Brand tokens and the documentation palette
description: Where the brand colours live, how the documentation sites and the diagram kit are painted from them, and what the drift gate asserts.
---

# Brand tokens and the documentation palette

Two brands ship from this repo, and each one lives in exactly one file under
`packages/ui/src/styles/`:

| File | Brand | Who imports it |
| --- | --- | --- |
| `kapi-colors.css` | Kapi Blue | Kapi Desktop, the Kapi Storybook, the kapi documentation site |
| `theme-colors.css` | Rainlight | The bowrain app shells, landing page and documentation site |
| `semantic-colors.css` | Neither | Imported by both, and by nothing on its own |

`semantic-colors.css` holds the tokens that say what a colour *means* rather
than what a product looks like: the judgement colours (`--success`,
`--warning`, `--info`) and one hue per coordinate axis. An approve button is
the same green on every surface, so those values sit above the brand line. See
`packages/ui/docs/judgement-colours.md`.

Each brand file also declares its faces, `--brand-font-sans`,
`--brand-font-mono` and, where the brand has one, `--brand-font-display`. An
app's `@theme inline` block maps Tailwind's `--font-*` onto them rather than
repeating the stack.

## The bridge

Docusaurus paints from Infima's `--ifm-*` variables and the diagram kit from
its own `--kdx-*` ones. Neither reads OKLCH, so `packages/docs-palette`
computes an sRGB bridge from the brand tokens and commits it:

| Generated file | Computed from |
| --- | --- |
| `web/src/css/palette.generated.css` | `kapi-colors.css` |
| `packages/docs-shared/src/diagram/palette.generated.css` | `kapi-colors.css` |
| `bowrain/web/docs/src/css/palette.generated.css` | `theme-colors.css` |

Each site's `custom.css` imports its generated file and holds only the rules
that are not colours. The diagram kit's file is its built-in default, which is
also what a Storybook story renders; a site shipping a different brand declares
the same `--kdx-*` properties at a higher specificity, so the drawing follows
the page it sits on whatever order the stylesheets load in.

`make generate-docs-palette` rewrites all three. `make check-docs-palette`
regenerates in memory and fails on a stale file; it runs in `make lint`, in
`make pre-push`, and in the Reference Data Drift Gate workflow.

## What is computed, and how

Surfaces cross over directly: the page, the card, the border, the band behind
inline code. Three things are derived.

**The primary ramp.** Infima wants seven steps, `--ifm-color-primary` plus a
`dark`/`darker`/`darkest` and `light`/`lighter`/`lightest` around it. The base
is the brand primary moved along its own lightness axis until it clears WCAG AA
against the tightest of the three grounds it can land on, and the six steps are
fixed multiples of that lightness. A brand primary chosen for a button on a
card is usually too light to be a link on a page, which is what the search
corrects.

**The diagram roles.** `--kdx-io` takes the brand primary; the other five take
a hue each from the semantic tokens, so a role means the same thing on both
sites.

| Role | Hue from | Reads as |
| --- | --- | --- |
| `io` | the brand primary | readers and writers |
| `annotate` | `--axis-brand` | annotators and overlays |
| `translate` | `--axis-language` | translators and targets |
| `check` | `--warning` | checks and enforcement |
| `resource` | `--success` | the content memory and terms |
| `plugin` | `--axis-product` | the plugin system |

All six sit at one lightness and chroma per theme, so a diagram drawing several
of them reads as one picture.

**The faces.** `--ifm-font-family-base` and `--ifm-font-family-monospace` come
from the brand's `--brand-font-*`, and the diagram kit's `--kdx-mono` from the
same monospace stack.

## What the tests assert

`packages/docs-palette` carries its own suite, run against the real palettes:

- Every step of the primary ramp is lighter than the one below it.
- Links, body text, headings, muted text and every diagram accent clear WCAG AA
  (4.5:1) against the page, the card and the code band, in both themes.
- Each committed file matches what its brand tokens render to, and a second
  render is byte-identical to the first.
- The output carries no timestamp and no path from outside the repo, and the
  kapi site's file never names bowrain, which `make check-docs-bowrain-clean` sweeps for.

A generated file is a tracked file, so the repo formatter reads it like any
other. The renderer wraps a long declaration the way the formatter would, which
keeps the tree a fixed point of both `make check-fmt-fixed-point` and the drift
gate.
