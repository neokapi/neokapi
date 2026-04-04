import { createPreview } from "@neokapi/storybook-config/preview";
import "../../apps/kapi-desktop/frontend/src/index.css";

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
});

export default preview;
