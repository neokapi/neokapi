import type { Rule, Node } from "@oxlint/plugins";
import { LIKELY_LABEL_KEYS } from "../shared/translatable-attrs.ts";

/**
 * Off by default — highest false-positive risk in the set, so gated
 * behind the `recommended-strict` preset.
 *
 * Flags the common pattern where a developer stores a user-facing
 * string in an object literal that gets fed to JSX as an expression:
 *
 *   const THEMES = [
 *     { value: 'system', label: 'System' },   // ← flagged
 *     { value: 'light',  label: 'Light' },
 *   ];
 *   return THEMES.map(({ value, label }) => <button>{label}</button>);
 *
 * `<button>{label}</button>` is an expression child and therefore not
 * extractable, and the string literal `'System'` lives in a data array
 * the extractor doesn't walk. Wrapping with `t('System')` makes the
 * literal visible to extraction.
 *
 * To reduce FPs we only flag string literals whose property name is
 * one of the canonical UI-label keys (`label`, `title`, `description`,
 * etc.). A developer using those key names is almost always talking
 * about user-facing copy.
 */
export const rule: Rule = {
  meta: {
    type: "suggestion",
    docs: {
      description:
        "suggest wrapping user-facing strings in data arrays with `t()` so the extractor can see them",
      recommended: false,
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
      useT: "String literal stored in `{{key}}` is not visible to the extractor when rendered as a JSX expression. Wrap with `t(...)` or store the translated value elsewhere.",
    },
  },
  create(context) {
    const opts = (context.options[0] ?? {}) as { keys?: string[] };
    const keys = opts.keys ? new Set(opts.keys) : LIKELY_LABEL_KEYS;
    return {
      Property(node: Node) {
        const prop = node as unknown as {
          key: { type: string; name?: string; value?: unknown };
          value: { type: string; value?: unknown };
          shorthand?: boolean;
          computed?: boolean;
        };
        if (prop.computed || prop.shorthand) return;
        const name = readKey(prop.key);
        if (!name || !keys.has(name)) return;
        if (prop.value.type !== "Literal" || typeof prop.value.value !== "string") return;
        if ((prop.value.value as string).trim() === "") return;
        context.report({
          node: node as unknown as Node,
          messageId: "useT",
          data: { key: name },
        });
      },
    };
  },
};

function readKey(key: { type: string; name?: string; value?: unknown }): string | null {
  if (key.type === "Identifier") return key.name ?? null;
  if (key.type === "Literal" && typeof key.value === "string") return key.value;
  return null;
}
