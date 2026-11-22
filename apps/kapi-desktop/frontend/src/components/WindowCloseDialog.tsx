import { useRef } from "react";
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
import { AlertCircle, FileText } from "lucide-react";
import type { UnsavedTab } from "../hooks/useWindowCloseGuard";

interface WindowCloseDialogProps {
  /** The tabs with unsaved changes, in tab order. */
  tabs: UnsavedTab[];
  /** The tabs whose last save failed; each is marked in the list. */
  failed?: UnsavedTab[];
  saving?: boolean;
  onSaveAll: () => void;
  onDiscardAll: () => void;
  onCancel: () => void;
}

/**
 * WindowCloseDialog asks once, when the window is closing, about every tab
 * with unsaved changes: Save all, Discard all or Cancel.
 */
export function WindowCloseDialog({
  tabs,
  failed = [],
  saving = false,
  onSaveAll,
  onDiscardAll,
  onCancel,
}: WindowCloseDialogProps) {
  const failedIDs = new Set(failed.map((tab) => tab.id));
  // Focus starts on Save all, so Enter keeps the work rather than discarding it.
  const saveAllRef = useRef<HTMLButtonElement>(null);
  return (
    <Dialog
      open
      onOpenChange={(o) => {
        if (!o && !saving) onCancel();
      }}
    >
      <DialogContent
        showCloseButton={false}
        data-testid="window-close-dialog"
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          saveAllRef.current?.focus();
        }}
      >
        <DialogHeader>
          <DialogTitle>Unsaved changes</DialogTitle>
          <DialogDescription>Save before closing?</DialogDescription>
        </DialogHeader>
        <ul className="flex flex-col gap-1" aria-label={t("Projects with unsaved changes")}>
          {tabs.map((tab) => (
            <li
              key={tab.id}
              className="flex items-center gap-2 text-sm"
              data-failed={failedIDs.has(tab.id) || undefined}
            >
              {failedIDs.has(tab.id) ? (
                <AlertCircle size={14} className="shrink-0 text-destructive" aria-hidden />
              ) : (
                <FileText size={14} className="shrink-0 text-muted-foreground" aria-hidden />
              )}
              <span className="truncate">{tab.name}</span>
              {failedIDs.has(tab.id) && (
                <span className="text-xs text-destructive">{t("Not saved")}</span>
              )}
            </li>
          ))}
        </ul>
        {failed.length > 0 && (
          <p className="text-sm text-destructive" role="alert">
            {t("Could not save {names}. The window stays open.", {
              names: failed.map((tab) => tab.name).join(", "),
            })}
          </p>
        )}
        <DialogFooter>
          <Button disabled={saving} variant="outline" onClick={onDiscardAll}>
            Discard all
          </Button>
          <Button disabled={saving} variant="outline" onClick={onCancel}>
            Cancel
          </Button>
          <Button ref={saveAllRef} disabled={saving} onClick={onSaveAll} className="sm:ml-4">
            Save all
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
