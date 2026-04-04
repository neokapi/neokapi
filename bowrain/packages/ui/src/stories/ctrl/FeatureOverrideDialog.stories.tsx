import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Switch,
} from "@neokapi/ui-primitives";
import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";

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

function FeatureOverrideDialog({ open, onOpenChange }: FeatureOverrideDialogProps) {
  const [feature, setFeature] = useState<string>(FEATURES[0]);
  const [enabled, setEnabled] = useState(true);
  const [reason, setReason] = useState("");
  const [expiresAt, setExpiresAt] = useState("");

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
            <label className="text-sm font-medium">Feature</label>
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
            <label className="text-sm font-medium">Enabled</label>
            <Switch checked={enabled} onCheckedChange={setEnabled} />
          </div>
          <div className="space-y-2">
            <label htmlFor="override-reason" className="text-sm font-medium">
              Reason
            </label>
            <Input
              id="override-reason"
              placeholder="e.g. Beta program participant"
              value={reason}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => setReason(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <label htmlFor="override-expires" className="text-sm font-medium">
              Expires At (optional)
            </label>
            <Input
              id="override-expires"
              type="date"
              value={expiresAt}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => setExpiresAt(e.target.value)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button disabled={!reason}>Save Override</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

const meta: Meta<typeof FeatureOverrideDialog> = {
  title: "Ctrl/FeatureOverrideDialog",
  component: FeatureOverrideDialog,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof FeatureOverrideDialog>;

export const Open: Story = {
  args: { open: true, onOpenChange: () => {}, workspaceId: "ws-1" },
};

export const Interactive: Story = {
  render: () => {
    const [open, setOpen] = useState(false);
    return (
      <div>
        <Button onClick={() => setOpen(true)}>Add Feature Override</Button>
        <FeatureOverrideDialog open={open} onOpenChange={setOpen} workspaceId="ws-1" />
      </div>
    );
  },
};
