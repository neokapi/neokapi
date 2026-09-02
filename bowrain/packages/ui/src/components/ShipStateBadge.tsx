import { cn, SimpleTooltip } from "@neokapi/ui-primitives";
import type { ShipState } from "../types/api";
import { Clock, ShieldCheck, Sparkles } from "./icons";

/**
 * ShipStateBadge renders the per-locale ship state with one consistent visual
 * language everywhere it appears (dashboard locale rows, collection rollups,
 * delivery panel): governed (human-approved), AI-shippable (machine-reviewed
 * only), pending (not ready). A tooltip explains what the state means and, when
 * counts are provided, why the scope holds it.
 */
export interface ShipStateBadgeProps {
  state: ShipState;
  /** Icon-only variant for dense surfaces (table cells). */
  compact?: boolean;
  /** Blocks with a human review decision, for the tooltip detail line. */
  approvedBlocks?: number;
  /** Total translatable blocks in the scope, for the tooltip detail line. */
  totalBlocks?: number;
  /** Translated blocks failing the checks, for the tooltip detail line. */
  failingChecks?: number;
  className?: string;
}

const stateStyles: Record<ShipState, { label: string; className: string; explanation: string }> = {
  governed: {
    label: "Governed",
    className: "border-success/40 bg-success/15 text-success",
    explanation: "Fully translated, the checks pass, and every translation is human-approved.",
  },
  ai_shippable: {
    label: "AI-shippable",
    className: "border-info/40 bg-info/15 text-info",
    explanation:
      "Fully translated and the checks pass, but not fully human-reviewed. Shippable on machine review only.",
  },
  pending: {
    label: "Pending",
    className: "border-border/60 bg-muted text-muted-foreground",
    explanation: "Not ready to ship: translation, checks, or review is still in progress.",
  },
};

const stateIcons: Record<ShipState, React.ComponentType<{ className?: string }>> = {
  governed: ShieldCheck,
  ai_shippable: Sparkles,
  pending: Clock,
};

function tooltipContent(props: ShipStateBadgeProps): React.ReactNode {
  const { state, approvedBlocks, totalBlocks, failingChecks } = props;
  const meta = stateStyles[state];
  const details: string[] = [];
  if (totalBlocks !== undefined && approvedBlocks !== undefined) {
    details.push(`${approvedBlocks} of ${totalBlocks} blocks human-approved`);
  }
  if (failingChecks !== undefined && failingChecks > 0) {
    details.push(`${failingChecks} failing ${failingChecks === 1 ? "check" : "checks"}`);
  }
  return (
    <div className="max-w-60 space-y-1">
      <p className="font-medium">{meta.label}</p>
      <p>{meta.explanation}</p>
      {details.length > 0 && <p className="text-muted-foreground">{details.join(" · ")}</p>}
    </div>
  );
}

export function ShipStateBadge(props: ShipStateBadgeProps) {
  const { state, compact, className } = props;
  const meta = stateStyles[state];
  const Icon = stateIcons[state];

  if (compact) {
    return (
      <SimpleTooltip content={tooltipContent(props)}>
        <span
          data-testid={`ship-state-${state}`}
          aria-label={meta.label}
          className={cn(
            "inline-flex h-4 w-4 items-center justify-center rounded-full border",
            meta.className,
            className,
          )}
        >
          <Icon className="h-2.5 w-2.5" />
        </span>
      </SimpleTooltip>
    );
  }

  return (
    <SimpleTooltip content={tooltipContent(props)}>
      <span
        data-testid={`ship-state-${state}`}
        className={cn(
          "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium",
          meta.className,
          className,
        )}
      >
        <Icon className="h-3 w-3" />
        {meta.label}
      </span>
    </SimpleTooltip>
  );
}
