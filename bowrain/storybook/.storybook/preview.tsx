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
});

export default preview;
