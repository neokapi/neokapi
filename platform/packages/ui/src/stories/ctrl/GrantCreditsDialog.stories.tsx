import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "@neokapi/ui-primitives/components/ui/button";
import { Input } from "@neokapi/ui-primitives/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@neokapi/ui-primitives/components/ui/dialog";

interface GrantCreditsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string;
}

function GrantCreditsDialog({ open, onOpenChange }: GrantCreditsDialogProps) {
  const [amount, setAmount] = useState("");
  const [reason, setReason] = useState("");

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle>Grant Credits</DialogTitle>
          <DialogDescription>
            Grant bonus credits to this workspace. This will be recorded in the credit ledger.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="space-y-2">
            <label htmlFor="amount" className="text-sm font-medium">
              Amount (tokens)
            </label>
            <Input
              id="amount"
              type="number"
              min="1"
              placeholder="e.g. 200000"
              value={amount}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => setAmount(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <label htmlFor="reason" className="text-sm font-medium">
              Reason
            </label>
            <Input
              id="reason"
              placeholder="e.g. Support compensation for outage"
              value={reason}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => setReason(e.target.value)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button disabled={!amount || Number(amount) <= 0 || !reason}>Grant Credits</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

const meta: Meta<typeof GrantCreditsDialog> = {
  title: "Ctrl/GrantCreditsDialog",
  component: GrantCreditsDialog,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof GrantCreditsDialog>;

export const Open: Story = {
  args: { open: true, onOpenChange: () => {}, workspaceId: "ws-1" },
};

export const Interactive: Story = {
  render: () => {
    const [open, setOpen] = useState(false);
    return (
      <div>
        <Button onClick={() => setOpen(true)}>Grant Credits</Button>
        <GrantCreditsDialog open={open} onOpenChange={setOpen} workspaceId="ws-1" />
      </div>
    );
  },
};
