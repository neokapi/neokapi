import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";

export interface ContextStoreApprovalDialogProps {
  /** The folder the recipe keeps its context in; the dialog is open while it is set. */
  location: string | null;
  onAllow: () => void;
  onCancel: () => void;
}

/**
 * Asked once per project before the first sync through a folder the recipe
 * names. Syncing writes the project's decisions, terms and voice there and
 * merges in what the folder holds, so whoever can write to it can add to the
 * project's context. The answer is remembered for this folder.
 */
export function ContextStoreApprovalDialog({
  location,
  onAllow,
  onCancel,
}: ContextStoreApprovalDialogProps) {
  return (
    <Dialog
      open={location !== null}
      onOpenChange={(o) => {
        if (!o) onCancel();
      }}
    >
      <DialogContent data-testid="context-store-approval">
        <DialogHeader>
          <DialogTitle>{t("Share through this folder?")}</DialogTitle>
          <DialogDescription>
            {t(
              "Syncing writes this project's decisions, terms and voice to the folder, and adds what other machines put there. Anyone who can write to it can add to this project's context.",
            )}
          </DialogDescription>
        </DialogHeader>
        <p
          data-slot="context-store-location"
          className="break-all font-mono text-sm"
          translate="no"
        >
          {location}
        </p>
        <DialogFooter>
          <Button variant="outline" onClick={onCancel}>
            {t("Cancel")}
          </Button>
          <Button onClick={onAllow} className="sm:ml-4">
            {t("Allow and sync")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
