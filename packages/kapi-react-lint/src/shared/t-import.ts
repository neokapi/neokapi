import type { Rule } from "eslint";
import type { ImportDeclaration, Node } from "estree";

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
export function collectTLocalNames(context: Rule.RuleContext): Set<string> {
  const names = new Set<string>();
  const program = context.sourceCode.ast;
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
