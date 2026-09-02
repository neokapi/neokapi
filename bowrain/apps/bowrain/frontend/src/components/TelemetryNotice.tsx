import { useState } from "react";
import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@neokapi/ui";
import {
  analyticsAvailable,
  isNoticeShown,
  markNoticeShown,
  setTelemetryEnabled,
} from "../analytics";

/**
 * One-time first-run notice for the opt-out desktop analytics (decision D1).
 * Shown once per install (persisted with the telemetry setting), only in
 * builds that carry an analytics key and when Do Not Track is unset. The
 * off-switch is inline; the settings page carries the same toggle afterwards.
 */
export function TelemetryNotice() {
  const [open, setOpen] = useState(() => analyticsAvailable() && !isNoticeShown());
  if (!open) return null;

  const close = () => {
    markNoticeShown();
    setOpen(false);
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) close();
      }}
    >
      <DialogContent className="sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle>Anonymous usage statistics</DialogTitle>
          <DialogDescription>
            Bowrain collects anonymous usage statistics: page views and feature usage, never your
            content, project names, or file paths. This helps prioritize improvements. You can turn
            it off now, or at any time in Settings.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => {
              setTelemetryEnabled(false);
              close();
            }}
          >
            Disable
          </Button>
          <Button onClick={close}>OK</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
