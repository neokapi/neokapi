import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Button, Input, Label } from "@neokapi/ui";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@neokapi/ui/components/ui/dialog";
import { grantCredits } from "../api";

interface GrantCreditsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string;
}

export function GrantCreditsDialog({ open, onOpenChange, workspaceId }: GrantCreditsDialogProps) {
  const queryClient = useQueryClient();
  const [amount, setAmount] = useState("");
  const [reason, setReason] = useState("");

  const mutation = useMutation({
    mutationFn: () => grantCredits(workspaceId, Number(amount), reason),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["admin", "workspace", workspaceId] });
      void queryClient.invalidateQueries({
        queryKey: ["admin", "workspace", workspaceId, "ledger"],
      });
      onOpenChange(false);
      setAmount("");
      setReason("");
    },
  });

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
            <Label htmlFor="amount">Amount (tokens)</Label>
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
            <Label htmlFor="reason">Reason</Label>
            <Input
              id="reason"
              placeholder="e.g. Support compensation for outage"
              value={reason}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => setReason(e.target.value)}
            />
          </div>
          {mutation.error && (
            <p className="text-sm text-destructive">
              {mutation.error instanceof Error ? mutation.error.message : "Failed to grant credits"}
            </p>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={() => mutation.mutate()}
            disabled={!amount || Number(amount) <= 0 || !reason || mutation.isPending}
          >
            Grant Credits
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
