import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ToolPalette } from "../ToolPalette";
import type { ToolInfo } from "../types";
import toolsData from "../../../../framework/apps/kapi-desktop/frontend/src/stories/fixtures/tools-metadata.json";

const tools = toolsData as ToolInfo[];

const meta: Meta<typeof ToolPalette> = {
  title: "Flow Editor/ToolPalette",
  component: ToolPalette,
  tags: ["autodocs"],
  args: {
    onAddTool: fn(),
  },
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <div style={{ height: 600, display: "flex" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ToolPalette>;

export const AllTools: Story = {
  args: { tools },
};

export const BuiltInOnly: Story = {
  args: {
    tools: tools.filter((t) => !t.name.startsWith("okapi:")),
  },
};

export const OkapiOnly: Story = {
  args: {
    tools: tools.filter((t) => t.name.startsWith("okapi:")),
  },
};

export const FewTools: Story = {
  args: {
    tools: tools.slice(0, 8),
  },
};
