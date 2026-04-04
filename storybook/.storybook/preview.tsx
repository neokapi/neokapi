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
    "Resource Browser",
    "Schema Form",
    "Flow Editor",
    "Formats & Tools",
    "Pages",
    "Components",
    "Interactions",
  ],
  decorators: [KapiProviders],
});

export default preview;
