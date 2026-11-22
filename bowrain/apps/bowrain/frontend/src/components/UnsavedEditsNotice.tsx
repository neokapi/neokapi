import { useSyncExternalStore } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { pendingEdits, subscribePendingSaves } from "@neokapi/bowrain-app";

/**
 * Lists the edits that have not reached the server, on the screens shown
 * while the app is signed out or cannot reach the server. The edits stay
 * recorded: signing in again and opening the flow saves them, and closing the
 * window asks first.
 */
export function UnsavedEditsNotice() {
  const edits = useSyncExternalStore(subscribePendingSaves, pendingEdits);
  if (edits.length === 0) return null;
  return (
    <div
      role="status"
      data-testid="unsaved-edits-notice"
      className="mx-auto mt-4 max-w-md rounded-md border border-destructive/40 bg-destructive/5 px-4 py-3 text-sm"
    >
      <p className="font-medium">{t("Not saved yet")}</p>
      <ul className="mt-1 list-disc pl-5">
        {edits.map((e) => (
          <li key={e.key}>{e.label}</li>
        ))}
      </ul>
      <p className="mt-1 text-muted-foreground">
        {t("Sign in again and open each one to save it.")}
      </p>
    </div>
  );
}
