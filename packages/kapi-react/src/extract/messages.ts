/**
 * Extract translatable blocks from user `t(...)` calls.
 *
 * The `t` export on `@neokapi/kapi-react/runtime` marks a string
 * for translation outside JSX — the common case is a label or
 * tooltip stored in a plain-JS data structure:
 *
 *   import { t } from '@neokapi/kapi-react/runtime';
 *
 *   const UI_LANGUAGES = [
 *     { value: 'en',  label: t('English') },
 *     { value: 'qps', label: t('Pseudo English (qps)') },
 *   ];
 *
 * The extractor matches only calls whose callee identifier resolves
 * to that runtime import — import-name tracking handles aliases
 * like `import { t as _ } from '@neokapi/kapi-react/runtime'`.
 * That keeps an unrelated local `t()` helper in the same file from
 * being swept up by mistake.
 *
 * Shared between walker.ts and plugin/transform.ts so extract and
 * transform stay hash-compatible.
 */

import type { CallExpression, Module } from '@swc/core';

const RUNTIME_IMPORT = '@neokapi/kapi-react/runtime';

/**
 * Collects every local identifier bound to the runtime `t` export.
 * Usually `{ t }` but supports aliases (`t as translate`).
 */
export function collectTIdentifiers(mod: Module): Set<string> {
  const names = new Set<string>();
  for (const item of mod.body) {
    if (item.type !== 'ImportDeclaration') continue;
    if (item.source.value !== RUNTIME_IMPORT) continue;
    for (const spec of item.specifiers) {
      if (spec.type !== 'ImportSpecifier') continue;
      const imported = spec.imported?.value ?? spec.local.value;
      if (imported === 't') names.add(spec.local.value);
    }
  }
  return names;
}

export interface TCall {
  node: CallExpression;
  /** The literal first argument — the source text. */
  text: string;
  /**
   * Context string from the second argument when it's a string
   * literal. Disambiguates identically-worded source strings with
   * different meanings — enters the hash descriptor.
   */
  context: string | null;
  /**
   * Raw source expression of the params object argument if present,
   * else null. Preserved verbatim so the transform can forward it
   * to `__t(hash, text, <params>)` unchanged. Accepts two shapes:
   *   t("text", { name })                 → params at arg 1
   *   t("text", "context", { name })      → params at arg 2
   */
  paramsSrc: string | null;
  /** Name of the identifier referenced as the callee (post-alias). */
  callee: string;
}

/**
 * Yields every `t("literal", ...)` call in the module. Calls whose
 * first argument isn't a StringLiteral are skipped — there's no
 * source text to extract, so they pass through at runtime as-is
 * via the fallback runtime implementation.
 *
 * Supported argument shapes (2nd arg disambiguates by type):
 *   t("text")
 *   t("text", "context")
 *   t("text", { params })
 *   t("text", "context", { params })
 */
export function* walkTCalls(
  mod: Module,
  names: ReadonlySet<string>,
  sourceSlice: (start: number, end: number) => string,
): Generator<TCall> {
  if (names.size === 0) return;
  yield* descend(mod, names, sourceSlice);
}

function* descend(
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  node: any,
  names: ReadonlySet<string>,
  sourceSlice: (start: number, end: number) => string,
): Generator<TCall> {
  if (!node || typeof node !== 'object') return;

  if (node.type === 'CallExpression') {
    const callee = node.callee;
    if (callee?.type === 'Identifier' && names.has(callee.value)) {
      const first = node.arguments?.[0]?.expression;
      const second = node.arguments?.[1]?.expression;
      const third = node.arguments?.[2]?.expression;
      if (first?.type === 'StringLiteral') {
        let context: string | null = null;
        let paramsNode: { span?: { start: number; end: number } } | null = null;

        if (second?.type === 'StringLiteral') {
          context = second.value as string;
          paramsNode = third ?? null;
        } else if (second) {
          paramsNode = second;
        }

        yield {
          node: node as CallExpression,
          text: first.value as string,
          context,
          paramsSrc:
            paramsNode?.span
              ? sourceSlice(paramsNode.span.start, paramsNode.span.end)
              : null,
          callee: callee.value,
        };
      }
    }
  }

  for (const key of Object.keys(node)) {
    if (key === 'type' || key === 'span') continue;
    const val = (node as Record<string, unknown>)[key];
    if (Array.isArray(val)) {
      for (const item of val) yield* descend(item, names, sourceSlice);
    } else if (val && typeof val === 'object') {
      yield* descend(val, names, sourceSlice);
    }
  }
}
