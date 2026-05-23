import type { Context } from "@oxlint/plugins";
import type { ImportDeclaration, Node, Program } from "estree";

/**
 * Walks an ESTree program and returns the set of local identifier
 * names bound to the `t` function from `@neokapi/kapi-react/runtime`.
 *
 * Handles:
 *   import { t } from '@neokapi/kapi-react/runtime'
 *   import { t as translate } from '@neokapi/kapi-react/runtime'
 *
 * Returns a Set because a file could in principle import `t` twice
 * under different aliases, and we want to catch all of them.
 */
export function collectTLocalNames(context: Context): Set<string> {
  const names = new Set<string>();
  // Both ESLint and oxlint hand JS rules an ESTree-shaped AST at runtime.
  // The oxlint types model the oxc AST, so bridge to ESTree node types here.
  const program = context.sourceCode.ast as unknown as Program;
  for (const stmt of program.body) {
    if (!isRuntimeImport(stmt)) continue;
    for (const spec of stmt.specifiers) {
      if (
        spec.type === "ImportSpecifier" &&
        spec.imported.type === "Identifier" &&
        spec.imported.name === "t"
      ) {
        names.add(spec.local.name);
      }
    }
  }
  return names;
}

function isRuntimeImport(node: Node): node is ImportDeclaration {
  if (node.type !== "ImportDeclaration") return false;
  const src = node.source.value;
  return (
    typeof src === "string" &&
    (src === "@neokapi/kapi-react/runtime" || src === "@neokapi/kapi-react")
  );
}
