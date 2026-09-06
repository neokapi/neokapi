/**
 * Storybook integration for @neokapi/i18n-react: a locale toolbar, a loader
 * that puts the catalog in force, and a decorator that renders the story in it.
 *
 * Usage:
 *
 *   import type { Preview } from '@storybook/react-vite';
 *   import {
 *     neokapiDecorator,
 *     neokapiGlobalType,
 *     neokapiLoader,
 *   } from '@neokapi/i18n-react/storybook';
 *
 *   const i18n = {
 *     locales: [
 *       { value: 'en', title: 'English' },
 *       { value: 'qps', title: 'Pseudo English', url: '/translations/qps.json' },
 *     ],
 *   };
 *
 *   const preview: Preview = {
 *     globalTypes: { locale: neokapiGlobalType(i18n) },
 *     loaders: [neokapiLoader(i18n)],
 *     decorators: [neokapiDecorator(i18n)],
 *   };
 *
 * Pass the same options object to all three: they share one record of which
 * locale is in force, which is what lets the decorator skip work the loader
 * has already done.
 */

import { createElement, Fragment, useEffect, useState } from "react";
import type { Decorator, Loader } from "@storybook/react-vite";

export interface NeokapiLocale {
  /** BCP-47 locale code, e.g. "en", "qps". */
  value: string;
  /** Human-readable label shown in the toolbar dropdown. */
  title: string;
  /**
   * URL the runtime fetches to load the translation dictionary.
   * Omit for the source locale — the runtime will fall back to source text.
   */
  url?: string;
}

export interface NeokapiStorybookOptions {
  locales: NeokapiLocale[];
  /**
   * Accept a translation dictionary posted in by the embedding page.
   *
   * A published story renders whatever catalog it was built with — the last
   * *shipped* translation. A reviewer working a queue holds something that
   * catalog has never seen: their own pending target. With this on, the host
   * embedding the story can post that dictionary in and watch the component
   * render it (see {@link HOST_TRANSLATIONS_MESSAGE}) — the difference between
   * reading a string in a list and reading it in the button it will ship in.
   *
   * The payload only replaces displayed text inside the preview iframe, and a
   * Storybook is a public gallery of that text already. It is opt-in rather
   * than always-on because a story rendering text from whoever framed it should
   * be a decision someone made.
   */
  hostTranslations?: boolean;
}

/** The message a host posts into a story's iframe to render its own strings. */
export const HOST_TRANSLATIONS_MESSAGE = "neokapi-i18n-translations";

/** The message a story posts to its host once it can accept translations. */
export const HOST_READY_MESSAGE = "neokapi-i18n-ready";

/** The payload of a {@link HOST_TRANSLATIONS_MESSAGE}. */
export interface HostTranslations {
  type: typeof HOST_TRANSLATIONS_MESSAGE;
  /** The locale the dictionary is for; ignored unless the story is showing it. */
  locale: string;
  /** Message key → translated text. Keys are `hashKey(text, descriptor)`. */
  translations: Record<string, string>;
}

/**
 * What the loader and the decorator know between them about one preview.
 *
 * Keyed on the options object, so the loader and the decorator built from the
 * same `i18n` share it and two previews in one process never do.
 */
interface Session {
  /** The locale whose catalog is in force, or null before the first apply. */
  locale: string | null;
  /** What the host most recently posted, so a locale switch can re-apply it. */
  host: { locale: string; translations: Record<string, string> } | null;
}

const sessions = new WeakMap<NeokapiStorybookOptions, Session>();

function sessionFor(opts: NeokapiStorybookOptions): Session {
  let session = sessions.get(opts);
  if (!session) {
    session = { locale: null, host: null };
    sessions.set(opts, session);
  }
  return session;
}

function readHostTranslations(data: unknown): HostTranslations | null {
  if (typeof data !== "object" || data === null) return null;
  const msg = data as Partial<HostTranslations>;
  if (msg.type !== HOST_TRANSLATIONS_MESSAGE || typeof msg.locale !== "string") return null;
  return {
    type: HOST_TRANSLATIONS_MESSAGE,
    locale: msg.locale,
    translations: msg.translations ?? {},
  };
}

/**
 * Lazy-import the runtime so projects that don't enable i18n pay
 * nothing for importing this module.
 */
async function getRuntime() {
  return (await import("../runtime/index.ts")) as {
    setTranslations: (
      locale: string,
      dict: Record<string, string>,
      options?: { merge?: boolean },
    ) => void;
    loadTranslations: (locale: string, url: string) => Promise<void>;
  };
}

/**
 * Which locale a story should be showing.
 *
 * A story embedded by a review surface is showing that surface's language,
 * which the toolbar knows nothing about: the host names the locale when it
 * posts its dictionary, and that wins for as long as it is in force.
 */
function resolveLocale(opts: NeokapiStorybookOptions, toolbar: string | undefined): string {
  return sessionFor(opts).host?.locale ?? toolbar ?? opts.locales[0]?.value ?? "en";
}

/**
 * Put one locale in force: its published catalog, then whatever the host has
 * posted on top. The two are layered: the strings under review are the host's,
 * and everything else on screen stays translated instead of falling back to
 * source and making the component look half-finished.
 */
function makeApplier(opts: NeokapiStorybookOptions): (locale: string) => Promise<void> {
  const byValue = new Map(opts.locales.map((l) => [l.value, l]));
  const session = sessionFor(opts);

  return async function apply(locale: string): Promise<void> {
    const runtime = await getRuntime();
    const declared = byValue.get(locale);
    if (!declared?.url || typeof fetch === "undefined") {
      runtime.setTranslations(locale, {});
    } else {
      try {
        await runtime.loadTranslations(locale, declared.url);
      } catch {
        runtime.setTranslations(locale, {});
      }
    }
    if (session.host && session.host.locale === locale) {
      runtime.setTranslations(locale, session.host.translations, { merge: true });
    }
    session.locale = locale;
  };
}

/**
 * Loads the active locale's catalog before the story renders.
 *
 * Storybook runs loaders, then mounts the story, then runs its `play`
 * function. Fetching here means the story mounts once, already reading the
 * right dictionary, and a `play` that opens a sheet or selects a block keeps
 * what it did. Fetching from the decorator instead put the catalog in force
 * after `play` had already run against a mount that was about to be replaced,
 * and the Interactions panel recorded every step as passed while the canvas
 * showed the unopened state.
 *
 * Register it alongside {@link neokapiDecorator}, built from the same options
 * object.
 */
export function neokapiLoader(opts: NeokapiStorybookOptions): Loader {
  const apply = makeApplier(opts);
  const session = sessionFor(opts);

  return async (context) => {
    const locale = resolveLocale(opts, context.globals?.locale as string | undefined);
    if (session.locale !== locale) await apply(locale);
    return {};
  };
}

/**
 * Renders the story in the locale that is in force.
 *
 * The plugin's `__t` / `__tx` call sites read the dictionary at render time
 * without subscribing to it, so a story shows whatever was in force when it
 * rendered. The decorator keys the story on the locale plus a revision counter
 * and bumps that counter whenever the dictionary changes under a mounted story:
 * a host posting its own, or the catalog landing in a preview that registered
 * no loader. A preview with {@link neokapiLoader} never sees the second case,
 * so the story mounts exactly once per locale.
 */
export function neokapiDecorator(opts: NeokapiStorybookOptions): Decorator {
  const apply = makeApplier(opts);
  const session = sessionFor(opts);

  return (Story, context) => {
    const toolbarLocale = context.globals.locale as string | undefined;
    const value = resolveLocale(opts, toolbarLocale);

    // Decorators render as React components, so hooks are available.
    const [revision, setRevision] = useState(0);

    useEffect(() => {
      // The loader already fetched this locale, so there is nothing to do and
      // nothing to re-key: the first render read the right dictionary.
      if (session.locale === value) return;
      let cancelled = false;
      void (async () => {
        await apply(value);
        if (!cancelled) setRevision((n) => n + 1);
      })();
      return () => {
        cancelled = true;
      };
    }, [value]);

    useEffect(() => {
      if (!opts.hostTranslations || typeof window === "undefined") return;

      const onMessage = (event: MessageEvent) => {
        const msg = readHostTranslations(event.data);
        if (!msg) return;
        session.host = { locale: msg.locale, translations: msg.translations };
        void (async () => {
          await apply(msg.locale);
          setRevision((n) => n + 1);
        })();
      };
      window.addEventListener("message", onMessage);

      // Announce readiness: the host cannot tell from `load` alone whether the
      // story has mounted, and a dictionary posted before this listener exists
      // is simply lost.
      window.parent?.postMessage({ type: HOST_READY_MESSAGE, locale: value }, "*");

      return () => window.removeEventListener("message", onMessage);
    }, [value]);

    return createElement(Fragment, { key: `${value}:${revision}` }, Story());
  };
}

/**
 * `globalTypes` entry that registers the toolbar dropdown. Assign to a
 * key (typically `locale`) on the Preview's `globalTypes` object.
 */
export function neokapiGlobalType(opts: NeokapiStorybookOptions) {
  return {
    name: "Language",
    description: "UI language",
    defaultValue: opts.locales[0]?.value ?? "en",
    toolbar: {
      icon: "globe",
      items: opts.locales.map((l) => ({ value: l.value, title: l.title })),
      dynamicTitle: true,
    },
  };
}
