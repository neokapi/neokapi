/**
 * W3C `translate="no"` is the standard hint for "don't try to
 * translate this subtree". kapi-react's extractor already respects
 * it (walker.ts early-returns from the element), and the lint rules
 * should too — otherwise a developer using the HTML-standard escape
 * hatch still gets noisy warnings for dynamic / code-like content.
 *
 * The helper walks up through the parent chain from an arbitrary
 * starting node (attribute, expression container, whatever) and
 * returns true as soon as it finds a JSX ancestor with the attribute.
 */
export function hasTranslateNoAncestor(start: unknown): boolean {
  let cursor = start as
    | {
        type?: string;
        parent?: unknown;
        // ESLint (acorn-jsx) calls it `openingElement`; SWC calls it
        // `opening`. Accept both so the helper works under either
        // AST provider.
        openingElement?: unknown;
        opening?: unknown;
      }
    | undefined;
  while (cursor) {
    if (cursor.type === "JSXElement") {
      const open = cursor.openingElement ?? cursor.opening;
      if (hasTranslateNoAttr(open)) return true;
    }
    cursor = cursor.parent as typeof cursor;
  }
  return false;
}

function hasTranslateNoAttr(opening: unknown): boolean {
  const attrs = (opening as { attributes?: unknown[] } | undefined)?.attributes;
  if (!Array.isArray(attrs)) return false;
  for (const a of attrs) {
    const attr = a as {
      type?: string;
      name?: { type?: string; name?: string };
      value?: { type?: string; value?: unknown };
    };
    if (attr.type !== "JSXAttribute") continue;
    if (attr.name?.type !== "JSXIdentifier" || attr.name.name !== "translate") continue;
    const v = attr.value;
    if (v?.type === "Literal" && v.value === "no") return true;
  }
  return false;
}
