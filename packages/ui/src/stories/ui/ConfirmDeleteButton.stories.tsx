import type { Meta, StoryObj } from "@storybook/react-vite";
import { ConfirmDeleteButton } from "../../components/ui/confirm-delete-button";

const meta: Meta<typeof ConfirmDeleteButton> = {
  title: "Foundations/ConfirmDeleteButton",
  component: ConfirmDeleteButton,
  tags: ["autodocs"],
  args: {
    onDelete: () => alert("Deleted!"),
  },
  parameters: {
    docs: {
      description: {
        component:
          "Two-click delete button. Click once to arm, click Confirm to execute. Three modes: icon (ghost trash icon), text (Delete button), inline (small footer text).",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof ConfirmDeleteButton>;

export const IconMode: Story = {
  name: "Icon Mode (default)",
  args: { mode: "icon" },
};

export const TextMode: Story = {
  name: "Text Mode",
  args: { mode: "text" },
};

export const InlineMode: Story = {
  name: "Inline Mode (card footers)",
  args: { mode: "inline" },
};

export const AllModes: Story = {
  name: "All Modes Side by Side",
  render: () => (
    <div className="flex items-center gap-6">
      <div className="text-center">
        <ConfirmDeleteButton onDelete={() => {}} mode="icon" />
        <p className="mt-1 text-[10px] text-muted-foreground">icon</p>
      </div>
      <div className="text-center">
        <ConfirmDeleteButton onDelete={() => {}} mode="text" />
        <p className="mt-1 text-[10px] text-muted-foreground">text</p>
      </div>
      <div className="text-center">
        <ConfirmDeleteButton onDelete={() => {}} mode="inline" />
        <p className="mt-1 text-[10px] text-muted-foreground">inline</p>
      </div>
    </div>
  ),
};
