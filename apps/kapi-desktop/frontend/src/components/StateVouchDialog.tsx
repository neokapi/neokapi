import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@neokapi/ui-primitives";
import type { StateToVouch } from "../types/api";

interface StateVouchDialogProps {
  state: StateToVouch;
  busy?: boolean;
  onVouch: () => void;
  onCancel: () => void;
}

// Asks the person to vouch for state kapi on this machine did not write
// before the project opens: the entries and where they appear to come from.
export function StateVouchDialog({
  state,
  busy = false,
  onVouch,
  onCancel,
}: StateVouchDialogProps) {
  return (
    <Dialog
      open
      onOpenChange={(o) => {
        if (!o && !busy) onCancel();
      }}
    >
      <DialogContent showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>State this machine did not write</DialogTitle>
          <DialogDescription>
            The project holds state kapi on this machine did not write. Use it only if it is yours:
            a copy, a restored backup or a project moved to another disk.
          </DialogDescription>
        </DialogHeader>
        <ul className="text-sm font-mono">
          {state.entries.map((e) => (
            <li key={e}>{e}</li>
          ))}
        </ul>
        <p className="text-sm text-muted-foreground">
          {state.origin
            ? `It appears to come from ${state.origin}.`
            : "Nothing in it says where it comes from."}
        </p>
        <DialogFooter>
          <Button disabled={busy} variant="outline" onClick={onCancel}>
            Cancel
          </Button>
          <Button disabled={busy} onClick={onVouch}>
            Use as this machine&apos;s own
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
