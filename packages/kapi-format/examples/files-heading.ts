/**
 * Example 1 — FilesHeading
 *
 * Exercises: nested inline <span> element (pcOpen/pcClose pair),
 * {count} variable (ph run), flat runs sequence with a paired code
 * wrapping text + placeholder + text.
 *
 * Source:
 *   export function FilesHeading({ count }: { count: number }) {
 *     return (
 *       <h2>
 *         Files
 *         <span className="muted">({count} matched)</span>
 *       </h2>
 *     );
 *   }
 *
 * Translator view (Level-1 preview, rendered via renderBlockHtml):
 *
 *   <kat-block id="files-heading" data-type="jsx:element">
 *     Files <span data-neokapi-span="1">(<span class="neokapi-var"
 *     data-var="count" data-type="number">count</span> matched)</span>
 *   </kat-block>
 *
 * The inline <span class="muted"> is a paired code whose `id` links
 * its open and close runs. The `{count}` variable is a self-closing
 * placeholder run between the two pairs.
 */

import type { Block } from "../src/block.ts";

export const filesHeading: Block = {
  id: "files-heading",
  hash: "2xykvb",
  translatable: true,
  type: "jsx:element",

  source: [
    { text: "Files " },
    {
      pcOpen: {
        id: "1",
        type: "jsx:element",
        subType: "span",
        data: '<span className="muted">',
        equiv: "muted",
        disp: "span",
      },
    },
    { text: "(" },
    {
      ph: {
        id: "2",
        type: "jsx:var",
        subType: "number",
        data: "{count}",
        equiv: "count",
        disp: "count",
      },
    },
    { text: " matched)" },
    {
      pcClose: {
        id: "1",
        type: "jsx:element",
        subType: "span",
        data: "</span>",
        equiv: "muted",
      },
    },
  ],

  placeholders: [
    {
      name: "muted",
      kind: "element",
      sourceExpr: '<span className="muted">...</span>',
      jsType: "ReactNode",
    },
    {
      name: "count",
      kind: "variable",
      sourceExpr: "count",
      jsType: "number",
    },
  ],

  properties: {
    file: "src/FilesHeading.tsx",
    line: 4,
    component: "FilesHeading",
    jsxPath: "FilesHeading > h2",
    element: "h2",
  },

  preview: {
    sampleValues: { count: 3 },
  },
};

/**
 * Expected Level-1 rendering. Hand-computed so the test asserts
 * equality against the reference renderer.
 */
export const filesHeadingExpectedHtml =
  '<kat-block id="files-heading" data-type="jsx:element">' +
  'Files <span data-neokapi-span="1">(' +
  '<span class="neokapi-var" data-var="count" data-type="number">count</span>' +
  " matched)</span>" +
  "</kat-block>";
