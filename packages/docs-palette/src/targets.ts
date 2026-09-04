/**
 * What the generator writes, and what each file is computed from.
 *
 * Two brands, one derivation. Each documentation site bridges its own brand
 * palette into Infima's variables, and the diagram kit carries the framework
 * brand as its built-in default so a story renders the same colours as the page
 * it is documenting.
 *
 * Paths are repo-relative and are the only place a path is written down.
 */

import type { RenderOptions } from "./render.ts";

export interface Target extends Omit<RenderOptions, "target"> {
  /** Where the generated file goes, repo-relative. */
  out: string;
}

export const MAKE_TARGET = "generate-docs-palette";

export const TARGETS: readonly Target[] = [
  {
    kind: "site",
    source: "packages/ui/src/styles/kapi-colors.css",
    out: "web/src/css/palette.generated.css",
    summary: 'The kapi documentation site in "Kapi Blue".',
  },
  {
    kind: "site",
    source: "packages/ui/src/styles/theme-colors.css",
    out: "bowrain/web/docs/src/css/palette.generated.css",
    summary: 'The Bowrain documentation site in "Rainlight".',
  },
  {
    kind: "diagram",
    source: "packages/ui/src/styles/kapi-colors.css",
    out: "packages/docs-shared/src/diagram/palette.generated.css",
    summary: "Default colours for the shared documentation diagram kit.",
  },
];
