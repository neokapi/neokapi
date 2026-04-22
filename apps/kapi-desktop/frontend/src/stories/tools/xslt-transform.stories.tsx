/**
 * XSLT Transform
 */
import type { Meta, StoryObj } from "@storybook/react-vite";

import { ToolConfig } from "../_lib/schema-story";

const meta: Meta = {
  title: "Formats & Tools/Tools/Other/XSLT Transform",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;
export const NativeConfig: Story = {
  name: "Configuration",
  render: () => <ToolConfig schemaName="xslt-transform" source="builtIn" />,
};
