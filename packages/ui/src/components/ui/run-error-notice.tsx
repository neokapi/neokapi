import * as React from "react";
import { AlertTriangle, ChevronDown, Check, Copy, ExternalLink, Settings2 } from "lucide-react";

import { cn } from "../../lib/utils";
import { Badge } from "./badge";
import { Button } from "./button";
import CodeView from "../preview/CodeView";

// RunErrorNotice — a *classified* run failure, presented as structure.
//
// ErrorNotice is the general friendly-first surface: it takes an error of
// unknown shape and does its best. RunErrorNotice is for the case where the
// backend already did the classification and handed over a decomposition —
// headline, remediation, the actions that fix it, the affected scope — so the UI
// has no inference left to do. The raw chain stays available behind a
// disclosure, never as the primary message.
//
// The visual order is the order a person needs it in: what broke → what to do →
// where → (on request) the machine's own words. Actions are affordances, not
// prose: a command is copyable, a settings hint navigates, an install page
// opens. Nothing here knows *how* to do those things — the host wires `onAction`
// — so this component stays runtime-free and shared across desktop and platform.

/** What affordance to render for a remedy. */
export type RunErrorActionKind = "command" | "open-settings" | "open-url";

/** One thing the user can do about the failure. */
export interface RunErrorActionView {
  kind: RunErrorActionKind;
  label: string;
  /** Shell command, for kind "command". */
  command?: string;
  /** Destination, for kind "open-url". */
  url?: string;
  /** In-app view id, for kind "open-settings". */
  target?: string;
}

/** A run failure decomposed by the backend's classifier. */
export interface RunErrorView {
  kind: string;
  headline: string;
  remediation?: string;
  actions?: RunErrorActionView[];
  locale?: string;
  file?: string;
  provider?: string;
  model?: string;
  raw: string;
}

export interface RunErrorNoticeProps {
  error: RunErrorView;
  /**
   * Invoked when the user takes an action. "command" actions copy themselves to
   * the clipboard first — the host only needs to handle navigation and URLs, and
   * may ignore "command" entirely.
   */
  onAction?: (action: RunErrorActionView) => void;
  /**
   * The action kinds this host can actually service. An action the host cannot
   * carry out is a dead button, so it is not rendered rather than rendered inert
   * — a compact surface (a feed row) may be able to copy a command but have
   * nowhere to navigate to. Defaults to all kinds.
   */
  actionKinds?: readonly RunErrorActionKind[];
  /** Contextual label above the headline (e.g. the flow or run name). */
  context?: React.ReactNode;
  /** Compact inline row, or a bordered destructive panel (default). */
  variant?: "inline" | "panel";
  /** Translated label for the raw-chain disclosure. */
  detailsLabel?: React.ReactNode;
  /** Translated label shown briefly after a command is copied. */
  copiedLabel?: React.ReactNode;
  className?: string;
}

const ACTION_ICON: Record<RunErrorActionKind, React.ComponentType<{ size?: number }>> = {
  command: Copy,
  "open-settings": Settings2,
  "open-url": ExternalLink,
};

const ALL_ACTION_KINDS: readonly RunErrorActionKind[] = ["command", "open-settings", "open-url"];

export function RunErrorNotice({
  error,
  onAction,
  actionKinds = ALL_ACTION_KINDS,
  context,
  variant = "panel",
  detailsLabel = "Details",
  copiedLabel = "Copied",
  className,
}: RunErrorNoticeProps): React.ReactElement {
  const [expanded, setExpanded] = React.useState(false);
  const [copied, setCopied] = React.useState<string | null>(null);

  const act = React.useCallback(
    (action: RunErrorActionView) => {
      if (action.kind === "command" && action.command) {
        void navigator.clipboard?.writeText(action.command).then(
          () => {
            setCopied(action.command!);
            setTimeout(() => setCopied(null), 1600);
          },
          () => {
            /* clipboard may be unavailable — the command is still on screen */
          },
        );
      }
      onAction?.(action);
    },
    [onAction],
  );

  // Only the actions this host can carry out.
  const actions = (error.actions ?? []).filter((a) => actionKinds.includes(a.kind));

  // The scope chips: only the fields the classifier actually knew.
  const scope: Array<[string, string]> = [];
  if (error.locale) scope.push(["locale", error.locale]);
  if (error.file) scope.push(["file", error.file]);
  if (error.model) scope.push(["model", error.model]);
  else if (error.provider) scope.push(["provider", error.provider]);

  return (
    <div
      role="alert"
      data-slot="run-error-notice"
      data-kind={error.kind}
      data-variant={variant}
      className={cn(
        "text-sm",
        variant === "panel" && "rounded-md border border-destructive/40 bg-destructive/5 p-3",
        className,
      )}
    >
      <div className="flex items-start gap-2.5">
        <AlertTriangle
          aria-hidden
          className="mt-0.5 size-4 shrink-0 text-destructive"
          strokeWidth={2.25}
        />
        <div className="min-w-0 flex-1">
          {context !== undefined && (
            <p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
              {context}
            </p>
          )}
          <p
            data-slot="run-error-headline"
            className="font-medium text-destructive break-words [text-wrap:pretty]"
          >
            {error.headline}
          </p>

          {error.remediation !== undefined && error.remediation !== "" && (
            <p
              data-slot="run-error-remediation"
              className="mt-0.5 text-xs text-muted-foreground [text-wrap:pretty]"
            >
              {error.remediation}
            </p>
          )}

          {scope.length > 0 && (
            <div className="mt-2 flex flex-wrap items-center gap-1.5" data-slot="run-error-scope">
              {scope.map(([label, value]) => (
                <Badge
                  key={label}
                  variant="outline"
                  className="max-w-full gap-1 border-border/70 px-1.5 py-0 font-normal"
                >
                  <span className="text-[10px] uppercase tracking-wide text-muted-foreground/80">
                    {label}
                  </span>
                  {/* Paths and locale tags are identifiers: monospaced, LTR,
                      and bidi-isolated so an RTL locale cannot reorder the row. */}
                  <bdi dir="ltr" className="truncate font-mono text-[11px]">
                    {value}
                  </bdi>
                </Badge>
              ))}
            </div>
          )}

          {actions.length > 0 && (
            <div className="mt-2.5 flex flex-wrap items-center gap-2" data-slot="run-error-actions">
              {actions.map((action, i) => {
                const Icon = ACTION_ICON[action.kind];
                const isCopied = action.kind === "command" && copied === action.command;
                return (
                  <Button
                    key={i}
                    variant={i === 0 ? "secondary" : "ghost"}
                    size="xs"
                    onClick={() => act(action)}
                    data-slot="run-error-action"
                    data-action-kind={action.kind}
                  >
                    {isCopied ? <Check size={11} /> : <Icon size={11} />}
                    {isCopied ? copiedLabel : action.label}
                    {action.kind === "command" && action.command && (
                      <code className="ml-1 rounded bg-background/70 px-1 font-mono text-[10.5px] text-muted-foreground">
                        {action.command}
                      </code>
                    )}
                  </Button>
                );
              })}
            </div>
          )}

          {error.raw !== "" && (
            <div className="mt-2">
              <button
                type="button"
                data-slot="run-error-details-toggle"
                aria-expanded={expanded}
                onClick={() => setExpanded((v) => !v)}
                className="inline-flex items-center gap-1 text-[11px] font-medium text-muted-foreground transition-colors hover:text-foreground"
              >
                <ChevronDown
                  aria-hidden
                  className={cn("size-3 transition-transform", expanded && "rotate-180")}
                />
                {detailsLabel}
              </button>
              {expanded && (
                <div data-slot="run-error-raw" className="mt-2">
                  <CodeView
                    text={error.raw}
                    lang="text"
                    lineNumbers={false}
                    wrap
                    maxHeight="12rem"
                    className="text-left"
                  />
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

export default RunErrorNotice;
