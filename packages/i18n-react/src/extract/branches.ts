/**
 * String literals in the branches of a conditional expression sitting in JSX
 * children position.
 *
 * `{saving ? "Saving..." : "Save"}` renders one of two sentences, and neither
 * is JSX text, so nothing carried them into the catalog. An author who wanted
 * them translated had to rewrite the ternary as two conditional elements
 * (#2581). The same held for `{cond && "Folder moved"}` and
 * `{label || "Untitled"}`.
 *
 * A branch is addressed by its slot position in source order, the scheme the
 * ternary attribute path already uses (`::0` / `::1`). Every leaf slot counts,
 * including one holding an expression rather than a literal, so turning a
 * variable branch into a literal later leaves its neighbour's key where it was.
 * A nested conditional contributes its own slots in place of the one it fills,
 * which is what gives `a ? "A" : b ? "B" : "C"` three of them.
 *
 * Both the extract walker and the plugin transform read branches through this
 * one function, so the two cannot disagree on which literals are blocks or on
 * what each one is called.
 */

import type { Expression } from "@swc/core";

/** The operators whose right-hand side is a rendered branch. */
const CONDITIONAL_OPERATORS = new Set(["&&", "||", "??"]);

/** One translatable string literal in a conditional's branch. */
export interface LiteralBranch {
  /** Slot position in source order, from 0. */
  index: number;
  /** The literal's text, as the author wrote it. */
  text: string;
  /** The literal's own span, parentheses excluded. */
  span: { start: number; end: number };
}

interface SpannedNode {
  type: string;
  span: { start: number; end: number };
}

/**
 * Every translatable string literal in `expr`'s branches, in source order.
 * Empty for anything that is not a conditional, so a caller can hand it any
 * expression container's contents.
 */
export function conditionalLiteralBranches(expr: Expression): LiteralBranch[] {
  const root = unwrapParens(expr);
  if (!isConditional(root)) return [];

  const out: LiteralBranch[] = [];
  let slot = 0;

  const takeSlot = (node: Expression): void => {
    const inner = unwrapParens(node);
    if (isConditional(inner)) {
      walkBranches(inner);
      return;
    }
    const index = slot++;
    const spanned = inner as unknown as SpannedNode;
    if (spanned.type !== "StringLiteral") return;
    const { value } = inner as unknown as { value: string };
    if (value.trim() === "") return;
    out.push({ index, text: value, span: spanned.span });
  };

  const walkBranches = (node: Expression): void => {
    const inner = unwrapParens(node);
    if ((inner as unknown as SpannedNode).type === "ConditionalExpression") {
      const cond = inner as unknown as { consequent: Expression; alternate: Expression };
      takeSlot(cond.consequent);
      takeSlot(cond.alternate);
      return;
    }
    // A logical operator: the left side is the test, the right the branch.
    takeSlot((inner as unknown as { right: Expression }).right);
  };

  walkBranches(root);
  return out;
}

/** The context suffix a branch's block descriptor carries. */
export function branchContext(base: string, index: number): string {
  return `${base}::${index}`;
}

function isConditional(expr: Expression): boolean {
  const node = expr as unknown as SpannedNode & { operator?: string };
  if (node.type === "ConditionalExpression") return true;
  return node.type === "BinaryExpression" && CONDITIONAL_OPERATORS.has(node.operator ?? "");
}

/**
 * `(…)` around a branch is punctuation, so the literal inside it is the node to
 * read and to replace. Keeping the parentheses out of the span is what lets the
 * transform swap `("Save")` for `(__t("…", "Save"))` rather than breaking it.
 */
function unwrapParens(expr: Expression): Expression {
  let node = expr;
  while ((node as unknown as SpannedNode).type === "ParenthesisExpression") {
    node = (node as unknown as { expression: Expression }).expression;
  }
  return node;
}
