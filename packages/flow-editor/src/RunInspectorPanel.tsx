// RunInspectorPanel — the per-step run inspector shown when a node is
// selected while a trace is loaded. It teaches the content model: for every
// block that passed through the step it shows the text before/after and the
// stand-off delta — which overlays (segmentation, term, entity, qa) and block
// annotations this tool ADDED or consumed — with the spans expandable down to
// covered text + payload note (AD-002).

import { useState } from "react";
import { ChevronDown, ChevronRight, Pencil } from "lucide-react";
import { Button, ScrollArea, PanelHeader, cn } from "@neokapi/ui-primitives";
import type { FlowTrace, OverlaySnapshot, PartSnapshot } from "./traceTypes";
import { partsThroughStep, snapshotDelta, type PartTransition } from "./traceSelectors";
import { getPortType as portStyle } from "./portTypes";

export interface RunInspectorPanelProps {
  trace: FlowTrace;
  stepToolCounts: number[];
  stepIndex: number;
  stepLabel: string;
  /** Switch the panel to the tool's configuration. */
  onConfigure?: () => void;
  onClose: () => void;
}

/** Chip for one overlay delta entry, colored by port type. */
function OverlayChip({ type, spans, removed }: { type: string; spans: number; removed?: boolean }) {
  const style = portStyle(type);
  return (
    <span
      className={cn(
        "inline-flex items-center gap-0.5 rounded px-1 py-px font-mono text-[9px] font-semibold",
        removed && "line-through opacity-60",
      )}
      style={{ background: `${style.color}1f`, color: style.color }}
      title={
        removed ? `${spans} ${type} span(s) consumed/dropped` : `${spans} ${type} span(s) added`
      }
    >
      {removed ? "−" : "+"}
      {spans} {type}
    </span>
  );
}

/** Expandable list of an overlay's spans (covered text + payload note). */
function OverlaySpans({ overlay }: { overlay: OverlaySnapshot }) {
  const style = portStyle(overlay.type);
  return (
    <div className="mt-1 flex flex-col gap-0.5">
      {(overlay.spans ?? []).map((s, i) => (
        <div key={i} className="flex items-baseline gap-1 text-[10px] leading-snug">
          <span className="font-mono text-[9px] text-muted-foreground">
            {s.start}–{s.end}
          </span>
          <span
            className="rounded-sm px-0.5"
            style={{ background: `${style.color}24`, color: "var(--foreground)" }}
          >
            {s.text || "(empty)"}
          </span>
          {s.note && <span className="text-[9px] text-muted-foreground">{s.note}</span>}
        </div>
      ))}
    </div>
  );
}

/** One block's transition through the step. */
function TransitionRow({ t }: { t: PartTransition }) {
  const [open, setOpen] = useState(false);
  const delta = snapshotDelta(t.before, t.after);
  const afterOverlays = t.after.detail?.overlays ?? [];
  const hasStandoff =
    delta.addedOverlays.length > 0 ||
    delta.removedOverlays.length > 0 ||
    delta.addedAnnotations.length > 0 ||
    delta.removedAnnotations.length > 0;

  return (
    <div className="mb-2 rounded-md border border-border p-2">
      <button
        type="button"
        className="flex w-full cursor-pointer items-center gap-1 border-0 bg-transparent p-0 text-left"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
      >
        {open ? (
          <ChevronDown size={10} className="shrink-0 text-muted-foreground" />
        ) : (
          <ChevronRight size={10} className="shrink-0 text-muted-foreground" />
        )}
        <span className="truncate text-[10px] text-muted-foreground">{t.before.summary}</span>
      </button>

      {/* Text changes */}
      {delta.sourceChanged && (
        <div className="mt-1">
          <div className="text-[9px] font-semibold text-muted-foreground">SOURCE REWRITTEN</div>
          <div className="text-[10px] leading-snug text-muted-foreground line-through opacity-60">
            {t.before.sourceText}
          </div>
          <div className="text-[11px] leading-snug text-foreground">{t.after.sourceText}</div>
        </div>
      )}
      {delta.targetChanged && (
        <div className="mt-1">
          <div className="text-[9px] font-semibold text-muted-foreground">TARGET</div>
          {t.before.targetText && (
            <div className="text-[10px] leading-snug text-muted-foreground line-through opacity-60">
              {t.before.targetText}
            </div>
          )}
          <div className="text-[11px] leading-snug text-foreground">
            {t.after.targetText || "(empty)"}
          </div>
        </div>
      )}

      {/* Stand-off delta: what this tool attached or consumed. */}
      {hasStandoff && (
        <div className="mt-1.5 flex flex-wrap items-center gap-1">
          {delta.addedOverlays.map((o, i) => (
            <OverlayChip key={`a${i}`} type={o.type} spans={o.spans} />
          ))}
          {delta.removedOverlays.map((o, i) => (
            <OverlayChip key={`r${i}`} type={o.type} spans={o.spans} removed />
          ))}
          {delta.addedAnnotations.map((k) => (
            <span
              key={`an-${k}`}
              className="rounded bg-secondary px-1 py-px font-mono text-[9px] text-muted-foreground"
              title={`Block annotation "${k}" added by this step`}
            >
              +{k}
            </span>
          ))}
          {delta.removedAnnotations.map((k) => (
            <span
              key={`rm-${k}`}
              className="rounded bg-secondary px-1 py-px font-mono text-[9px] text-muted-foreground line-through opacity-60"
              title={`Block annotation "${k}" removed by this step`}
            >
              −{k}
            </span>
          ))}
        </div>
      )}
      {!hasStandoff && !delta.sourceChanged && !delta.targetChanged && (
        <div className="mt-1 text-[10px] italic text-muted-foreground">
          passed through unchanged
        </div>
      )}

      {/* Expanded: the block's full stand-off state after this step. */}
      {open && (
        <div className="mt-2 border-t border-border pt-1.5">
          <div className="text-[9px] font-semibold text-muted-foreground">
            OVERLAYS AFTER THIS STEP
          </div>
          {afterOverlays.length === 0 && (
            <div className="text-[10px] italic text-muted-foreground">none</div>
          )}
          {afterOverlays.map((o, i) => (
            <div key={i} className="mt-1">
              <span
                className="font-mono text-[9px] font-semibold"
                style={{ color: portStyle(o.type).color }}
              >
                {o.type}
              </span>
              <span className="ml-1 text-[9px] text-muted-foreground">on {o.side}</span>
              <OverlaySpans overlay={o} />
            </div>
          ))}
          {(t.after.detail?.annotations?.length ?? 0) > 0 && (
            <div className="mt-1.5">
              <div className="text-[9px] font-semibold text-muted-foreground">ANNOTATIONS</div>
              {t.after.detail!.annotations!.map((a) => (
                <div key={a.key} className="text-[10px] leading-snug">
                  <span className="font-mono text-[9px]">{a.key}</span>
                  {a.summary && <span className="ml-1 text-muted-foreground">{a.summary}</span>}
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export function RunInspectorPanel({
  trace,
  stepToolCounts,
  stepIndex,
  stepLabel,
  onConfigure,
  onClose,
}: RunInspectorPanelProps) {
  const transitions = partsThroughStep(trace, stepToolCounts, stepIndex).filter(
    (t) => t.before.type === "Block",
  );

  return (
    <div
      className="flex flex-col overflow-hidden border-l border-border bg-background"
      style={{ width: 300, minWidth: 300, maxWidth: 300 }}
    >
      <PanelHeader className="flex-col items-start gap-0.5 py-2.5">
        <div className="flex w-full items-center justify-between">
          <div className="text-[11px] font-semibold text-foreground">Run inspector</div>
          <div className="flex items-center gap-1">
            {onConfigure && (
              <Button
                variant="ghost"
                size="xs"
                className="h-5 px-1.5 text-[9px]"
                onClick={onConfigure}
                title="Edit this step's configuration"
              >
                <Pencil size={9} className="mr-0.5" /> Configure
              </Button>
            )}
            <Button variant="ghost" size="xs" className="h-5 px-1.5 text-[9px]" onClick={onClose}>
              Close
            </Button>
          </div>
        </div>
        <div className="text-[10px] text-muted-foreground">
          {stepLabel} — {transitions.length} block{transitions.length !== 1 ? "s" : ""}
        </div>
      </PanelHeader>

      <ScrollArea className="flex-1">
        <div className="px-3 py-2">
          {transitions.length === 0 && (
            <div className="py-5 text-center text-[11px] italic text-muted-foreground">
              No blocks passed through this step in the last run.
            </div>
          )}
          {transitions.map((t) => (
            <TransitionRow key={t.partId} t={t} />
          ))}
        </div>
      </ScrollArea>
    </div>
  );
}

// Re-exported so hosts can show a quick textual summary elsewhere if needed.
export type { PartSnapshot };
