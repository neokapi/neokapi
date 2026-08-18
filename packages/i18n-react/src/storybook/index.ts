/**
 * Storybook integration for @neokapi/i18n-react — locale toolbar and
 * translation-applying decorator.
 *
 * Usage:
 *
 *   import type { Preview } from '@storybook/react-vite';
 *   import { neokapiDecorator, neokapiGlobalType } from '@neokapi/i18n-react/storybook';
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
 *     decorators: [neokapiDecorator(i18n)],
 *   };
 */

import { createElement, Fragment, useEffect, useState } from "react";
import type { Decorator } from "@storybook/react-vite";

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

/** What the host most recently posted, so a locale switch can re-apply it. */
let hostDict: { locale: string; translations: Record<string, string> } | null = null;

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
 * Locale-switching decorator. Applies translations whenever the user
 * picks a new value from the toolbar, then re-keys the story once the
 * dictionary has actually landed — the plugin's `__t`/`__tx` call
 * sites don't subscribe to the store, so without the remount a story
 * kept rendering the previous locale until the next interaction.
 * Falls back to the empty dictionary (source text) when the
 * translation file can't be fetched or when running in an SSR
 * context without `fetch`.
 */
export function neokapiDecorator(opts: NeokapiStorybookOptions): Decorator {
  const byValue = new Map(opts.locales.map((l) => [l.value, l]));

  /**
   * Put one locale in force: its published catalog, then whatever the host has
   * posted on top. The two are layered rather than exclusive — the strings
   * under review are the host's, and everything else on screen stays
   * translated instead of falling back to source and making the component look
   * half-finished.
   */
  async function apply(locale: string): Promise<void> {
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
    if (hostDict && hostDict.locale === locale) {
      runtime.setTranslations(locale, hostDict.translations, { merge: true });
    }
  }

  return (Story, context) => {
    const toolbarLocale =
      (context.globals.locale as string | undefined) ?? opts.locales[0]?.value ?? "en";
    // A story embedded by a review surface is showing that surface's language,
    // which the toolbar knows nothing about: the host names the locale when it
    // posts its dictionary, and that wins for as long as it is in force.
    const value = hostDict?.locale ?? toolbarLocale;

    // Decorators render as React components, so hooks are available.
    const [applied, setApplied] = useState<string | null>(null);
    // Bumped whenever the host posts a dictionary, so the story re-keys and
    // its non-subscribing lookups read the new text.
    const [hostVersion, setHostVersion] = useState(0);

    useEffect(() => {
      let cancelled = false;
      void (async () => {
        await apply(value);
        if (!cancelled) setApplied(value);
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
        hostDict = { locale: msg.locale, translations: msg.translations };
        void (async () => {
          await apply(msg.locale);
          setHostVersion((n) => n + 1);
        })();
      };
      window.addEventListener("message", onMessage);

      // Announce readiness: the host cannot tell from `load` alone whether the
      // story has mounted, and a dictionary posted before this listener exists
      // is simply lost.
      window.parent?.postMessage({ type: HOST_READY_MESSAGE, locale: value }, "*");

      return () => window.removeEventListener("message", onMessage);
    }, [value]);

    // Key on (locale, landed?, host dictionary) — the story remounts once when
    // the dict is active, so non-subscribing lookups re-read it.
    return createElement(
      Fragment,
      { key: `${value}:${applied === value}:${hostVersion}` },
      Story(),
    );
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
