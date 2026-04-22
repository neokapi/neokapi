import type { Meta, StoryObj } from "@storybook/react-vite";

import { FormatCompare, FormatConfig } from "../_lib/schema-story";

const meta: Meta = {
  title: "Formats & Tools/Formats/Document/Wiki Format",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;
export const NativeConfig: Story = {
  name: "Native Configuration",
  render: () => <FormatConfig schemaName="wiki" source="builtIn" />,
};
export const OkapiConfig: Story = {
  name: "Okapi Configuration",
  render: () => <FormatConfig schemaName="okf_wiki" source="bridge" />,
};
export const Compare: Story = {
  name: "Side by Side",
  render: () => <FormatCompare nativeName="wiki" okapiName="okf_wiki" />,
};
