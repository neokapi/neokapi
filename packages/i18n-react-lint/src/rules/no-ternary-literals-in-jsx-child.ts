import type { Rule, Node } from "@oxlint/plugins";
import { hasTranslateNoAncestor } from "../shared/translate-no.ts";

/**
 * Flags a template-literal branch in a JSX-child ternary, as in
 * ``<X>{cond ? `Loading ${n}...` : "Done"}</X>``. The extractor reads a
 * template as one opaque expression, so the words inside it never reach the
 * catalog.
 *
 * Fix: wrap the branch with `t()` and pass the interpolation as a parameter —
 * `` cond ? t("Loading {n}...", { n }) : "Done" ``.
 *
 * A plain string-literal branch is extracted as its own block (#2581), so it is
 * not flagged; `<Button>{saving ? "Saving..." : "Save"}</Button>` is two
 * messages the way it stands.
 *
 * Ignores:
 * - Branches that are neither templates nor strings (computed values, `t()`
 *   calls, React elements).
 * - Templates carrying no words, like `` `${pct}%` `` — formatting, not copy.
 * - Elements with `translate="no"` on any ancestor.
 */
export const rule: Rule = {
  meta: {
    type: "problem",
    docs: {
      description:
        "flag ternary with template-literal branches in JSX children — the extractor reads a template as one opaque expression; wrap the branch with t()",
      recommended: true,
    },
    schema: [],
    messages: {
      literalBranch:
        'Ternary branch {{text}} renders as JSX text, and the extractor reads a template literal as one opaque expression, so its words never get translated. Wrap it with t() and pass the interpolation as a parameter (e.g. `t("Loading {n}...", { n })`).',
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

        const cKind = templateKind(expr.consequent);
        const aKind = templateKind(expr.alternate);
        // Warn only for a template-literal branch. A string literal is
        // extracted on its own, and anything else (a t() call, an element, a
        // computed value) is deliberate.
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

function templateKind(node: unknown): "template" | null {
  if (!node || typeof node !== "object") return null;
  const n = node as {
    type?: string;
    value?: unknown;
    quasis?: { value?: { raw?: string; cooked?: string } }[];
  };
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
