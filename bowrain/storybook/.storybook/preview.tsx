import { createPreview } from "@neokapi/storybook-config/preview";
import { TooltipProvider } from "@neokapi/ui";
import "./storybook.css";

function BowrainProviders(Story: React.ComponentType) {
  return (
    <TooltipProvider>
      <Story />
    </TooltipProvider>
  );
}

const preview = createPreview({
  layout: "centered",
  defaultTheme: "dark",
  sortOrder: [
    "Foundations",
    "Resource Browser",
    "Schema Form",
    "Flow Editor",
    "Components",
    "Layout",
    "Workspace",
    "Streams",
    "Editor",
    "Pages",
    "Auth",
    "Brand",
    "Bravo",
    "Billing",
    "Pulse",
    "Ctrl",
    "Emails",
  ],
  decorators: [BowrainProviders],
  i18n: {
    locales: [
      { value: "en", title: "English" },
      {
        value: "nb",
        title: "Norsk (bokmål)",
        // Resolve against the Storybook base path, which the storybook-preview
        // workflow sets per PR (STORYBOOK_BASE_PATH).
        url: `${import.meta.env.BASE_URL}translations/nb.json`,
      },
      {
        value: "qps",
        title: "Pseudo English (qps)",
        url: `${import.meta.env.BASE_URL}translations/qps.json`,
      },
    ],
    // A reviewer's pending translation is not in any built catalog, so bowrain
    // posts it in and the story renders it: the string read in the component it
    // ships in rather than in a list beside it.
    hostTranslations: true,
  },
});

export default preview;
