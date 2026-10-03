import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@neokapi/ui-primitives";
import { ABSENT_REVISION } from "../../api/contentChanges";
import type { StaleAction, StaleDialogState } from "../../hooks/useContentChanges";
import { useLocales } from "../../hooks/useLocales";
import { PlaceholderText } from "./PlaceholderText";

export interface StaleChangeDialogProps {
  /** The open prompt (`useContentChanges().staleDialog`), or null when none is open. */
  state: StaleDialogState | null;
}

const REAPPLY_LABEL: Record<StaleAction, string> = {
  save: "Save my version",
  establish: "Approve this version",
  reject: "Reject this version",
  withdraw: "Withdraw this version",
};

const INTRO: Record<StaleAction, string> = {
  save: "Someone saved a different translation after you opened this block. Your edit has not been saved.",
  establish:
    "Someone changed this translation after you opened it, so your approval was not recorded. Read the translation as it stands before you approve it.",
  reject:
    "Someone changed this translation after you opened it, so your rejection was not recorded. Read the translation as it stands before you reject it.",
  withdraw:
    "Someone changed this translation after you opened it, so your decision was not recorded. Read the translation as it stands before you decide.",
};

const REMOVED_INTRO =
  "Someone removed this translation after you opened it, so your decision was not recorded. There is no translation left to decide on.";

/**
 * Asks what to do when a change a person sent meets a translation that moved
 * since they opened it: the server refused it, and the dialog shows the
 * translation as it stands now (and, for a save, the person's version) so they
 * apply their change to it or keep it. A decision on a translation someone
 * removed has nothing to apply to, so the dialog offers only to keep things as
 * they stand.
 */
export function StaleChangeDialog({ state }: StaleChangeDialogProps) {
  const { getDisplayName } = useLocales();
  const action = state?.action ?? "save";
  const language = state ? getDisplayName(state.locale) || state.locale : "";
  const removed = action !== "save" && state?.current.rev === ABSENT_REVISION;

  return (
    <Dialog open={state !== null} onOpenChange={(open) => !open && state?.onKeep()}>
      <DialogContent className="sm:max-w-[560px]" data-testid="stale-change-dialog">
        <DialogHeader>
          <DialogTitle>This translation changed since you opened it</DialogTitle>
          <DialogDescription>{removed ? REMOVED_INTRO : INTRO[action]}</DialogDescription>
        </DialogHeader>

        <section className="space-y-1">
          <h3 className="text-xs font-medium text-muted-foreground">{language} now</h3>
          <p
            className="rounded-md border border-border bg-muted/20 p-2 text-sm"
            data-testid="stale-current"
          >
            <PlaceholderText text={state?.current.text ?? ""} />
          </p>
        </section>
        {action === "save" && state?.mine !== undefined && (
          <section className="space-y-1">
            <h3 className="text-xs font-medium text-muted-foreground">Your version</h3>
            <p
              className="rounded-md border border-primary/40 bg-primary/5 p-2 text-sm"
              data-testid="stale-mine"
            >
              <PlaceholderText text={state.mine} />
            </p>
          </section>
        )}

        <DialogFooter>
          <Button
            variant={removed ? "default" : "outline"}
            onClick={() => state?.onKeep()}
            data-testid="stale-keep"
          >
            {removed ? "Close" : "Keep the current translation"}
          </Button>
          {!removed && (
            <Button onClick={() => state?.onReapply()} data-testid="stale-reapply">
              {REAPPLY_LABEL[action]}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
