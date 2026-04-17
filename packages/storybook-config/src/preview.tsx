import type { Decorator, Preview, ReactRenderer } from "@storybook/react-vite";
import { withThemeByClassName } from "@storybook/addon-themes";
import {
  neokapiDecorator,
  neokapiGlobalType,
  type NeokapiStorybookOptions,
} from "@neokapi/kapi-react/storybook";
import { themes } from "storybook/theming";
import React from "react";

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

export type I18nOptions = NeokapiStorybookOptions;

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
   * Enable a locale toolbar driven by @neokapi/kapi-react. Pair with `i18n: true`
   * in createMainConfig() so stories receive the runtime transform.
   */
  i18n?: I18nOptions;
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
      ...(i18n ? [neokapiDecorator(i18n)] : []),
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
        locale: neokapiGlobalType(i18n),
      },
    }),
  };

  return preview;
}
