import type { Meta, StoryObj } from "@storybook/react-vite";

import { FormatConfig } from "../_lib/schema-story";

const meta: Meta = {
  title: "Formats & Tools/Formats/Data/CSV Format",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;
export const NativeConfig: Story = {
  name: "Configuration",
  render: () => <FormatConfig schemaName="csv" source="builtIn" />,
};
