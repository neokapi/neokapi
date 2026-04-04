import { addons } from "storybook/manager-api";
import { themes } from "storybook/theming";

/**
 * Configure the Storybook manager UI to follow system dark/light preference.
 * Import this file from a product-specific manager.ts to apply.
 */
const prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;

addons.setConfig({
  theme: prefersDark ? themes.dark : themes.light,
});
