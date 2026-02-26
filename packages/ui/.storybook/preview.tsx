import type { Preview, ReactRenderer } from "@storybook/react";
import { withThemeByClassName } from "@storybook/addon-themes";
import React, { useEffect } from "react";
import "../src/styles/globals.css";

/**
 * Decorator that syncs the shadcn-glass-ui data-theme attribute whenever
 * Storybook toggles the dark class, so semantic tokens (--semantic-*,
 * --orb-*, --glow-*) activate correctly.
 *
 * Also wraps each story in a div with bg-background / text-foreground so
 * the correct theme surface shows through — especially in the Docs tab
 * where stories are inlined in an otherwise-white documentation page.
 */
function ThemeSyncDecorator(Story: React.ComponentType) {
  useEffect(() => {
    const root = document.documentElement;
    const sync = () => {
      const isDark = root.classList.contains("dark");
      root.setAttribute("data-theme", isDark ? "aurora" : "light");
    };
    sync();
    const observer = new MutationObserver(sync);
    observer.observe(root, { attributes: true, attributeFilter: ["class"] });
    return () => observer.disconnect();
  }, []);

  return (
    <div className="bg-background text-foreground" style={{ minHeight: "100%" }}>
      <Story />
    </div>
  );
}

const preview: Preview = {
  parameters: {
    controls: {
      matchers: {
        color: /(background|color)$/i,
        date: /Date$/i,
      },
    },
    layout: "centered",
    backgrounds: { disable: true },
  },
  decorators: [
    ThemeSyncDecorator,
    withThemeByClassName<ReactRenderer>({
      themes: {
        light: "",
        dark: "dark",
      },
      defaultTheme: "dark",
    }),
  ],
};

export default preview;
