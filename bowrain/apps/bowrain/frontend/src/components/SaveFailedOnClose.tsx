import { Button } from "@neokapi/ui-primitives/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@neokapi/ui-primitives/components/ui/dialog";
import { t } from "@neokapi/i18n-react/runtime";
import type { FlushFailure } from "../hooks/useFlushOnClose";

interface SaveFailedOnCloseProps {
  failure: FlushFailure;
  onCloseAnyway: () => void;
  onStay: () => void;
}

/**
 * Shown when the window was closing and a waiting save did not reach the
 * server. The window stays open until the person chooses.
 */
export function SaveFailedOnClose({ failure, onCloseAnyway, onStay }: SaveFailedOnCloseProps) {
  return (
    <Dialog
      open
      onOpenChange={(o) => {
        if (!o) onStay();
      }}
    >
      <DialogContent showCloseButton={false} data-testid="save-failed-on-close">
        <DialogHeader>
          <DialogTitle>{t("Changes not saved")}</DialogTitle>
          <DialogDescription>
            {failure.held
              ? t(
                  "{count} changes are waiting for their account to sign in again. Closing the window discards them.",
                  { count: failure.held },
                )
              : failure.timedOut
                ? t("The server did not answer in time.")
                : t("The server did not accept every change.")}
          </DialogDescription>
        </DialogHeader>
        {failure.errors.length > 0 && (
          <ul className="flex flex-col gap-1 text-sm text-destructive" role="alert">
            {failure.errors.map(({ key, label, error }) => (
              <li key={key} className="break-words">
                {label}: {error instanceof Error ? error.message : String(error)}
              </li>
            ))}
          </ul>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={onCloseAnyway}>
            Close anyway
          </Button>
          <Button autoFocus onClick={onStay} className="sm:ml-4">
            Stay
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
