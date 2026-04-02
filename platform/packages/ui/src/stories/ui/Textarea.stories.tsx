import type { Meta, StoryObj } from "@storybook/react-vite";
import { Textarea } from "@neokapi/ui-primitives/components/ui/textarea";
import { Label } from "@neokapi/ui-primitives/components/ui/label";

const meta: Meta<typeof Textarea> = {
  title: "Foundations/Textarea",
  component: Textarea,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 400, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof Textarea>;

export const Default: Story = {
  args: { placeholder: "Enter translation notes..." },
};

export const WithLabel: Story = {
  render: () => (
    <div className="grid w-full gap-1.5">
      <Label htmlFor="notes">Translation Notes</Label>
      <Textarea id="notes" placeholder="Add context for translators..." />
    </div>
  ),
};

export const Disabled: Story = {
  args: {
    placeholder: "This textarea is disabled",
    disabled: true,
  },
};
