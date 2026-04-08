import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { InlineCodeLegend } from "../../components/editor/InlineCodeLegend";
import { richSpans, simpleBoldSpans, linkAndItalicSpans } from "./fixtures";

const meta: Meta<typeof InlineCodeLegend> = {
  title: "Editor/Tags/InlineCodeLegend",
  component: InlineCodeLegend,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 320, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof InlineCodeLegend>;

function LegendWrapper({ spans }: { spans: typeof richSpans }) {
  const [open, setOpen] = useState(true);
  if (!open) return <button onClick={() => setOpen(true)}>Show legend</button>;
  return <InlineCodeLegend spans={spans} onClose={() => setOpen(false)} />;
}

export const AllTagTypes: Story = {
  render: () => <LegendWrapper spans={richSpans} />,
};

export const BoldOnly: Story = {
  render: () => <LegendWrapper spans={simpleBoldSpans} />,
};

export const LinksAndItalic: Story = {
  render: () => <LegendWrapper spans={linkAndItalicSpans} />,
};

export const Empty: Story = {
  args: { spans: [], onClose: () => {} },
};
