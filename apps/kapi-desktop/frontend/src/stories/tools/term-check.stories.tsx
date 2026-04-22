/**
 * Terminology Check
 */
import type { Meta, StoryObj } from "@storybook/react-vite";

import { ToolConfig } from "../_lib/schema-story";

const meta: Meta = {
  title: "Formats & Tools/Tools/Quality Assurance/Terminology Check",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;
export const NativeConfig: Story = {
  name: "Configuration",
  render: () => <ToolConfig schemaName="term-check" source="builtIn" />,
};
