import { addons } from "storybook/manager-api";
import { themes } from "storybook/theming";

// Manager UI follows system preference (cannot react to story globals)
const prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;

addons.setConfig({
  theme: prefersDark ? themes.dark : themes.light,
});
