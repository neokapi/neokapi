import type { Rule, Node } from "@oxlint/plugins";

/**
 * Flags `<p>{'Hello'}</p>` — a bare string literal inside a JSX
 * expression container. This looks extractable but isn't: the
 * kapi-react transform walks JSXText nodes, not expression containers
 * with string literals inside them.
 *
 * A developer who writes this usually meant `<p>Hello</p>` (trivially
 * extractable) or meant to interpolate with `t()` but forgot.
 */
export const rule: Rule = {
  meta: {
    type: "problem",
    docs: {
      description:
        "flag string literals wrapped in JSX expression containers — not extractable, probably unintended",
      recommended: true,
    },
    schema: [],
    fixable: "code",
    messages: {
      bareLiteral:
        "String literal inside a JSX expression container is not extractable. Write it as JSX text (`<p>Hello</p>`) or wrap with `t()` if it must be an expression.",
    },
  },
  create(context) {
    return {
      JSXExpressionContainer(node: Node) {
        const container = node as unknown as {
          parent: { type: string };
          expression: { type: string; value?: unknown; raw?: string };
        };
        // Only flag inside JSX *children*, not JSX attributes.
        if (container.parent?.type !== "JSXElement" && container.parent?.type !== "JSXFragment") {
          return;
        }
        const expr = container.expression;
        if (!expr) return;
        if (expr.type !== "Literal") return;
        if (typeof expr.value !== "string") return;
        if (expr.value.trim() === "") return;
        context.report({
          node: node as unknown as Node,
          messageId: "bareLiteral",
          fix(fixer) {
            // Strip the quotes, keep the text. JSX text is literal
            // so this is a safe transform as long as it has no
            // newlines or braces (rare in this pattern).
            if (typeof expr.value !== "string") return null;
            if (/[\r\n{}]/.test(expr.value)) return null;
            return fixer.replaceText(node, expr.value);
          },
        });
      },
    };
  },
};
