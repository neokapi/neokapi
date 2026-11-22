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
import { AlertCircle, FileText, Loader2 } from "lucide-react";
import type { CloseRow, UnsavedTab } from "../hooks/useWindowCloseGuard";

interface WindowCloseDialogProps {
  /** Each tab that holds the window, in tab order, with its reasons. */
  rows: CloseRow[];
  /** The tabs whose last save failed; each is marked in the list. */
  failed?: UnsavedTab[];
  saving?: boolean;
  onSaveAll: () => void;
  onDiscardAll: () => void;
  onCancel: () => void;
}

/**
 * WindowCloseDialog asks once, when the window is closing, about every tab
 * that holds it: unsaved changes, a running flow, or both. With unsaved
 * changes it offers Save all, Discard all or Cancel; a running flow stops when
 * the window closes. With only running flows it offers what closing a tab
 * offers: Cancel Flow & Close, or Keep running.
 */
export function WindowCloseDialog({
  rows,
  failed = [],
  saving = false,
  onSaveAll,
  onDiscardAll,
  onCancel,
}: WindowCloseDialogProps) {
  const failedIDs = new Set(failed.map((tab) => tab.id));
  const anyUnsaved = rows.some((row) => row.unsaved);
  const anyRunning = rows.some((row) => row.running);
  // Focus starts on the action that keeps the work, so Enter never drops it.
  const keepRef = useRef<HTMLButtonElement>(null);

  const title = !anyUnsaved
    ? t("A flow is running")
    : anyRunning
      ? t("Unsaved changes and a running flow")
      : t("Unsaved changes");
  const description = !anyUnsaved
    ? t("Cancel the flow and close, or keep it running?")
    : anyRunning
      ? t("Save before closing? Running flows stop when the window closes.")
      : t("Save before closing?");

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
          keepRef.current?.focus();
        }}
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <ul className="flex flex-col gap-1" aria-label={t("Projects that keep the window open")}>
          {rows.map((row) => (
            <li
              key={row.id}
              className="flex items-center gap-2 text-sm"
              data-failed={failedIDs.has(row.id) || undefined}
            >
              {failedIDs.has(row.id) ? (
                <AlertCircle size={14} className="shrink-0 text-destructive" aria-hidden />
              ) : row.running && !row.unsaved ? (
                <Loader2 size={14} className="shrink-0 text-muted-foreground" aria-hidden />
              ) : (
                <FileText size={14} className="shrink-0 text-muted-foreground" aria-hidden />
              )}
              <span className="truncate">{row.name}</span>
              {row.unsaved && (
                <span className="text-xs text-muted-foreground">{t("Unsaved changes")}</span>
              )}
              {row.running && (
                <span className="text-xs text-muted-foreground">{t("Flow running")}</span>
              )}
              {failedIDs.has(row.id) && (
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
          {anyUnsaved ? (
            <>
              <Button disabled={saving} variant="outline" onClick={onDiscardAll}>
                Discard all
              </Button>
              <Button disabled={saving} variant="outline" onClick={onCancel}>
                Cancel
              </Button>
              <Button ref={keepRef} disabled={saving} onClick={onSaveAll} className="sm:ml-4">
                Save all
              </Button>
            </>
          ) : (
            <>
              <Button disabled={saving} variant="outline" onClick={onDiscardAll}>
                Cancel Flow &amp; Close
              </Button>
              <Button ref={keepRef} disabled={saving} onClick={onCancel} className="sm:ml-4">
                Keep running
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
