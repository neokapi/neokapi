import type { Preview, ReactRenderer } from "@storybook/react-vite";
import { withThemeByClassName } from "@storybook/addon-themes";
import "../src/index.css";

function ThemeDecorator(Story: React.ComponentType) {
  return (
    <div className="bg-background text-foreground p-4">
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
      defaultTheme: "dark",
    }),
  ],
  parameters: {
    layout: "fullscreen",
    options: {
      storySort: {
        order: ["Schema Language", "Browsers", "Foundations", "Flow Editor", "Pages", "Components"],
      },
    },
  },
};

export default preview;
