/**
 * Storybook integration for @neokapi/kapi-react — locale toolbar and
 * translation-applying decorator.
 *
 * Usage:
 *
 *   import type { Preview } from '@storybook/react-vite';
 *   import { neokapiDecorator, neokapiGlobalType } from '@neokapi/kapi-react/storybook';
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
 * picks a new value from the toolbar. Falls back to the empty
 * dictionary (source text) when the translation file can't be fetched
 * or when running in an SSR context without `fetch`.
 */
export function neokapiDecorator(opts: NeokapiStorybookOptions): Decorator {
  const byValue = new Map(opts.locales.map((l) => [l.value, l]));
  let lastApplied: string | null = null;

  return (Story, context) => {
    const value = (context.globals.locale as string | undefined) ?? opts.locales[0]?.value;

    if (value && value !== lastApplied) {
      lastApplied = value;
      void (async () => {
        const runtime = await getRuntime();
        const locale = byValue.get(value);
        if (!locale?.url || typeof fetch === "undefined") {
          runtime.setTranslations(value, {});
          return;
        }
        try {
          await runtime.loadTranslations(value, locale.url);
        } catch {
          runtime.setTranslations(value, {});
        }
      })();
    }

    return Story();
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
