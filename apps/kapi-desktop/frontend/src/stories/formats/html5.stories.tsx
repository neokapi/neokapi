import type { Meta, StoryObj } from "@storybook/react-vite";

import { FormatConfig } from "../_lib/schema-story";

const meta: Meta = {
  title: "Formats & Tools/Formats/Document/HTML5-ITS Filter",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;
export const OkapiConfig: Story = {
  name: "Configuration",
  render: () => <FormatConfig schemaName="okf_html5" source="bridge" />,
};
