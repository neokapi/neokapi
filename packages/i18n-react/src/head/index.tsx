"use client";

/**
 * @neokapi/i18n-react/head — translate & review the document head.
 *
 * `<title>`, `<meta name="description">`, and the Open Graph / Twitter card
 * strings are page metadata with no rendered text node, so the build transform
 * (which anchors to JSX text) never touches them and the review overlay can't
 * outline them inline. These opt-in hooks close that gap: each translates its
 * source through the runtime dictionary, applies the result to the real head
 * element, and registers the string with the head registry so the review
 * overlay can list and edit it in a panel — through the same review-store path
 * a body string uses.
 *
 * Resolution rides the runtime dictionary (OTA / `mode: 'runtime'`), exactly
 * like `t()`: load a locale with `loadTranslations()`/`setTranslations()` and
 * the head repaints. In pure inline builds (no runtime dict) they render source
 * text. Marked "use client": they use React state/effects and touch `document`,
 * so under RSC they must run on the client.
 *
 *   import { useTranslatedTitle, useTranslatedDescription } from
 *     "@neokapi/i18n-react/head";
 *
 *   function Head() {
 *     useTranslatedTitle("Dashboard · Acme");
 *     useTranslatedDescription("Manage your team, projects, and billing.");
 *     return null;
 *   }
 */

import { useEffect, useMemo } from "react";

import { __t, useNeokapi } from "../runtime/index.ts";
import { hashKey } from "../runtime/hash.ts";
import {
  registerHeadTranslatable,
  unregisterHeadTranslatable,
  type HeadKind,
} from "../runtime/head.ts";
import { HEAD_DESCRIPTOR } from "../types.ts";

export {
  headTranslatables,
  subscribeHeadRegistry,
  type HeadKind,
  type HeadTranslatable,
} from "../runtime/head.ts";

/** Which `<meta>` element a head string targets. Provide `name` or `property`. */
export interface MetaTarget {
  /** `<meta name="…">` selector, e.g. "description". */
  name?: string;
  /** `<meta property="…">` selector, e.g. "og:title". */
  property?: string;
  /**
   * Registry label shown in the review panel. Defaults to `property ?? name`,
   * which already matches the well-known kinds (`og:title`, `description`, …).
   */
  kind?: HeadKind;
}

/**
 * Translate `source` and set `document.title`. Registers a `"title"` head
 * translatable so the review overlay can surface and edit it. Returns the
 * resolved title (useful for RSC frameworks that also render a `<title>`).
 */
export function useTranslatedTitle(source: string): string {
  useNeokapi(); // repaint on locale switch / dict merge (incl. a review edit)
  const hash = useMemo(() => hashKey(source, HEAD_DESCRIPTOR), [source]);
  const value = __t(hash, source);
  useEffect(() => {
    if (typeof document === "undefined") return;
    document.title = value;
    registerHeadTranslatable({ kind: "title", hash, source });
    return () => unregisterHeadTranslatable("title");
  }, [hash, source, value]);
  return value;
}

/**
 * Translate `source` and set the `content` of a `<meta>` element (created if
 * absent), registering it with the head registry. The generic primitive behind
 * `useTranslatedDescription` and the Open Graph / Twitter helpers.
 */
export function useTranslatedMeta(source: string, target: MetaTarget): string {
  const name = target.name;
  const property = target.property;
  const kind: HeadKind = target.kind ?? property ?? name ?? "meta";
  useNeokapi();
  const hash = useMemo(() => hashKey(source, HEAD_DESCRIPTOR), [source]);
  const value = __t(hash, source);
  useEffect(() => {
    if (typeof document === "undefined") return;
    applyMeta(name, property, value);
    registerHeadTranslatable({ kind, hash, source });
    return () => unregisterHeadTranslatable(kind);
  }, [name, property, kind, hash, source, value]);
  return value;
}

/** Translate `source` and set `<meta name="description">`. */
export function useTranslatedDescription(source: string): string {
  return useTranslatedMeta(source, { name: "description", kind: "description" });
}

/**
 * Upsert a `<meta>` element by `name` or `property` and set its `content`.
 * Prefers `property` (Open Graph / Twitter) when both are supplied.
 */
function applyMeta(name: string | undefined, property: string | undefined, content: string): void {
  const attr = property ? "property" : "name";
  const selectorValue = property ?? name;
  if (!selectorValue) return; // nothing to target — no-op rather than throw
  const head = document.head;
  if (!head) return;
  let el = head.querySelector<HTMLMetaElement>(`meta[${attr}="${cssEscape(selectorValue)}"]`);
  if (!el) {
    el = document.createElement("meta");
    el.setAttribute(attr, selectorValue);
    head.appendChild(el);
  }
  el.setAttribute("content", content);
}

function cssEscape(value: string): string {
  const fn = (globalThis as { CSS?: { escape?: (v: string) => string } }).CSS?.escape;
  return fn ? fn(value) : value.replace(/["\\]/g, "\\$&");
}
