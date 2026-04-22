/**
 * Diff Leverage
 */
import type { Meta, StoryObj } from "@storybook/react-vite";

import { ToolConfig } from "../_lib/schema-story";

const meta: Meta = {
  title: "Formats & Tools/Tools/Translation/Diff Leverage",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;
export const NativeConfig: Story = {
  name: "Native Configuration",
  render: () => <ToolConfig schemaName="diff-leverage" source="builtIn" />,
};
export const OkapiConfig: Story = {
  name: "Okapi Configuration",
  render: () => <ToolConfig schemaName="diff-leverage" source="bridge" />,
};
