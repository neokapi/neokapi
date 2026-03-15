import type { Preview, ReactRenderer } from "@storybook/react-vite";
import { withThemeByClassName } from "@storybook/addon-themes";
import React from "react";
import "../src/styles/globals.css";

/**
 * Wraps each story in a div with bg-background / text-foreground so
 * the correct theme surface shows through — especially in the Docs tab
 * where stories are inlined in an otherwise-white documentation page.
 */
function ThemeDecorator(Story: React.ComponentType) {
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
    backgrounds: { disabled: true },
  },
  decorators: [
    ThemeDecorator,
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
