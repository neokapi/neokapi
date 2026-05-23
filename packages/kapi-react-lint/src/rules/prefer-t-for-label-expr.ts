import type { Rule, Node } from "@oxlint/plugins";
import { LIKELY_LABEL_KEYS } from "../shared/translatable-attrs.ts";
import { hasTranslateNoAncestor } from "../shared/translate-no.ts";

/**
 * Complement of `prefer-t-for-label-props`: that rule flags the
 * *declaration* side (`const X = [{ label: 'Foo' }]`); this one flags
 * the *render* side (`<div>{meta.label}</div>`). Either pattern alone
 * silently bypasses translation — the extractor doesn't walk data
 * arrays, and it treats a JSX expression child as an opaque
 * placeholder.
 *
 * Flags: a JSX expression container whose expression is a member
 * access with a label-like property (`label`, `title`, `description`,
 * …). Covers single-dot (`meta.label`) and two-deep
 * (`items[i].title`). Members whose property name isn't in
 * `LIKELY_LABEL_KEYS` are ignored — keeping FP low.
 *
 * Fix guidance in the message: wrap the source string with `t()` in
 * the data array, or use an explicit `t(key)` lookup here.
 */
export const rule: Rule = {
  meta: {
    type: "problem",
    docs: {
      description:
        "flag `{obj.label}` / `{obj.title}` rendered as JSX text — the dereferenced property is almost certainly user-visible copy that silently bypasses translation",
      recommended: true,
    },
    schema: [
      {
        type: "object",
        additionalProperties: false,
        properties: {
          keys: { type: "array", items: { type: "string" } },
        },
      },
    ],
    messages: {
      dynLabel:
        '`{{expr}}` is rendered as JSX text — the `.{{key}}` property name suggests a user-visible string that won\'t be translated. Wrap the source data with `t()` or use an explicit `t("key")` here.',
    },
  },
  create(context) {
    const opts = (context.options[0] ?? {}) as { keys?: string[] };
    const keys = opts.keys ? new Set(opts.keys) : LIKELY_LABEL_KEYS;
    return {
      JSXExpressionContainer(node: Node) {
        const container = node as unknown as {
          parent: unknown;
          expression: { type: string };
        };
        // Only JSX children, not attribute values — attributes have
        // their own extraction path and separate lint rules.
        const parent = container.parent as { type?: string } | undefined;
        if (parent?.type !== "JSXElement" && parent?.type !== "JSXFragment") return;

        const expr = container.expression;
        if (!expr) return;

        const match = labelLikeMember(expr, keys);
        if (!match) return;

        // Respect the W3C `translate="no"` hint on any JSX ancestor —
        // developers use it to mark dynamic / code-like / non-copy
        // content; surfacing a "needs translation" warning on it is
        // noise.
        if (hasTranslateNoAncestor(container.parent)) return;

        context.report({
          node: node as unknown as Node,
          messageId: "dynLabel",
          data: { expr: match.display, key: match.key },
        });
      },
    };
  },
};

interface LabelMatch {
  key: string;
  display: string;
}

function labelLikeMember(expr: unknown, keys: ReadonlySet<string>): LabelMatch | null {
  if (!expr || typeof expr !== "object") return null;
  const node = expr as {
    type?: string;
    property?: { type?: string; name?: string; value?: unknown };
    object?: { type?: string; name?: string; property?: { type?: string; name?: string } };
    computed?: boolean;
  };
  if (node.type !== "MemberExpression") return null;
  if (node.computed) return null;
  const propName = readPropName(node.property);
  if (!propName || !keys.has(propName)) return null;
  return { key: propName, display: renderAccess(node) };
}

function readPropName(prop?: { type?: string; name?: string; value?: unknown }): string | null {
  if (!prop) return null;
  if (prop.type === "Identifier") return prop.name ?? null;
  if (prop.type === "Literal" && typeof prop.value === "string") return prop.value;
  return null;
}

function renderAccess(node: {
  property?: { name?: string };
  object?: { type?: string; name?: string; property?: { name?: string } };
}): string {
  const propName = node.property?.name ?? "?";
  const obj = node.object;
  if (obj?.type === "Identifier" && obj.name) return `${obj.name}.${propName}`;
  if (obj?.type === "MemberExpression" && obj.property?.name) {
    return `${obj.property.name}.${propName}`;
  }
  return propName;
}
