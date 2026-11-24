import { useEffect, useRef, useState } from "react";
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
import { api } from "../hooks/useApi";
import type { CheckoutStateNotice, StateTransfer } from "../types/api";

export interface CheckoutStateDialogProps {
  /** The project the lines are about, named because its tab need not be the one in front. */
  projectName: string;
  /** The lines to show; the dialog is open while there is one. */
  notices: CheckoutStateNotice[];
  /** Takes the transfer a line offers, as shown. */
  onApply: (notice: CheckoutStateNotice, removeSource: boolean) => Promise<StateTransfer | null>;
  onClose: () => void;
}

/**
 * Shown once when a project opens whose checkout holds state an earlier kapi
 * left inside it, or whose location has state recorded for another folder.
 * It lists what an import or a relink would take, with the sizes and the
 * servers its sync state was issued by, and takes exactly that when the
 * person confirms. The terminal says the same through `kapi state import`
 * and `kapi state relink`.
 */
export function CheckoutStateDialog({
  projectName,
  notices,
  onApply,
  onClose,
}: CheckoutStateDialogProps) {
  return (
    <Dialog
      open={notices.length > 0}
      onOpenChange={(o) => {
        if (!o) onClose();
      }}
    >
      <DialogContent data-testid="checkout-state" className="sm:max-w-[560px]">
        <DialogHeader>
          <DialogTitle>{t("State from an earlier kapi")}</DialogTitle>
          <p data-slot="checkout-state-project" className="font-medium" translate="no">
            {projectName}
          </p>
          <DialogDescription>
            {t(
              "kapi keeps this checkout's own state outside it. Take state into this checkout only if it is yours: a vault holds withheld originals, and sync state can hold a token for a server project.",
            )}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 text-sm">
          {notices.map((n) => (
            <NoticeRow key={n.name} notice={n} onApply={onApply} />
          ))}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("Close")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function noticeTitle(name: CheckoutStateNotice["name"]): string {
  switch (name) {
    case "earlier-at-path":
      return t("State recorded for an earlier checkout at this location");
    case "work-in-tree":
      return t(".kapi/work is no longer used, and you can delete it");
    case "state-in-tree":
      return t("State left inside this checkout");
  }
}

function NoticeRow({
  notice,
  onApply,
}: {
  notice: CheckoutStateNotice;
  onApply: CheckoutStateDialogProps["onApply"];
}) {
  const [removeSource, setRemoveSource] = useState(false);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState<StateTransfer | null>(null);
  const [error, setError] = useState<string | null>(null);
  const transfer = notice.transfer;

  const apply = async () => {
    setBusy(true);
    setError(null);
    try {
      setDone(await onApply(notice, removeSource));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <section data-slot={`checkout-state-${notice.name}`} className="space-y-2">
      <h3 className="font-medium">{noticeTitle(notice.name)}</h3>
      {transfer && (
        <>
          <ul className="font-mono text-xs" translate="no">
            {transfer.files.map((f) => (
              <li key={f.source}>
                {f.source} <span className="text-muted-foreground">({f.size} B)</span>
              </li>
            ))}
          </ul>
          {transfer.servers && transfer.servers.length > 0 && (
            <p data-slot="checkout-state-servers">
              <span className="font-medium">{t("Sync state issued by")}:</span>{" "}
              <span className="font-mono text-xs" translate="no">
                {transfer.servers.join(", ")}
              </span>
            </p>
          )}
          {done?.taken ? (
            <p data-slot="checkout-state-taken">
              {transfer.action === "import" ? t("Imported.") : t("Relinked.")}
            </p>
          ) : (
            <div className="flex flex-wrap items-center gap-4">
              {transfer.action === "import" && (
                <label className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    checked={removeSource}
                    onChange={(e) => setRemoveSource(e.target.checked)}
                  />
                  {t("Remove the copies from the checkout")}
                </label>
              )}
              <Button size="sm" disabled={busy} onClick={() => void apply()}>
                {transfer.action === "import" ? t("Import") : t("Relink")}
              </Button>
            </div>
          )}
          {error && (
            <p data-slot="checkout-state-error" className="text-destructive">
              {error}
            </p>
          )}
        </>
      )}
    </section>
  );
}

/**
 * useCheckoutStateNotices asks, once per opened project tab, which one-time
 * lines about its checkout state nobody has been shown, and opens the dialog
 * with them. Closing the dialog records them as shown, so neither the app nor
 * the terminal repeats them. A transfer that changed since it was shown is
 * refused by the backend, and the dialog shows the plan again as it stands.
 */
export function useCheckoutStateNotices(
  tabID: string | null,
  projectName: string,
  enabled: boolean,
) {
  const [open, setOpen] = useState<{
    tabID: string;
    projectName: string;
    notices: CheckoutStateNotice[];
  } | null>(null);
  const asked = useRef<Set<string>>(new Set());

  useEffect(() => {
    if (!tabID || !enabled || asked.current.has(tabID)) return;
    asked.current.add(tabID);
    void api
      .pendingCheckoutState(tabID)
      .then((pending) => {
        if (pending && pending.length > 0) setOpen({ tabID, projectName, notices: pending });
      })
      .catch(() => {});
  }, [tabID, projectName, enabled]);

  const onApply = async (notice: CheckoutStateNotice, removeSource: boolean) => {
    if (!open) return null;
    try {
      return await api.applyCheckoutStateTransfer(
        open.tabID,
        notice.name,
        notice.digest ?? "",
        removeSource,
      );
    } catch (e) {
      // Shown again as it stands now, so the next confirm covers what is there.
      const pending = await api.pendingCheckoutState(open.tabID).catch(() => null);
      const again = pending?.find((n) => n.name === notice.name);
      if (again) {
        setOpen(
          (prev) =>
            prev && {
              ...prev,
              notices: prev.notices.map((n) => (n.name === notice.name ? again : n)),
            },
        );
      }
      throw e;
    }
  };

  const close = () => {
    if (open) {
      void api
        .markCheckoutStateShown(
          open.tabID,
          open.notices.map((n) => n.name),
        )
        .catch(() => {});
    }
    setOpen(null);
  };

  const dialog = (
    <CheckoutStateDialog
      projectName={open?.projectName ?? ""}
      notices={open?.notices ?? []}
      onApply={onApply}
      onClose={close}
    />
  );
  return { dialog };
}
