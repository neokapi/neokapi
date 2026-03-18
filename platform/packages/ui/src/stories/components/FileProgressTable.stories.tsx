import type { Meta, StoryObj } from "@storybook/react-vite";
import { FileProgressTable } from "../../components/FileProgressTable";
import { sampleItemStats } from "./fixtures";

const meta: Meta<typeof FileProgressTable> = {
  title: "Components/FileProgressTable",
  component: FileProgressTable,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 800, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof FileProgressTable>;

export const Default: Story = {
  args: { itemStats: sampleItemStats, locales: ["fr-FR", "de-DE"] },
};
