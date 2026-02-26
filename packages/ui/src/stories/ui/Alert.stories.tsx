import type { Meta, StoryObj } from "@storybook/react";
import { AlertGlass, AlertGlassTitle, AlertGlassDescription } from "../../components/ui/alert";

const meta: Meta<typeof AlertGlass> = {
  title: "UI/Alert",
  component: AlertGlass,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 500, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof AlertGlass>;

export const Default: Story = {
  render: () => (
    <AlertGlass>
      <AlertGlassTitle>Heads up</AlertGlassTitle>
      <AlertGlassDescription>
        3 blocks are missing translations for fr-FR.
      </AlertGlassDescription>
    </AlertGlass>
  ),
};

export const Destructive: Story = {
  render: () => (
    <AlertGlass variant="destructive">
      <AlertGlassTitle>Error</AlertGlassTitle>
      <AlertGlassDescription>
        Failed to export file. The target locale has untranslated blocks.
      </AlertGlassDescription>
    </AlertGlass>
  ),
};
