import { Button } from "@neokapi/ui-primitives";

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
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="w-full max-w-sm rounded-xl border border-border bg-background p-6 shadow-lg">
        <h2 className="mb-2 text-lg font-semibold">Flow Running</h2>
        <p className="mb-5 text-sm text-muted-foreground">
          A flow is still running. Do you want to cancel it and close, or keep it running?
        </p>
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={onCancelFlow}>
            Cancel Flow &amp; Close
          </Button>
          <Button onClick={onKeepRunning}>Keep Running</Button>
        </div>
      </div>
    </div>
  );
}
