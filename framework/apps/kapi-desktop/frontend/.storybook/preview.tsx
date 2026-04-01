import type { Preview, ReactRenderer } from "@storybook/react-vite";
import { withThemeByClassName } from "@storybook/addon-themes";
import { themes } from "storybook/theming";
import "../src/index.css";

// Detect system preference for default theme
const prefersDark = typeof window !== "undefined"
  && window.matchMedia("(prefers-color-scheme: dark)").matches;

function ThemeDecorator(Story: React.ComponentType) {
  return (
    <div className="bg-background text-foreground min-h-screen p-4">
      <Story />
    </div>
  );
}

const preview: Preview = {
  decorators: [
    ThemeDecorator,
    withThemeByClassName<ReactRenderer>({
      themes: {
        light: "",
        dark: "dark",
      },
      defaultTheme: prefersDark ? "dark" : "light",
    }),
  ],
  parameters: {
    layout: "fullscreen",
    docs: {
      theme: prefersDark ? themes.dark : themes.light,
    },
    options: {
      storySort: {
        order: ["Foundations", "Formats & Tools", ["Browsers", "Schema", "Formats", "Tools"], "Flow Editor", "Pages", "Components", "Interactions"],
      },
    },
  },
};

export default preview;
