import { useMemo, useState } from "react";
import { cn } from "@neokapi/ui-primitives";
import { AlertTriangle, ChevronDown, ChevronRight, Copy, Check } from "../components/icons";
import { parseAppError, hasStructuredRaw } from "./parseAppError";
import { JsonHighlight } from "./JsonHighlight";

/**
 * ReferenceChip renders the per-request correlation ID with a copy button.
 * This is the value a user quotes to support so a single ID resolves to the
 * exact server logs and Sentry issue.
 */
function ReferenceChip({ reference, testId }: { reference: string; testId: string }) {
  const [copied, setCopied] = useState(false);
  const copy = () => {
    void navigator.clipboard?.writeText(reference).then(
      () => {
        setCopied(true);
        setTimeout(() => setCopied(false), 1500);
      },
      () => {},
    );
  };
  return (
    <button
      type="button"
      onClick={copy}
      data-testid={`${testId}-reference`}
      title="Copy reference: quote this to support"
      className={cn(
        "inline-flex items-center gap-1 rounded border border-border/60 bg-muted/40 px-1.5 py-0.5",
        "font-mono text-[11px] text-muted-foreground hover:text-foreground transition-colors cursor-pointer",
      )}
    >
      <span className="opacity-70">ref</span>
      <span className="break-all">{reference}</span>
      {copied ? (
        <Check className="w-3 h-3 shrink-0 text-emerald-500" />
      ) : (
        <Copy className="w-3 h-3 shrink-0" />
      )}
    </button>
  );
}

export interface ErrorNoticeProps {
  /** The raw error value (anything thrown/rejected — Error, envelope, string). */
  error: unknown;
  /**
   * Contextual headline, e.g. "Couldn't create the API token". When given it
   * becomes the headline and the parsed message moves to the detail line.
   */
  title?: string;
  /** Recovery hint; overrides any hint derived from the error. */
  hint?: string;
  /**
   * - `inline`: compact single-block notice for dialogs/forms.
   * - `panel`: bordered panel for page/section level errors.
   */
  variant?: "inline" | "panel";
  className?: string;
  /** Optional retry callback; renders a "Try again" action. */
  onRetry?: () => void;
  "data-testid"?: string;
}

/**
 * ErrorNotice — the one way to show an error to a user. Renders a friendly
 * one-liner (via parseAppError), an optional recovery hint, and — when the
 * error carried a structured payload — a "Details" disclosure with the raw
 * error as syntax-highlighted JSON.
 */
export function ErrorNotice({
  error,
  title,
  hint,
  variant = "panel",
  className,
  onRetry,
  "data-testid": testId = "error-notice",
}: ErrorNoticeProps) {
  const [open, setOpen] = useState(false);
  const parsed = useMemo(() => parseAppError(error), [error]);

  const headline = title ?? parsed.title;
  // When a contextual title is given, demote the parsed message to the detail
  // line — unless there was no underlying error value to parse.
  const secondary = title && error != null && parsed.title !== title ? parsed.title : undefined;
  const detail = [secondary, parsed.detail].filter((t): t is string => Boolean(t)).join(" · ");
  const recovery = hint ?? parsed.hint;
  const showDetails = hasStructuredRaw(parsed);
  const referenceChip = parsed.reference && (
    <ReferenceChip reference={parsed.reference} testId={testId} />
  );

  const disclosure = showDetails && (
    <button
      type="button"
      onClick={() => setOpen((o) => !o)}
      aria-expanded={open}
      data-testid={`${testId}-details-toggle`}
      className={cn(
        "inline-flex items-center gap-0.5 text-xs font-medium",
        "text-destructive/80 hover:text-destructive transition-colors cursor-pointer",
        "bg-transparent border-none p-0",
      )}
    >
      {open ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />}
      Details
    </button>
  );

  const rawJson = showDetails && open && (
    <pre
      data-testid={`${testId}-details`}
      className={cn(
        "mt-1.5 max-h-56 overflow-auto rounded-md border border-border",
        "bg-muted/40 p-2 text-xs leading-relaxed whitespace-pre",
      )}
    >
      <JsonHighlight value={parsed.raw} />
    </pre>
  );

  const retry = onRetry && (
    <button
      type="button"
      onClick={onRetry}
      data-testid={`${testId}-retry`}
      className={cn(
        "text-xs font-medium underline underline-offset-2",
        "text-destructive hover:text-destructive/80 transition-colors",
        "bg-transparent border-none p-0 cursor-pointer",
      )}
    >
      Try again
    </button>
  );

  if (variant === "inline") {
    return (
      <div role="alert" data-testid={testId} className={cn("text-sm", className)}>
        <div className="flex items-start gap-1.5 text-destructive">
          <AlertTriangle className="w-4 h-4 shrink-0 mt-0.5" aria-hidden />
          <span className="min-w-0 break-words">
            <span className="font-medium">{headline}</span>
            {detail && <span className="text-destructive/80"> · {detail}</span>}
            {recovery && <span className="text-muted-foreground"> {recovery}</span>}
          </span>
          <span className="ml-auto flex shrink-0 items-center gap-2 pl-2">
            {retry}
            {disclosure}
          </span>
        </div>
        {referenceChip && <div className="mt-1.5">{referenceChip}</div>}
        {rawJson}
      </div>
    );
  }

  return (
    <div
      role="alert"
      data-testid={testId}
      className={cn(
        "rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm",
        className,
      )}
    >
      <div className="flex items-start gap-2">
        <AlertTriangle className="w-4 h-4 shrink-0 mt-0.5 text-destructive" aria-hidden />
        <div className="min-w-0 flex-1">
          <p className="font-medium text-destructive break-words">{headline}</p>
          {detail && <p className="mt-0.5 text-destructive/80 break-words">{detail}</p>}
          {recovery && <p className="mt-0.5 text-xs text-muted-foreground">{recovery}</p>}
          {(retry || disclosure || referenceChip) && (
            <div className="mt-1.5 flex flex-wrap items-center gap-3">
              {retry}
              {disclosure}
              {referenceChip}
            </div>
          )}
          {rawJson}
        </div>
      </div>
    </div>
  );
}
