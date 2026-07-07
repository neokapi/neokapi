import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { UserTable } from "./UserTable";
import { sampleUsers } from "../ctrl-fixtures";

const meta: Meta<typeof UserTable> = {
  title: "Ctrl/UserTable",
  component: UserTable,
  parameters: { layout: "padded" },
  args: { onRowClick: fn() },
  decorators: [
    (Story) => (
      <div className="w-full max-w-4xl">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof UserTable>;

export const Default: Story = {
  args: { users: sampleUsers, loading: false, selectedUserId: null },
};

export const RowSelected: Story = {
  args: { users: sampleUsers, loading: false, selectedUserId: "usr_2" },
};

export const Loading: Story = {
  args: { users: [], loading: true, selectedUserId: null },
};

export const Empty: Story = {
  args: { users: [], loading: false, selectedUserId: null },
};
