/**
 * Frozen v1 key scheme — kept verbatim so `kapi-react migrate-keys`
 * can reproduce the hashes a pre-2.0 extractor emitted and map them
 * onto the v2 scheme. Nothing else may import from this module.
 *
 * v1 ingredients:
 *   - 32-bit Jenkins one-at-a-time, base62-encoded
 *   - descriptor = full JSX ancestor path ("div > section > p")
 *   - expression naming that collapses deep member chains
 *     ({a.b.c} → "c") and call chains ({fmt.date(d)} → "date")
 */

import type { Expression, JSXElement } from "@swc/core";

import { getTagName, resolveHTMLElement } from "../extract/ast.ts";

// ─── v1 hash ─────────────────────────────────────────────────

function toUtf8(str: string) {
  const result = [];
  const len = str.length;
  for (let i = 0; i < len; i++) {
    let charcode = str.charCodeAt(i);
    if (charcode < 0x80) {
      result.push(charcode);
    } else if (charcode < 0x8_00) {
      result.push(0xc0 | (charcode >> 6), 0x80 | (charcode & 0x3f));
    } else if (charcode < 0xd8_00 || charcode >= 0xe0_00) {
      result.push(
        0xe0 | (charcode >> 12),
        0x80 | ((charcode >> 6) & 0x3f),
        0x80 | (charcode & 0x3f),
      );
    } else {
      i++;
      charcode = 0x1_00_00 + (((charcode & 0x3_ff) << 10) | (str.charCodeAt(i) & 0x3_ff));
      result.push(
        0xf0 | (charcode >> 18),
        0x80 | ((charcode >> 12) & 0x3f),
        0x80 | ((charcode >> 6) & 0x3f),
        0x80 | (charcode & 0x3f),
      );
    }
  }
  return result;
}

function jenkinsHash(str: string): number {
  if (!str) return 0;
  const utf8 = toUtf8(str);
  let hash = 0;
  for (let i = 0; i < utf8.length; i++) {
    hash += utf8[i];
    hash = (hash + (hash << 10)) >>> 0;
    hash ^= hash >>> 6;
  }
  hash = (hash + (hash << 3)) >>> 0;
  hash ^= hash >>> 11;
  hash = (hash + (hash << 15)) >>> 0;
  return hash;
}

const BaseNSymbols = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ";

function uintToBaseN(numberArg: number, base: number) {
  let number = numberArg;
  if (base < 2 || base > 62 || number < 0) return "";
  let output = "";
  do {
    output = BaseNSymbols.charAt(number % base).concat(output);
    number = Math.floor(number / base);
  } while (number > 0);
  return output;
}

/** v1 `hashKey`. */
export function legacyHashKey(text: string, desc: string): string {
  const key = JSON.stringify(text) + "|" + desc;
  return uintToBaseN(jenkinsHash(key), 62);
}

// ─── v1 descriptor (full ancestor path) ──────────────────────

/** v1 `buildJSXPath`: every ancestor tag joined by " > ". */
export function legacyJSXPath(
  ancestors: readonly JSXElement[],
  current: JSXElement,
  componentMap: Record<string, string>,
): string {
  const parts: string[] = [];
  for (const anc of ancestors) appendResolvedTag(parts, anc, componentMap);
  appendResolvedTag(parts, current, componentMap);
  return parts.join(" > ");
}

function appendResolvedTag(
  parts: string[],
  el: JSXElement,
  componentMap: Record<string, string>,
): void {
  const tag = getTagName(el);
  if (!tag) return;
  parts.push(resolveHTMLElement(tag, componentMap) ?? tag);
}

// ─── v1 expression naming ────────────────────────────────────

/** v1 `exprToName`: {a.b.c} → "c", {fmt.date(d)} → "date". */
export function legacyExprToName(expr: Expression): string {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const anyExpr = expr as any;
  if (anyExpr.type === "Identifier") return anyExpr.value;
  if (anyExpr.type === "MemberExpression") {
    const prop = anyExpr.property;
    if (prop?.type === "Identifier") {
      const obj = anyExpr.object;
      if (obj?.type === "Identifier" && obj.value) {
        return `${obj.value}.${prop.value}`;
      }
      return prop.value ?? "value";
    }
  }
  if (anyExpr.type === "CallExpression") {
    const callee = anyExpr.callee;
    if (callee?.type === "Identifier") return callee.value;
    if (callee?.type === "MemberExpression" && callee.property?.type === "Identifier") {
      return callee.property.value;
    }
  }
  return "value";
}
