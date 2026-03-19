import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Button, Label } from "@neokapi/ui";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@neokapi/ui/components/ui/dialog";
import { updatePlan } from "../api";

const PLANS = ["free", "pro", "team", "enterprise"] as const;

interface ChangePlanDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string;
  currentPlan: string;
}

export function ChangePlanDialog({
  open,
  onOpenChange,
  workspaceId,
  currentPlan,
}: ChangePlanDialogProps) {
  const queryClient = useQueryClient();
  const [selectedPlan, setSelectedPlan] = useState(currentPlan);

  const mutation = useMutation({
    mutationFn: () => updatePlan(workspaceId, selectedPlan),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["admin", "workspace", workspaceId] });
      void queryClient.invalidateQueries({ queryKey: ["admin", "workspaces"] });
      onOpenChange(false);
    },
  });

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
            <Label>Select Plan</Label>
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
          {mutation.error && (
            <p className="text-sm text-destructive">
              {mutation.error instanceof Error ? mutation.error.message : "Failed to change plan"}
            </p>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={() => mutation.mutate()}
            disabled={selectedPlan === currentPlan || mutation.isPending}
          >
            Change Plan
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
