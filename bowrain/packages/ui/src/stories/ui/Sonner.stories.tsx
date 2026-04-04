import { Button, Toaster } from "@neokapi/ui-primitives";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { toast } from "sonner";

const meta: Meta<typeof Toaster> = {
  title: "Foundations/Sonner",
  component: Toaster,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ padding: 16 }}>
        <Story />
        <Toaster />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof Toaster>;

export const Default: Story = {
  render: () => (
    <div className="flex flex-wrap gap-2">
      <Button
        variant="outline"
        onClick={() => toast.success("Translation saved")}
      >
        Save Translation
      </Button>
      <Button
        variant="outline"
        onClick={() => toast.success("Import complete — 1,243 segments processed")}
      >
        Import Complete
      </Button>
      <Button
        variant="outline"
        onClick={() => toast.error("Failed to connect to translation memory")}
      >
        Show Error
      </Button>
      <Button
        variant="outline"
        onClick={() => toast.info("3 files queued for review")}
      >
        Show Info
      </Button>
    </div>
  ),
};
