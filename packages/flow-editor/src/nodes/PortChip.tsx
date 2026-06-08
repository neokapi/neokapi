// PortChip — the shared visual for one IO-contract port (a typed overlay,
// annotation, or source/target content). Colored + iconed by port family
// (see ../portTypes), so the same chip reads consistently on nodes, edges, the
// palette, the config panel, and the legend.

import { cn } from "@neokapi/ui-primitives";
import type { IOPort, Side } from "../types";
import { getPortType } from "../portTypes";

export interface PortChipProps {
  type: string;
  side?: Side;
  /** Render dashed + faded to signal an optional (graceful-degradation) input. */
  optional?: boolean;
  /** Show the port label next to the icon (default icon-only for compactness). */
  showLabel?: boolean;
  /** "consumes" tints the verb in the tooltip; "produces" is the default. */
  verb?: "consumes" | "produces";
  className?: string;
}

const SIDE_LABEL: Record<Side, string> = { source: "source", target: "target" };

export function PortChip({
  type,
  side,
  optional,
  showLabel = false,
  verb = "produces",
  className,
}: PortChipProps) {
  const pt = getPortType(type);
  const Icon = pt.icon;
  const sideText = side ? ` · ${SIDE_LABEL[side]}` : "";
  const verbText = verb === "consumes" ? "Consumes" : "Produces";
  const optText = optional ? " (optional)" : "";
  return (
    <span
      title={`${verbText}: ${pt.label}${sideText}${optText} — ${pt.description}`}
      className={cn(
        "inline-flex items-center gap-0.5 rounded px-1 py-px text-[8px] font-medium leading-none",
        optional && "border border-dashed",
        className,
      )}
      style={{
        background: pt.bg,
        color: pt.color,
        borderColor: optional ? pt.color : undefined,
        opacity: optional ? 0.7 : 1,
      }}
    >
      <Icon size={9} style={{ color: pt.color }} aria-hidden />
      {showLabel && <span className="whitespace-nowrap">{pt.label}</span>}
    </span>
  );
}

/**
 * IoContract renders a tool's "reads → writes" port row: consumed ports, an
 * arrow, then produced ports. `max` caps how many chips render before a "+N"
 * overflow (keeps nodes compact); pass a large number for the config panel.
 */
export function IoContract({
  consumes,
  produces,
  max = 4,
  showLabels = false,
}: {
  consumes?: IOPort[];
  produces?: IOPort[];
  max?: number;
  showLabels?: boolean;
}) {
  const ins = consumes ?? [];
  const outs = produces ?? [];
  if (ins.length === 0 && outs.length === 0) return null;

  // Budget the chip allowance across both sides so a wide-IO tool stays compact.
  const insShown = ins.slice(0, max);
  const outsShown = outs.slice(0, max);
  const hidden = ins.length - insShown.length + (outs.length - outsShown.length);

  return (
    <div className="flex items-center gap-0.5 flex-wrap">
      {insShown.map((f, i) => (
        <PortChip
          key={`in-${f.type}-${f.side ?? ""}-${i}`}
          type={f.type}
          side={f.side}
          optional={f.optional}
          showLabel={showLabels}
          verb="consumes"
        />
      ))}
      {insShown.length > 0 && outsShown.length > 0 && (
        <span className="text-[9px] text-muted-foreground px-px" aria-hidden>
          →
        </span>
      )}
      {outsShown.map((f, i) => (
        <PortChip
          key={`out-${f.type}-${f.side ?? ""}-${i}`}
          type={f.type}
          side={f.side}
          showLabel={showLabels}
          verb="produces"
        />
      ))}
      {hidden > 0 && (
        <span className="text-[8px] text-muted-foreground" title={`${hidden} more`}>
          +{hidden}
        </span>
      )}
    </div>
  );
}
