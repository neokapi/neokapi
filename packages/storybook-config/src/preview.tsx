import type { Decorator, Preview, ReactRenderer } from "@storybook/react-vite";
import { withThemeByClassName } from "@storybook/addon-themes";
import { themes } from "storybook/theming";
import React from "react";

/** Lazy reference to the @neokapi/react runtime — only resolved when i18n is enabled. */
type NeokapiRuntime = {
  setTranslations: (locale: string, dict: Record<string, string>) => void;
  loadTranslations: (locale: string, url: string) => Promise<void>;
};
let neokapiRuntime: NeokapiRuntime | null = null;
async function getNeokapiRuntime(): Promise<NeokapiRuntime> {
  if (neokapiRuntime) return neokapiRuntime;
  neokapiRuntime = (await import(
    /* @vite-ignore */ "@neokapi/react/runtime"
  )) as unknown as NeokapiRuntime;
  return neokapiRuntime;
}

/**
 * Wraps each story in a themed container so the correct theme surface
 * shows through — especially in the Docs tab where stories are inlined
 * in an otherwise-white documentation page.
 */
export function ThemeDecorator(Story: React.ComponentType) {
  return (
    <div className="bg-background text-foreground" style={{ minHeight: "100%" }}>
      <Story />
    </div>
  );
}

/** Detect system dark mode preference. */
export const prefersDark =
  typeof window !== "undefined" && window.matchMedia("(prefers-color-scheme: dark)").matches;

export interface I18nLocale {
  /** BCP-47 locale code, e.g. "en", "qps". */
  value: string;
  /** Human-readable label shown in the toolbar dropdown. */
  title: string;
  /**
   * URL the runtime fetches to load the translation dictionary.
   * Omit for the source locale (no fetch — the runtime falls back to source text).
   */
  url?: string;
}

export interface I18nOptions {
  /** Locales offered in the toolbar dropdown. The first entry is the default. */
  locales: I18nLocale[];
}

export interface CreatePreviewOptions {
  /** Default Storybook layout: "centered" | "fullscreen" | "padded". */
  layout?: "centered" | "fullscreen" | "padded";
  /** Default theme: "light" | "dark" | "system". */
  defaultTheme?: "light" | "dark" | "system";
  /** Sidebar sort order (array of top-level category names). */
  sortOrder?: string[];
  /** Additional decorators inserted before theme decorators. */
  decorators?: Decorator[];
  /**
   * Enable a locale toolbar driven by @neokapi/react. Pair with `i18n: true`
   * in createMainConfig() so stories receive the runtime transform.
   */
  i18n?: I18nOptions;
}

/**
 * Decorator that applies the active locale via @neokapi/react's runtime
 * whenever the user picks a new value from the toolbar dropdown.
 */
function createLocaleDecorator(i18n: I18nOptions): Decorator {
  const byValue = new Map(i18n.locales.map((l) => [l.value, l]));
  let lastApplied: string | null = null;

  return (Story, context) => {
    const value = (context.globals.locale as string) || i18n.locales[0]?.value || "en";
    if (value !== lastApplied) {
      lastApplied = value;
      const locale = byValue.get(value);
      void (async () => {
        const runtime = await getNeokapiRuntime();
        if (!locale || !locale.url) {
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
    return <Story />;
  };
}

/**
 * Creates a Storybook preview config with shared defaults.
 * Product-specific Storybooks call this with their own overrides.
 */
export function createPreview(options: CreatePreviewOptions = {}): Preview {
  const {
    layout = "centered",
    defaultTheme = "system",
    sortOrder,
    decorators: extraDecorators = [],
    i18n,
  } = options;

  const resolvedDefault =
    defaultTheme === "system" ? (prefersDark ? "dark" : "light") : defaultTheme;

  const preview: Preview = {
    parameters: {
      controls: {
        matchers: {
          color: /(background|color)$/i,
          date: /Date$/i,
        },
      },
      layout,
      backgrounds: { disabled: true },
      docs: {
        theme: resolvedDefault === "dark" ? themes.dark : themes.light,
      },
      ...(sortOrder && {
        options: {
          storySort: {
            order: sortOrder,
          },
        },
      }),
    },
    decorators: [
      ...extraDecorators,
      ...(i18n ? [createLocaleDecorator(i18n)] : []),
      ThemeDecorator,
      withThemeByClassName<ReactRenderer>({
        themes: {
          light: "",
          dark: "dark",
        },
        defaultTheme: resolvedDefault,
      }),
    ],
    ...(i18n && {
      globalTypes: {
        locale: {
          name: "Language",
          description: "UI language",
          defaultValue: i18n.locales[0]?.value || "en",
          toolbar: {
            icon: "globe",
            items: i18n.locales.map((l) => ({ value: l.value, title: l.title })),
            dynamicTitle: true,
          },
        },
      },
    }),
  };

  return preview;
}
