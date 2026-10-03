import {
  Button,
  DirectionalText,
  LocaleLabel,
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type { FailedChange, FailedEdit } from "../types/api";
import { Loader2, RefreshCw, WifiOff, X } from "./icons";

/** The four states the desktop backend reports for its server connection. */
export type ConnectionState = "disconnected" | "connecting" | "connected" | "offline";

export interface ConnectionIndicatorProps {
  /** Undefined on the web, where there is no working copy to be offline from. */
  connectionState?: ConnectionState;
  /** Edits waiting in the offline queue. */
  pendingChanges?: number;
  /**
   * Offline changes that did not reach the server: those it refused on replay,
   * and those an earlier version queued in a form this one no longer sends.
   */
  failedChanges?: FailedChange[];
  /** Remove one failed change from the list, once a person has dealt with it. */
  onDismissFailedChange?: (id: number) => void;
  /** Remove every failed change from the list. */
  onDismissFailedChanges?: () => void;
  /** Ask the backend to attempt a reconnection now. */
  onRetryConnection?: () => void;
}

/**
 * The chrome's account of the server connection: whether the working copy is
 * offline, how much work is waiting behind that, and whether an attempt to get
 * back is in flight.
 *
 * A connected app shows nothing here. Only the changes that did not reach the
 * server outlive the outage that produced them, because they stay unapplied
 * until someone makes them again: the count opens the list of them, each with
 * what it was and why it was not sent.
 */
export function ConnectionIndicator({
  connectionState,
  pendingChanges,
  failedChanges,
  onDismissFailedChange,
  onDismissFailedChanges,
  onRetryConnection,
}: ConnectionIndicatorProps) {
  const isOffline = connectionState === "offline";
  const isConnecting = connectionState === "connecting";
  const hasPending = pendingChanges != null && pendingChanges > 0;
  const failed = failedChanges ?? [];

  return (
    <>
      {isConnecting && (
        <span
          className="flex items-center gap-1 text-xs text-muted-foreground"
          data-testid="connection-reconnecting"
        >
          <Loader2 className="size-3 animate-spin" />
          <span>Reconnecting</span>
        </span>
      )}

      {isOffline && (
        <span
          className="flex items-center gap-1 text-xs text-warning"
          data-testid="connection-offline"
        >
          <WifiOff className="size-3" />
          {hasPending ? (
            <span data-testid="offline-pending">{pendingChanges} pending</span>
          ) : (
            <span>Offline</span>
          )}
          {onRetryConnection && (
            <Button
              variant="ghost"
              size="sm"
              className="h-5 gap-1 px-1.5 text-xs"
              onClick={onRetryConnection}
              data-testid="connection-retry"
            >
              <RefreshCw className="size-3" />
              Retry
            </Button>
          )}
        </span>
      )}

      {failed.length > 0 && (
        <Popover>
          <PopoverTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              className="h-6 gap-1 px-1.5 text-xs text-destructive"
              data-testid="connection-failed"
            >
              <WifiOff className="size-3" />
              <span>{failed.length} not sent</span>
            </Button>
          </PopoverTrigger>
          <PopoverContent align="end" className="w-96 p-0" data-testid="failed-changes">
            <div className="flex items-center justify-between border-b px-3 py-2">
              <span className="text-sm font-medium">Changes that were not sent</span>
              {onDismissFailedChanges && (
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-6 px-2 text-xs"
                  onClick={onDismissFailedChanges}
                  data-testid="failed-changes-dismiss-all"
                >
                  Dismiss all
                </Button>
              )}
            </div>
            <ul className="max-h-80 overflow-y-auto">
              {failed.map((c) => (
                <FailedChangeRow
                  key={c.id}
                  change={c}
                  onDismiss={onDismissFailedChange ? () => onDismissFailedChange(c.id) : undefined}
                />
              ))}
            </ul>
          </PopoverContent>
        </Popover>
      )}
    </>
  );
}

/** One change that was not sent: what it did, where, the wording it carried, and why it stayed. */
function FailedChangeRow({ change, onDismiss }: { change: FailedChange; onDismiss?: () => void }) {
  const edits = change.edits ?? [];
  return (
    <li
      className="space-y-1 border-b px-3 py-2 last:border-b-0"
      data-testid={`failed-change-${change.id}`}
      data-status={change.status}
    >
      <div className="flex items-start justify-between gap-2">
        <span className="text-xs font-medium">
          {edits.length > 0 ? editLabel(edits[0]) : operationLabel(change.operation)}
          {edits.length > 1 && (
            <span className="ml-1 text-muted-foreground">and {edits.length - 1} more</span>
          )}
        </span>
        {onDismiss && (
          <Button
            variant="ghost"
            size="sm"
            className="h-5 w-5 shrink-0 p-0"
            onClick={onDismiss}
            aria-label={t("Dismiss")}
            data-testid={`failed-change-dismiss-${change.id}`}
          >
            <X className="size-3" />
          </Button>
        )}
      </div>
      {edits.map((e, i) => (
        <div key={i} className="space-y-0.5 text-xs text-muted-foreground">
          {(e.item || e.locale) && (
            <div className="flex flex-wrap items-center gap-1">
              {e.item && <span className="truncate">{e.item}</span>}
              {e.locale && <LocaleLabel locale={e.locale} compact />}
            </div>
          )}
          {e.text && (
            <DirectionalText
              locale={e.locale}
              className="block rounded bg-muted px-1.5 py-1 text-foreground"
              data-testid="failed-change-text"
            >
              {e.text}
            </DirectionalText>
          )}
        </div>
      ))}
      <p className="text-xs text-muted-foreground" data-testid="failed-change-reason">
        {change.status === "dropped"
          ? t(
              "Queued by an earlier version of Bowrain, which this version does not send. Make the change again.",
            )
          : change.reason}
      </p>
    </li>
  );
}

/** What one operation of a change set did. */
function editLabel(e: FailedEdit): string {
  switch (e.op) {
    case "set_content":
    case "replace_text":
      return e.locale ? t("Translation") : t("Source edit");
    case "remove_edition":
      return t("Translation removed");
    case "decide":
      switch (e.outcome) {
        case "establish":
          return t("Approval");
        case "reject":
          return t("Rejection");
        default:
          return t("Review decision");
      }
    case "annotate":
      return e.type === "entity" ? t("Entity mark") : t("Note");
    case "unannotate":
      return e.type === "entity" ? t("Entity mark removed") : t("Note removed");
    default:
      return t("Content change");
  }
}

/** What a queued change of a kind other than a change set did. */
function operationLabel(operation: string): string {
  switch (operation) {
    case "add_tm_entry":
      return t("Content memory entry added");
    case "update_tm_entry":
      return t("Content memory entry changed");
    case "delete_tm_entry":
      return t("Content memory entry removed");
    case "add_concept":
      return t("Term added");
    case "update_concept":
      return t("Term changed");
    case "delete_concept":
      return t("Term removed");
    case "add_items":
      return t("Files uploaded");
    case "remove_item":
      return t("File removed");
    case "pseudo_translate_item":
      return t("Pseudo-translation");
    case "tm_translate_item":
      return t("Translation from content memory");
    default:
      return t("Change");
  }
}
