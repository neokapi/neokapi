import { useSyncExternalStore } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { subscribePendingSaves, waitingEditCount } from "@neokapi/bowrain-app";

/**
 * Says how many edits have not reached the server, on the screens shown while
 * no one is signed in. It names none of them: whoever is at the machine may
 * not be the person who made them. The edits stay recorded for that person,
 * and signing in again as them, in the same workspace, lists and saves them.
 */
export function UnsavedEditsNotice() {
  const count = useSyncExternalStore(subscribePendingSaves, waitingEditCount);
  if (count === 0) return null;
  return (
    <div
      role="status"
      data-testid="unsaved-edits-notice"
      className="mx-auto mt-4 max-w-md rounded-md border border-destructive/40 bg-destructive/5 px-4 py-3 text-sm"
    >
      <p className="font-medium">
        {count === 1
          ? t("1 change has not reached the server.")
          : t("{count} changes have not reached the server.", { count })}
      </p>
      <p className="mt-1 text-muted-foreground">
        {t("Sign in again with the same account to save them. Closing the window discards them.")}
      </p>
    </div>
  );
}
