/**
 * How a source file is handed to SWC.
 *
 * This lived in two places — the extractor's walker and the plugin's transform
 * — and both spelled it `filename.endsWith(".tsx") ? "typescript" :
 * "ecmascript"`. A `.ts` file was therefore parsed as plain ECMAScript, its
 * first type annotation threw, and the file was skipped. Fixing one copy moved
 * nothing: the labels were extracted into the dictionary and then rendered in
 * the source language, because the half that reads a file and the half that
 * rewrites it disagreed about what a TypeScript file is.
 *
 * One function now answers it for both, which is the only arrangement in which
 * they cannot drift apart again.
 */

/** SWC parse options for a file, decided by its extension. */
export function parseSyntaxFor(filename: string): {
  syntax: "typescript" | "ecmascript";
  tsx: boolean;
  jsx: boolean;
} {
  // .ts, .tsx, .mts, .cts are TypeScript; .js, .jsx, .mjs, .cjs are not.
  const isTS = /\.[cm]?tsx?$/.test(filename);
  // JSX is the trailing x, and it matters beyond the parser: in a .ts file
  // `<Foo>x` is a type assertion, and parsing it as JSX changes its meaning.
  const isJSX = filename.endsWith('x');
  return {
    syntax: isTS ? "typescript" : "ecmascript",
    tsx: isTS && isJSX,
    jsx: !isTS && isJSX,
  };
}
