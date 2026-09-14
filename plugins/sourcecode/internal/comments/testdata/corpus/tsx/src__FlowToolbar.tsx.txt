import { t } from "@neokapi/i18n-react/runtime";
import { Play, Loader2, Lock } from "lucide-react";
import { Button, PanelHeader, SimpleTooltip } from "@neokapi/ui-primitives";
import type { FlowSpec } from "./types";

export interface FlowToolbarProps {
  stepCount: number;
  onRun?: (flow: FlowSpec) => void;
  runDisabled?: boolean;
  /** A run is actually in flight (drives the spinner/label; disabled alone
   *  can also mean "not ready yet" or "read-only replay"). */
  running?: boolean;
  flow: FlowSpec;
  /** Whether the flow currently has the redaction wrap (redact … unredact). */
  redacted?: boolean;
  /** Remove the redaction wrap; absent in read-only flows. The wrap is ADDED
   *  from the Add-tool dialog — the toolbar only shows the active state. */
  onToggleRedaction?: () => void;
}

export function FlowToolbar({
  stepCount,
  onRun,
  runDisabled,
  running,
  flow,
  redacted,
  onToggleRedaction,
}: FlowToolbarProps) {
  return (
    <PanelHeader
      title={`${stepCount} step${stepCount !== 1 ? "s" : ""}`}
      className="py-1.5"
      actions={
        <>
          {/* Protection is a STATE chip, not a primary action: it appears only
              while the redact … unredact wrap is on. Wrapping the flow lives
              with the other composition actions in the Add-tool dialog. */}
          {redacted && (
            <SimpleTooltip
              content={
                onToggleRedaction
                  ? "Protected: sensitive content is redacted before the tools run and restored after. Click to remove the wrap."
                  : "Protected: sensitive content is redacted before the tools run and restored after."
              }
            >
              <span className="inline-flex">
                <Button
                  variant="ghost"
                  size="xs"
                  onClick={onToggleRedaction}
                  disabled={!onToggleRedaction}
                  className="border border-[oklch(0.6_0.2_15/0.4)] text-[oklch(0.6_0.2_15)]"
                >
                  <Lock size={12} />
                  {t("Protected")}
                </Button>
              </span>
            </SimpleTooltip>
          )}

          {onRun && (
            <Button
              size="xs"
              onClick={() => onRun(flow)}
              disabled={runDisabled}
              aria-label="Run flow"
            >
              {running ? <Loader2 size={12} className="animate-spin" /> : <Play size={12} />}
              {running ? t("Running...") : t("Run")}
            </Button>
          )}
        </>
      }
    />
  );
}
