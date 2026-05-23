import type { Rule } from "@oxlint/plugins";
import { collectTLocalNames } from "../shared/t-import.ts";

/**
 * Flags `t(variable)` / `t(someExpr)` — the first argument to `t()`
 * must be a string literal so the extractor can see it at build time.
 *
 * Allows template literals **only** if they have no expressions
 * (`t(\`Hello world\`)` is fine; `t(\`Hello ${name}\`)` is not — that
 * case is caught by `t-no-concat`).
 */
export const rule: Rule = {
  meta: {
    type: "problem",
    docs: {
      description:
        "require `t()` first argument to be a string literal so kapi-react can extract it",
      recommended: true,
    },
    schema: [],
    messages: {
      notLiteral:
        "`t()` first argument must be a string literal — kapi-react reads it statically at build time. Use a literal, or compute the value elsewhere.",
      emptyString:
        '`t("")` — empty string has nothing to translate. Remove the call or pass real source text.',
    },
  },
  create(context) {
    const tNames = collectTLocalNames(context);
    if (tNames.size === 0) return {};
    return {
      CallExpression(node) {
        if (node.callee.type !== "Identifier") return;
        if (!tNames.has(node.callee.name)) return;
        const first = node.arguments[0];
        if (!first) return;
        if (first.type === "Literal" && typeof first.value === "string") {
          if (first.value === "") {
            context.report({ node: first, messageId: "emptyString" });
          }
          return;
        }
        if (first.type === "TemplateLiteral" && first.expressions.length === 0) {
          if (first.quasis[0]?.value.cooked === "") {
            context.report({ node: first, messageId: "emptyString" });
          }
          return;
        }
        // t-no-concat owns these cases — avoid duplicate diagnostics.
        if (first.type === "BinaryExpression" && first.operator === "+") return;
        if (first.type === "TemplateLiteral" && first.expressions.length > 0) return;
        context.report({ node: first, messageId: "notLiteral" });
      },
    };
  },
};
