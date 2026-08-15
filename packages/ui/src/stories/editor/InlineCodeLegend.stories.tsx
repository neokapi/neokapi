import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { InlineCodeLegend } from "../../components/editor/InlineCodeLegend";
import { richSpans, simpleBoldSpans, linkAndItalicSpans, unknownTypeSpans } from "./fixtures";

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

/**
 * A span type no vocabulary pack defines. The chip reads "widget>", derived
 * from the type name — the bowrain kit's story of the same name renders it
 * identically, because both kits resolve through one registry.
 */
export const UnknownType: Story = {
  render: () => <LegendWrapper spans={unknownTypeSpans} />,
};

export const Empty: Story = {
  args: { spans: [], onClose: () => {} },
};
