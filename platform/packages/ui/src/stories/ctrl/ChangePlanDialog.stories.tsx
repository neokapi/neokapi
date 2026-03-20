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

const PLANS = ["free", "pro", "team", "enterprise"] as const;

interface ChangePlanDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string;
  currentPlan: string;
}

function ChangePlanDialog({ open, onOpenChange, currentPlan }: ChangePlanDialogProps) {
  const [selectedPlan, setSelectedPlan] = useState(currentPlan);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle>Change Plan</DialogTitle>
          <DialogDescription>
            Current plan: {currentPlan}. Select a new plan for this workspace.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="space-y-2">
            <label className="text-sm font-medium">Select Plan</label>
            <div className="grid grid-cols-2 gap-2">
              {PLANS.map((plan) => (
                <button
                  key={plan}
                  onClick={() => setSelectedPlan(plan)}
                  className={`rounded-md border px-3 py-2 text-sm capitalize cursor-pointer transition-colors ${
                    selectedPlan === plan
                      ? "border-primary bg-primary/10 text-primary font-medium"
                      : "border-border hover:bg-muted"
                  }`}
                >
                  {plan}
                </button>
              ))}
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button disabled={selectedPlan === currentPlan}>Change Plan</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

const meta: Meta<typeof ChangePlanDialog> = {
  title: "Ctrl/ChangePlanDialog",
  component: ChangePlanDialog,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof ChangePlanDialog>;

export const FromFree: Story = {
  args: { open: true, onOpenChange: () => {}, workspaceId: "ws-1", currentPlan: "free" },
};

export const FromPro: Story = {
  args: { open: true, onOpenChange: () => {}, workspaceId: "ws-1", currentPlan: "pro" },
};

export const FromTeam: Story = {
  args: { open: true, onOpenChange: () => {}, workspaceId: "ws-1", currentPlan: "team" },
};

export const FromEnterprise: Story = {
  args: { open: true, onOpenChange: () => {}, workspaceId: "ws-1", currentPlan: "enterprise" },
};

export const Interactive: Story = {
  render: () => {
    const [open, setOpen] = useState(false);
    return (
      <div>
        <Button onClick={() => setOpen(true)}>Change Plan</Button>
        <ChangePlanDialog
          open={open}
          onOpenChange={setOpen}
          workspaceId="ws-1"
          currentPlan="free"
        />
      </div>
    );
  },
};
