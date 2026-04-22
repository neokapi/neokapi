/**
 * Sentence Alignment
 */
import type { Meta, StoryObj } from "@storybook/react-vite";

import { ToolConfig } from "../_lib/schema-story";

const meta: Meta = {
  title: "Formats & Tools/Tools/Alignment/Sentence Alignment",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;
export const OkapiConfig: Story = {
  name: "Configuration",
  render: () => <ToolConfig schemaName="sentence-aligner" source="bridge" />,
};
