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
import type { CheckoutStateNotice, StateFile, StateTransfer } from "../types/api";

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
 * Shown once when a project opens whose folder holds state an earlier kapi
 * kept there, or whose location has state kept for an earlier checkout. Each
 * part says what the state is, how many files and how large, and takes exactly
 * those files when the person confirms. The terminal offers the same through
 * `kapi state import` and `kapi state relink`.
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
          <DialogDescription>{t("Use it only if it is yours.")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-5 text-sm">
          {notices.map((n) => (
            <NoticeSection key={n.name} notice={n} onApply={onApply} />
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

/** formatSize writes a byte count for a person: bytes, KB or MB. */
export function formatSize(bytes: number): string {
  const [value, unit] =
    bytes < 1024
      ? [bytes, "B"]
      : bytes < 1024 * 1024
        ? [bytes / 1024, "KB"]
        : [bytes / (1024 * 1024), "MB"];
  const n = new Intl.NumberFormat(undefined, { maximumFractionDigits: unit === "B" ? 0 : 1 });
  return `${n.format(value)} ${unit}`;
}

/**
 * What the files are, for a person: withheld text, the saved sign-in for the
 * server project the sync state is bound to, saved filters.
 */
function contentsOf(files: StateFile[], transfer: StateTransfer): string[] {
  const out: string[] = [];
  if (files.some((f) => f.target.startsWith("vault/")))
    out.push(t("The original text of withheld values"));
  if (files.some((f) => f.target.startsWith("sync/"))) {
    const servers = transfer.servers ?? [];
    out.push(
      transfer.sync_server && transfer.sync_project
        ? t("The saved sign-in for {server}, project {project}", {
            server: transfer.sync_server,
            project: transfer.sync_project,
          })
        : servers.length > 0
          ? t("The saved sign-in for {servers}", { servers: servers.join(", ") })
          : t("The saved sign-in for a server"),
    );
  }
  if (files.some((f) => f.target === "filters.json")) out.push(t("Your saved filters"));
  return out;
}

function sectionTitle(notice: CheckoutStateNotice): string {
  switch (notice.name) {
    case "earlier-at-path":
      return notice.other_project
        ? t("Kept for another project at this location")
        : t("Kept for an earlier checkout at this location");
    case "work-in-tree":
      return t(".kapi/work is no longer used");
    case "state-in-tree":
      return t("Kept inside this project's folder");
  }
}

function FileSummary({ files, slot }: { files: StateFile[]; slot: string }) {
  const total = files.reduce((sum, f) => sum + f.size, 0);
  return (
    <div data-slot={slot}>
      <p className="tabular-nums">
        <span data-slot="checkout-state-count">
          {t("{count, plural, one {# file} other {# files}}", { count: files.length })}
        </span>
        {" · "}
        <span data-slot="checkout-state-total">{formatSize(total)}</span>
      </p>
      <ul className="font-mono text-xs text-muted-foreground" translate="no">
        {files.map((f) => (
          <li key={f.source}>{f.target}</li>
        ))}
      </ul>
    </div>
  );
}

function NoticeSection({
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
      <h3 className="font-medium">{sectionTitle(notice)}</h3>
      {notice.name === "work-in-tree" && (
        <p className="text-muted-foreground">{t("You can delete it.")}</p>
      )}
      {notice.other_project && (
        <p data-slot="checkout-state-other-project" className="text-muted-foreground">
          {t("It belongs to {project}, so kapi keeps it where it is.", {
            project: notice.other_project,
          })}
        </p>
      )}
      {transfer && (
        <>
          <ul data-slot="checkout-state-contents" className="list-disc pl-5">
            {contentsOf([...transfer.files, ...(transfer.withheld ?? [])], transfer).map((line) => (
              <li key={line}>{line}</li>
            ))}
          </ul>
          <FileSummary files={transfer.files} slot="checkout-state-files" />
          {transfer.withheld && transfer.withheld.length > 0 && (
            <p data-slot="checkout-state-withheld" className="text-muted-foreground">
              {transfer.unverified
                ? t(
                    "kapi cannot tell whether this state belongs to this project, so withheld text and the saved sign-in stay where they are.",
                  )
                : t(
                    "The saved sign-in stays where it is, because this project does not connect to the server project it belongs to.",
                  )}
            </p>
          )}
          {done?.taken ? (
            <div data-slot="checkout-state-taken" className="space-y-1">
              <p>{t("This state is now in use here.")}</p>
              {done.remaining && done.remaining.length > 0 && (
                <div data-slot="checkout-state-remaining">
                  <p className="text-muted-foreground">
                    {t("These copies could not be removed from the folder:")}
                  </p>
                  <ul className="font-mono text-xs" translate="no">
                    {done.remaining.map((r) => (
                      <li key={r}>{r}</li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
          ) : (
            <div className="space-y-2">
              {transfer.action === "import" && (
                <label className="flex items-start gap-2">
                  <input
                    type="checkbox"
                    className="mt-1"
                    checked={removeSource}
                    onChange={(e) => setRemoveSource(e.target.checked)}
                  />
                  <span>
                    {t("Remove the copies from the folder")}
                    <span className="block text-muted-foreground">
                      {t("They are deleted from .kapi once kapi has its own copy.")}
                    </span>
                  </span>
                </label>
              )}
              {transfer.action === "relink" && transfer.removes_source && (
                <p className="text-muted-foreground">
                  {t("The earlier copy is deleted once its files are in place here.")}
                </p>
              )}
              <Button size="sm" disabled={busy} onClick={() => void apply()}>
                {transfer.action === "import"
                  ? t("Bring in the state from this folder")
                  : t("Use this state here")}
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
