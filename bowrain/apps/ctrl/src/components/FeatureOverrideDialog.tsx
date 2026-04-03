import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Button,
  Input,
  Label,
  Switch,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@neokapi/ui";
import { setFeatureOverride } from "../api";

const FEATURES = [
  "bravo-code-exec",
  "connectors-git",
  "connectors-custom",
  "api-access",
  "sso-saml",
  "custom-mt-providers",
] as const;

interface FeatureOverrideDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string;
}

export function FeatureOverrideDialog({
  open,
  onOpenChange,
  workspaceId,
}: FeatureOverrideDialogProps) {
  const queryClient = useQueryClient();
  const [feature, setFeature] = useState<string>(FEATURES[0]);
  const [enabled, setEnabled] = useState(true);
  const [reason, setReason] = useState("");
  const [expiresAt, setExpiresAt] = useState("");

  const mutation = useMutation({
    mutationFn: () =>
      setFeatureOverride(workspaceId, feature, enabled, reason, expiresAt || undefined),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ["admin", "workspace", workspaceId, "overrides"],
      });
      void queryClient.invalidateQueries({ queryKey: ["admin", "overrides"] });
      onOpenChange(false);
      setReason("");
      setExpiresAt("");
    },
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle>Add Feature Override</DialogTitle>
          <DialogDescription>
            Override a feature flag for this workspace, bypassing the plan matrix.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="space-y-2">
            <Label>Feature</Label>
            <select
              value={feature}
              onChange={(e) => setFeature(e.target.value)}
              className="w-full h-9 rounded-md border bg-background px-3 text-sm"
            >
              {FEATURES.map((f) => (
                <option key={f} value={f}>
                  {f}
                </option>
              ))}
            </select>
          </div>
          <div className="flex items-center justify-between">
            <Label>Enabled</Label>
            <Switch checked={enabled} onCheckedChange={setEnabled} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="override-reason">Reason</Label>
            <Input
              id="override-reason"
              placeholder="e.g. Beta program participant"
              value={reason}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => setReason(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="override-expires">Expires At (optional)</Label>
            <Input
              id="override-expires"
              type="date"
              value={expiresAt}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => setExpiresAt(e.target.value)}
            />
          </div>
          {mutation.error && (
            <p className="text-sm text-destructive">
              {mutation.error instanceof Error ? mutation.error.message : "Failed to set override"}
            </p>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={() => mutation.mutate()} disabled={!reason || mutation.isPending}>
            Save Override
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
