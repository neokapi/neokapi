import { useCallback, useState } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { ConfirmDialog } from "@neokapi/ui";
import { discardOwnPendingSaves, hasPendingSaves } from "./pendingSaves";

/**
 * Asks before a sign-out that would discard edits the server has not taken.
 * Signing out drops every edit of the signing-out user on this server, in
 * every workspace, so with any waiting the person confirms first; with none
 * the sign-out goes ahead at once. Other users' edits stay. Render `dialog`
 * once, and call `request` where the sign-out was called.
 */
export function useSignOutGuard(signOut: () => Promise<void>) {
  const [open, setOpen] = useState(false);
  const request = useCallback(() => {
    if (hasPendingSaves()) {
      setOpen(true);
      return;
    }
    void signOut();
  }, [signOut]);
  const dialog = (
    <ConfirmDialog
      open={open}
      onOpenChange={setOpen}
      title={t("Sign out and discard unsaved changes?")}
      description={t("Some changes have not reached the server. Signing out discards them.")}
      confirmLabel={t("Sign out")}
      variant="destructive"
      onConfirm={() => {
        setOpen(false);
        discardOwnPendingSaves();
        void signOut();
      }}
    />
  );
  return { request, dialog };
}
