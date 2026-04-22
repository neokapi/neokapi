/**
 * Word Count
 */
import type { Meta, StoryObj } from "@storybook/react-vite";

import { ToolConfig } from "../_lib/schema-story";

const meta: Meta = {
  title: "Formats & Tools/Tools/Analysis/Word Count",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;
export const OkapiConfig: Story = {
  name: "Okapi Configuration",
  render: () => <ToolConfig schemaName="word-count" source="bridge" />,
};
