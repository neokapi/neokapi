import { cn, SimpleTooltip } from "@neokapi/ui-primitives";
import type { LocaleTranslationStats, ShipState } from "../types/api";
import { CircleCheck, Clock, ShieldCheck, Sparkles } from "./icons";

/**
 * ShipStateBadge renders the per-locale ship state with one consistent visual
 * language everywhere it appears (dashboard locale rows, collection rollups,
 * delivery panel): governed (human-approved, with terminology governing the
 * language), approved (human-approved, with nothing beyond the checks governing
 * it), AI-shippable (machine-reviewed only), pending (not ready). A tooltip
 * explains what the state means and, when counts are provided, why the scope
 * holds it.
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
  /**
   * Stale pairs waiting on a convergence pass, and stale pairs already
   * re-drafted and waiting on a reviewer. They sum to the scope's stale count,
   * and they are shown apart because they are different work: one is a run away
   * and the other needs a person.
   */
  staleAwaitingDraft?: number;
  staleAwaitingReview?: number;
  /**
   * Pairs a reviewer turned down that the loop has not drafted again since.
   * They sit outside the stale count: rejecting a translation of the source the
   * project still holds moves neither hash, so the basis grading reads such a
   * pair as settled. A convergence pass is what moves them.
   */
  rejectedAwaitingDraft?: number;
  /**
   * Translated blocks in a language terms govern that have no terminology
   * result. They withhold the scope as failing checks do.
   */
  termsNotCheckedBlocks?: number;
  /**
   * Terminology governs nothing in the language. It withholds nothing, and the
   * tooltip names it so a reader does not take the scope for a governed one.
   */
  termsNotGoverned?: boolean;
  className?: string;
}

/**
 * Whether terminology governs nothing in a locale scope, read from the
 * compliance basis the server derived. False when the server derived no basis,
 * which claims nothing either way.
 */
export function termsNotGoverned(stats: Pick<LocaleTranslationStats, "compliance_basis">): boolean {
  const basis = stats.compliance_basis;
  return basis !== undefined && !basis.endsWith("terms");
}

const stateStyles: Record<ShipState, { label: string; className: string; explanation: string }> = {
  governed: {
    label: "Governed",
    className: "border-success/40 bg-success/15 text-success",
    explanation: "Fully translated, the checks pass, and every translation is human-approved.",
  },
  approved: {
    label: "Approved",
    className: "border-success/30 bg-success/5 text-success",
    explanation:
      "Fully translated, the checks pass, and every translation is human-approved. No terms apply to this language, so it is not governed.",
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
    explanation:
      "Not ready to ship: translation, checks, terminology, or review is still in progress.",
  },
};

const stateIcons: Record<ShipState, React.ComponentType<{ className?: string }>> = {
  governed: ShieldCheck,
  approved: CircleCheck,
  ai_shippable: Sparkles,
  pending: Clock,
};

function tooltipContent(props: ShipStateBadgeProps): React.ReactNode {
  const { state, approvedBlocks, totalBlocks, failingChecks } = props;
  const awaitingDraft = props.staleAwaitingDraft ?? 0;
  const awaitingReview = props.staleAwaitingReview ?? 0;
  const rejected = props.rejectedAwaitingDraft ?? 0;
  const termsNotChecked = props.termsNotCheckedBlocks ?? 0;
  const meta = stateStyles[state];
  const details: string[] = [];
  if (totalBlocks !== undefined && approvedBlocks !== undefined) {
    details.push(`${approvedBlocks} of ${totalBlocks} blocks human-approved`);
  }
  if (failingChecks !== undefined && failingChecks > 0) {
    details.push(`${failingChecks} failing ${failingChecks === 1 ? "check" : "checks"}`);
  }
  if (termsNotChecked > 0) {
    details.push(`${termsNotChecked} with no terminology result`);
  }
  // What the stale pairs are waiting on, so a reader knows whether to run the
  // loop or to open the review queue.
  if (awaitingDraft > 0) {
    details.push(`${awaitingDraft} stale, awaiting a draft`);
  }
  if (awaitingReview > 0) {
    details.push(`${awaitingReview} re-drafted, awaiting review`);
  }
  if (rejected > 0) {
    details.push(`${rejected} turned down, awaiting a draft`);
  }
  return (
    <div className="max-w-60 space-y-1">
      <p className="font-medium">{meta.label}</p>
      <p>{meta.explanation}</p>
      {details.length > 0 && <p className="text-muted-foreground">{details.join(" · ")}</p>}
      {/* The approved explanation already says it; every other state names it here. */}
      {props.termsNotGoverned && state !== "approved" && (
        <p className="text-muted-foreground">
          No terms apply to this language, so terminology is not governed here.
        </p>
      )}
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
