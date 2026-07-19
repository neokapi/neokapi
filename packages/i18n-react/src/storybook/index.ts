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
}

/**
 * Lazy-import the runtime so projects that don't enable i18n pay
 * nothing for importing this module.
 */
async function getRuntime() {
  return (await import("../runtime/index.ts")) as {
    setTranslations: (locale: string, dict: Record<string, string>) => void;
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

  return (Story, context) => {
    const value = (context.globals.locale as string | undefined) ?? opts.locales[0]?.value ?? "en";

    // Decorators render as React components, so hooks are available.
    const [applied, setApplied] = useState<string | null>(null);

    useEffect(() => {
      let cancelled = false;
      void (async () => {
        const runtime = await getRuntime();
        const locale = byValue.get(value);
        if (!locale?.url || typeof fetch === "undefined") {
          runtime.setTranslations(value, {});
        } else {
          try {
            await runtime.loadTranslations(value, locale.url);
          } catch {
            runtime.setTranslations(value, {});
          }
        }
        if (!cancelled) setApplied(value);
      })();
      return () => {
        cancelled = true;
      };
    }, [value]);

    // Key on (locale, landed?) — the story remounts once when the
    // dict is active, so non-subscribing lookups re-read it.
    return createElement(Fragment, { key: `${value}:${applied === value}` }, Story());
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
