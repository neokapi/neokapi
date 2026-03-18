import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { BravoApprovalCard } from "../../components/bravo/BravoApprovalCard";

const meta: Meta<typeof BravoApprovalCard> = {
  title: "Bravo/BravoApprovalCard",
  component: BravoApprovalCard,
  tags: ["autodocs"],
  decorators: [(Story) => <div style={{ maxWidth: 400, padding: 16 }}><Story /></div>],
};

export default meta;
type Story = StoryObj<typeof BravoApprovalCard>;

export const Default: Story = {
  args: {
    toolCallId: "tc-1",
    toolName: "connector_push",
    input: { connector_id: "git-main", project_id: "proj-1" },
    onApprove: fn(),
    onDeny: fn(),
  },
};

export const NoInput: Story = {
  args: {
    toolCallId: "tc-2",
    toolName: "connector_pull",
    onApprove: fn(),
    onDeny: fn(),
  },
};

export const Loading: Story = {
  args: {
    toolCallId: "tc-3",
    toolName: "execute_script",
    input: { language: "python", code: "import os; os.listdir('/')" },
    onApprove: fn(),
    onDeny: fn(),
    loading: true,
  },
};
