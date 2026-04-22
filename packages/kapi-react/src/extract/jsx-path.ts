/**
 * Builds the structural selector path that identifies a JSX element
 * within its source file. Feeds into `hashKey` as the `desc` argument,
 * so the path must match what `plugin/transform.ts` emits at the same
 * element — otherwise extract-side hashes won't match transform-side
 * hashes and the runtime dict won't resolve.
 */

import type { JSXElement } from "@swc/core";

import { getTagName, resolveHTMLElement } from "./ast.ts";

/**
 * Joins ancestor tag names + the element's own tag into a path like
 * `"h1"`, `"li > button"`, `"section > p > strong"`. PascalCase
 * components are resolved through `componentMap` if possible, so a
 * `<Button>Save</Button>` entry yields `button` when the map has
 * `Button → button`.
 */
export function buildJSXPath(
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
