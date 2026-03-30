import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { BulkActionBar } from "../../components/resource-browser/BulkActionBar";

const meta: Meta<typeof BulkActionBar> = {
  title: "Resource Browser/BulkActionBar",
  component: BulkActionBar,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Floating bottom bar that appears when items are selected. Shows selection count and bulk action buttons. Delete requires a two-click confirmation.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof BulkActionBar>;

export const FewSelected: Story = {
  args: {
    selectedCount: 3,
    onDelete: () => {},
    onDeselectAll: () => {},
  },
};

export const ManySelected: Story = {
  args: {
    selectedCount: 42,
    onDelete: () => {},
    onDeselectAll: () => {},
  },
};

export const WithAnnotateEntities: Story = {
  args: {
    selectedCount: 12,
    onDelete: () => {},
    onAnnotateEntities: () => {},
    onDeselectAll: () => {},
  },
};

export const ConfirmDelete: Story = {
  args: {
    selectedCount: 5,
    onDelete: () => {},
    confirmDelete: true,
    onDeselectAll: () => {},
  },
};

/** Interactive demo showing the full delete confirmation flow. */
export const Interactive: Story = {
  render: function InteractiveBulkAction() {
    const [confirmDelete, setConfirmDelete] = useState(false);
    const [count, setCount] = useState(7);

    return (
      <div>
        <p className="mb-4 text-sm text-muted-foreground">
          Click Delete to see the confirmation state. Click Cancel or Deselect all to reset.
        </p>
        <BulkActionBar
          selectedCount={count}
          onDelete={() => {
            if (confirmDelete) {
              setCount(0);
              setConfirmDelete(false);
            } else {
              setConfirmDelete(true);
            }
          }}
          confirmDelete={confirmDelete}
          onAnnotateEntities={() => {}}
          onDeselectAll={() => {
            setCount(0);
            setConfirmDelete(false);
          }}
        />
      </div>
    );
  },
};
