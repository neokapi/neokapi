/**
 * Extract translatable strings from `@neokapi/i18n-react/head` hook calls.
 *
 * The head hooks translate document-head metadata (`<title>`,
 * `<meta name="description">`, Open Graph / Twitter cards) that has no DOM text
 * node for the build transform to anchor to. Instead each hook self-hashes its
 * source at runtime with the shared `head` descriptor and looks the translation
 * up in the runtime dictionary. For that lookup to resolve, the source must be
 * in the catalog — so the extractor recognizes these hook calls here and emits
 * one block per head string, hashed with the identical descriptor.
 *
 * Only calls whose callee resolves to a `@neokapi/i18n-react/head` import are
 * matched (aliases included), mirroring how `messages.ts` matches `t()`. The
 * transform deliberately does NOT rewrite these calls — the hook resolves at
 * runtime — so head recognition lives on the extract side only.
 */

import type { CallExpression, Module } from "@swc/core";

const HEAD_IMPORT = "@neokapi/i18n-react/head";

/** Hook exports whose first argument is the translatable source string. */
const HEAD_HELPERS = new Set([
  "useTranslatedTitle",
  "useTranslatedDescription",
  "useTranslatedMeta",
]);

/**
 * Collect every local identifier bound to a head hook export
 * (`useTranslatedTitle`, `useTranslatedDescription`, `useTranslatedMeta`),
 * following aliases (`useTranslatedTitle as useTitle`).
 */
export function collectHeadIdentifiers(mod: Module): Set<string> {
  const names = new Set<string>();
  for (const item of mod.body) {
    if (item.type !== "ImportDeclaration") continue;
    if (item.source.value !== HEAD_IMPORT) continue;
    for (const spec of item.specifiers) {
      if (spec.type !== "ImportSpecifier") continue;
      const imported = spec.imported?.value ?? spec.local.value;
      if (HEAD_HELPERS.has(imported)) names.add(spec.local.value);
    }
  }
  return names;
}

export interface HeadCall {
  node: CallExpression;
  /** The literal first argument — the source text. */
  text: string;
}

/**
 * Yield every `useTranslated*("literal", …)` call bound to a head import. Calls
 * whose first argument isn't a string literal are skipped (no extractable
 * source), exactly like the `t()` walker.
 */
export function* walkHeadCalls(mod: Module, names: ReadonlySet<string>): Generator<HeadCall> {
  if (names.size === 0) return;
  yield* descend(mod, names);
}

function* descend(
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  node: any,
  names: ReadonlySet<string>,
): Generator<HeadCall> {
  if (!node || typeof node !== "object") return;

  if (node.type === "CallExpression") {
    const callee = node.callee;
    if (callee?.type === "Identifier" && names.has(callee.value)) {
      const first = node.arguments?.[0]?.expression;
      if (first?.type === "StringLiteral") {
        yield { node: node as CallExpression, text: first.value as string };
      }
    }
  }

  for (const key of Object.keys(node)) {
    if (key === "type" || key === "span") continue;
    const val = (node as Record<string, unknown>)[key];
    if (Array.isArray(val)) {
      for (const item of val) yield* descend(item, names);
    } else if (val && typeof val === "object") {
      yield* descend(val, names);
    }
  }
}
