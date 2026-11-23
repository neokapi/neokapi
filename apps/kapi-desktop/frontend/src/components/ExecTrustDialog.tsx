import { useCallback, useRef, useState } from "react";
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
import type { ExecTrustRequest } from "../types/api";

export interface ExecTrustDialogProps {
  /** The question; the dialog is open while it is set. */
  request: ExecTrustRequest | null;
  onAllow: () => void;
  onCancel: () => void;
}

/**
 * Asked before a run when the project's commands have no answer as they stand
 * now: on first use, after a command was added, and after a checked file
 * changed, mid-session included. It shows what runs, the files kapi checks,
 * what it names and does not check, and the limit of the check.
 */
export function ExecTrustDialog({ request, onAllow, onCancel }: ExecTrustDialogProps) {
  return (
    <Dialog
      open={request !== null}
      onOpenChange={(o) => {
        if (!o) onCancel();
      }}
    >
      <DialogContent data-testid="exec-trust">
        <DialogHeader>
          <DialogTitle>{t("Run this project's commands?")}</DialogTitle>
          <DialogDescription>
            {t(
              "They run with your privileges and your environment. Allow them only if you trust the source of this project.",
            )}
          </DialogDescription>
        </DialogHeader>
        {request && (
          <div className="space-y-2 text-sm">
            <ul data-slot="exec-trust-sites" className="font-mono text-xs" translate="no">
              {request.sites.map((s) => (
                <li key={s.where + s.name}>
                  {s.where} {s.name} {s.detail}
                </li>
              ))}
            </ul>
            <Group
              slot="exec-trust-checked"
              label={t("Checked, by content")}
              items={request.checked}
            />
            <Group
              slot="exec-trust-not-checked"
              label={t("Named and not checked (directories)")}
              items={request.not_checked}
            />
            <Group
              slot="exec-trust-inline"
              label={t("Runs code given inline, so kapi cannot see which files it uses")}
              items={request.inline}
            />
            <Group
              slot="exec-trust-added"
              label={t("Added since it was answered")}
              items={request.added}
            />
            <Group
              slot="exec-trust-changed"
              label={t("Changed since it was answered")}
              items={request.changed}
            />
            <p data-slot="exec-trust-limit" className="text-muted-foreground">
              {t(
                "kapi checks the content of the files listed, not what those files or the commands run or read.",
              )}
            </p>
          </div>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={onCancel}>
            {t("Cancel")}
          </Button>
          <Button onClick={onAllow} className="sm:ml-4">
            {t("Allow and run")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Group({ slot, label, items }: { slot: string; label: string; items?: string[] }) {
  if (!items || items.length === 0) return null;
  return (
    <p data-slot={slot}>
      <span className="font-medium">{label}:</span>{" "}
      <span className="font-mono text-xs" translate="no">
        {items.join(", ")}
      </span>
    </p>
  );
}

/**
 * useExecTrustGate asks, before a run, whatever the tab's project's commands
 * need answered. gate() resolves true when the run may go ahead: nothing to
 * answer, or the person allowed it. The dialog it returns is rendered once.
 */
export function useExecTrustGate(tabID: string) {
  const [request, setRequest] = useState<ExecTrustRequest | null>(null);
  const resolver = useRef<((ok: boolean) => void) | null>(null);

  const gate = useCallback(async (): Promise<boolean> => {
    const pending = await api.pendingExecTrust(tabID);
    if (!pending) return true;
    setRequest(pending);
    return new Promise<boolean>((resolve) => {
      resolver.current = resolve;
    });
  }, [tabID]);

  const settle = (ok: boolean) => {
    setRequest(null);
    resolver.current?.(ok);
    resolver.current = null;
  };

  const dialog = (
    <ExecTrustDialog
      request={request}
      onCancel={() => settle(false)}
      onAllow={() => {
        const digest = request?.digest ?? "";
        // A project that changed since the dialog opened is not recorded:
        // the dialog shows it again as it stands now.
        void api
          .approveExecTrust(tabID, digest)
          .then(() => settle(true))
          .catch(async () => {
            const again = await api.pendingExecTrust(tabID).catch(() => null);
            if (again) setRequest(again);
            else settle(false);
          });
      }}
    />
  );
  return { gate, dialog };
}
