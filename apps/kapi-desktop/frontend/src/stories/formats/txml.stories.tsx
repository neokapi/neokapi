import type { Meta, StoryObj } from "@storybook/react-vite";

import { FormatConfig } from "../_lib/schema-story";

const meta: Meta = {
  title: "Formats & Tools/Formats/Localization/TXML Filter",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;
export const OkapiConfig: Story = {
  name: "Configuration",
  render: () => <FormatConfig schemaName="okf_txml" source="bridge" />,
};
