import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "../../components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "../../components/ui/dialog";

interface AddNoteDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string;
}

function AddNoteDialog({ open, onOpenChange }: AddNoteDialogProps) {
  const [content, setContent] = useState("");

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle>Add Internal Note</DialogTitle>
          <DialogDescription>
            Add a note visible only to admins. Useful for tracking support interactions and
            decisions.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="space-y-2">
            <label htmlFor="note-content" className="text-sm font-medium">
              Note
            </label>
            <textarea
              id="note-content"
              className="w-full min-h-[100px] rounded-md border bg-background px-3 py-2 text-sm resize-y"
              placeholder="e.g. Customer called about upgrade options. Offered 2-week Pro trial."
              value={content}
              onChange={(e) => setContent(e.target.value)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button disabled={!content.trim()}>Add Note</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

const meta: Meta<typeof AddNoteDialog> = {
  title: "Ctrl/AddNoteDialog",
  component: AddNoteDialog,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof AddNoteDialog>;

export const Open: Story = {
  args: { open: true, onOpenChange: () => {}, workspaceId: "ws-1" },
};

export const Interactive: Story = {
  render: () => {
    const [open, setOpen] = useState(false);
    return (
      <div>
        <Button onClick={() => setOpen(true)}>Add Note</Button>
        <AddNoteDialog open={open} onOpenChange={setOpen} workspaceId="ws-1" />
      </div>
    );
  },
};
