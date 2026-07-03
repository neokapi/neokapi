import * as React from "react";
import { AlertCircle, ChevronDown } from "lucide-react";

import { cn } from "../../lib/utils";
import { parseAppError } from "../../lib/parse-app-error";
import CodeView from "../preview/CodeView";

// ErrorNotice — the friendly-first error surface. Renders the parsed friendly
// line (via parseAppError), an optional hint, and a "Details" disclosure that
// shows the raw structured payload as theme-aware, syntax-highlighted JSON.
//
// Two variants:
// - "inline": compact row for embedding inside cards, dialogs, and forms.
// - "panel":  bordered destructive box for page/section-level errors.
//
// The same contract exists in bowrain's kit; keep names, states, and behavior
// in sync (no cross-kit imports).

export interface ErrorNoticeProps {
  /** The error in any shape — string, Error, Wails envelope, JSON string. */
  error: unknown;
  /**
   * Optional contextual first line (e.g. "Failed to run checks"). When given,
   * the parsed friendly message becomes the secondary line.
   */
  title?: React.ReactNode;
  /** Optional hint rendered under the message (e.g. how to recover). */
  hint?: React.ReactNode;
  /** Compact inline row (default) or bordered panel. */
  variant?: "inline" | "panel";
  /** Label for the details disclosure (pass a translated string). */
  detailsLabel?: React.ReactNode;
  /** Extra actions rendered next to the details toggle. */
  actions?: React.ReactNode;
  className?: string;
}

function rawLang(raw: string): "json" | "text" {
  const t = raw.trimStart();
  return t.startsWith("{") || t.startsWith("[") ? "json" : "text";
}

export function ErrorNotice({
  error,
  title,
  hint,
  variant = "inline",
  detailsLabel = "Details",
  actions,
  className,
}: ErrorNoticeProps): React.ReactElement {
  const parsed = React.useMemo(() => parseAppError(error), [error]);
  const [expanded, setExpanded] = React.useState(false);

  const primary = title ?? parsed.title;
  // When a contextual title is supplied, the parsed friendly line becomes the
  // secondary line so the underlying reason stays visible without disclosure.
  const secondary = title ? parsed.title : parsed.detail;
  const hasDetails = parsed.raw !== undefined;

  return (
    <div
      role="alert"
      data-slot="error-notice"
      data-variant={variant}
      className={cn(
        "text-sm",
        variant === "panel" && "rounded-md border border-destructive/50 bg-destructive/10 p-3",
        className,
      )}
    >
      <div className="flex items-start gap-2">
        <AlertCircle
          aria-hidden
          className={cn("mt-0.5 shrink-0 text-destructive", variant === "panel" ? "size-4" : "size-3.5")}
        />
        <div className="min-w-0 flex-1">
          <p
            data-slot="error-notice-title"
            className={cn("font-medium text-destructive break-words", variant === "inline" && "text-sm")}
          >
            {primary}
          </p>
          {secondary !== undefined && secondary !== primary && (
            <p
              data-slot="error-notice-secondary"
              className="mt-0.5 text-xs text-destructive/90 break-words"
            >
              {secondary}
            </p>
          )}
          {hint !== undefined && (
            <p data-slot="error-notice-hint" className="mt-0.5 text-xs text-muted-foreground">
              {hint}
            </p>
          )}
          {(hasDetails || actions) && (
            <div className="mt-1 flex items-center gap-3">
              {hasDetails && (
                <button
                  type="button"
                  data-slot="error-notice-toggle"
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
              )}
              {actions}
            </div>
          )}
          {expanded && parsed.raw !== undefined && (
            <div data-slot="error-notice-raw" className="mt-2">
              <CodeView
                text={parsed.raw}
                lang={rawLang(parsed.raw)}
                lineNumbers={false}
                wrap
                maxHeight="16rem"
                className="text-left"
              />
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
