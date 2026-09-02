import { cn, SimpleTooltip } from "@neokapi/ui-primitives";
import type { ProjectType } from "../types/api";
import { Layers, Package, Plug } from "./icons";

/**
 * ProjectTypeBadge renders a project's source-of-truth classification with one
 * consistent visual language: Connected (content synced from a connector — the
 * source is read-only here), Managed (UI-native — Bowrain owns the source), or
 * Hybrid (a mix of connected and managed collections). A tooltip explains what
 * the type means and, for connected projects, names where to edit instead.
 */
export interface ProjectTypeBadgeProps {
  type: ProjectType;
  /** Icon-only variant for dense surfaces. */
  compact?: boolean;
  className?: string;
}

const typeStyles: Record<ProjectType, { label: string; className: string; explanation: string }> = {
  connected: {
    label: "Connected",
    className: "border-info/40 bg-info/15 text-info",
    explanation:
      "Content is synced from a connector (kapi push, the GitHub App, or a git repository). The source is read-only here: edit it at source and it re-syncs. Review, governance, and configuration still apply.",
  },
  managed: {
    label: "Managed",
    className: "border-success/40 bg-success/15 text-success",
    explanation: "Bowrain owns the source: upload, edit, and delete content directly here.",
  },
  hybrid: {
    label: "Hybrid",
    className: "border-border/60 bg-muted text-muted-foreground",
    explanation:
      "A mix of collections: some are synced from a connector (read-only source), others are managed here.",
  },
};

const typeIcons: Record<ProjectType, React.ComponentType<{ className?: string }>> = {
  connected: Plug,
  managed: Package,
  hybrid: Layers,
};

function tooltipContent(type: ProjectType): React.ReactNode {
  const meta = typeStyles[type];
  return (
    <div className="max-w-60 space-y-1">
      <p className="font-medium">{meta.label}</p>
      <p>{meta.explanation}</p>
    </div>
  );
}

export function ProjectTypeBadge({ type, compact, className }: ProjectTypeBadgeProps) {
  const meta = typeStyles[type];
  const Icon = typeIcons[type];

  if (compact) {
    return (
      <SimpleTooltip content={tooltipContent(type)}>
        <span
          data-testid={`project-type-${type}`}
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
    <SimpleTooltip content={tooltipContent(type)}>
      <span
        data-testid={`project-type-${type}`}
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
