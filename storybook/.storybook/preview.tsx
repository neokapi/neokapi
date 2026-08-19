import { createPreview } from "@neokapi/storybook-config/preview";
import { ErrorProvider } from "../../apps/kapi-desktop/frontend/src/components/ErrorBanner";
import "./storybook.css";

function KapiProviders(Story: React.ComponentType) {
  return (
    <ErrorProvider>
      <Story />
    </ErrorProvider>
  );
}

const preview = createPreview({
  layout: "fullscreen",
  defaultTheme: "system",
  sortOrder: [
    "Foundations",
    "Diagrams",
    "Resource Browser",
    "Schema Form",
    "Flow Editor",
    "Concept UI",
    "Formats & Tools",
    "Pages",
    "Components",
    "Interactions",
  ],
  decorators: [KapiProviders],
  i18n: {
    locales: [
      { value: "en", title: "English" },
      {
        value: "nb",
        title: "Norsk (bokmål)",
        url: `${import.meta.env.BASE_URL}translations/nb.json`,
      },
      {
        value: "qps",
        title: "Pseudo English (qps)",
        // Resolve against the Storybook base path (set per-PR by the
        // storybook-preview workflow via STORYBOOK_BASE_PATH).
        url: `${import.meta.env.BASE_URL}translations/qps.json`,
      },
    ],
    // A reviewer's pending translation is not in any built catalog, so bowrain
    // posts it in and the story renders it: the string read in the component it
    // ships in rather than in a list beside it. Without this the desktop and
    // framework stories still frame and draw — they simply ignore what the
    // reviewer is holding and show the last shipped wording, which reads as the
    // translation being missing rather than as the story not listening.
    hostTranslations: true,
  },
});

export default preview;
