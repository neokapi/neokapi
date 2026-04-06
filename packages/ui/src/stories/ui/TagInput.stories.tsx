import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { TagInput } from "../../components/ui/tag-input";

function Wrapper({
  initial = [],
  placeholder,
  disabled,
}: {
  initial?: string[];
  placeholder?: string;
  disabled?: boolean;
}) {
  const [value, setValue] = useState(initial);
  return (
    <div className="max-w-sm space-y-2">
      <TagInput value={value} onChange={setValue} placeholder={placeholder} disabled={disabled} />
      <pre className="p-2 rounded bg-muted text-xs font-mono">{value.join(", ") || "(empty)"}</pre>
    </div>
  );
}

const meta: Meta<typeof TagInput> = {
  title: "Foundations/TagInput",
  component: TagInput,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Chip-based input for tags and comma-separated values. Type and press Enter or comma to add. Backspace removes the last tag. Click the X on a chip to remove it.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof TagInput>;

export const Empty: Story = {
  render: () => <Wrapper placeholder="Add tag..." />,
};

export const WithTags: Story = {
  render: () => <Wrapper initial={["HTML", "XML", "JSON", "YAML"]} />,
};

export const FileExtensions: Story = {
  render: () => <Wrapper initial={[".html", ".htm", ".xhtml"]} placeholder="Add extension..." />,
};

export const Disabled: Story = {
  render: () => <Wrapper initial={["read-only", "tags"]} disabled />,
};
