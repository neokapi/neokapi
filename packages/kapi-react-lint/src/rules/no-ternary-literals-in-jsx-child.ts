import type { Rule, Node } from "@oxlint/plugins";
import { hasTranslateNoAncestor } from "../shared/translate-no.ts";

/**
 * Flags `<X>{cond ? "A" : "B"}</X>` (and template-literal variants)
 * where at least one branch is a plain string literal. kapi-react's
 * extractor treats the entire conditional as an opaque `jsx:var`
 * placeholder — neither branch's string is visible to extraction,
 * so both silently bypass translation.
 *
 * Fix: wrap each literal branch with `t()`. The t-call walker
 * extracts `t("A")` / `t("B")` separately and both become
 * translatable.
 *
 * Ignores:
 * - Ternaries whose branches are neither strings nor templates
 *   (computed values, `t()` calls, React elements).
 * - Elements with `translate="no"` on any ancestor.
 */
export const rule: Rule = {
  meta: {
    type: "problem",
    docs: {
      description:
        "flag ternary with string-literal branches in JSX children — extractor treats them as an opaque placeholder; wrap each branch with t()",
      recommended: true,
    },
    schema: [],
    messages: {
      literalBranch:
        'Ternary branches `{{text}}` render as JSX text — the extractor treats the whole conditional as an opaque placeholder, so the string never gets translated. Wrap with t() (e.g. `cond ? t("A") : t("B")`).',
    },
  },
  create(context) {
    return {
      JSXExpressionContainer(node: Node) {
        const container = node as unknown as {
          parent: unknown;
          expression: { type: string; consequent?: unknown; alternate?: unknown };
        };
        // Only JSX children, not attribute values — attributes have
        // their own rule (`no-ternary-in-translatable-attr`).
        const parent = container.parent as { type?: string } | undefined;
        if (parent?.type !== "JSXElement" && parent?.type !== "JSXFragment") return;

        const expr = container.expression;
        if (!expr || expr.type !== "ConditionalExpression") return;

        const cKind = stringyKind(expr.consequent);
        const aKind = stringyKind(expr.alternate);
        // Warn only if at least one branch is a literal string or
        // template literal. All-non-literal (t() calls, elements,
        // computed values) is assumed intentional.
        if (!cKind && !aKind) return;

        if (hasTranslateNoAncestor(container.parent)) return;

        const shown =
          cKind && aKind
            ? `${summary(expr.consequent)} / ${summary(expr.alternate)}`
            : cKind
              ? summary(expr.consequent)
              : summary(expr.alternate);

        context.report({
          node: node as unknown as Node,
          messageId: "literalBranch",
          data: { text: shown },
        });
      },
    };
  },
};

function stringyKind(node: unknown): "literal" | "template" | null {
  if (!node || typeof node !== "object") return null;
  const n = node as {
    type?: string;
    value?: unknown;
    quasis?: { value?: { raw?: string; cooked?: string } }[];
  };
  if (n.type === "Literal" && typeof n.value === "string") return "literal";
  if (n.type === "TemplateLiteral") {
    // Only flag when the template has translatable-looking text —
    // at least one quasi with alphabetic characters. Pure formatting
    // like `${pct}%` or `v${version}` is code-level, not UI copy,
    // and shouldn't be flagged.
    const quasis = n.quasis ?? [];
    const hasWord = quasis.some((q) => /[A-Za-z]{2,}/.test(q.value?.cooked ?? q.value?.raw ?? ""));
    return hasWord ? "template" : null;
  }
  return null;
}

function summary(node: unknown): string {
  if (!node || typeof node !== "object") return "…";
  const n = node as { type?: string; value?: unknown; quasis?: { value?: { raw?: string } }[] };
  if (n.type === "Literal" && typeof n.value === "string") {
    const s = n.value as string;
    return s.length > 24 ? `"${s.slice(0, 24)}…"` : `"${s}"`;
  }
  if (n.type === "TemplateLiteral") return "`…`";
  return "…";
}
