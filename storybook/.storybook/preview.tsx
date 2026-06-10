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
        value: "qps",
        title: "Pseudo English (qps)",
        // Resolve against the Storybook base path (set per-PR by the
        // storybook-preview workflow via STORYBOOK_BASE_PATH).
        url: `${import.meta.env.BASE_URL}translations/qps.json`,
      },
    ],
  },
});

export default preview;
