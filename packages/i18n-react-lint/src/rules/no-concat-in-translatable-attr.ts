import type { Rule, Node } from "@oxlint/plugins";
import { TRANSLATABLE_ATTRS } from "../shared/translatable-attrs.ts";

/**
 * Flags `<img alt={'Logo ' + brand} />` (and template-literal variants)
 * on any attribute in the translatable set. These attributes are
 * extracted as source strings, so dynamic concatenation produces
 * either missing or broken translations.
 */
export const rule: Rule = {
  meta: {
    type: "problem",
    docs: {
      description:
        "reject string concatenation inside translatable JSX attributes (alt, title, placeholder, aria-label, etc.)",
      recommended: true,
    },
    schema: [],
    messages: {
      concat:
        'Attribute `{{attr}}` is translatable — concatenating values produces non-extractable text. Use a literal with a placeholder pattern, or compute via `t("... {name}")`.',
      template:
        "Attribute `{{attr}}` is translatable — template interpolations are not extractable. Use a literal with a placeholder pattern.",
    },
  },
  create(context) {
    return {
      JSXAttribute(node: Node) {
        const attr = node as unknown as {
          name: { type: string; name?: string };
          value: unknown;
        };
        const attrName = readAttrName(attr.name);
        if (!attrName || !TRANSLATABLE_ATTRS.has(attrName)) return;
        const value = attr.value as
          | {
              type: string;
              expression?: {
                type: string;
                operator?: string;
                left?: unknown;
                right?: unknown;
                expressions?: unknown[];
              };
            }
          | null
          | undefined;
        if (!value || value.type !== "JSXExpressionContainer") return;
        const expr = value.expression;
        if (!expr) return;
        if (expr.type === "BinaryExpression" && expr.operator === "+" && isStringish(expr)) {
          context.report({
            node: node as unknown as Node,
            messageId: "concat",
            data: { attr: attrName },
          });
          return;
        }
        if (expr.type === "TemplateLiteral" && (expr.expressions?.length ?? 0) > 0) {
          context.report({
            node: node as unknown as Node,
            messageId: "template",
            data: { attr: attrName },
          });
        }
      },
    };
  },
};

function readAttrName(name: { type: string; name?: string }): string | null {
  if (name.type === "JSXIdentifier") return name.name ?? null;
  if (name.type === "JSXNamespacedName") return null;
  return null;
}

function isStringish(expr: { type: string; left?: unknown; right?: unknown }): boolean {
  return isStringNode(expr.left) || isStringNode(expr.right);
}

function isStringNode(node: unknown): boolean {
  if (typeof node !== "object" || node === null) return false;
  const n = node as { type?: string; value?: unknown };
  if (n.type === "Literal" && typeof n.value === "string") return true;
  if (n.type === "TemplateLiteral") return true;
  return false;
}
