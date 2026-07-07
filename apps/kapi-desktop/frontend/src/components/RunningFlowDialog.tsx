import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@neokapi/ui-primitives";

interface RunningFlowDialogProps {
  /** Called when user chooses to cancel the running flow and proceed. */
  onCancelFlow: () => void;
  /** Called when user chooses to keep the flow running (dismiss dialog). */
  onKeepRunning: () => void;
}

/**
 * Confirmation dialog shown when closing a project tab or quitting
 * the app while a flow is still running.
 */
export function RunningFlowDialog({ onCancelFlow, onKeepRunning }: RunningFlowDialogProps) {
  return (
    <Dialog
      open
      onOpenChange={(o) => {
        if (!o) onKeepRunning();
      }}
    >
      <DialogContent showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>Flow Running</DialogTitle>
          <DialogDescription>
            A flow is still running. Do you want to cancel it and close, or keep it running?
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={onCancelFlow}>
            Cancel Flow &amp; Close
          </Button>
          <Button onClick={onKeepRunning}>Keep Running</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
