import { Button } from "@neokapi/ui-primitives";

interface UnsavedDialogProps {
  onSave: () => void;
  onDiscard: () => void;
  onCancel: () => void;
}

export function UnsavedDialog({ onSave, onDiscard, onCancel }: UnsavedDialogProps) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="w-full max-w-sm rounded-xl border border-border bg-background p-6 shadow-lg">
        <h2 className="mb-2 text-lg font-semibold">Unsaved Changes</h2>
        <p className="mb-5 text-sm text-muted-foreground">
          Do you want to save changes before closing?
        </p>
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={onDiscard}>
            Don&apos;t Save
          </Button>
          <Button variant="outline" onClick={onCancel}>
            Cancel
          </Button>
          <Button onClick={onSave}>Save</Button>
        </div>
      </div>
    </div>
  );
}
