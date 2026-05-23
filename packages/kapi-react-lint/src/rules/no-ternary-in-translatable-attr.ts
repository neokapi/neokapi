import type { Rule, Node } from "@oxlint/plugins";
import { TRANSLATABLE_ATTRS } from "../shared/translatable-attrs.ts";
import { hasTranslateNoAncestor } from "../shared/translate-no.ts";

/**
 * Sibling of `no-concat-in-translatable-attr`. Flags a translatable
 * attribute whose value is a conditional expression where at least
 * one branch is NOT a plain string literal — e.g.
 *
 *   <Input placeholder={disabled ? getLabel() : "Type…"} />
 *
 * The kapi-react extractor handles the all-string case
 * (`cond ? "A" : "B"`) by emitting one Block per branch; any other
 * branch shape is un-extractable and silently bypasses translation.
 *
 * The guidance is to lift the strings to be the direct branch values
 * (so the extractor can see them) or use `t()` explicitly.
 */
export const rule: Rule = {
  meta: {
    type: "problem",
    docs: {
      description:
        "reject translatable-attribute conditionals whose branches aren't both string literals — the extractor can't see them",
      recommended: true,
    },
    schema: [],
    messages: {
      mixed:
        'Attribute `{{attr}}` uses a conditional whose branches aren\'t both plain string literals — can\'t be extracted. Lift strings to the branches (`cond ? "A" : "B"`) or wrap with `t("key")` explicitly.',
    },
  },
  create(context) {
    return {
      JSXAttribute(node: Node) {
        const attr = node as unknown as {
          name: { type: string; name?: string };
          value: unknown;
          parent?: unknown;
        };
        const attrName = readAttrName(attr.name);
        if (!attrName || !TRANSLATABLE_ATTRS.has(attrName)) return;
        const value = attr.value as
          | {
              type: string;
              expression?: { type: string; consequent?: unknown; alternate?: unknown };
            }
          | null
          | undefined;
        if (!value || value.type !== "JSXExpressionContainer") return;
        const expr = value.expression;
        if (!expr || expr.type !== "ConditionalExpression") return;

        const cBranch = isStringLiteral(expr.consequent);
        const aBranch = isStringLiteral(expr.alternate);
        // All-literal case is handled by the extractor — no warning.
        if (cBranch && aBranch) return;
        // All-non-literal case is something else entirely (computed
        // values, `t()` calls, etc.) — also skip; likely intentional.
        if (!cBranch && !aBranch) return;

        // Respect W3C `translate="no"` on the element itself or any
        // JSX ancestor — same semantics as the extractor skip rule.
        if (hasTranslateNoAncestor(attr.parent)) return;

        context.report({
          node: node as unknown as Node,
          messageId: "mixed",
          data: { attr: attrName },
        });
      },
    };
  },
};

function readAttrName(name: { type: string; name?: string }): string | null {
  if (name.type === "JSXIdentifier") return name.name ?? null;
  return null;
}

function isStringLiteral(node: unknown): boolean {
  if (!node || typeof node !== "object") return false;
  const n = node as { type?: string; value?: unknown };
  return n.type === "Literal" && typeof n.value === "string";
}
