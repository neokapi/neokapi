/**
 * Printing a bridge as CSS.
 *
 * The output is sorted, carries no timestamp and no path outside the repo, and
 * every number in it comes from the canonical palette, so a second run of the
 * generator reproduces the file byte for byte and `make check-docs-palette` can
 * gate on that.
 */

import type { Bridge, Declaration, ThemeBridge } from "./derive.ts";

/** What a target file is for, which decides the selectors it is written under. */
export type BridgeKind = "site" | "diagram";

export interface RenderOptions {
  kind: BridgeKind;
  /** The canonical palette this file was computed from, repo-relative. */
  source: string;
  /** The make target that rewrites it. */
  target: string;
  /** A sentence saying what the file is for, wrapped into the header. */
  summary: string;
}

// Infima declares its own dark surfaces under `html[data-theme="dark"]`, which
// out-specifies a bare `[data-theme="dark"]`: a bridge written that way keeps
// its primary and loses its background to the starter theme. `:root[…]` carries
// one more class-level weight and wins.
const SITE_SELECTORS = {
  light: [":root"],
  dark: [':root[data-theme="dark"]'],
} as const;

// The diagram tokens are declared on `.kdx` by the kit itself, so a site's own
// values have to out-specify that rather than merely follow it: CSS load order
// between a Docusaurus `customCss` file and a component's own import is not
// something either site controls.
const SITE_DIAGRAM_SELECTORS = {
  light: [":root .kdx"],
  dark: [':root[data-theme="dark"] .kdx'],
} as const;

// The kit's own defaults, which also cover Storybook: it marks dark mode with a
// class on the document element rather than with a data attribute.
const KIT_DIAGRAM_SELECTORS = {
  light: [".kdx"],
  dark: [".dark .kdx", '[data-theme="dark"] .kdx'],
} as const;

/**
 * The repo formatter's line width. A generated file is a tracked file, so `vp
 * fmt --check` reads it like any other and would rewrite a long declaration the
 * moment it landed. Printing it already wrapped keeps the tree a fixed point of
 * both the formatter and the drift gate; `make check-fmt-fixed-point` is what
 * says so.
 */
const PRINT_WIDTH = 100;

/**
 * One declaration, wrapped the way the formatter wraps a long value: the value
 * moves to its own indented run of lines, filled greedily on its commas.
 */
export function declaration(name: string, value: string, indent = 2): string {
  const pad = " ".repeat(indent);
  const oneLine = `${pad}--${name}: ${value};`;
  if (oneLine.length <= PRINT_WIDTH) return oneLine;

  const inner = " ".repeat(indent + 2);
  const parts = value.split(",").map((part) => part.trim());
  const lines: string[] = [];
  let current = "";
  for (const part of parts) {
    const candidate = current === "" ? inner + part : `${current}, ${part}`;
    if (current !== "" && candidate.length > PRINT_WIDTH) {
      lines.push(`${current},`);
      current = inner + part;
    } else {
      current = candidate;
    }
  }
  lines.push(`${current};`);
  return [`${pad}--${name}:`, ...lines].join("\n");
}

function block(selectors: readonly string[], declarations: Declaration[]): string {
  if (declarations.length === 0) return "";
  const body = declarations.map((d) => declaration(d.name, d.value)).join("\n");
  return `${selectors.join(",\n")} {\n${body}\n}\n`;
}

function header(options: RenderOptions): string {
  return [
    "/*",
    ` * ${options.summary}`,
    " *",
    ` * GENERATED FILE, DO NOT EDIT. Computed from ${options.source}`,
    ` * by packages/docs-palette. Run \`make ${options.target}\` after editing that palette;`,
    " * `make check-docs-palette` fails on a stale copy.",
    " */",
    "",
  ].join("\n");
}

function themeBlocks(kind: BridgeKind, theme: "light" | "dark", bridge: ThemeBridge): string[] {
  if (kind === "diagram") return [block(KIT_DIAGRAM_SELECTORS[theme], bridge.diagram)];
  return [
    block(SITE_SELECTORS[theme], bridge.site),
    block(SITE_DIAGRAM_SELECTORS[theme], bridge.diagram),
  ];
}

/** The whole file, ending in a newline. */
export function renderBridge(bridge: Bridge, options: RenderOptions): string {
  const parts = [
    header(options),
    ...themeBlocks(options.kind, "light", bridge.light),
    ...themeBlocks(options.kind, "dark", bridge.dark),
  ].filter((part) => part !== "");
  return parts.join("\n");
}
