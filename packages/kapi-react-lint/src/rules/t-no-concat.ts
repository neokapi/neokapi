import type { Rule } from "@oxlint/plugins";
import { collectTLocalNames } from "../shared/t-import.ts";

/**
 * Flags `t('Hello ' + name)` and `` t(`Hello ${name}`) `` — neither
 * extracts because the runtime value isn't visible at build time.
 *
 * Paired with `t-literal-first-arg`: that rule fires for bare
 * expressions, this one fires for concatenations that *look* like
 * strings but aren't a single literal.
 */
export const rule: Rule = {
  meta: {
    type: "problem",
    docs: {
      description:
        "reject string concatenation and template expressions as the first argument of `t()`",
      recommended: true,
    },
    schema: [],
    messages: {
      concat:
        '`t()` cannot translate concatenated strings — the extractor only sees a single literal. Use a placeholder: `t("Hello {name}")` and bind values at render.',
      template:
        '`t()` cannot translate template literals with interpolations. Use a placeholder: `t("Hello {name}")` and bind values at render.',
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
        if (first.type === "BinaryExpression" && first.operator === "+" && looksStringLike(first)) {
          context.report({ node: first, messageId: "concat" });
          return;
        }
        if (first.type === "TemplateLiteral" && first.expressions.length > 0) {
          context.report({ node: first, messageId: "template" });
        }
      },
    };
  },
};

/**
 * Heuristic: either side is a string-producing node. Keeps the rule
 * specific to the "developer was trying to build a translatable
 * message" case and avoids flagging `t(1 + 2)` which `t-literal-first-arg`
 * already catches.
 */
function looksStringLike(node: {
  type: string;
  left: { type: string; value?: unknown };
  right: { type: string; value?: unknown };
}): boolean {
  return isStringish(node.left) || isStringish(node.right);
}

function isStringish(node: { type: string; value?: unknown }): boolean {
  if (node.type === "Literal" && typeof node.value === "string") return true;
  if (node.type === "TemplateLiteral") return true;
  return false;
}
