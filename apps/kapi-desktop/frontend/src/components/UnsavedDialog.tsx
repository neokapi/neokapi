import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@neokapi/ui-primitives";

interface UnsavedDialogProps {
  saving?: boolean;
  onSave: () => void;
  onDiscard: () => void;
  onCancel: () => void;
}

export function UnsavedDialog({ saving = false, onSave, onDiscard, onCancel }: UnsavedDialogProps) {
  return (
    <Dialog
      open
      onOpenChange={(o) => {
        if (!o && !saving) onCancel();
      }}
    >
      <DialogContent showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>Unsaved changes</DialogTitle>
          <DialogDescription>Do you want to save changes before closing?</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button disabled={saving} variant="outline" onClick={onDiscard}>
            Don&apos;t Save
          </Button>
          <Button disabled={saving} variant="outline" onClick={onCancel}>
            Cancel
          </Button>
          <Button disabled={saving} onClick={onSave}>
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
